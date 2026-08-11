package autowatering

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	StateVersion = 1

	DefaultStatePath = "/var/lib/herbhub-manager/automatic-watering.json"

	PlantBasil   = "basil"
	PlantChilli  = "chilli"
	PlantOregano = "oregano"
)

var (
	fixedPlants              = []string{PlantBasil, PlantChilli, PlantOregano}
	promIdentifierPattern    = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	safeLabelValuePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/-]{0,63}$`)
	allowedFutureSampleSkew  = time.Minute
	defaultEvaluationSeconds = 300
	defaultCooldownSeconds   = 21600
	defaultMaxAgeSeconds     = 900
	defaultPromTimeout       = 10
	defaultExpirySeconds     = 300
)

type Config struct {
	Enabled                   bool                   `json:"enabled"`
	EvaluationIntervalSeconds int                    `json:"evaluation_interval_seconds"`
	CooldownSeconds           int                    `json:"cooldown_seconds"`
	MaxMetricAgeSeconds       int                    `json:"max_metric_age_seconds"`
	PrometheusTimeoutSeconds  int                    `json:"prometheus_timeout_seconds"`
	MessageExpirySeconds      int                    `json:"message_expiry_seconds"`
	MoistureMetric            string                 `json:"moisture_metric"`
	PlantLabel                string                 `json:"plant_label"`
	Plants                    map[string]PlantConfig `json:"plants"`
}

type PlantConfig struct {
	Enabled          bool    `json:"enabled"`
	ThresholdPercent float64 `json:"threshold_percent"`
	MetricLabelValue string  `json:"metric_label_value"`
}

type Decision string

const (
	DecisionDisabled            Decision = "disabled"
	DecisionNoMetric            Decision = "no_metric"
	DecisionMultipleMetrics     Decision = "multiple_metrics"
	DecisionInvalidMetric       Decision = "invalid_metric"
	DecisionStaleMetric         Decision = "stale_metric"
	DecisionFutureMetric        Decision = "future_metric"
	DecisionHealthy             Decision = "healthy"
	DecisionCooldown            Decision = "cooldown"
	DecisionDeliveryUnavailable Decision = "delivery_unavailable"
	DecisionWaterPublished      Decision = "water_published"
	DecisionPublishUncertain    Decision = "publish_uncertain"
	DecisionPublishFailed       Decision = "publish_failed"
)

type PlantRuntimeState struct {
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	LastSampleAt    *time.Time `json:"last_sample_at,omitempty"`
	LastValue       *float64   `json:"last_value,omitempty"`
	LastDecision    string     `json:"last_decision,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CooldownUntil   *time.Time `json:"cooldown_until,omitempty"`
	LastMessageID   string     `json:"last_message_id,omitempty"`
}

type Document struct {
	Version         int                          `json:"version"`
	ConfigRevision  int64                        `json:"config_revision"`
	ConfigUpdatedAt time.Time                    `json:"config_updated_at"`
	ConfigUpdatedBy string                       `json:"config_updated_by"`
	Config          Config                       `json:"config"`
	Plants          map[string]PlantRuntimeState `json:"plants"`
}

type RuntimeStatus struct {
	Running          bool       `json:"running"`
	Faulted          bool       `json:"faulted"`
	Fault            string     `json:"fault,omitempty"`
	InstanceID       string     `json:"instance_id,omitempty"`
	NextEvaluationAt *time.Time `json:"next_evaluation_at,omitempty"`
}

func FixedPlants() []string {
	out := make([]string, len(fixedPlants))
	copy(out, fixedPlants)
	return out
}

