package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"HerbHub365/services/herbhub-manager/internal/autowatering"
	"HerbHub365/services/herbhub-manager/internal/config"
)

func newAutoManagerForAPI(t *testing.T) *autowatering.Manager {
	t.Helper()
	st, err := autowatering.NewStore(autowatering.Bootstrap{
		Path:   filepath.Join(t.TempDir(), "state.json"),
		Config: autowatering.DefaultConfig(),
		Actor:  "test",
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return autowatering.NewManager(st, nil)
}

func TestAutomaticWateringGetAndETag(t *testing.T) {
	am := newAutoManagerForAPI(t)
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil, am)
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/automatic-watering", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.Code)
	}
	if resp.Header().Get("ETag") != `"1"` {
		t.Fatalf("missing or bad etag: %q", resp.Header().Get("ETag"))
	}
}

func TestAutomaticWateringPutRequiresIfMatchAndConfirmEnable(t *testing.T) {
	am := newAutoManagerForAPI(t)
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil, am)

	body := map[string]any{"config": autowatering.DefaultConfig(), "confirm_enable": false}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(buf))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 got %d", resp.Code)
	}

	cfg := autowatering.DefaultConfig()
	cfg.Enabled = true
	body = map[string]any{"config": cfg, "confirm_enable": false}
	buf, _ = json.Marshal(body)
	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(buf))
	req.Header.Set("If-Match", `"1"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", resp.Code)
	}

	body = map[string]any{"config": cfg, "confirm_enable": true}
	buf, _ = json.Marshal(body)
	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(buf))
	req.Header.Set("If-Match", `"1"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.Code)
	}
	if resp.Header().Get("ETag") != `"2"` {
		t.Fatalf("expected etag 2 got %q", resp.Header().Get("ETag"))
	}
}

func TestAutomaticWateringPutStaleRevision(t *testing.T) {
	am := newAutoManagerForAPI(t)
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil, am)
	body := map[string]any{"config": autowatering.DefaultConfig(), "confirm_enable": false}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(buf))
	req.Header.Set("If-Match", `"99"`)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 got %d", resp.Code)
	}
}

func TestAutomaticWateringPutInvalidIfMatchAndStrictBody(t *testing.T) {
	am := newAutoManagerForAPI(t)
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil, am)

	body := []byte(`{"config":{},"confirm_enable":false}`)
	req := httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(body))
	req.Header.Set("If-Match", `"bad"`)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(body))
	req.Header.Set("If-Match", `W/"1"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected weak etag 400 got %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(body))
	req.Header.Set("If-Match", `"1","2"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected multi etag 400 got %d", resp.Code)
	}

	body = []byte(`{"config":{},"confirm_enable":false,"extra":true}`)
	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(body))
	req.Header.Set("If-Match", `"1"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", resp.Code)
	}

	body = []byte(`{"config":{"enabled":true,"enabled":false},"confirm_enable":false}`)
	req = httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(body))
	req.Header.Set("If-Match", `"1"`)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected duplicate-key 400 got %d", resp.Code)
	}
}

func TestAutomaticWateringPutReturns503WhenUnsafe(t *testing.T) {
	am := autowatering.NewFaultedManager("faulted")
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil, am)
	body := map[string]any{"config": autowatering.DefaultConfig(), "confirm_enable": false}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/messaging/automatic-watering", bytes.NewReader(buf))
	req.Header.Set("If-Match", `"1"`)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", resp.Code)
	}
}
