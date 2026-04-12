package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Config struct {
	RabbitURL    string
	QueueName    string
	DLQName      string
	OutputDir    string
	ClientSecret string
	TokenPath    string
	Privacy      string
	CategoryID   string
	Tags         []string
}

type ProducedMessage struct {
	Slug       string `json:"slug"`
	Date       string `json:"date"`
	OutputFile string `json:"output_file"`
	Status     string `json:"status"`
	Timestamp  string `json:"timestamp"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	uploader, err := newUploader(ctx, cfg)
	if err != nil {
		log.Fatalf("youtube auth: %v", err)
	}

	if err := runConsumer(ctx, cfg, uploader); err != nil {
		log.Fatalf("consumer: %v", err)
	}
}

func loadConfig() (Config, error) {
	cfg := Config{
		RabbitURL:    os.Getenv("RABBITMQ_URL"),
		QueueName:    getEnv("RABBITMQ_QUEUE", "video.produced"),
		DLQName:      getEnv("RABBITMQ_DLQ", "video.produced.dlq"),
		OutputDir:    getEnv("VIDEO_OUTPUT_DIR", "/output/video"),
		ClientSecret: os.Getenv("YOUTUBE_CLIENT_SECRET"),
		TokenPath:    os.Getenv("YOUTUBE_TOKEN"),
		Privacy:      getEnv("YOUTUBE_PRIVACY", "unlisted"),
		CategoryID:   getEnv("YOUTUBE_CATEGORY_ID", "22"),
	}
	if tags := strings.TrimSpace(os.Getenv("YOUTUBE_TAGS")); tags != "" {
		cfg.Tags = splitCSV(tags)
	}

	if cfg.RabbitURL == "" {
		return cfg, fmt.Errorf("RABBITMQ_URL is required")
	}
	if cfg.ClientSecret == "" {
		return cfg, fmt.Errorf("YOUTUBE_CLIENT_SECRET is required")
	}
	if cfg.TokenPath == "" {
		return cfg, fmt.Errorf("YOUTUBE_TOKEN is required")
	}

	return cfg, nil
}

func runConsumer(ctx context.Context, cfg Config, uploader *youtube.Service) error {
	conn, err := amqp.Dial(cfg.RabbitURL)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("amqp channel: %w", err)
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(cfg.QueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %s: %w", cfg.QueueName, err)
	}
	if _, err := ch.QueueDeclare(cfg.DLQName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dlq %s: %w", cfg.DLQName, err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	msgs, err := ch.Consume(cfg.QueueName, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	log.Printf("video-publisher listening on %s", cfg.QueueName)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("amqp channel closed")
			}
			if err := handleMessage(ctx, cfg, uploader, ch, msg); err != nil {
				log.Printf("message failed: %v", err)
			}
		}
	}
}

func handleMessage(ctx context.Context, cfg Config, uploader *youtube.Service, ch *amqp.Channel, msg amqp.Delivery) error {
	var payload ProducedMessage
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		publishDLQ(ch, cfg.DLQName, msg.Body, fmt.Errorf("decode payload: %w", err))
		msg.Ack(false)
		return err
	}

	outputFile := strings.TrimSpace(payload.OutputFile)
	if outputFile == "" && payload.Slug != "" {
		outputFile = payload.Slug + ".mp4"
	}
	if outputFile == "" {
		publishDLQ(ch, cfg.DLQName, msg.Body, fmt.Errorf("missing output_file"))
		msg.Ack(false)
		return fmt.Errorf("missing output_file")
	}

	videoPath := filepath.Join(cfg.OutputDir, outputFile)
	if _, err := os.Stat(videoPath); err != nil {
		publishDLQ(ch, cfg.DLQName, msg.Body, fmt.Errorf("video not found: %w", err))
		msg.Ack(false)
		return err
	}

	title := titleFromSlug(payload.Slug)
	if title == "" {
		title = strings.TrimSuffix(outputFile, filepath.Ext(outputFile))
	}

	videoID, err := uploadToYouTube(ctx, uploader, videoPath, title, cfg)
	if err != nil {
		publishDLQ(ch, cfg.DLQName, msg.Body, fmt.Errorf("upload: %w", err))
		msg.Ack(false)
		return err
	}

	if err := os.Remove(videoPath); err != nil {
		log.Printf("uploaded video %s but failed to delete %s: %v", videoID, videoPath, err)
	}

	msg.Ack(false)
	log.Printf("uploaded %s to youtube id=%s", outputFile, videoID)
	return nil
}

func publishDLQ(ch *amqp.Channel, queue string, body []byte, err error) {
	if ch == nil {
		return
	}
	payload := map[string]any{
		"error":     err.Error(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"original":  json.RawMessage(body),
	}
	data, _ := json.Marshal(payload)
	_ = ch.Publish("", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         data,
		Timestamp:    time.Now().UTC(),
		DeliveryMode: amqp.Persistent,
	})
}

func newUploader(ctx context.Context, cfg Config) (*youtube.Service, error) {
	secret, err := os.ReadFile(cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("read client secret: %w", err)
	}
	config, err := google.ConfigFromJSON(secret, youtube.YoutubeUploadScope)
	if err != nil {
		return nil, fmt.Errorf("parse client secret: %w", err)
	}
	client, err := clientFromToken(ctx, config, cfg.TokenPath)
	if err != nil {
		return nil, err
	}

	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("youtube service: %w", err)
	}
	return service, nil
}

func clientFromToken(ctx context.Context, config *oauth2.Config, tokenPath string) (*http.Client, error) {
	tok, err := readToken(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}
	return config.Client(ctx, tok), nil
}

func readToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tok oauth2.Token
	if err := json.NewDecoder(f).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func uploadToYouTube(ctx context.Context, service *youtube.Service, videoPath, title string, cfg Config) (string, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	video := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: "",
			CategoryId:  cfg.CategoryID,
			Tags:        cfg.Tags,
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus: cfg.Privacy,
		},
	}

	call := service.Videos.Insert([]string{"snippet", "status"}, video).Media(file)
	call.Context(ctx)
	resp, err := call.Do()
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Id == "" {
		return "", fmt.Errorf("youtube response missing id")
	}
	return resp.Id, nil
}

func titleFromSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