func DefaultConfig() Config {
	return Config{
		Enabled:                   false,
		EvaluationIntervalSeconds: defaultEvaluationSeconds,
		CooldownSeconds:           defaultCooldownSeconds,
		MaxMetricAgeSeconds:       defaultMaxAgeSeconds,
		PrometheusTimeoutSeconds:  defaultPromTimeout,
		MessageExpirySeconds:      defaultExpirySeconds,
		MoistureMetric:            "herbhub_soil_percent",
		PlantLabel:                "plant",
		Plants: map[string]PlantConfig{
			PlantBasil:   {Enabled: true, ThresholdPercent: 30, MetricLabelValue: PlantBasil},
			PlantChilli:  {Enabled: true, ThresholdPercent: 30, MetricLabelValue: PlantChilli},
			PlantOregano: {Enabled: true, ThresholdPercent: 30, MetricLabelValue: PlantOregano},
		},
	}
}

func DefaultDocument(now time.Time, actor string, cfg Config) Document {
	plants := map[string]PlantRuntimeState{}
	for _, p := range fixedPlants {
		plants[p] = PlantRuntimeState{}
	}
	if strings.TrimSpace(actor) == "" {
		actor = "bootstrap"
	}
	return Document{
		Version:         StateVersion,
		ConfigRevision:  1,
		ConfigUpdatedAt: now.UTC(),
		ConfigUpdatedBy: actor,
		Config:          cfg,
		Plants:          plants,
	}
}

func ValidateConfig(cfg Config) error {
	var errs []string
	checkRange := func(name string, value, min, max int) {
		if value < min || value > max {
			errs = append(errs, fmt.Sprintf("%s must be in range [%d,%d]", name, min, max))
		}
	}

	checkRange("evaluation_interval_seconds", cfg.EvaluationIntervalSeconds, 60, 3600)
	checkRange("cooldown_seconds", cfg.CooldownSeconds, 3600, 604800)
	checkRange("max_metric_age_seconds", cfg.MaxMetricAgeSeconds, 60, 3600)
	checkRange("prometheus_timeout_seconds", cfg.PrometheusTimeoutSeconds, 1, 30)
	checkRange("message_expiry_seconds", cfg.MessageExpirySeconds, 30, 300)

	if cfg.MaxMetricAgeSeconds < cfg.EvaluationIntervalSeconds {
		errs = append(errs, "max_metric_age_seconds must be >= evaluation_interval_seconds")
	}

	if !promIdentifierPattern.MatchString(cfg.MoistureMetric) {
		errs = append(errs, "moisture_metric must be a valid Prometheus identifier")
	}
	if !promIdentifierPattern.MatchString(cfg.PlantLabel) {
		errs = append(errs, "plant_label must be a valid Prometheus identifier")
	}

	if cfg.Plants == nil {
		errs = append(errs, "plants is required")
	} else {
		enabledLabelValues := map[string]string{}
		for name := range cfg.Plants {
			if !isFixedPlant(name) {
				errs = append(errs, fmt.Sprintf("plants.%s is unknown", name))
			}
		}
		for _, plant := range fixedPlants {
			pc, ok := cfg.Plants[plant]
			if !ok {
				errs = append(errs, fmt.Sprintf("plants.%s is required", plant))
				continue
			}
			if math.IsNaN(pc.ThresholdPercent) || math.IsInf(pc.ThresholdPercent, 0) || pc.ThresholdPercent < 5 || pc.ThresholdPercent > 80 {
				errs = append(errs, fmt.Sprintf("plants.%s.threshold_percent must be in range [5,80]", plant))
			}
			if !safeLabelValuePattern.MatchString(strings.TrimSpace(pc.MetricLabelValue)) {
				errs = append(errs, fmt.Sprintf("plants.%s.metric_label_value must be a safe non-empty label value", plant))
			}
			if pc.Enabled {
				label := strings.TrimSpace(pc.MetricLabelValue)
				if otherPlant, exists := enabledLabelValues[label]; exists {
					errs = append(errs, fmt.Sprintf("enabled plants must have unique metric_label_value; %s and %s both use %q", otherPlant, plant, label))
				} else {
					enabledLabelValues[label] = plant
				}
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("invalid automatic watering config: %s", strings.Join(errs, "; "))
	}
	return nil
}

func isFixedPlant(plant string) bool {
	for _, p := range fixedPlants {
		if p == plant {
			return true
		}
	}
	return false
}
