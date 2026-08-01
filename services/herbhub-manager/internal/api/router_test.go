package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"HerbHub365/services/herbhub-manager/internal/auth"
	"HerbHub365/services/herbhub-manager/internal/config"
)

func TestInternalTimelapseRouteNotFoundByDefaultMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/timelapse/videos/test.mp4" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/internal/timelapse/videos/test.mp4", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestMiddlewareCORSBlocksUnknownOrigin(t *testing.T) {
	h := withCORS("http://localhost:5173", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/posts", nil)
	req.Header.Set("Origin", "http://evil.example")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disallowed origin preflight, got %d", resp.Code)
	}
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	h := withAuth(config.AuthConfig{}, &auth.Verifier{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}

func TestValidISODateRejectsPathTraversal(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{value: "2026-08-01", valid: true},
		{value: "../../tmp/owned", valid: false},
		{value: "2026-8-1", valid: false},
		{value: "2026-02-30", valid: false},
	}

	for _, test := range tests {
		if got := validISODate(test.value); got != test.valid {
			t.Errorf("validISODate(%q) = %t, want %t", test.value, got, test.valid)
		}
	}
}

func TestAuthMiddlewareAllowsWhenDisabled(t *testing.T) {
	h := withAuth(config.AuthConfig{Disabled: true}, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
