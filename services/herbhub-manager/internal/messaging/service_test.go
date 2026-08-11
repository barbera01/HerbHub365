package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/prometheus"
	"HerbHub365/services/herbhub-manager/internal/rabbitmq"
)

type fakeRabbit struct {
	pingErr       error
	exchanges     map[string]rabbitmq.Exchange
	queues        map[string]rabbitmq.Queue
	bindings      map[string][]rabbitmq.Binding
	publishResp   rabbitmq.PublishResponse
	publishErr    error
	ensureCalls   []string
	publishCalled bool
	lastPublish   rabbitmq.PublishRequest
}

func (f *fakeRabbit) Ping(context.Context) error { return f.pingErr }
func (f *fakeRabbit) GetExchange(_ context.Context, name string) (rabbitmq.Exchange, error) {
	v, ok := f.exchanges[name]
	if !ok {
		return rabbitmq.Exchange{}, rabbitmq.ErrNotFound
	}
	return v, nil
}
func (f *fakeRabbit) EnsureExchange(_ context.Context, name, kind string, durable bool) error {
	f.ensureCalls = append(f.ensureCalls, "exchange:"+name)
	f.exchanges[name] = rabbitmq.Exchange{Name: name, Type: kind, Durable: durable, AutoDelete: false, Internal: false}
	return nil
}
func (f *fakeRabbit) GetQueue(_ context.Context, name string) (rabbitmq.Queue, error) {
	v, ok := f.queues[name]
	if !ok {
		return rabbitmq.Queue{}, rabbitmq.ErrNotFound
	}
	return v, nil
}
func (f *fakeRabbit) EnsureQueue(_ context.Context, name string, durable bool, arguments map[string]any) error {
	f.ensureCalls = append(f.ensureCalls, "queue:"+name)
	f.queues[name] = rabbitmq.Queue{Name: name, Durable: durable, AutoDelete: false, Exclusive: false, Arguments: arguments}
	return nil
}
func (f *fakeRabbit) ListBindings(_ context.Context, exchange, queue string) ([]rabbitmq.Binding, error) {
	return f.bindings[exchange+"->"+queue], nil
}
func (f *fakeRabbit) EnsureBinding(_ context.Context, exchange, queue, routingKey string, arguments map[string]any) error {
	f.ensureCalls = append(f.ensureCalls, "binding:"+exchange+"->"+queue+":"+routingKey)
	k := exchange + "->" + queue
	f.bindings[k] = append(f.bindings[k], rabbitmq.Binding{Source: exchange, Destination: queue, DestinationType: "queue", RoutingKey: routingKey, Arguments: arguments})
	return nil
}
func (f *fakeRabbit) ListSourceBindings(_ context.Context, exchange string) ([]rabbitmq.Binding, error) {
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
func (f *fakeRabbit) Publish(_ context.Context, req rabbitmq.PublishRequest) (rabbitmq.PublishResponse, error) {
	f.publishCalled = true
	f.lastPublish = req
	if f.publishErr != nil {
		return rabbitmq.PublishResponse{}, f.publishErr
	}
	return f.publishResp, nil
}

type fakeProm struct {
	enabled bool
	err     error
	value   float64
}

func (f *fakeProm) Enabled() bool { return f.enabled }
func (f *fakeProm) InstantQuery(context.Context, string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.value, nil
}

func enabledConfig() config.MessagingConfig {
	return config.MessagingConfig{
		Management: config.RabbitMQManagementConfig{
			Enabled:  true,
			URL:      "http://rabbitmq:15672",
			VHost:    "/",
			User:     "admin",
			Password: "secret",
			Timeout:  10 * time.Second,
		},
	}
}

func TestCataloguesExactSpec(t *testing.T) {
	cat := Catalogues()
	if got := cat["watering"].MainQueue.Arguments["x-message-ttl"]; got != float64(86400000) {
		t.Fatalf("watering ttl mismatch: %v", got)
	}
	if got := cat["plant-health"].MainQueue.Arguments["x-dead-letter-exchange"]; got != "herbhub.plant.dlx" {
		t.Fatalf("plant dlx mismatch: %v", got)
	}
}

func TestProvisionCreatesMissingIdempotent(t *testing.T) {
	fr := &fakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}}
	svc := NewService(enabledConfig(), fr)

	status, err := svc.ProvisionCatalogue(context.Background(), "watering")
	if err != nil {
		var drift *DriftError
		if errors.As(err, &drift) {
			t.Fatalf("provision drift details: %+v", drift.Details)
		}
		t.Fatalf("provision: %v", err)
	}
	if status.State != "ready" {
		t.Fatalf("state=%s", status.State)
	}
	firstCalls := len(fr.ensureCalls)
	if firstCalls == 0 {
		t.Fatalf("expected ensure calls")
	}
	fr.ensureCalls = nil
	status, err = svc.ProvisionCatalogue(context.Background(), "watering")
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if status.State != "ready" {
		t.Fatalf("state=%s", status.State)
	}
	if len(fr.ensureCalls) != 0 {
		t.Fatalf("expected idempotent second run, got %d ensure calls", len(fr.ensureCalls))
	}
}

