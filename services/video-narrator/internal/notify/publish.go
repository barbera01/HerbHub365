package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"HerbHub365/services/video-narrator/internal/config"
	"HerbHub365/services/video-narrator/internal/queue"
)

// PublishCompletion emits a completion message and writes a JSON marker file.
func PublishCompletion(ctx context.Context, cfg config.Config, slug, outputFile string) error {
	if err := os.MkdirAll(cfg.Concat.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	markerName := markerFilename(outputFile, slug)
	if markerName == "" {
		return nil
	}
	markerPath := filepath.Join(cfg.Concat.OutputDir, markerName)

	payload := map[string]any{
		"slug":        slug,
		"date":        parseDateFromFilename(outputFile),
		"output_file": outputFile,
		"status":      "completed",
		"youtube_id":  "",
		"youtube_url": "",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if err := os.WriteFile(markerPath, data, 0644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	publisher := queue.NewPublisher(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if publisher == nil || !publisher.Enabled() {
		return nil
	}
	if err := publisher.Publish(ctx, data); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
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

func parseDateFromFilename(name string) string {
	if len(name) < 10 {
		return ""
	}
	date := name[:10]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ""
	}
	return date
}
