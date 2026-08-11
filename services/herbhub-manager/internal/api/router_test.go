package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"HerbHub365/services/herbhub-manager/internal/auth"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/messaging"
	"HerbHub365/services/herbhub-manager/internal/rabbitmq"
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

type routerFakeRabbit struct {
	exchanges map[string]rabbitmq.Exchange
	queues    map[string]rabbitmq.Queue
	bindings  map[string][]rabbitmq.Binding
}

func (f *routerFakeRabbit) Ping(_ context.Context) error { return nil }
func (f *routerFakeRabbit) GetExchange(_ context.Context, name string) (rabbitmq.Exchange, error) {
	v, ok := f.exchanges[name]
	if !ok {
		return rabbitmq.Exchange{}, rabbitmq.ErrNotFound
	}
	return v, nil
}
func (f *routerFakeRabbit) EnsureExchange(_ context.Context, name, kind string, durable bool) error {
	f.exchanges[name] = rabbitmq.Exchange{Name: name, Type: kind, Durable: durable, AutoDelete: false, Internal: false}
	return nil
}
func (f *routerFakeRabbit) GetQueue(_ context.Context, name string) (rabbitmq.Queue, error) {
	v, ok := f.queues[name]
	if !ok {
		return rabbitmq.Queue{}, rabbitmq.ErrNotFound
	}
	return v, nil
}
func (f *routerFakeRabbit) EnsureQueue(_ context.Context, name string, durable bool, arguments map[string]any) error {
	f.queues[name] = rabbitmq.Queue{Name: name, Durable: durable, AutoDelete: false, Exclusive: false, Arguments: arguments}
	return nil
}
func (f *routerFakeRabbit) ListBindings(_ context.Context, exchange, queue string) ([]rabbitmq.Binding, error) {
	return f.bindings[exchange+"->"+queue], nil
}
func (f *routerFakeRabbit) EnsureBinding(_ context.Context, exchange, queue, routingKey string, arguments map[string]any) error {
	k := exchange + "->" + queue
	f.bindings[k] = append(f.bindings[k], rabbitmq.Binding{Source: exchange, Destination: queue, DestinationType: "queue", RoutingKey: routingKey, Arguments: arguments})
	return nil
}
func (f *routerFakeRabbit) ListSourceBindings(_ context.Context, exchange string) ([]rabbitmq.Binding, error) {
	out := []rabbitmq.Binding{}
	for _, list := range f.bindings {
		for _, b := range list {
			if b.Source == exchange {
				out = append(out, b)
			}
		}
	}
	return out, nil
}
func (f *routerFakeRabbit) Publish(_ context.Context, req rabbitmq.PublishRequest) (rabbitmq.PublishResponse, error) {
	return rabbitmq.PublishResponse{Routed: true}, nil
}

func TestMessagingRoutesUnknownAndDisabled(t *testing.T) {
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/messaging/catalogues/watering/provision", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", resp.Code)
	}
}

func TestMessagingRoutesStatusCodes(t *testing.T) {
	fr := &routerFakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}}
	for _, cat := range messaging.Catalogues() {
		fr.bindings["self"] = append(fr.bindings["self"], rabbitmq.Binding{Source: cat.Exchange.Name}, rabbitmq.Binding{Source: cat.DLX.Name})
		fr.exchanges[cat.Exchange.Name] = rabbitmq.Exchange{Name: cat.Exchange.Name, Type: cat.Exchange.Type, Durable: cat.Exchange.Durable, AutoDelete: cat.Exchange.AutoDelete, Internal: cat.Exchange.Internal}
		fr.exchanges[cat.DLX.Name] = rabbitmq.Exchange{Name: cat.DLX.Name, Type: cat.DLX.Type, Durable: cat.DLX.Durable, AutoDelete: cat.DLX.AutoDelete, Internal: cat.DLX.Internal}
		fr.queues[cat.MainQueue.Name] = rabbitmq.Queue{Name: cat.MainQueue.Name, Durable: cat.MainQueue.Durable, AutoDelete: cat.MainQueue.AutoDelete, Exclusive: cat.MainQueue.Exclusive, Arguments: cat.MainQueue.Arguments}
		fr.queues[cat.DLQ.Name] = rabbitmq.Queue{Name: cat.DLQ.Name, Durable: cat.DLQ.Durable, AutoDelete: cat.DLQ.AutoDelete, Exclusive: cat.DLQ.Exclusive, Arguments: cat.DLQ.Arguments}
		for _, b := range cat.Bindings {
			k := b.Exchange + "->" + b.Queue
			fr.bindings[k] = append(fr.bindings[k], rabbitmq.Binding{Source: b.Exchange, Destination: b.Queue, DestinationType: "queue", RoutingKey: b.RoutingKey, Arguments: b.Arguments})
		}
		for _, b := range cat.LegacyExpectedBindings {
			k := b.Exchange + "->" + b.Queue
			fr.bindings[k] = append(fr.bindings[k], rabbitmq.Binding{Source: b.Exchange, Destination: b.Queue, DestinationType: "queue", RoutingKey: b.RoutingKey, Arguments: b.Arguments})
		}
	}
	msvc := messaging.NewService(config.MessagingConfig{Management: config.RabbitMQManagementConfig{Enabled: true, URL: "http://rabbitmq:15672", VHost: "/", User: "admin", Password: "secret", Timeout: 10 * time.Second}}, fr)
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}, Messaging: config.MessagingConfig{Management: config.RabbitMQManagementConfig{Enabled: true, URL: "http://rabbitmq:15672", VHost: "/", User: "admin", Password: "secret"}}}, nil, nil, nil, nil, nil, nil, msvc)

	unknownCatalogue := httptest.NewRecorder()
	r.ServeHTTP(unknownCatalogue, httptest.NewRequest(http.MethodPost, "/api/messaging/catalogues/nope/provision", nil))
	if unknownCatalogue.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", unknownCatalogue.Code)
	}

	unknownTemplate := httptest.NewRecorder()
	r.ServeHTTP(unknownTemplate, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/nope/publish", bytes.NewReader([]byte(`{"payload":{},"confirmed":true}`))))
	if unknownTemplate.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", unknownTemplate.Code)
	}

	badValidation := httptest.NewRecorder()
	r.ServeHTTP(badValidation, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-water/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":12},"confirmed":true}`))))
	if badValidation.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", badValidation.Code)
	}

	missingConfirm := httptest.NewRecorder()
	r.ServeHTTP(missingConfirm, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-water/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"water","value":12},"confirmed":false}`))))
	if missingConfirm.Code != http.StatusConflict {
		t.Fatalf("expected 409 got %d", missingConfirm.Code)
	}

	ok := httptest.NewRecorder()
	r.ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/plant-health-json/publish", bytes.NewReader([]byte(`{"payload":{"foo":"bar"},"routing_key":"plant.health.left","confirmed":true}`))))
	if ok.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", ok.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(ok.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["message_id"] == "" {
		t.Fatalf("expected message_id")
	}
}