func readyStateSeed(cat Catalogue) *fakeRabbit {
	fr := &fakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}}
	fr.exchanges[cat.Exchange.Name] = rabbitmq.Exchange{Name: cat.Exchange.Name, Type: cat.Exchange.Type, Durable: cat.Exchange.Durable, AutoDelete: cat.Exchange.AutoDelete, Internal: cat.Exchange.Internal}
	fr.exchanges[cat.DLX.Name] = rabbitmq.Exchange{Name: cat.DLX.Name, Type: cat.DLX.Type, Durable: cat.DLX.Durable, AutoDelete: cat.DLX.AutoDelete, Internal: cat.DLX.Internal}
	fr.queues[cat.MainQueue.Name] = rabbitmq.Queue{Name: cat.MainQueue.Name, Durable: cat.MainQueue.Durable, AutoDelete: cat.MainQueue.AutoDelete, Exclusive: cat.MainQueue.Exclusive, Arguments: cat.MainQueue.Arguments}
	fr.queues[cat.DLQ.Name] = rabbitmq.Queue{Name: cat.DLQ.Name, Durable: cat.DLQ.Durable, AutoDelete: cat.DLQ.AutoDelete, Exclusive: cat.DLQ.Exclusive, Arguments: cat.DLQ.Arguments}
	fr.bindings["self"] = []rabbitmq.Binding{{Source: cat.Exchange.Name}, {Source: cat.DLX.Name}}
	for _, b := range cat.Bindings {
		k := b.Exchange + "->" + b.Queue
		fr.bindings[k] = append(fr.bindings[k], rabbitmq.Binding{Source: b.Exchange, Destination: b.Queue, DestinationType: "queue", RoutingKey: b.RoutingKey, Arguments: b.Arguments})
	}
	for _, b := range cat.LegacyExpectedBindings {
		k := b.Exchange + "->" + b.Queue
		fr.bindings[k] = append(fr.bindings[k], rabbitmq.Binding{Source: b.Exchange, Destination: b.Queue, DestinationType: "queue", RoutingKey: b.RoutingKey, Arguments: b.Arguments})
	}
	return fr
}

func TestProvisionDetectsDrift(t *testing.T) {
	fr := &fakeRabbit{
		exchanges: map[string]rabbitmq.Exchange{"herbhub.watering": {Name: "herbhub.watering", Type: "fanout", Durable: true, AutoDelete: false, Internal: false}},
		queues:    map[string]rabbitmq.Queue{},
		bindings:  map[string][]rabbitmq.Binding{},
	}
	svc := NewService(enabledConfig(), fr)
	before := len(fr.ensureCalls)
	_, err := svc.ProvisionCatalogue(context.Background(), "watering")
	var drift *DriftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected drift error, got %v", err)
	}
	if len(fr.ensureCalls) != before {
		t.Fatalf("expected no mutation calls on drift preflight")
	}
}

