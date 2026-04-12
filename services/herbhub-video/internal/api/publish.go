package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"HerbHub365/services/herbhub-video/internal/config"
	"HerbHub365/services/herbhub-video/internal/queue"
	"HerbHub365/services/herbhub-video/internal/video"
)

// PublishCompletedJobs publishes completion messages for completed jobs.
// Safe to call from background pollers; a JSON marker prevents duplicates.
func PublishCompletedJobs(cfg config.Config, jobs []video.JobStatus) {
	publisher := queue.NewPublisher(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if publisher == nil || !publisher.Enabled() {
		return
	}
	if err := publishCompletedJobsWithPublisher(context.Background(), cfg, publisher, jobs); err != nil {
		log.Printf("publish jobs: %v", err)
	}
}

func publishCompletedJobsWithPublisher(ctx context.Context, cfg config.Config, publisher *queue.Publisher, jobs []video.JobStatus) error {
	if publisher == nil || !publisher.Enabled() {
		return nil
	}
	for _, job := range jobs {
		if strings.ToLower(job.Phase) != "completed" {
			continue
		}
		if err := publishCompletedJob(ctx, cfg, publisher, job); err != nil {
			log.Printf("publish completion for %s: %v", job.ID, err)
		}
	}
	return nil
}

func publishCompletedJob(ctx context.Context, cfg config.Config, publisher *queue.Publisher, job video.JobStatus) error {
	markerName := markerFilename(job)
	if markerName == "" {
		return nil
	}

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	markerPath := filepath.Join(cfg.OutputDir, markerName)
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	}

	outputFile := job.VideoFile
	if outputFile == "" && job.Slug != "" {
		outputFile = job.Slug + ".mp4"
	}
	if outputFile == "" {
		outputFile = strings.TrimSuffix(markerName, ".json") + ".mp4"
	}

	payload := map[string]any{
		"slug":        job.Slug,
		"date":        parseDateFromFilename(outputFile),
		"output_file": outputFile,
		"status":      "completed",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if err := publisher.Publish(ctx, data); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	if err := os.WriteFile(markerPath, data, 0644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	return nil
}

func markerFilename(job video.JobStatus) string {
	if job.VideoFile != "" {
		return strings.TrimSuffix(job.VideoFile, ".mp4") + ".json"
	}
	if job.Slug != "" {
		return job.Slug + ".json"
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
