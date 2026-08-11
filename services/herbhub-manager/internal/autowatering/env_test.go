package autowatering

import "testing"

func TestBootstrapFromEnvDefaults(t *testing.T) {
	t.Setenv("AUTOWATERING_STATE_PATH", "")
	b, err := BootstrapFromEnv()
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if b.Path != DefaultStatePath {
		t.Fatalf("path=%q", b.Path)
	}
	if b.Config.Enabled {
		t.Fatalf("default enabled must be false")
	}
}

func TestBootstrapFromEnvStrictErrors(t *testing.T) {
	t.Setenv("AUTOWATERING_ENABLED", "maybe")
	if _, err := BootstrapFromEnv(); err == nil {
		t.Fatalf("expected strict bool error")
	}
	t.Setenv("AUTOWATERING_ENABLED", "true")
	t.Setenv("AUTOWATERING_EVALUATION_INTERVAL_SECONDS", "abc")
	if _, err := BootstrapFromEnv(); err == nil {
		t.Fatalf("expected strict int error")
	}
}