func TestWateringValidationRouteAndConfirmation(t *testing.T) {
	fr := readyStateSeed(Catalogues()["watering"])
	fr.publishResp = rabbitmq.PublishResponse{Routed: true}
	svc := NewService(enabledConfig(), fr)

	_, err := svc.PublishTemplate(context.Background(), "watering-water", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"water","value":35.2}`), Confirmed: false})
	if !errors.Is(err, ErrConfirmationNeeded) {
		t.Fatalf("expected confirmation error got %v", err)
	}

	fr.queues[Catalogues()["watering"].MainQueue.Name] = rabbitmq.Queue{Name: Catalogues()["watering"].MainQueue.Name, Durable: true, AutoDelete: false, Exclusive: false, Arguments: Catalogues()["watering"].MainQueue.Arguments, Consumers: 1}
	res, err := svc.PublishTemplate(context.Background(), "watering-water", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"water","value":35.2}`), Confirmed: true})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.RoutingKey != "watering.basil" {
		t.Fatalf("routing=%s", res.RoutingKey)
	}
	if !fr.publishCalled {
		t.Fatalf("expected publish call")
	}
	if fr.lastPublish.ContentType != "application/json" || fr.lastPublish.DeliveryMode != 2 || fr.lastPublish.AppID != "herbhub-manager" {
		t.Fatalf("unexpected publish properties")
	}
	if fr.lastPublish.Expiration != "300000" {
		t.Fatalf("expected expiration 300000, got %q", fr.lastPublish.Expiration)
	}
}

func TestPlantHealthValidationRoutingAndSize(t *testing.T) {
	fr := readyStateSeed(Catalogues()["plant-health"])
	fr.publishResp = rabbitmq.PublishResponse{Routed: true}
	svc := NewService(enabledConfig(), fr)

	_, err := svc.PublishTemplate(context.Background(), "plant-health-json", PublishRequest{Payload: json.RawMessage(`{"k":1}`), RoutingKey: "plant.health.other"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error got %v", err)
	}

	_, err = svc.PublishTemplate(context.Background(), "plant-health-json", PublishRequest{Payload: json.RawMessage(`[]`), RoutingKey: "plant.health.left"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error got %v", err)
	}
}

func TestPublishRoutedFalseBecomesUnavailable(t *testing.T) {
	fr := readyStateSeed(Catalogues()["watering"])
	fr.queues[Catalogues()["watering"].MainQueue.Name] = rabbitmq.Queue{Name: Catalogues()["watering"].MainQueue.Name, Durable: true, AutoDelete: false, Exclusive: false, Arguments: Catalogues()["watering"].MainQueue.Arguments, Consumers: 1}
	fr.publishResp = rabbitmq.PublishResponse{Routed: false}
	svc := NewService(enabledConfig(), fr)
	_, err := svc.PublishTemplate(context.Background(), "watering-skip", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"skip","value":81.73}`)})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestPhysicalWateringRequiresConsumer(t *testing.T) {
	fr := readyStateSeed(Catalogues()["watering"])
	fr.publishResp = rabbitmq.PublishResponse{Routed: true}
	svc := NewService(enabledConfig(), fr)
	_, err := svc.PublishTemplate(context.Background(), "watering-water", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"water","value":20}`), Confirmed: true})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable when no consumer, got %v", err)
	}
	if fr.publishCalled {
		t.Fatalf("must not publish when no consumer")
	}
}

func TestSkipDoesNotRequireConsumer(t *testing.T) {
	fr := readyStateSeed(Catalogues()["watering"])
	fr.publishResp = rabbitmq.PublishResponse{Routed: true}
	svc := NewService(enabledConfig(), fr)
	_, err := svc.PublishTemplate(context.Background(), "watering-skip", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"skip","value":80}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fr.publishCalled {
		t.Fatalf("expected publish call")
	}
	if fr.lastPublish.Expiration != "" {
		t.Fatalf("skip must not set expiration")
	}
}

func TestPublishFailsClosedWhenTopologyNotReady(t *testing.T) {
	fr := &fakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}, publishResp: rabbitmq.PublishResponse{Routed: true}}
	svc := NewService(enabledConfig(), fr)
	_, err := svc.PublishTemplate(context.Background(), "watering-skip", PublishRequest{Payload: json.RawMessage(`{"plant":"basil","action":"skip","value":81.73}`)})
	if !errors.Is(err, ErrTopologyNotReady) {
		t.Fatalf("expected topology not ready, got %v", err)
	}
	if fr.publishCalled {
		t.Fatalf("publish must not be attempted when topology is not ready")
	}
}

func TestQueueUnexpectedArgumentIsDrift(t *testing.T) {
	cat := Catalogues()["watering"]
	fr := readyStateSeed(cat)
	q := fr.queues[cat.MainQueue.Name]
	q.Arguments = map[string]any{"x-dead-letter-exchange": "herbhub.watering.dlx", "x-message-ttl": float64(86400000), "x-extra": "unexpected"}
	fr.queues[cat.MainQueue.Name] = q
	svc := NewService(enabledConfig(), fr)
	status, err := svc.catalogueStatus(context.Background(), cat)
	if err != nil {
		t.Fatalf("catalogueStatus: %v", err)
	}
	if status.State != "drifted" {
		t.Fatalf("expected drifted, got %s", status.State)
	}
}

