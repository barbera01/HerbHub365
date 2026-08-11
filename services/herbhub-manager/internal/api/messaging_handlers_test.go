package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/messaging"
	"HerbHub365/services/herbhub-manager/internal/rabbitmq"
)

type apiFakeRabbit struct {
	exchanges map[string]rabbitmq.Exchange
	queues    map[string]rabbitmq.Queue
	bindings  map[string][]rabbitmq.Binding
	publish   rabbitmq.PublishResponse
	pingErr   error
}

func (f *apiFakeRabbit) Ping(context.Context) error { return f.pingErr }
func (f *apiFakeRabbit) GetExchange(_ context.Context, name string) (rabbitmq.Exchange, error) {
	if v, ok := f.exchanges[name]; ok {
		return v, nil
	}
	return rabbitmq.Exchange{}, rabbitmq.ErrNotFound
}
func (f *apiFakeRabbit) EnsureExchange(_ context.Context, name, kind string, durable bool) error {
	f.exchanges[name] = rabbitmq.Exchange{Name: name, Type: kind, Durable: durable, AutoDelete: false, Internal: false}
	return nil
}
func (f *apiFakeRabbit) GetQueue(_ context.Context, name string) (rabbitmq.Queue, error) {
	if v, ok := f.queues[name]; ok {
		return v, nil
	}
	return rabbitmq.Queue{}, rabbitmq.ErrNotFound
}
func (f *apiFakeRabbit) EnsureQueue(_ context.Context, name string, durable bool, arguments map[string]any) error {
	f.queues[name] = rabbitmq.Queue{Name: name, Durable: durable, AutoDelete: false, Exclusive: false, Arguments: arguments}
	return nil
}
func (f *apiFakeRabbit) ListBindings(_ context.Context, exchange, queue string) ([]rabbitmq.Binding, error) {
	return f.bindings[exchange+"->"+queue], nil
}
func (f *apiFakeRabbit) EnsureBinding(_ context.Context, exchange, queue, routingKey string, arguments map[string]any) error {
	k := exchange + "->" + queue
	f.bindings[k] = append(f.bindings[k], rabbitmq.Binding{Source: exchange, Destination: queue, DestinationType: "queue", RoutingKey: routingKey, Arguments: arguments})
	return nil
}
func (f *apiFakeRabbit) ListSourceBindings(_ context.Context, exchange string) ([]rabbitmq.Binding, error) {
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
func (f *apiFakeRabbit) Publish(_ context.Context, req rabbitmq.PublishRequest) (rabbitmq.PublishResponse, error) {
	return f.publish, nil
}

func seedCatalogueReady(fr *apiFakeRabbit, cat messaging.Catalogue) {
	fr.bindings["self"] = append(fr.bindings["self"], rabbitmq.Binding{Source: cat.Exchange.Name}, rabbitmq.Binding{Source: cat.DLX.Name})
	fr.exchanges[cat.Exchange.Name] = rabbitmq.Exchange{Name: cat.Exchange.Name, Type: cat.Exchange.Type, Durable: cat.Exchange.Durable, AutoDelete: cat.Exchange.AutoDelete, Internal: cat.Exchange.Internal}
	fr.exchanges[cat.DLX.Name] = rabbitmq.Exchange{Name: cat.DLX.Name, Type: cat.DLX.Type, Durable: cat.DLX.Durable, AutoDelete: cat.DLX.AutoDelete, Internal: cat.DLX.Internal}
	fr.queues[cat.MainQueue.Name] = rabbitmq.Queue{Name: cat.MainQueue.Name, Durable: cat.MainQueue.Durable, AutoDelete: cat.MainQueue.AutoDelete, Exclusive: cat.MainQueue.Exclusive, Arguments: cat.MainQueue.Arguments, Consumers: 1}
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

func messagingEnabledConfig() config.Config {
	return config.Config{
		Auth: config.AuthConfig{Disabled: true},
		Messaging: config.MessagingConfig{
			Management: config.RabbitMQManagementConfig{
				Enabled:  true,
				URL:      "http://rabbitmq:15672",
				VHost:    "/",
				User:     "admin",
				Password: "secret",
				Timeout:  10 * time.Second,
			},
		},
	}
}

func TestMessagingOverviewDisabled(t *testing.T) {
	r := NewRouter(config.Config{Auth: config.AuthConfig{Disabled: true}}, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/messaging/overview", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.Code)
	}
}

func TestMessagingProvisionUnknownCatalogue(t *testing.T) {
	fr := &apiFakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}, publish: rabbitmq.PublishResponse{Routed: true}}
	svc := messaging.NewService(messagingEnabledConfig().Messaging, fr)
	r := NewRouter(messagingEnabledConfig(), nil, nil, nil, nil, nil, nil, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/messaging/catalogues/unknown/provision", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", resp.Code)
	}
}

