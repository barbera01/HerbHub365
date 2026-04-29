package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr      string
	ShutdownTimeout time.Duration
	MaxConcurrent   int
	LLM             LLMConfig
}

type LLMConfig struct {
	Provider              string
	BaseURL               string
	APIKey                string
	Model                 string
	FallbackProvider      string
	FallbackBaseURL       string
	FallbackAPIKey        string
	FallbackModel         string
	Temperature           float64
	TopP                  float64
	RepeatPenalty         float64
	MaxTokens             int
	RequestTimeout        time.Duration
	ResponseHeaderTimeout time.Duration // how long to wait for the LLM to send the first byte
	Debug                 bool
}

func Load() Config {
	return Config{
		ListenAddr:      getEnv("LLM_SERVICE_LISTEN_ADDR", ":8080"),
		ShutdownTimeout: getDurationEnv("LLM_SERVICE_SHUTDOWN_TIMEOUT", 25*time.Minute),
		MaxConcurrent:   getIntEnv("LLM_SERVICE_MAX_CONCURRENT", 1),
		LLM: LLMConfig{
			Provider:              getEnv("LLM_PROVIDER", "auto"),
			BaseURL:               getEnv("LLM_BASE_URL", "http://ollama.la.home-cloud.uk"),
			APIKey:                trimEnvValue(os.Getenv("LLM_API_KEY")),
			Model:                 getEnv("LLM_MODEL", "qwen2.5:latest"),
			FallbackProvider:      getEnv("LLM_FALLBACK_PROVIDER", "gemini"),
			FallbackBaseURL:       getEnv("LLM_FALLBACK_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
			FallbackAPIKey:        trimEnvValue(getEnv("LLM_FALLBACK_API_KEY", os.Getenv("GEMINI_API_KEY"))),
			FallbackModel:         getEnv("LLM_FALLBACK_MODEL", "gemini-3.1-flash-lite-preview"),
			Temperature:           getFloatEnv("LLM_TEMPERATURE", 0.6),
			TopP:                  getFloatEnv("LLM_TOP_P", 0.9),
			RepeatPenalty:         getFloatEnv("LLM_REPEAT_PENALTY", 1.1),
			MaxTokens:             getIntEnv("LLM_MAX_TOKENS", 900),
			RequestTimeout:        getDurationEnv("LLM_REQUEST_TIMEOUT", 20*time.Minute),
			ResponseHeaderTimeout: getDurationEnv("LLM_RESPONSE_HEADER_TIMEOUT", 60*time.Second),
			Debug:                 getBoolEnv("LLM_DEBUG", false),
		},
	}
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

// Trim trailing whitespace/newlines from env values for multi-line secrets.
func trimEnvValue(value string) string {
	return strings.TrimSpace(value)
}

var _ = trimEnvValue // suppress unused warning — available for future use
