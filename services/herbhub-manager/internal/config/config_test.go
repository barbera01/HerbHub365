package config

import "testing"

func TestLoadMessagingDefaults(t *testing.T) {
	t.Setenv("RABBITMQ_MANAGEMENT_ENABLED", "")
	t.Setenv("RABBITMQ_MANAGEMENT_URL", "")
	t.Setenv("RABBITMQ_MANAGEMENT_VHOST", "")
	t.Setenv("PROMETHEUS_URL", "")
	t.Setenv("GRAFANA_URL", "")

	cfg := Load()
	if cfg.Messaging.Management.Enabled {
		t.Fatalf("expected management disabled by default")
	}
	if cfg.Messaging.Management.URL != "http://rabbitmq:15672" {
		t.Fatalf("unexpected default management URL %q", cfg.Messaging.Management.URL)
	}
	if cfg.Messaging.Management.VHost != "/" {
		t.Fatalf("unexpected default vhost %q", cfg.Messaging.Management.VHost)
	}
}

func TestLoadMessagingOverride(t *testing.T) {
	t.Setenv("RABBITMQ_MANAGEMENT_ENABLED", "true")
	t.Setenv("RABBITMQ_MANAGEMENT_URL", "http://mq.local:15672")
	t.Setenv("RABBITMQ_MANAGEMENT_VHOST", "my-vhost")
	t.Setenv("RABBITMQ_MANAGEMENT_USER", "admin")
	t.Setenv("RABBITMQ_MANAGEMENT_PASSWORD", "secret")
	t.Setenv("PROMETHEUS_URL", "http://prometheus:9090")
	t.Setenv("GRAFANA_URL", "https://grafana.example")

	cfg := Load()
	if !cfg.Messaging.Management.Enabled {
		t.Fatalf("expected enabled")
	}
	if cfg.Messaging.Management.URL != "http://mq.local:15672" {
		t.Fatalf("unexpected management URL %q", cfg.Messaging.Management.URL)
	}
	if cfg.Messaging.Management.VHost != "my-vhost" {
		t.Fatalf("unexpected vhost %q", cfg.Messaging.Management.VHost)
	}
	if cfg.Messaging.Prometheus.URL != "http://prometheus:9090" {
		t.Fatalf("unexpected prometheus URL %q", cfg.Messaging.Prometheus.URL)
	}
	if cfg.Messaging.GrafanaURL != "https://grafana.example" {
		t.Fatalf("unexpected grafana URL %q", cfg.Messaging.GrafanaURL)
	}
}

func TestLoadAutoWateringStatePath(t *testing.T) {
	t.Setenv("AUTOWATERING_STATE_PATH", "/tmp/aw.json")
	cfg := Load()
	if cfg.AutoWatering.StatePath != "/tmp/aw.json" {
		t.Fatalf("unexpected state path %q", cfg.AutoWatering.StatePath)
	}
}