func TestMessagingTemplatePublishValidationAndUnknown(t *testing.T) {
	fr := &apiFakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}, publish: rabbitmq.PublishResponse{Routed: true}}
	seedCatalogueReady(fr, messaging.Catalogues()["watering"])
	seedCatalogueReady(fr, messaging.Catalogues()["plant-health"])
	svc := messaging.NewService(messagingEnabledConfig().Messaging, fr)
	r := NewRouter(messagingEnabledConfig(), nil, nil, nil, nil, nil, nil, svc)

	unknownReq := httptest.NewRequest(http.MethodPost, "/api/messaging/templates/nope/publish", bytes.NewReader([]byte(`{"payload":{},"confirmed":true}`)))
	unknownResp := httptest.NewRecorder()
	r.ServeHTTP(unknownResp, unknownReq)
	if unknownResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", unknownResp.Code)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-water/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":10},"confirmed":true}`)))
	invalidResp := httptest.NewRecorder()
	r.ServeHTTP(invalidResp, invalidReq)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", invalidResp.Code)
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-water/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"water","value":10},"confirmed":false}`)))
	confirmResp := httptest.NewRecorder()
	r.ServeHTTP(confirmResp, confirmReq)
	if confirmResp.Code != http.StatusConflict {
		t.Fatalf("expected 409 got %d", confirmResp.Code)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/api/messaging/templates/plant-health-json/publish", bytes.NewReader([]byte(`{"payload":{"k":"v"},"routing_key":"plant.health.left","confirmed":true}`)))
	okResp := httptest.NewRecorder()
	r.ServeHTTP(okResp, okReq)
	if okResp.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", okResp.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(okResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["message_id"] == "" {
		t.Fatalf("expected message_id")
	}
}

func TestMessagingTemplatePublishStrictBodyValidation(t *testing.T) {
	fr := &apiFakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}, publish: rabbitmq.PublishResponse{Routed: true}}
	seedCatalogueReady(fr, messaging.Catalogues()["watering"])
	svc := messaging.NewService(messagingEnabledConfig().Messaging, fr)
	r := NewRouter(messagingEnabledConfig(), nil, nil, nil, nil, nil, nil, svc)

	badField := httptest.NewRecorder()
	r.ServeHTTP(badField, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-skip/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":10},"confirmed":true,"confirmd":true}`))))
	if badField.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", badField.Code)
	}

	trailing := httptest.NewRecorder()
	r.ServeHTTP(trailing, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-skip/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":10},"confirmed":true}{}`))))
	if trailing.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", trailing.Code)
	}

	tooLargePayload := `{"payload":{"blob":"` + strings.Repeat("a", 72*1024) + `"},"routing_key":"plant.health.left","confirmed":true}`
	tooLarge := httptest.NewRecorder()
	r.ServeHTTP(tooLarge, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/plant-health-json/publish", bytes.NewReader([]byte(tooLargePayload))))
	if tooLarge.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", tooLarge.Code)
	}
}

func TestMessagingTemplatePublishTopologyNotReadyAndUnavailable(t *testing.T) {
	fr := &apiFakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}, publish: rabbitmq.PublishResponse{Routed: true}}
	svc := messaging.NewService(messagingEnabledConfig().Messaging, fr)
	r := NewRouter(messagingEnabledConfig(), nil, nil, nil, nil, nil, nil, svc)

	notReady := httptest.NewRecorder()
	r.ServeHTTP(notReady, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-skip/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":10},"confirmed":true}`))))
	if notReady.Code != http.StatusConflict {
		t.Fatalf("expected 409 got %d", notReady.Code)
	}

	fr.pingErr = errors.New("down")
	unavailable := httptest.NewRecorder()
	r.ServeHTTP(unavailable, httptest.NewRequest(http.MethodPost, "/api/messaging/templates/watering-skip/publish", bytes.NewReader([]byte(`{"payload":{"plant":"basil","action":"skip","value":10},"confirmed":true}`))))
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", unavailable.Code)
	}
}