func TestBindingArgumentsMismatchIsDrift(t *testing.T) {
	cat := Catalogues()["watering"]
	fr := readyStateSeed(cat)
	k := cat.Bindings[0].Exchange + "->" + cat.Bindings[0].Queue
	fr.bindings[k] = []rabbitmq.Binding{{Source: cat.Bindings[0].Exchange, Destination: cat.Bindings[0].Queue, RoutingKey: cat.Bindings[0].RoutingKey, Arguments: map[string]any{"x": "y"}}}
	svc := NewService(enabledConfig(), fr)
	status, err := svc.catalogueStatus(context.Background(), cat)
	if err != nil {
		t.Fatalf("catalogueStatus: %v", err)
	}
	if status.State != "drifted" {
		t.Fatalf("expected drifted, got %s", status.State)
	}
}

func TestUnexpectedSourceBindingIsDrift(t *testing.T) {
	cat := Catalogues()["watering"]
	fr := readyStateSeed(cat)
	fr.bindings["x"] = []rabbitmq.Binding{{Source: cat.Exchange.Name, DestinationType: "queue", Destination: "unknown.queue", RoutingKey: "watering.foo", Arguments: map[string]any{}}}
	svc := NewService(enabledConfig(), fr)
	status, err := svc.catalogueStatus(context.Background(), cat)
	if err != nil {
		t.Fatalf("catalogueStatus: %v", err)
	}
	if status.State != "drifted" {
		t.Fatalf("expected drifted, got %s", status.State)
	}
}

func TestLegacySourceBindingIsAllowed(t *testing.T) {
	cat := Catalogues()["watering"]
	fr := readyStateSeed(cat)
	fr.bindings["legacy"] = []rabbitmq.Binding{{Source: cat.Exchange.Name, DestinationType: "queue", Destination: cat.MainQueue.Name, RoutingKey: "watering.action", Arguments: map[string]any{}}}
	svc := NewService(enabledConfig(), fr)
	status, err := svc.catalogueStatus(context.Background(), cat)
	if err != nil {
		t.Fatalf("catalogueStatus: %v", err)
	}
	if status.State != "ready" {
		t.Fatalf("expected ready, got %s drift=%+v", status.State, status.Drift)
	}
}

func TestOverviewPrometheusNoSeriesUnavailable(t *testing.T) {
	fr := readyStateSeed(Catalogues()["watering"])
	fr2 := readyStateSeed(Catalogues()["plant-health"])
	for k, v := range fr2.exchanges {
		fr.exchanges[k] = v
	}
	for k, v := range fr2.queues {
		fr.queues[k] = v
	}
	for k, v := range fr2.bindings {
		fr.bindings[k] = v
	}
	svc := NewService(enabledConfig(), fr)
	svc.prom = &fakeProm{enabled: true, err: prometheus.ErrNoSeries}
	ov := svc.Overview(context.Background())
	if ov.Prometheus == nil || ov.Prometheus.Available {
		t.Fatalf("expected prometheus unavailable")
	}
}

func TestOverviewPrometheusUnavailableDoesNotFailBroker(t *testing.T) {
	fr := &fakeRabbit{exchanges: map[string]rabbitmq.Exchange{}, queues: map[string]rabbitmq.Queue{}, bindings: map[string][]rabbitmq.Binding{}}
	svc := NewService(enabledConfig(), fr)
	svc.prom = &fakeProm{enabled: true, err: errors.New("down")}
	ov := svc.Overview(context.Background())
	if ov.BrokerStatus != "ok" {
		t.Fatalf("broker_status=%s", ov.BrokerStatus)
	}
	if ov.Prometheus == nil || ov.Prometheus.Available {
		t.Fatalf("expected unavailable prometheus summary")
	}
}

func TestDisabledService(t *testing.T) {
	svc := NewService(config.MessagingConfig{}, &fakeRabbit{})
	if svc.Enabled() {
		t.Fatalf("service should be disabled")
	}
	if _, err := svc.ProvisionCatalogue(context.Background(), "watering"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected disabled error")
	}
}
