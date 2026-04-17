package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr    string
	InputDir      string
	OutputDir     string
	InputFPS      int
	OutputFPS     int
	CRF           int
	MinBrightness float64
	BuildTimeout  time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:    getEnv("LISTEN_ADDR", ":8082"),
		InputDir:      getEnv("INPUT_DIR", "/input"),
		OutputDir:     getEnv("OUTPUT_DIR", "/output"),
		InputFPS:      getIntEnv("INPUT_FPS", 8),
		OutputFPS:     getIntEnv("OUTPUT_FPS", 30),
		CRF:           getIntEnv("CRF", 23),
		MinBrightness: getFloatEnv("MIN_BRIGHTNESS", 0),
		BuildTimeout:  getDurationEnv("BUILD_TIMEOUT", 2*time.Hour),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
