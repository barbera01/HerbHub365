package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Mode               string
	DataDir            string
	GenerateSchedule   string
	GenerateTimeout    time.Duration
	Generation         GenerationConfig
	RunGenerateOnStart bool
	ReconnectDelay     time.Duration
	TargetDate         string
	RabbitMQ           RabbitMQConfig
	LLM                LLMConfig
	Blog               BlogConfig
	RepoPost           RepoPostConfig
	Git                GitConfig
}

type RabbitMQConfig struct {
	URL         string
	QueueName   string
	ConsumerTag string
	Prefetch    int
}

type GenerationConfig struct {
	PeriodMode string
	SplitHour  int
}

type LLMConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	Temperature     float64
	TopP            float64
	RepeatPenalty   float64
	MaxTokens       int
	RequestTimeout  time.Duration
	Debug           bool
	SystemPrompt    string
	PromptPlantName string
	PromptSiteName  string
	PromptSiteURL   string
}

type BlogConfig struct {
	HubDir          string
	PostsDir        string
	DraftsDir       string
	DraftPrefix     string
	Layout          string
	Categories      string
	Author          string
	Overwrite       bool
	SlugMaxWords    int
	ImageSourceDir  string
	ImageOutputDir  string
	ImagePublicPath string
	IncludeDayImage bool
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

type RepoPostConfig struct {
	Prompt        string
	Title         string
	Paths         []string
	Draft         bool
	Categories    string
	MaxFiles      int
	MaxFileBytes  int
	MaxTotalBytes int
}

func Load() Config {
	hubDir := getEnv("HUB_DIR", "/workspace/hub")
	postsDir := getEnv("BLOG_POSTS_DIR", filepath.Join(hubDir, "_posts"))

	return Config{
		Mode:             getEnv("BLOG_POSTER_MODE", "daemon"),
		DataDir:          getEnv("DATA_DIR", "/data"),
		GenerateSchedule: getEnv("BLOG_GENERATE_SCHEDULE", "5 0 * * *"),
		GenerateTimeout:  getDurationEnv("BLOG_GENERATE_TIMEOUT", 2*time.Minute),
		Generation: GenerationConfig{
			PeriodMode: getEnv("BLOG_GENERATE_PERIOD_MODE", "auto"),
			SplitHour:  getIntEnv("BLOG_GENERATE_SPLIT_HOUR", 12),
		},
		RunGenerateOnStart: getBoolEnv("BLOG_RUN_GENERATE_ON_START", false),
		ReconnectDelay:     getDurationEnv("RABBITMQ_RECONNECT_DELAY", 10*time.Second),
		TargetDate:         getEnv("BLOG_TARGET_DATE", "yesterday"),
		RabbitMQ: RabbitMQConfig{
			URL:         getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
			QueueName:   getEnv("RABBITMQ_QUEUE", "sensor.snapshots"),
			ConsumerTag: getEnv("RABBITMQ_CONSUMER_TAG", "blog-poster"),
			Prefetch:    getIntEnv("RABBITMQ_PREFETCH", 25),
		},
		LLM: LLMConfig{
			Provider:        getEnv("LLM_PROVIDER", "auto"),
			BaseURL:         getEnv("LLM_BASE_URL", "http://ollama.la.home-cloud.uk"),
			APIKey:          os.Getenv("LLM_API_KEY"),
			Model:           getEnv("LLM_MODEL", "qwen2.5:latest"),
			Temperature:     getFloatEnv("LLM_TEMPERATURE", 0.6),
			TopP:            getFloatEnv("LLM_TOP_P", 0.9),
			RepeatPenalty:   getFloatEnv("LLM_REPEAT_PENALTY", 1.1),
			MaxTokens:       getIntEnv("LLM_MAX_TOKENS", 900),
			RequestTimeout:  getDurationEnv("LLM_REQUEST_TIMEOUT", 90*time.Second),
			Debug:           getBoolEnv("LLM_DEBUG", false),
			SystemPrompt:    getEnv("LLM_SYSTEM_PROMPT", defaultSystemPrompt()),
			PromptPlantName: getEnv("PROMPT_PLANT_NAME", "Herb Hub 365"),
			PromptSiteName:  getEnv("PROMPT_SITE_NAME", "Herb Hub 365"),
			PromptSiteURL:   getEnv("PROMPT_SITE_URL", "https://www.herbhub365.com"),
		},
		Blog: BlogConfig{
			HubDir:          hubDir,
			PostsDir:        postsDir,
			DraftsDir:       getEnv("BLOG_DRAFTS_DIR", filepath.Join(hubDir, "_drafts")),
			DraftPrefix:     getEnv("BLOG_DRAFT_PREFIX", "draft"),
			Layout:          getEnv("BLOG_LAYOUT", "post"),
			Categories:      getEnv("BLOG_CATEGORIES", "Herb Hub Update"),
			Author:          os.Getenv("BLOG_AUTHOR"),
			Overwrite:       getBoolEnv("BLOG_OVERWRITE", false),
			SlugMaxWords:    getIntEnv("BLOG_SLUG_MAX_WORDS", 8),
			ImageSourceDir:  getEnv("BLOG_IMAGE_SOURCE_DIR", "/timelapse"),
			ImageOutputDir:  getEnv("BLOG_IMAGE_OUTPUT_DIR", filepath.Join(hubDir, "assets", "images", "blog")),
			ImagePublicPath: getEnv("BLOG_IMAGE_PUBLIC_PATH", "/assets/images/blog"),
			IncludeDayImage: getBoolEnv("BLOG_INCLUDE_DAY_IMAGE", false),
		},
		RepoPost: RepoPostConfig{
			Prompt:        os.Getenv("BLOG_REPO_POST_PROMPT"),
			Title:         os.Getenv("BLOG_REPO_POST_TITLE"),
			Paths:         getCSVEnv("BLOG_REPO_POST_PATHS"),
			Draft:         getBoolEnv("BLOG_REPO_POST_DRAFT", true),
			Categories:    getEnv("BLOG_REPO_POST_CATEGORIES", "Platform Update"),
			MaxFiles:      getIntEnv("BLOG_REPO_POST_MAX_FILES", 12),
			MaxFileBytes:  getIntEnv("BLOG_REPO_POST_MAX_FILE_BYTES", 12000),
			MaxTotalBytes: getIntEnv("BLOG_REPO_POST_MAX_TOTAL_BYTES", 48000),
		},
		Git: GitConfig{
			PublishEnabled: getBoolEnv("GIT_PUBLISH_ENABLED", false),
			RepoDir:        getEnv("GIT_REPO_DIR", "/repo"),
			RemoteName:     getEnv("GIT_REMOTE_NAME", "origin"),
			PushBranch:     getEnv("GIT_PUSH_BRANCH", "main"),
			PAT:            os.Getenv("GIT_PAT"),
			AuthorName:     getEnv("GIT_AUTHOR_NAME", "Herb Hub Bot"),
			AuthorEmail:    getEnv("GIT_AUTHOR_EMAIL", "bot@herbhub365.com"),
		},
	}
}

func (c Config) ResolveTargetDate(now time.Time) (time.Time, error) {
	value := strings.TrimSpace(strings.ToLower(c.TargetDate))
	today := now.UTC()

	switch value {
	case "", "today":
		return startOfDay(today), nil
	case "yesterday":
		return startOfDay(today.AddDate(0, 0, -1)), nil
	default:
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid BLOG_TARGET_DATE %q: use today, yesterday, or YYYY-MM-DD", c.TargetDate)
		}
		return startOfDay(parsed.UTC()), nil
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func defaultSystemPrompt() string {
	return "You are writing the daily Herb Hub 365 greenhouse blog entry. Write clear, grounded markdown for a public website. Use the supplied sensor summary only. Do not invent readings. Keep the tone warm, observational, and concise. Return markdown beginning with a level-1 heading for the title, followed by 3 to 6 short paragraphs."
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		panic(fmt.Sprintf("invalid boolean for %s: %v", key, err))
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("invalid integer for %s: %v", key, err))
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid float for %s: %v", key, err))
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Sprintf("invalid duration for %s: %v", key, err))
	}
	return parsed
}

func getCSVEnv(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
