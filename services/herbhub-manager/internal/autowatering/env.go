package autowatering

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func BootstrapFromEnv() (Bootstrap, error) {
	cfg := DefaultConfig()
	path := strings.TrimSpace(os.Getenv("AUTOWATERING_STATE_PATH"))
	if path == "" {
		path = DefaultStatePath
	}

	var err error
	if cfg.Enabled, err = parseBoolStrictEnv("AUTOWATERING_ENABLED", cfg.Enabled); err != nil {
		return Bootstrap{}, err
	}
	if cfg.EvaluationIntervalSeconds, err = parseIntStrictEnv("AUTOWATERING_EVALUATION_INTERVAL_SECONDS", cfg.EvaluationIntervalSeconds); err != nil {
		return Bootstrap{}, err
	}
	if cfg.CooldownSeconds, err = parseIntStrictEnv("AUTOWATERING_COOLDOWN_SECONDS", cfg.CooldownSeconds); err != nil {
		return Bootstrap{}, err
	}
	if cfg.MaxMetricAgeSeconds, err = parseIntStrictEnv("AUTOWATERING_MAX_METRIC_AGE_SECONDS", cfg.MaxMetricAgeSeconds); err != nil {
		return Bootstrap{}, err
	}
	if cfg.PrometheusTimeoutSeconds, err = parseIntStrictEnv("AUTOWATERING_PROMETHEUS_TIMEOUT_SECONDS", cfg.PrometheusTimeoutSeconds); err != nil {
		return Bootstrap{}, err
	}
	if cfg.MessageExpirySeconds, err = parseIntStrictEnv("AUTOWATERING_MESSAGE_EXPIRY_SECONDS", cfg.MessageExpirySeconds); err != nil {
		return Bootstrap{}, err
	}

	if val := strings.TrimSpace(os.Getenv("AUTOWATERING_MOISTURE_METRIC")); val != "" {
		cfg.MoistureMetric = val
	}
	if val := strings.TrimSpace(os.Getenv("AUTOWATERING_PLANT_LABEL")); val != "" {
		cfg.PlantLabel = val
	}

	for _, plant := range fixedPlants {
		pc := cfg.Plants[plant]
		enabledKey := "AUTOWATERING_PLANT_" + strings.ToUpper(plant) + "_ENABLED"
		if pc.Enabled, err = parseBoolStrictEnv(enabledKey, pc.Enabled); err != nil {
			return Bootstrap{}, err
		}
		thresholdKey := "AUTOWATERING_PLANT_" + strings.ToUpper(plant) + "_THRESHOLD_PERCENT"
		if pc.ThresholdPercent, err = parseFloatStrictEnv(thresholdKey, pc.ThresholdPercent); err != nil {
			return Bootstrap{}, err
		}
		labelKey := "AUTOWATERING_PLANT_" + strings.ToUpper(plant) + "_METRIC_LABEL_VALUE"
		if val := strings.TrimSpace(os.Getenv(labelKey)); val != "" {
			pc.MetricLabelValue = val
		}
		cfg.Plants[plant] = pc
	}

	if err := ValidateConfig(cfg); err != nil {
		return Bootstrap{}, err
	}

	return Bootstrap{Path: path, Config: cfg, Actor: "bootstrap"}, nil
}

func parseIntStrictEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return v, nil
}

func parseFloatStrictEnv(key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return v, nil
}

func parseBoolStrictEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback, nil
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a strict boolean", key)
	}
}
