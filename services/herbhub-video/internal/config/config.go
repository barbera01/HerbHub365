package config

import (
	"os"
	"time"
)

// Config holds all runtime configuration for herbhub-video.
type Config struct {
	// ListenAddr is the HTTP server address (default: ":8080").
	ListenAddr string

	// NarratorURL is the video-narrator API base URL (default: "http://localhost:8090").
	NarratorURL string

	// Post describes where Jekyll posts live.
	Post PostConfig

	// OutputDir is where generated MP4 files are stored.
	OutputDir string

	// PollInterval is how often to poll the narrator for job status.
	PollInterval time.Duration

	// RequestTimeout caps HTTP calls to the narrator.
	RequestTimeout time.Duration
}

// PostConfig describes where Jekyll posts live.
type PostConfig struct {
	// PostsDir is the Jekyll _posts directory.
	PostsDir string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	hubDir := getEnv("HUB_DIR", "/repo/hub")
	postsDir := getEnv("BLOG_POSTS_DIR", hubDir+"/_posts")

	return Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		NarratorURL:    getEnv("NARRATOR_URL", "http://localhost:8090"),
		OutputDir:      getEnv("VIDEO_OUTPUT_DIR", "/output/video"),
		PollInterval:   getDurationEnv("POLL_INTERVAL", 3*time.Second),
		RequestTimeout: getDurationEnv("REQUEST_TIMEOUT", 120*time.Second),

		Post: PostConfig{
			PostsDir: postsDir,
		},
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
