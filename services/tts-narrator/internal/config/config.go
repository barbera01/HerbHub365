package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for tts-narrator.
type Config struct {
	// Mode controls how the service runs:
	//   daemon    — cron-scheduled, watches for new posts nightly
	//   generate  — narrate a specific post and exit (use TARGET_DATE or TARGET_SLUG)
	//   backfill  — narrate all existing posts that lack audio_url and exit
	//   dry-run   — preprocess + print text; no TTS call, no file writes
	Mode string

	// NarrateSchedule is a cron expression for daemon mode (default: "10 0 * * *")
	NarrateSchedule string

	// RunOnStart triggers one narration pass immediately in daemon mode.
	RunOnStart bool

	// TargetDate is used in generate mode (yesterday / today / YYYY-MM-DD).
	TargetDate string

	// TargetSlug selects a specific post file by slug fragment in generate mode.
	TargetSlug string

	// TTS is the Kokoro API configuration.
	TTS TTSConfig

	// Post describes where Jekyll posts and audio assets live.
	Post PostConfig

	// Git is used to commit + push the narrated post and MP3 back to the repo.
	Git GitConfig

	// RulesFile is the path to tts-rules.json; defaults to the bundled copy.
	RulesFile string

	// SkipPatterns is a list of filename substrings; posts whose filename
	// contains any of these strings will be skipped by backfill.
	// Populated from TTS_SKIP_PATTERN (comma-separated).
	SkipPatterns []string

	// RequestTimeout caps the TTS HTTP call.
	RequestTimeout time.Duration
}

// TTSConfig holds Kokoro TTS API settings.
type TTSConfig struct {
	BaseURL        string
	Model          string
	Voice          string   // single friendly name ("eve", "rowan") or raw Kokoro voice string
	Voices         []string // ordered list for rotation; populated from TTS_VOICES (csv)
	Speed          float64
	ResponseFormat string
}

// PostConfig describes where posts and audio assets live.
type PostConfig struct {
	// HubDir is the Jekyll hub root, e.g. /repo/hub
	HubDir string
	// PostsDir is the _posts directory.
	PostsDir string
	// AudioDir is where MP3 files are written, e.g. /repo/hub/assets/audio/blog
	AudioDir string
	// AudioPublicPath is the URL-path prefix served by Jekyll, e.g. /assets/audio/blog
	AudioPublicPath string
}

// GitConfig mirrors blog-poster's GitConfig.
type GitConfig struct {
	PublishEnabled bool
	RepoDir        string
	RemoteName     string
	PushBranch     string
	PAT            string
	AuthorName     string
	AuthorEmail    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	hubDir := getEnv("HUB_DIR", "/repo/hub")
	postsDir := getEnv("BLOG_POSTS_DIR", filepath.Join(hubDir, "_posts"))
	audioDir := getEnv("TTS_AUDIO_DIR", filepath.Join(hubDir, "assets", "audio", "blog"))

	return Config{
		Mode:            getEnv("TTS_MODE", "daemon"),
		NarrateSchedule: getEnv("TTS_SCHEDULE", "10 0 * * *"),
		RunOnStart:      getBoolEnv("TTS_RUN_ON_START", false),
		TargetDate:      getEnv("TTS_TARGET_DATE", "yesterday"),
		TargetSlug:      os.Getenv("TTS_TARGET_SLUG"),
		RulesFile:       getEnv("TTS_RULES_FILE", "/app/config/tts-rules.json"),
		SkipPatterns:    getCSVEnv("TTS_SKIP_PATTERN"),
		RequestTimeout:  getDurationEnv("TTS_REQUEST_TIMEOUT", 120*time.Second),
		TTS: TTSConfig{
			BaseURL:        getEnv("TTS_BASE_URL", "https://kokoro-api.lab.home-cloud.uk/v1/audio/speech"),
			Model:          getEnv("TTS_MODEL", "kokoro"),
			Voice:          getEnv("TTS_VOICE", "eve"),
			Voices:         getCSVEnv("TTS_VOICES"),
			Speed:          getFloatEnv("TTS_SPEED", 0), // 0 = use voice default
			ResponseFormat: getEnv("TTS_FORMAT", "mp3"),
		},
		Post: PostConfig{
			HubDir:          hubDir,
			PostsDir:        postsDir,
			AudioDir:        audioDir,
			AudioPublicPath: getEnv("TTS_AUDIO_PUBLIC_PATH", "/assets/audio/blog"),
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

// ResolveTargetDate converts TargetDate string to a UTC midnight time.Time.
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
			return time.Time{}, fmt.Errorf("invalid TTS_TARGET_DATE %q: use today, yesterday, or YYYY-MM-DD", c.TargetDate)
		}
		return startOfDay(parsed.UTC()), nil
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ── helpers ────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("invalid boolean for %s: %v", key, err))
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid float for %s: %v", key, err))
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("invalid duration for %s: %v", key, err))
	}
	return parsed
}

func getCSVEnv(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
