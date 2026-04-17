package config

import (
	"os"
	"time"
)

// Config holds all runtime configuration for herbhub-manager.
type Config struct {
	ListenAddr     string
	NarratorURL    string
	Post           PostConfig
	OutputDir      string
	PollInterval   time.Duration
	RequestTimeout time.Duration
	Blog           BlogConfig
}

// PostConfig describes where Jekyll posts live.
type PostConfig struct {
	PostsDir string
}

// BlogConfig holds settings for calling llm-service to generate blog posts.
type BlogConfig struct {
	LLMServiceURL string
	LLMTimeout    time.Duration
	SystemPrompt  string
	SiteName      string
	SiteURL       string
	PlantName     string
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

		Blog: BlogConfig{
			LLMServiceURL: getEnv("LLM_SERVICE_URL", "http://llm-service:8080"),
			LLMTimeout:    getDurationEnv("LLM_SERVICE_TIMEOUT", 25*time.Minute),
			SystemPrompt:  os.Getenv("BLOG_SYSTEM_PROMPT"),
			SiteName:      getEnv("BLOG_SITE_NAME", "HerbHub365"),
			SiteURL:       getEnv("BLOG_SITE_URL", "https://herbhub365.com"),
			PlantName:     getEnv("BLOG_PLANT_NAME", "herbs"),
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
