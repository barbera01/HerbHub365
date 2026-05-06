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
	LLMService         LLMServiceConfig
	Prompt             PromptConfig
	Blog               BlogConfig
	RepoPost           RepoPostConfig
	PromPost           PromPostConfig
	Git                GitConfig
	SensorData         SensorDataConfig
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

// LLMServiceConfig holds connection details for the llm-service HTTP API.
type LLMServiceConfig struct {
	BaseURL string
	Timeout time.Duration
}

// PromptConfig holds the prompt content values used when building LLM requests.
// These stay in blog-poster because it is responsible for constructing prompts.
type PromptConfig struct {
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
	// BlobSASURL is the full Azure Blob SAS URL for the blog-images container,
	// e.g. https://herbhub365.blob.core.windows.net/blog-images?sv=...&sig=...
	// When set, images are uploaded via HTTP PUT instead of written to ImageOutputDir.
	BlobSASURL     string
	BlobPublicBase string
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

type PromPostConfig struct {
	BaseURL     string
	QueryPath   string
	QueryFile   string
	DefaultSpan time.Duration
	DefaultStep time.Duration
	Timeout     time.Duration
	Draft       bool
	Categories  string
	Layout      string
	ExportDir   string
	Schedule    string
	TargetDate  string
}

// SensorDataConfig controls writing of hub/_data/live_sensors.yml after each
// successful blog post publish.
type SensorDataConfig struct {
	// Enabled gates the feature entirely (SENSOR_DATA_ENABLED, default true).
	Enabled bool
	// DataFilePath is the absolute path to the YAML file to write
	// (SENSOR_DATA_FILE, default $HUB_DIR/_data/live_sensors.yml).
	DataFilePath string
	// ReservoirTempKey is the key in the snapshot Temperatures map that holds
	// the water reservoir temperature (SENSOR_RESERVOIR_TEMP_KEY, default "water").
	ReservoirTempKey string
	// MonitorBelow is the soil moisture % threshold below which a herb is
	// flagged as "monitor" (SENSOR_MOISTURE_MONITOR_BELOW, default 65).
	MonitorBelow float64
	// AlertBelow is the soil moisture % threshold below which a herb is
	// flagged as "alert" (SENSOR_MOISTURE_ALERT_BELOW, default 30).
	AlertBelow float64
	// herbNames maps sensor key → display name (from SENSOR_HERB_NAMES CSV "key:Name,...").
	herbNames map[string]string
	// herbIcons maps sensor key → icon name (from SENSOR_HERB_ICONS CSV "key:icon,...").
	herbIcons map[string]string
}

// HerbDisplayName returns the display name for a sensor key.
// Falls back to title-casing the key when no mapping exists.
func (c SensorDataConfig) HerbDisplayName(key string) string {
	if name, ok := c.herbNames[strings.ToLower(key)]; ok {
		return name
	}
	// Title-case the raw key as a fallback.
	parts := strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// HerbIcon returns the icon name for a sensor key.
// Falls back to substring matching against known herb names, then the raw key.
func (c SensorDataConfig) HerbIcon(key string) string {
	lower := strings.ToLower(key)
	if icon, ok := c.herbIcons[lower]; ok {
		return icon
	}
	// Substring fallback for common herbs.
	knownIcons := map[string]string{
		"basil": "basil", "chilli": "chilli", "chili": "chilli",
		"oregano": "oregano", "mint": "mint", "thyme": "thyme",
		"rosemary": "rosemary", "parsley": "parsley",
		"cilantro": "cilantro", "dill": "dill",
	}
	for fragment, icon := range knownIcons {
		if strings.Contains(lower, fragment) {
			return icon
		}
	}
	return lower
}

func Load() Config {
	hubDir := getEnv("HUB_DIR", "/workspace/hub")
	postsDir := getEnv("BLOG_POSTS_DIR", filepath.Join(hubDir, "_posts"))

	return Config{
		Mode:             getEnv("BLOG_POSTER_MODE", "daemon"),
		DataDir:          getEnv("DATA_DIR", "/data"),
		GenerateSchedule: getEnv("BLOG_GENERATE_SCHEDULE", "5 0 * * *"),
		GenerateTimeout:  getDurationEnv("BLOG_GENERATE_TIMEOUT", 10*time.Minute),
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
		LLMService: LLMServiceConfig{
			BaseURL: getEnv("LLM_SERVICE_URL", "http://llm-service:8080"),
			Timeout: getDurationEnv("LLM_SERVICE_TIMEOUT", 20*time.Minute),
		},
		Prompt: PromptConfig{
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
			BlobSASURL:      os.Getenv("BLOG_IMAGE_BLOB_SAS_URL"),
			BlobPublicBase:  getEnv("BLOG_IMAGE_BLOB_PUBLIC_BASE", "https://herbhub365.blob.core.windows.net/blog-images"),
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
		PromPost: PromPostConfig{
			BaseURL:     getEnv("PROM_POST_BASE_URL", "https://prometheus.home-cloud.uk"),
			QueryPath:   getEnv("PROM_POST_PATH", "/api/v1/query_range"),
			QueryFile:   getEnv("PROM_POST_QUERY_FILE", filepath.Join("/repo", "services", "blog-poster", "config", "prom-queries.json")),
			DefaultSpan: getDurationEnv("PROM_POST_RANGE", 24*time.Hour),
			DefaultStep: getDurationEnv("PROM_POST_STEP", 5*time.Minute),
			Timeout:     getDurationEnv("PROM_POST_TIMEOUT", 30*time.Second),
			Draft:       getBoolEnv("PROM_POST_DRAFT", true),
			Categories:  getEnv("PROM_POST_CATEGORIES", "Metrics Observability"),
			Layout:      getEnv("PROM_POST_LAYOUT", "post"),
			ExportDir:   getEnv("PROM_POST_EXPORT_DIR", filepath.Join(hubDir, "assets", "data", "prometheus")),
			Schedule:    getEnv("PROM_POST_SCHEDULE", ""),
			TargetDate:  getEnv("PROM_POST_TARGET_DATE", "today"),
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
		SensorData: SensorDataConfig{
			Enabled:          getBoolEnv("SENSOR_DATA_ENABLED", true),
			DataFilePath:     getEnv("SENSOR_DATA_FILE", filepath.Join(hubDir, "_data", "live_sensors.yml")),
			ReservoirTempKey: getEnv("SENSOR_RESERVOIR_TEMP_KEY", "water"),
			MonitorBelow:     getFloatEnv("SENSOR_MOISTURE_MONITOR_BELOW", 65),
			AlertBelow:       getFloatEnv("SENSOR_MOISTURE_ALERT_BELOW", 30),
			herbNames:        parseKVCSV(os.Getenv("SENSOR_HERB_NAMES")),
			herbIcons:        parseKVCSV(os.Getenv("SENSOR_HERB_ICONS")),
		},
	}
}

func (c Config) ResolveDate(dateStr string, now time.Time) (time.Time, error) {
	value := strings.TrimSpace(strings.ToLower(dateStr))
	today := now.UTC()
	switch value {
	case "", "today":
		return startOfDay(today), nil
	case "yesterday":
		return startOfDay(today.AddDate(0, 0, -1)), nil
	default:
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q: use today, yesterday, or YYYY-MM-DD", dateStr)
		}
		return startOfDay(parsed.UTC()), nil
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

// parseKVCSV parses a "key:value,key:value" string into a lowercased-key map.
// Used for SENSOR_HERB_NAMES and SENSOR_HERB_ICONS.
// Example: "basil:Basil,chilli:Chilli Pepper,water_res:Reservoir"
func parseKVCSV(value string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(value, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		if k != "" && v != "" {
			result[k] = v
		}
	}
	return result
}
