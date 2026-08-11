package config

import (
	"os"
	"strings"
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
	AllowedOrigin  string
	Blog           BlogConfig
	Timelapse      TimelapseConfig
	RabbitMQ       RabbitMQConfig
	Messaging      MessagingConfig
	Auth           AuthConfig
}

// AuthConfig holds workforce Entra token validation configuration.
type AuthConfig struct {
	Disabled     bool
	IssuerURL    string
	Audience     string
	RequiredRole string
	DiscoveryURL string
	JWKSRefresh  time.Duration
	HTTPTimeout  time.Duration
}

// RabbitMQConfig holds settings for publishing to the video.produced queue.
type RabbitMQConfig struct {
	URL   string // RABBITMQ_URL (amqp://...)
	Queue string // RABBITMQ_QUEUE
}

// MessagingConfig holds settings for curated RabbitMQ management operations.
type MessagingConfig struct {
	Management RabbitMQManagementConfig
	Prometheus PrometheusConfig
	GrafanaURL string
}

// RabbitMQManagementConfig holds RabbitMQ Management API settings.
type RabbitMQManagementConfig struct {
	Enabled  bool
	URL      string
	VHost    string
	User     string
	Password string
	Timeout  time.Duration
}

// PrometheusConfig holds optional Prometheus query settings.
type PrometheusConfig struct {
	URL     string
	Timeout time.Duration
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

// TimelapseConfig holds settings for calling timelapse-builder.
type TimelapseConfig struct {
	ServiceURL  string
	PublicURL   string
	InternalURL string
	Timeout     time.Duration
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
		AllowedOrigin:  strings.TrimSpace(os.Getenv("ALLOWED_ORIGIN")),

		Post: PostConfig{
			PostsDir: postsDir,
		},

		Timelapse: TimelapseConfig{
			ServiceURL:  getEnv("TIMELAPSE_SERVICE_URL", "http://timelapse-builder:8082"),
			PublicURL:   getEnv("TIMELAPSE_PUBLIC_URL", "https://manager.herbhub365.com"),
			InternalURL: getEnv("TIMELAPSE_INTERNAL_URL", "http://localhost:8080"),
			Timeout:     getDurationEnv("TIMELAPSE_SERVICE_TIMEOUT", 30*time.Second),
		},

		RabbitMQ: RabbitMQConfig{
			URL:   os.Getenv("RABBITMQ_URL"),
			Queue: getEnv("RABBITMQ_QUEUE", "video.produced"),
		},

		Messaging: MessagingConfig{
			Management: RabbitMQManagementConfig{
				Enabled:  getBoolEnv("RABBITMQ_MANAGEMENT_ENABLED", false),
				URL:      getEnv("RABBITMQ_MANAGEMENT_URL", "http://rabbitmq:15672"),
				VHost:    getEnv("RABBITMQ_MANAGEMENT_VHOST", "/"),
				User:     strings.TrimSpace(os.Getenv("RABBITMQ_MANAGEMENT_USER")),
				Password: os.Getenv("RABBITMQ_MANAGEMENT_PASSWORD"),
				Timeout:  getDurationEnv("RABBITMQ_MANAGEMENT_TIMEOUT", 10*time.Second),
			},
			Prometheus: PrometheusConfig{
				URL:     strings.TrimSpace(os.Getenv("PROMETHEUS_URL")),
				Timeout: getDurationEnv("PROMETHEUS_TIMEOUT", 10*time.Second),
			},
			GrafanaURL: strings.TrimSpace(os.Getenv("GRAFANA_URL")),
		},

		Blog: BlogConfig{
			LLMServiceURL: getEnv("LLM_SERVICE_URL", "http://llm-service:8080"),
			LLMTimeout:    getDurationEnv("LLM_SERVICE_TIMEOUT", 25*time.Minute),
			SystemPrompt:  os.Getenv("BLOG_SYSTEM_PROMPT"),
			SiteName:      getEnv("BLOG_SITE_NAME", "HerbHub365"),
			SiteURL:       getEnv("BLOG_SITE_URL", "https://herbhub365.com"),
			PlantName:     getEnv("BLOG_PLANT_NAME", "herbs"),
		},

		Auth: AuthConfig{
			Disabled:     getBoolEnv("AUTH_DISABLED", false),
			IssuerURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("AUTH_ISSUER_URL")), "/"),
			Audience:     strings.TrimSpace(os.Getenv("AUTH_AUDIENCE")),
			RequiredRole: getEnv("AUTH_REQUIRED_ROLE", "Manager.Operator"),
			DiscoveryURL: strings.TrimSpace(os.Getenv("AUTH_DISCOVERY_URL")),
			JWKSRefresh:  getDurationEnv("AUTH_JWKS_MIN_REFRESH", time.Minute),
			HTTPTimeout:  getDurationEnv("AUTH_HTTP_TIMEOUT", 10*time.Second),
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

func getBoolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
