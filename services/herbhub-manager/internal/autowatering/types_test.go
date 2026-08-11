package autowatering

import "testing"

func TestValidateConfigAcceptsDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestValidateConfigRejectsUnknownPlantAndCrossField(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxMetricAgeSeconds = 100
	cfg.EvaluationIntervalSeconds = 120
	cfg.Plants["mint"] = PlantConfig{Enabled: true, ThresholdPercent: 20, MetricLabelValue: "mint"}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateConfigRejectsPromIdentifiers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MoistureMetric = `rate(x[5m])`
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected metric validation failure")
	}
	cfg = DefaultConfig()
	cfg.PlantLabel = `plant"`
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected label validation failure")
	}
}

func TestValidateConfigRejectsThresholdAndLabelValue(t *testing.T) {
	cfg := DefaultConfig()
	plant := cfg.Plants[PlantBasil]
	plant.ThresholdPercent = 81
	plant.MetricLabelValue = `basil"} or up`
	cfg.Plants[PlantBasil] = plant
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateConfigRejectsDuplicateEnabledMetricLabelValues(t *testing.T) {
	cfg := DefaultConfig()
	basil := cfg.Plants[PlantBasil]
	chilli := cfg.Plants[PlantChilli]
	basil.Enabled = true
	chilli.Enabled = true
	basil.MetricLabelValue = "shared"
	chilli.MetricLabelValue = "shared"
	cfg.Plants[PlantBasil] = basil
	cfg.Plants[PlantChilli] = chilli
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected duplicate enabled metric label value error")
	}
}
