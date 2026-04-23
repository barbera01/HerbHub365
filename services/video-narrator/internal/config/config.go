package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for video-narrator.
type Config struct {
	// Mode controls how the service runs:
	//   daemon    — cron-scheduled, watches for new posts nightly
	//   generate  — generate a specific post and exit (use TARGET_DATE or TARGET_SLUG)
	//   backfill  — generate all existing posts that lack an output MP4 and exit
	//   dry-run   — preprocess + print text; no API calls, no file writes
	Mode string

	// GenerateSchedule is a cron expression for daemon mode (default: "15 0 * * *")
	GenerateSchedule string

	// RunOnStart triggers one generation pass immediately in daemon mode.
	RunOnStart bool

	// TargetDate is used in generate mode (yesterday / today / YYYY-MM-DD).
	TargetDate string

	// TargetSlug selects a specific post file by slug fragment in generate mode.
	TargetSlug string

	// Video is the avatar video API configuration.
	Video VideoConfig

	// TTS is the direct speech synthesis configuration used for timelapse narration.
	TTS TTSConfig

	// Concat controls the ffmpeg intro/outro post-processing step.
	Concat ConcatConfig

	// Post describes where Jekyll posts live.
	Post PostConfig

	// Server holds configuration for the HTTP server mode.
	Server ServerConfig

	// RulesFile is the path to tts-rules.json (preprocessing rules are shared with tts-narrator).
	RulesFile string

	// SkipPatterns is a list of filename substrings; posts whose filename
	// contains any of these strings will be skipped by backfill.
	SkipPatterns []string

	// RequestTimeout caps the HTTP download call.
	RequestTimeout time.Duration

	// RabbitMQURL is the AMQP connection string for completion notifications.
	RabbitMQURL string

	// RabbitMQQueue is the queue name for completion notifications.
	RabbitMQQueue string

	// TimelapseOutputDir is the directory where timelapse-builder writes its MP4s.
	// The timelapse narration pipeline reads source files from here.
	TimelapseOutputDir string

	// TimelapseAllowedHosts is an optional allowlist of hostnames (and optional
	// :port) that are permitted as timelapse_url download sources.
	// When empty, any public HTTPS/HTTP host is accepted.
	// Example: "timelapse.internal.example.com,storage.example.com:9000"
	TimelapseAllowedHosts []string
}

// VideoConfig holds avatar video API settings.
type VideoConfig struct {
	// BaseURL is the root of the video generation API, e.g. http://172.16.106.81:8011
	BaseURL string

	// AvatarID is the default avatar to use, e.g. "eve".
	AvatarID string

	// AvatarIDs is an ordered list for rotation in backfill mode; populated from VIDEO_AVATARS (csv).
	AvatarIDs []string

	// PollInterval is how often to check job status (default: 10s).
	PollInterval time.Duration

	// MaxWait is the longest we will wait for a single MuseTalk job to complete (default: 40m).
	MaxWait time.Duration

	// PipelineBuffer is extra time added to MaxWait for the overall pipeline context,
	// covering download + stitch after MuseTalk finishes (default: 10m).
	PipelineBuffer time.Duration

	// Note: resolution and fps are NOT sent to the API — MuseTalk preserves the
	// source avatar resolution (e.g. 1280×720 for a 720p green-screen loop) and
	// ignores any resolution/fps hint. Upscaling is handled by ffmpeg in concat.
}

// TTSConfig holds direct text-to-speech API settings.
type TTSConfig struct {
	BaseURL        string
	Model          string
	Voice          string
	Voices         []string
	Speed          float64
	ResponseFormat string
}

// ConcatConfig controls the ffmpeg intro/outro stitching step.
type ConcatConfig struct {
	// Enabled — when false the raw avatar MP4 is written directly to OutputDir without stitching.
	Enabled bool

	// IntroPath is the absolute path to the intro MP4.
	IntroPath string

	// OutroPath is the absolute path to the outro MP4.
	OutroPath string

	// OutputDir is where the final stitched MP4s are written.
	OutputDir string

	// FFmpegPath is the ffmpeg binary (default: "ffmpeg").
	FFmpegPath string

	// VideoCodec is the ffmpeg video encoder (default: "libx264").
	// Use "h264_nvenc" on machines with an NVIDIA GPU for hardware acceleration.
	VideoCodec string

	// CRF is the H.264 quality factor for libx264 (default: 18).
	// When VideoCodec is h264_nvenc this maps to -cq instead of -crf.
	CRF int

	// Preset is the encoder preset (default: "fast" for libx264, "p4" for h264_nvenc).
	Preset string

	// Threads limits the number of ffmpeg encoding threads (default: 1).
	// Only applies to software encoders (libx264). NVENC ignores this.
	Threads int

	// StitchTimeout is the maximum time allowed for the ffmpeg stitch step.
	// Uses a fresh context independent of the MuseTalk polling timeout.
	// Default: 30m (generous for CPU fallback; NVENC typically finishes in <1m).
	StitchTimeout time.Duration

	// ChromaKey controls green-screen removal on the avatar segment.
	ChromaKey ChromaKeyConfig
}

// ChromaKeyConfig controls the chroma-key (green-screen removal) step applied
// to the raw avatar video before it is composited and stitched.
type ChromaKeyConfig struct {
	// Enabled turns chroma keying on or off (default: false).
	Enabled bool

	// Color is the screen colour to remove in hex, e.g. "0x19AB3B" (default).
	Color string

	// Similarity is the normalised colour-distance threshold (0.0–1.0).
	// Higher values key out more shades of the target colour (default: 0.08).
	Similarity float64

	// Blend softens the key edge — pixels near the threshold are made
	// semi-transparent (0.0 = hard cut, default: 0.0).
	Blend float64

	// Despill removes green colour cast reflected onto the subject from the
	// screen (hair edges, shoulders). 0.0 = off, 1.0 = full despill (default: 0.5).
	Despill float64

	// BGPath is an optional path to a background image (.jpg/.png) or video
	// (.mp4) to composite the keyed avatar over. When empty, BGColor is used.
	BGPath string

	// BGColor is a hex colour used as the background when BGPath is empty,
	// e.g. "0x1a1a2e" (dark navy, default).
	BGColor string
}

