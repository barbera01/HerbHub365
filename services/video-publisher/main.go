package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
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
	RabbitURL              string
	QueueName              string
	DLQName                string
	OutputDir              string
	ClientSecret           string
	TokenPath              string
	Privacy                string
	CategoryID             string
	Tags                   []string
	MadeForKids            bool
	PostsDir               string
	BlogBaseURL            string
	ContainsSyntheticMedia bool
	Git                    GitConfig
}

type GitConfig struct {
	PublishEnabled bool
	RepoDir        string
	RemoteName     string
	PushBranch     string
	PAT            string
	AuthorName     string
	AuthorEmail    string
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

	runWithReconnect(ctx, cfg, uploader)
}

func loadConfig() (Config, error) {
	cfg := Config{
		RabbitURL:              os.Getenv("RABBITMQ_URL"),
		QueueName:              getEnv("RABBITMQ_QUEUE", "video.produced"),
		DLQName:                getEnv("RABBITMQ_DLQ", "video.produced.dlq"),
		OutputDir:              getEnv("VIDEO_OUTPUT_DIR", "/output/video"),
		ClientSecret:           os.Getenv("YOUTUBE_CLIENT_SECRET"),
		TokenPath:              os.Getenv("YOUTUBE_TOKEN"),
		Privacy:                getEnv("YOUTUBE_PRIVACY", "unlisted"),
		CategoryID:             getEnv("YOUTUBE_CATEGORY_ID", "22"),
		MadeForKids:            getBoolEnv("YOUTUBE_MADE_FOR_KIDS", false),
		PostsDir:               getEnv("BLOG_POSTS_DIR", "/repo/hub/_posts"),
		BlogBaseURL:            strings.TrimRight(os.Getenv("BLOG_BASE_URL"), "/"),
		ContainsSyntheticMedia: getBoolEnv("YOUTUBE_CONTAINS_SYNTHETIC_MEDIA", true),
		Git: GitConfig{
			PublishEnabled: getBoolEnv("BLOG_POSTER_GIT_PUBLISH_ENABLED", false),
			RepoDir:        getEnv("BLOG_POSTER_GIT_REPO_DIR", "/repo"),
			RemoteName:     getEnv("BLOG_POSTER_GIT_REMOTE_NAME", "origin"),
			PushBranch:     getEnv("BLOG_POSTER_GIT_PUSH_BRANCH", "main"),
			PAT:            os.Getenv("BLOG_POSTER_GIT_PAT"),
			AuthorName:     getEnv("BLOG_POSTER_GIT_AUTHOR_NAME", "Herb Hub Bot"),
			AuthorEmail:    getEnv("BLOG_POSTER_GIT_AUTHOR_EMAIL", "bot@herbhub365.com"),
		},
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

func runWithReconnect(ctx context.Context, cfg Config, uploader *youtube.Service) {
	backoff := 2 * time.Second
	for {
		err := runConsumer(ctx, cfg, uploader)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("consumer stopped: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
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

	meta, err := loadPostMetadata(cfg.PostsDir, payload, outputFile)
	if err != nil {
		log.Printf("metadata: %v", err)
	}
	if meta.Title != "" {
		title = meta.Title
	}

	description := buildDescription(title, meta.Excerpt, cfg.BlogBaseURL, payload.Date, payload.Slug)
	tags := buildTags(cfg.Tags, payload.Slug, meta.Tags, meta.Categories)

	videoID, err := uploadToYouTube(ctx, uploader, videoPath, title, description, tags, cfg)
	if err != nil {
		publishDLQ(ch, cfg.DLQName, msg.Body, fmt.Errorf("upload: %w", err))
		msg.Ack(false)
		return err
	}

	youtubeURL := "https://youtu.be/" + videoID
	if err := updateMarker(cfg.OutputDir, outputFile, payload.Slug, videoID, youtubeURL); err != nil {
		log.Printf("uploaded video %s but failed to update marker: %v", videoID, err)
	}
	if postPath, err := updatePostEmbed(cfg.PostsDir, payload, outputFile, videoID); err != nil {
		log.Printf("embed update failed: %v", err)
	} else if postPath != "" {
		if err := publishPostUpdate(ctx, cfg.Git, postPath, payload); err != nil {
			log.Printf("publish post update failed: %v", err)
		}
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

func updateMarker(outputDir, outputFile, slug, youtubeID, youtubeURL string) error {
	markerName := markerFilename(outputFile, slug)
	if markerName == "" {
		return fmt.Errorf("marker filename not resolved")
	}
	path := filepath.Join(outputDir, markerName)

	data := map[string]any{}
	if existing, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(existing, &data)
	}

	data["slug"] = slug
	if outputFile != "" {
		data["output_file"] = outputFile
	}
	data["status"] = "completed"
	data["youtube_id"] = youtubeID
	data["youtube_url"] = youtubeURL
	data["timestamp"] = time.Now().UTC().Format(time.RFC3339)

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, encoded, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func markerFilename(outputFile, slug string) string {
	if outputFile != "" {
		return strings.TrimSuffix(outputFile, ".mp4") + ".json"
	}
	if slug != "" {
		return slug + ".json"
	}
	return ""
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

func uploadToYouTube(ctx context.Context, service *youtube.Service, videoPath, title, description string, tags []string, cfg Config) (string, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	video := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: description,
			CategoryId:  cfg.CategoryID,
			Tags:        tags,
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus:           cfg.Privacy,
			SelfDeclaredMadeForKids: cfg.MadeForKids,
			ContainsSyntheticMedia:  cfg.ContainsSyntheticMedia,
			// ForceSendFields ensures boolean false values are included in the
			// JSON payload — omitempty in the generated client would otherwise
			// drop them, leaving YouTube without a made-for-kids declaration.
			ForceSendFields: []string{"SelfDeclaredMadeForKids", "ContainsSyntheticMedia"},
		},
	}
	log.Printf("youtube upload payload: title=%q privacy=%q madeForKids=%v synthetic=%v tags=%v category=%s",
		title, cfg.Privacy, cfg.MadeForKids, cfg.ContainsSyntheticMedia, tags, cfg.CategoryID)
	if description != "" {
		log.Printf("youtube upload description: %q", description)
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

	// Reinforce status fields via update to ensure flags are persisted.
	update := &youtube.Video{
		Id: resp.Id,
		Status: &youtube.VideoStatus{
			PrivacyStatus:           cfg.Privacy,
			SelfDeclaredMadeForKids: cfg.MadeForKids,
			ContainsSyntheticMedia:  cfg.ContainsSyntheticMedia,
			ForceSendFields:         []string{"SelfDeclaredMadeForKids", "ContainsSyntheticMedia"},
		},
	}
	if updated, err := service.Videos.Update([]string{"status"}, update).Context(ctx).Do(); err != nil {
		log.Printf("youtube status update failed for %s: %v", resp.Id, err)
	} else if updated != nil && updated.Status != nil {
		log.Printf("youtube status confirmed: madeForKids=%v synthetic=%v privacy=%s", updated.Status.SelfDeclaredMadeForKids, updated.Status.ContainsSyntheticMedia, updated.Status.PrivacyStatus)
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

func getBoolEnv(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}

type PostMetadata struct {
	Title      string
	Excerpt    string
	Tags       []string
	Categories []string
}

var (
	postFileRe         = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})-(.+)\.(markdown|md)$`)
	frontMatterRe      = regexp.MustCompile(`(?s)^---\n.*?\n---\n?`)
	frontMatterTitleRe = regexp.MustCompile(`(?m)^title:\s*["']?(.+?)["']?\s*$`)
	mdImageRe          = regexp.MustCompile(`!\[[^\]]*\]\([^\)]*\)`)
	mdLinkRe           = regexp.MustCompile(`\[([^\]]+)\]\([^\)]*\)`)
	htmlTagRe          = regexp.MustCompile(`<[^>]+>`)
	mdHeadingRe        = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdEmphRe           = regexp.MustCompile(`[*_]{1,2}([^*_]+)[*_]{1,2}`)
)

func loadPostMetadata(postsDir string, payload ProducedMessage, outputFile string) (PostMetadata, error) {
	var meta PostMetadata
	path := resolvePostPath(postsDir, payload, outputFile)
	if path == "" {
		return meta, fmt.Errorf("post not found for slug=%s", payload.Slug)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	raw := string(data)
	meta.Title = extractTitle(raw, payload.Slug)
	meta.Excerpt = extractExcerpt(raw)
	meta.Tags = extractFrontMatterList(raw, "tags")
	meta.Categories = extractFrontMatterList(raw, "categories")
	return meta, nil
}

func resolvePostPath(postsDir string, payload ProducedMessage, outputFile string) string {
	if payload.Date != "" && payload.Slug != "" {
		candidate := filepath.Join(postsDir, payload.Date+"-"+payload.Slug+".md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(postsDir, payload.Date+"-"+payload.Slug+".markdown")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if outputFile != "" {
		name := strings.TrimSuffix(outputFile, ".mp4")
		for _, ext := range []string{".md", ".markdown"} {
			candidate := filepath.Join(postsDir, name+ext)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !postFileRe.MatchString(e.Name()) {
			continue
		}
		if payload.Slug != "" && strings.Contains(e.Name(), payload.Slug) {
			return filepath.Join(postsDir, e.Name())
		}
	}
	return ""
}

func extractTitle(raw, slug string) string {
	if m := frontMatterTitleRe.FindStringSubmatch(raw); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if slug == "" {
		return ""
	}
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func extractExcerpt(raw string) string {
	body := frontMatterRe.ReplaceAllString(raw, "")
	body = mdImageRe.ReplaceAllString(body, "")
	body = htmlTagRe.ReplaceAllString(body, "")
	body = mdHeadingRe.ReplaceAllString(body, "")
	body = mdLinkRe.ReplaceAllString(body, "$1")
	body = mdEmphRe.ReplaceAllString(body, "$1")
	body = strings.TrimSpace(body)
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return body
}

func extractFrontMatterList(raw, key string) []string {
	front := frontMatterRe.FindString(raw)
	if front == "" {
		return nil
	}
	lines := strings.Split(front, "\n")
	var out []string
	readingList := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, key+":") {
			readingList = true
			value := strings.TrimSpace(strings.TrimPrefix(trim, key+":"))
			if value != "" {
				value = strings.Trim(value, "[]")
				out = append(out, splitCSV(value)...)
			}
			continue
		}
		if readingList {
			if strings.HasPrefix(trim, "-") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "-"))
				if item != "" {
					out = append(out, item)
				}
				continue
			}
			if strings.Contains(trim, ":") {
				readingList = false
			}
		}
	}
	return normalizeTags(out)
}

func buildDescription(title, excerpt, baseURL, date, slug string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	if excerpt != "" {
		b.WriteString(excerpt)
		b.WriteString("\n\n")
	}
	if baseURL != "" && date != "" && slug != "" {
		parts := strings.Split(date, "-")
		if len(parts) == 3 {
			b.WriteString("Read more: ")
			b.WriteString(baseURL)
			b.WriteString("/")
			b.WriteString(parts[0])
			b.WriteString("/")
			b.WriteString(parts[1])
			b.WriteString("/")
			b.WriteString(parts[2])
			b.WriteString("/")
			b.WriteString(slug)
		}
	}
	return strings.TrimSpace(b.String())
}

func buildTags(base []string, slug string, tags []string, categories []string) []string {
	all := append([]string{}, base...)
	if slug != "" {
		all = append(all, strings.Split(slug, "-")...)
	}
	all = append(all, tags...)
	all = append(all, categories...)
	return normalizeTags(all)
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		clean := strings.TrimSpace(strings.ToLower(t))
		clean = strings.Trim(clean, "\"'")
		if clean == "" {
			continue
		}
		clean = strings.ReplaceAll(clean, " ", "-")
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
		if len(out) >= 15 {
			break
		}
	}
	return out
}

func updatePostEmbed(postsDir string, payload ProducedMessage, outputFile, videoID string) (string, error) {
	path := resolvePostPath(postsDir, payload, outputFile)
	if path == "" {
		return "", fmt.Errorf("post not found for slug=%s", payload.Slug)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	raw := string(content)
	if strings.Contains(raw, videoID) {
		return "", nil
	}

	front := frontMatterRe.FindString(raw)
	body := strings.TrimPrefix(raw, front)
	lines := strings.Split(body, "\n")

	insertIdx := -1
	mdImgRe := regexp.MustCompile(`!\[[^\]]*\]\([^\)]+\)`)
	htmlImgRe := regexp.MustCompile(`(?i)<img\s[^>]*>`)
	for i, line := range lines {
		if mdImgRe.MatchString(line) || htmlImgRe.MatchString(line) {
			insertIdx = i + 1
			break
		}
	}

	embed := buildEmbed(videoID)
	if insertIdx == -1 {
		lines = append(lines, "", embed)
	} else {
		lines = append(lines[:insertIdx], append([]string{embed, ""}, lines[insertIdx:]...)...)
	}

	updated := front + strings.Join(lines, "\n")
	if updated == raw {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func buildEmbed(videoID string) string {
	return fmt.Sprintf(
		"<div class=\"video-embed\">\n  <iframe src=\"https://www.youtube.com/embed/%s\" title=\"YouTube video player\" frameborder=\"0\" allow=\"accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share; compute-pressure\" allowfullscreen></iframe>\n</div>",
		videoID,
	)
}

func publishPostUpdate(ctx context.Context, cfg GitConfig, postPath string, payload ProducedMessage) error {
	if !cfg.PublishEnabled {
		return nil
	}
	if strings.TrimSpace(cfg.PAT) == "" {
		return fmt.Errorf("BLOG_POSTER_GIT_PAT is required when BLOG_POSTER_GIT_PUBLISH_ENABLED=true")
	}
	repoDir, err := filepath.Abs(cfg.RepoDir)
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}
	postAbs, err := filepath.Abs(postPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(repoDir, postAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("post path %s outside repo %s", postAbs, repoDir)
	}

	gitEnv := gitEnv(repoDir)
	if err := runCmd(ctx, gitEnv, "git add post", "git", "-C", repoDir, "add", "--", rel); err != nil {
		return err
	}

	changed, err := hasStagedChanges(ctx, repoDir, rel)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	msg := fmt.Sprintf("Embed YouTube video for %s", payload.Slug)
	if payload.Date != "" {
		msg = fmt.Sprintf("Embed YouTube video for %s", payload.Date)
	}
	if err := runCmd(ctx, gitEnv, "git commit post",
		"git", "-C", repoDir,
		"-c", "user.name="+cfg.AuthorName,
		"-c", "user.email="+cfg.AuthorEmail,
		"commit", "-m", msg,
	); err != nil {
		return err
	}

	pushURL, authEnv, err := pushTarget(ctx, cfg, repoDir)
	if err != nil {
		return err
	}
	return runCmd(ctx, authEnv, "git push post", "git", "-C", repoDir, "push", pushURL, "HEAD:"+cfg.PushBranch)
}

func hasStagedChanges(ctx context.Context, repoDir string, relPaths ...string) (bool, error) {
	args := []string{"-C", repoDir, "diff", "--cached", "--quiet", "--"}
	args = append(args, relPaths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(cmd.Environ(), gitEnv(repoDir)...)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("check staged changes: %w", err)
	}
	return false, nil
}

func pushTarget(ctx context.Context, cfg GitConfig, repoDir string) (string, []string, error) {
	remoteURL, err := captureCmd(ctx, gitEnv(repoDir), "resolve git remote", "git", "-C", repoDir, "remote", "get-url", cfg.RemoteName)
	if err != nil {
		return "", nil, err
	}
	pushURL, err := normalizeGitHubURL(strings.TrimSpace(remoteURL))
	if err != nil {
		return "", nil, err
	}
	parsed, err := url.Parse(pushURL)
	if err != nil {
		return "", nil, fmt.Errorf("parse push url: %w", err)
	}
	headerKey := fmt.Sprintf("http.%s://%s/.extraheader", parsed.Scheme, parsed.Host)
	token := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + cfg.PAT))
	authEnv := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=" + repoDir,
		"GIT_CONFIG_KEY_1=" + headerKey,
		"GIT_CONFIG_VALUE_1=AUTHORIZATION: basic " + token,
	}
	return pushURL, authEnv, nil
}

func normalizeGitHubURL(value string) (string, error) {
	t := strings.TrimSpace(value)
	if strings.HasPrefix(t, "git@github.com:") {
		return "https://github.com/" + strings.TrimPrefix(t, "git@github.com:"), nil
	}
	if strings.HasPrefix(t, "ssh://git@github.com/") {
		return "https://github.com/" + strings.TrimPrefix(t, "ssh://git@github.com/"), nil
	}
	if strings.HasPrefix(t, "https://github.com/") || strings.HasPrefix(t, "http://github.com/") {
		return t, nil
	}
	return "", fmt.Errorf("unsupported git remote for PAT push: %s", t)
}

func gitEnv(repoDir string) []string {
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=" + repoDir,
	}
}

func runCmd(ctx context.Context, extraEnv []string, description, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", description, msg)
	}
	return nil
}

func captureCmd(ctx context.Context, extraEnv []string, description, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", description, msg)
	}
	return strings.TrimSpace(string(out)), nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