// ServerConfig holds configuration for the HTTP server mode.
type ServerConfig struct {
	// ListenAddr is the address to bind the HTTP server (default: ":8090").
	ListenAddr string

	// ResourcesDir is the base directory for video resources (intros/outros).
	ResourcesDir string

	// BGDir is the directory containing background images for chroma-key.
	BGDir string
}

// PostConfig describes where Jekyll posts live.
type PostConfig struct {
	// PostsDir is the Jekyll _posts directory.
	PostsDir string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	hubDir := getEnv("HUB_DIR", "/repo/hub")
	postsDir := getEnv("BLOG_POSTS_DIR", filepath.Join(hubDir, "_posts"))

	return Config{
		Mode:                  getEnv("VIDEO_MODE", "daemon"),
		GenerateSchedule:      getEnv("VIDEO_SCHEDULE", "15 0 * * *"),
		RunOnStart:            getBoolEnv("VIDEO_RUN_ON_START", false),
		TargetDate:            getEnv("VIDEO_TARGET_DATE", "yesterday"),
		TargetSlug:            os.Getenv("VIDEO_TARGET_SLUG"),
		RulesFile:             getEnv("VIDEO_RULES_FILE", "/app/config/tts-rules.json"),
		SkipPatterns:          getCSVEnv("VIDEO_SKIP_PATTERN"),
		RequestTimeout:        getDurationEnv("VIDEO_REQUEST_TIMEOUT", 120*time.Second),
		RabbitMQURL:           os.Getenv("RABBITMQ_URL"),
		RabbitMQQueue:         getEnv("RABBITMQ_QUEUE", "video.produced"),
		TimelapseOutputDir:    getEnv("TIMELAPSE_OUTPUT_DIR", "/output"),
		TimelapseAllowedHosts: getCSVEnv("TIMELAPSE_ALLOWED_HOSTS"),

		Video: VideoConfig{
			BaseURL:        getEnv("VIDEO_BASE_URL", "http://172.16.106.81:8011"),
			AvatarID:       getEnv("VIDEO_AVATAR", "eve"),
			AvatarIDs:      getCSVEnv("VIDEO_AVATARS"),
			PollInterval:   getDurationEnv("VIDEO_POLL_INTERVAL", 10*time.Second),
			MaxWait:        getDurationEnv("VIDEO_MAX_WAIT", 40*time.Minute),
			PipelineBuffer: getDurationEnv("VIDEO_PIPELINE_BUFFER", 10*time.Minute),
		},

		TTS: TTSConfig{
			BaseURL:        getEnv("TTS_BASE_URL", "https://kokoro-api.lab.home-cloud.uk/v1/audio/speech"),
			Model:          getEnv("TTS_MODEL", "kokoro"),
			Voice:          getEnv("TTS_VOICE", "eve"),
			Voices:         getCSVEnv("TTS_VOICES"),
			Speed:          getFloatEnv("TTS_SPEED", 0),
			ResponseFormat: getEnv("TTS_FORMAT", "mp3"),
		},

		Concat: ConcatConfig{
			Enabled:       getBoolEnv("CONCAT_ENABLED", true),
			IntroPath:     getEnv("CONCAT_INTRO", "/app/resources/video/intro.mp4"),
			OutroPath:     getEnv("CONCAT_OUTRO", "/app/resources/video/outro.mp4"),
			OutputDir:     getEnv("VIDEO_OUTPUT_DIR", "/output/video"),
			FFmpegPath:    getEnv("FFMPEG_PATH", "ffmpeg"),
			VideoCodec:    getEnv("CONCAT_VIDEO_CODEC", "libx264"),
			CRF:           getIntEnv("CONCAT_CRF", 18),
			Preset:        getEnv("CONCAT_PRESET", "fast"),
			Threads:       getIntEnv("CONCAT_THREADS", 1),
			StitchTimeout: getDurationEnv("CONCAT_STITCH_TIMEOUT", 30*time.Minute),
			ChromaKey: ChromaKeyConfig{
				Enabled:    getBoolEnv("CHROMA_KEY_ENABLED", false),
				Color:      getEnv("CHROMA_KEY_COLOR", "0x19AB3B"),
				Similarity: getFloatEnv("CHROMA_KEY_SIMILARITY", 0.08),
				Blend:      getFloatEnv("CHROMA_KEY_BLEND", 0.0),
				Despill:    getFloatEnv("CHROMA_KEY_DESPILL", 0.5),
				BGPath:     os.Getenv("CHROMA_KEY_BG_PATH"),
				BGColor:    getEnv("CHROMA_KEY_BG_COLOR", "0x1a1a2e"),
			},
		},

		Post: PostConfig{
			PostsDir: postsDir,
		},

		Server: ServerConfig{
			ListenAddr:   getEnv("SERVER_LISTEN_ADDR", ":8090"),
			ResourcesDir: getEnv("SERVER_RESOURCES_DIR", "/app/resources/video"),
			BGDir:        getEnv("SERVER_BG_DIR", "/app/resources/video_backgrounds"),
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
			return time.Time{}, fmt.Errorf("invalid VIDEO_TARGET_DATE %q: use today, yesterday, or YYYY-MM-DD", c.TargetDate)
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

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("invalid int for %s: %v", key, err))
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
