package messaging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/prometheus"
	"HerbHub365/services/herbhub-manager/internal/rabbitmq"
)

var (
	ErrDisabled           = errors.New("messaging: disabled")
	ErrUnavailable        = errors.New("messaging: unavailable")
	ErrUnknownCatalogue   = errors.New("messaging: unknown catalogue")
	ErrUnknownTemplate    = errors.New("messaging: unknown template")
	ErrValidation         = errors.New("messaging: validation")
	ErrConfirmationNeeded = errors.New("messaging: confirmation required")
	ErrDrift              = errors.New("messaging: topology drift")
	ErrTopologyNotReady   = errors.New("messaging: topology not ready")
	ErrPublishUncertain   = errors.New("messaging: publish uncertain")
	ErrPublishFailed      = errors.New("messaging: publish failed")
)

const maxGenericJSONBytes = 64 * 1024
const physicalWaterExpiration = "300000"

type RabbitClient interface {
	Ping(ctx context.Context) error
	GetExchange(ctx context.Context, name string) (rabbitmq.Exchange, error)
	EnsureExchange(ctx context.Context, name, kind string, durable bool) error
	GetQueue(ctx context.Context, name string) (rabbitmq.Queue, error)
	EnsureQueue(ctx context.Context, name string, durable bool, arguments map[string]any) error
	ListBindings(ctx context.Context, exchange, queue string) ([]rabbitmq.Binding, error)
	ListSourceBindings(ctx context.Context, exchange string) ([]rabbitmq.Binding, error)
	EnsureBinding(ctx context.Context, exchange, queue, routingKey string, arguments map[string]any) error
	Publish(ctx context.Context, req rabbitmq.PublishRequest) (rabbitmq.PublishResponse, error)
}

type PromClient interface {
	Enabled() bool
	InstantQuery(ctx context.Context, expr string) (float64, error)
}

type Service struct {
	enabled    bool
	grafanaURL string
	vhost      string
	rabbit     RabbitClient
	prom       PromClient
}

type Overview struct {
	Enabled      bool                     `json:"enabled"`
	BrokerStatus string                   `json:"broker_status"`
	GrafanaURL   string                   `json:"grafana_url,omitempty"`
	Catalogues   []CatalogueStatus        `json:"catalogues"`
	Templates    []TemplateInfo           `json:"templates"`
	Prometheus   *PrometheusSummaryStatus `json:"prometheus,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

type QueueCounters struct {
	Ready     int `json:"ready"`
	Unacked   int `json:"unacked"`
	Consumers int `json:"consumers"`
}

type DriftDetail struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type CatalogueStatus struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name"`
	State  string                   `json:"state"`
	Queues map[string]QueueCounters `json:"queues,omitempty"`
	Drift  []DriftDetail            `json:"drift,omitempty"`
	Error  string                   `json:"error,omitempty"`
}

type TemplateInfo struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	CatalogueID          string         `json:"catalogue_id"`
	Description          string         `json:"description"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	AllowedRoutingKeys   []string       `json:"allowed_routing_keys,omitempty"`
	DefaultRoutingKey    string         `json:"default_routing_key,omitempty"`
	DefaultPayload       map[string]any `json:"default_payload"`
}

type PrometheusSummaryStatus struct {
	Available bool                     `json:"available"`
	Queues    map[string]QueueCounters `json:"queues,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

type PublishRequest struct {
	Payload    json.RawMessage `json:"payload"`
	RoutingKey string          `json:"routing_key,omitempty"`
	Confirmed  bool            `json:"confirmed"`
}

type PublishResult struct {
	MessageID  string `json:"message_id"`
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routing_key"`
	Routed     bool   `json:"routed"`
}

type AutomaticWaterResult struct {
	MessageID   string
	Exchange    string
	RoutingKey  string
	Routed      bool
	PublishedAt time.Time
}

type publishContext struct {
	Route     string
	MessageID string
}

type DriftError struct {
	Details []DriftDetail
}

func (e *DriftError) Error() string {
	return "messaging: topology drift detected"
}

func (e *DriftError) Is(target error) bool {
	return target == ErrDrift
}

func NewService(cfg config.MessagingConfig, rabbit RabbitClient) *Service {
	enabled := cfg.Management.Enabled && strings.TrimSpace(cfg.Management.User) != "" && cfg.Management.Password != "" && rabbit != nil
	return &Service{
		enabled:    enabled,
		grafanaURL: strings.TrimSpace(cfg.GrafanaURL),
		vhost:      cfg.Management.VHost,
		rabbit:     rabbit,
		prom:       prometheus.NewClient(cfg.Prometheus.URL, cfg.Prometheus.Timeout),
	}
}

func (s *Service) TemplateCatalogueID(templateID string) (string, bool) {
	t, ok := templates()[templateID]
	if !ok {
		return "", false
	}
	return t.CatalogueID, true
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Service) Overview(ctx context.Context) Overview {
	ov := Overview{
		Enabled:      s.Enabled(),
		BrokerStatus: "disabled",
		GrafanaURL:   s.grafanaURL,
		Catalogues:   make([]CatalogueStatus, 0, len(Catalogues())),
		Templates:    templateInfos(),
	}
	if !s.Enabled() {
		for _, id := range SortedCatalogueIDs() {
			c := Catalogues()[id]
			ov.Catalogues = append(ov.Catalogues, CatalogueStatus{ID: c.ID, Name: c.Name, State: "unavailable", Error: "messaging disabled"})
		}
		ov.Prometheus = &PrometheusSummaryStatus{Available: false, Error: "disabled"}
		return ov
	}

	if err := s.rabbit.Ping(ctx); err != nil {
		ov.BrokerStatus = "unavailable"
		ov.Error = "rabbitmq management unavailable"
		for _, id := range SortedCatalogueIDs() {
			c := Catalogues()[id]
			ov.Catalogues = append(ov.Catalogues, CatalogueStatus{ID: c.ID, Name: c.Name, State: "unavailable", Error: "broker unavailable"})
		}
		ov.Prometheus = s.prometheusSummary(ctx)
		return ov
	}
	ov.BrokerStatus = "ok"

	for _, id := range SortedCatalogueIDs() {
		c := Catalogues()[id]
		status, err := s.catalogueStatus(ctx, c)
		if err != nil {
			ov.Catalogues = append(ov.Catalogues, CatalogueStatus{ID: c.ID, Name: c.Name, State: "unavailable", Error: "broker unavailable"})
			continue
		}
		ov.Catalogues = append(ov.Catalogues, status)
	}
	ov.Prometheus = s.prometheusSummary(ctx)
	return ov
}

func (s *Service) ProvisionCatalogue(ctx context.Context, id string) (CatalogueStatus, error) {
	start := time.Now()
	var outcome = "failed"
	defer func() {
		log.Printf("messaging audit: action=provision catalogue=%s outcome=%s duration_ms=%d", id, outcome, time.Since(start).Milliseconds())
	}()

	if !s.Enabled() {
		outcome = "disabled"
		return CatalogueStatus{}, ErrDisabled
	}
	c, ok := Catalogues()[id]
	if !ok {
		outcome = "unknown"
		return CatalogueStatus{}, ErrUnknownCatalogue
	}
	if err := s.rabbit.Ping(ctx); err != nil {
		outcome = "unavailable"
		return CatalogueStatus{}, fmt.Errorf("%w: broker ping failed", ErrUnavailable)
	}
	preflight, err := s.catalogueStatus(ctx, c)
	if err != nil {
		outcome = "unavailable"
		return CatalogueStatus{}, fmt.Errorf("%w: status preflight failed", ErrUnavailable)
	}
	if preflight.State == "drifted" {
		outcome = "drifted"
		return preflight, &DriftError{Details: preflight.Drift}
	}

	missing, err := s.missingParts(ctx, c)
	if err != nil {
		outcome = "unavailable"
		return CatalogueStatus{}, err
	}

	if missing.exchange[c.Exchange.Name] {
		if err := s.rabbit.EnsureExchange(ctx, c.Exchange.Name, c.Exchange.Type, c.Exchange.Durable); err != nil {
			outcome = "unavailable"
			return CatalogueStatus{}, fmt.Errorf("%w: create exchange", ErrUnavailable)
		}
	}
	if missing.exchange[c.DLX.Name] {
		if err := s.rabbit.EnsureExchange(ctx, c.DLX.Name, c.DLX.Type, c.DLX.Durable); err != nil {
			outcome = "unavailable"
			return CatalogueStatus{}, fmt.Errorf("%w: create exchange", ErrUnavailable)
		}
	}
	if missing.queue[c.MainQueue.Name] {
		if err := s.rabbit.EnsureQueue(ctx, c.MainQueue.Name, c.MainQueue.Durable, c.MainQueue.Arguments); err != nil {
			outcome = "unavailable"
			return CatalogueStatus{}, fmt.Errorf("%w: create queue", ErrUnavailable)
		}
	}
	if missing.queue[c.DLQ.Name] {
		if err := s.rabbit.EnsureQueue(ctx, c.DLQ.Name, c.DLQ.Durable, c.DLQ.Arguments); err != nil {
			outcome = "unavailable"
			return CatalogueStatus{}, fmt.Errorf("%w: create queue", ErrUnavailable)
		}
	}
	for _, b := range c.Bindings {
		k := b.Exchange + "->" + b.Queue + "->" + b.RoutingKey + "->" + normalizedArgsKey(b.Arguments)
		if missing.binding[k] {
			if err := s.rabbit.EnsureBinding(ctx, b.Exchange, b.Queue, b.RoutingKey, b.Arguments); err != nil {
				outcome = "unavailable"
				return CatalogueStatus{}, fmt.Errorf("%w: create binding", ErrUnavailable)
			}
		}
	}

	status, err := s.catalogueStatus(ctx, c)
	if err != nil {
		outcome = "unavailable"
		return CatalogueStatus{}, fmt.Errorf("%w: status check failed", ErrUnavailable)
	}
	if status.State == "drifted" {
		outcome = "drifted"
		return status, &DriftError{Details: status.Drift}
	}
	if status.State != "ready" {
		outcome = "not_ready"
		return status, ErrTopologyNotReady
	}
	outcome = "success"
	return status, nil
}

func (s *Service) PublishTemplate(ctx context.Context, templateID string, req PublishRequest) (PublishResult, error) {
	start := time.Now()
	audit := publishContext{}
	outcome := "failed"
	defer func() {
		log.Printf("messaging audit: action=publish template=%s route=%s message_id=%s outcome=%s duration_ms=%d", templateID, audit.Route, audit.MessageID, outcome, time.Since(start).Milliseconds())
	}()

	if !s.Enabled() {
		outcome = "disabled"
		return PublishResult{}, ErrDisabled
	}
	if err := s.rabbit.Ping(ctx); err != nil {
		outcome = "unavailable"
		return PublishResult{}, fmt.Errorf("%w: broker unavailable", ErrUnavailable)
	}

	spec, ok := templates()[templateID]
	if !ok {
		outcome = "unknown"
		return PublishResult{}, ErrUnknownTemplate
	}
	catalogue, ok := Catalogues()[spec.CatalogueID]
	if !ok {
		outcome = "unavailable"
		return PublishResult{}, ErrUnavailable
	}
	status, err := s.catalogueStatus(ctx, catalogue)
	if err != nil {
		outcome = "unavailable"
		return PublishResult{}, fmt.Errorf("%w: topology unavailable", ErrUnavailable)
	}
	if status.State == "drifted" {
		outcome = "drifted"
		return PublishResult{}, &DriftError{Details: status.Drift}
	}
	if status.State != "ready" {
		outcome = "not_ready"
		return PublishResult{}, ErrTopologyNotReady
	}

	body, route, err := validateTemplateRequest(spec, req)
	if err != nil {
		outcome = "validation"
		return PublishResult{}, err
	}
	audit.Route = route

	if spec.ID == "watering-water" {
		queueState, ok := status.Queues[Catalogues()["watering"].MainQueue.Name]
		if !ok || queueState.Consumers != 1 {
			outcome = "unavailable"
			return PublishResult{}, fmt.Errorf("%w: watering consumer count must be exactly one", ErrUnavailable)
		}
	}

	msgID, err := newMessageID()
	if err != nil {
		outcome = "unavailable"
		return PublishResult{}, fmt.Errorf("%w: message id generation failed", ErrUnavailable)
	}
	audit.MessageID = msgID

	expiration := ""
	if spec.ID == "watering-water" {
		expiration = physicalWaterExpiration
	}

	resp, err := s.rabbit.Publish(ctx, rabbitmq.PublishRequest{
		Exchange:     spec.Exchange,
		RoutingKey:   route,
		Payload:      string(body),
		DeliveryMode: 2,
		ContentType:  "application/json",
		AppID:        "herbhub-manager",
		MessageID:    msgID,
		Timestamp:    time.Now().UTC(),
		Expiration:   expiration,
	})
	if err != nil {
		outcome = "unavailable"
		return PublishResult{}, fmt.Errorf("%w: publish failed", ErrUnavailable)
	}
	if !resp.Routed {
		outcome = "unavailable"
		return PublishResult{}, fmt.Errorf("%w: message was not routed", ErrUnavailable)
	}

	outcome = "success"
	return PublishResult{MessageID: msgID, Exchange: spec.Exchange, RoutingKey: route, Routed: resp.Routed}, nil
}

func (s *Service) PublishAutomaticWater(ctx context.Context, plant string, moisture float64, expiry time.Duration) (AutomaticWaterResult, error) {
	out := AutomaticWaterResult{}
	if !s.Enabled() {
		return out, ErrDisabled
	}
	if err := s.rabbit.Ping(ctx); err != nil {
		return out, fmt.Errorf("%w: broker unavailable", ErrUnavailable)
	}
	spec := templates()["watering-water"]
	catalogue := Catalogues()[spec.CatalogueID]
	status, err := s.catalogueStatus(ctx, catalogue)
	if err != nil {
		return out, fmt.Errorf("%w: topology unavailable", ErrUnavailable)
	}
	if status.State == "drifted" {
		return out, &DriftError{Details: status.Drift}
	}
	if status.State != "ready" {
		return out, ErrTopologyNotReady
	}
	queueState, ok := status.Queues[catalogue.MainQueue.Name]
	if !ok || queueState.Consumers != 1 {
		return out, fmt.Errorf("%w: watering consumer count must be exactly one", ErrUnavailable)
	}

	body, route, err := validateWateringTemplate("water", false)(PublishRequest{
		Payload:    mustMarshal(map[string]any{"plant": plant, "action": "water", "value": moisture}),
		RoutingKey: "",
		Confirmed:  true,
	})
	if err != nil {
		return out, err
	}

	msgID, err := newMessageID()
	if err != nil {
		return out, fmt.Errorf("%w: message id generation failed", ErrUnavailable)
	}
	expiration := strconv.FormatInt(expiry.Milliseconds(), 10)
	timestamp := time.Now().UTC()
	out = AutomaticWaterResult{MessageID: msgID, Exchange: spec.Exchange, RoutingKey: route, Routed: false, PublishedAt: timestamp}

	resp, err := s.rabbit.Publish(ctx, rabbitmq.PublishRequest{
		Exchange:     spec.Exchange,
		RoutingKey:   route,
		Payload:      string(body),
		DeliveryMode: 2,
		ContentType:  "application/json",
		AppID:        "herbhub-manager",
		MessageID:    msgID,
		Timestamp:    timestamp,
		Expiration:   expiration,
	})
	if err != nil {
		return out, fmt.Errorf("%w: publish failed", ErrPublishUncertain)
	}
	if !resp.Routed {
		return out, fmt.Errorf("%w: message was not routed", ErrPublishFailed)
	}
	out.Routed = true
	return out, nil
}

func mustMarshal(v any) json.RawMessage {
	encoded, _ := json.Marshal(v)
	return encoded
}

func (s *Service) catalogueStatus(ctx context.Context, c Catalogue) (CatalogueStatus, error) {
	status := CatalogueStatus{ID: c.ID, Name: c.Name, State: "ready", Queues: map[string]QueueCounters{}, Drift: []DriftDetail{}}

	checkExchange := func(spec ExchangeSpec) error {
		ex, err := s.rabbit.GetExchange(ctx, spec.Name)
		if errors.Is(err, rabbitmq.ErrNotFound) {
			status.State = "missing"
			return nil
		}
		if err != nil {
			return err
		}
		if ex.Type != spec.Type {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "exchange:" + spec.Name, Field: "type", Expected: spec.Type, Actual: ex.Type})
		}
		if ex.Durable != spec.Durable {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "exchange:" + spec.Name, Field: "durable", Expected: fmt.Sprintf("%t", spec.Durable), Actual: fmt.Sprintf("%t", ex.Durable)})
		}
		if ex.AutoDelete != spec.AutoDelete {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "exchange:" + spec.Name, Field: "auto_delete", Expected: fmt.Sprintf("%t", spec.AutoDelete), Actual: fmt.Sprintf("%t", ex.AutoDelete)})
		}
		if ex.Internal != spec.Internal {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "exchange:" + spec.Name, Field: "internal", Expected: fmt.Sprintf("%t", spec.Internal), Actual: fmt.Sprintf("%t", ex.Internal)})
		}
		return nil
	}
	if err := checkExchange(c.Exchange); err != nil {
		return CatalogueStatus{}, err
	}
	if err := checkExchange(c.DLX); err != nil {
		return CatalogueStatus{}, err
	}

	checkQueue := func(spec QueueSpec) error {
		q, err := s.rabbit.GetQueue(ctx, spec.Name)
		if errors.Is(err, rabbitmq.ErrNotFound) {
			status.State = "missing"
			return nil
		}
		if err != nil {
			return err
		}
		status.Queues[spec.Name] = QueueCounters{Ready: q.MessagesReady, Unacked: q.MessagesUnacknowledged, Consumers: q.Consumers}
		if q.Durable != spec.Durable {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "durable", Expected: fmt.Sprintf("%t", spec.Durable), Actual: fmt.Sprintf("%t", q.Durable)})
		}
		if q.AutoDelete != spec.AutoDelete {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "auto_delete", Expected: fmt.Sprintf("%t", spec.AutoDelete), Actual: fmt.Sprintf("%t", q.AutoDelete)})
		}
		if q.Exclusive != spec.Exclusive {
			status.State = "drifted"
			status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "exclusive", Expected: fmt.Sprintf("%t", spec.Exclusive), Actual: fmt.Sprintf("%t", q.Exclusive)})
		}
		for key, expected := range spec.Arguments {
			actual, ok := q.Arguments[key]
			if !ok {
				status.State = "drifted"
				status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "arguments." + key, Expected: fmt.Sprintf("%v", expected), Actual: "<missing>"})
				continue
			}
			if !equivalentArg(actual, expected) {
				status.State = "drifted"
				status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "arguments." + key, Expected: fmt.Sprintf("%v", expected), Actual: fmt.Sprintf("%v", actual)})
			}
		}
		if len(q.Arguments) != len(spec.Arguments) {
			for key := range q.Arguments {
				if _, ok := spec.Arguments[key]; !ok {
					status.State = "drifted"
					status.Drift = append(status.Drift, DriftDetail{Resource: "queue:" + spec.Name, Field: "arguments." + key, Expected: "<absent>", Actual: fmt.Sprintf("%v", q.Arguments[key])})
				}
			}
		}
		return nil
	}
	if err := checkQueue(c.MainQueue); err != nil {
		return CatalogueStatus{}, err
	}
	if err := checkQueue(c.DLQ); err != nil {
		return CatalogueStatus{}, err
	}

	for _, b := range c.Bindings {
		bindings, err := s.rabbit.ListBindings(ctx, b.Exchange, b.Queue)
		if err != nil {
			if errors.Is(err, rabbitmq.ErrNotFound) {
				status.State = "missing"
				continue
			}
			return CatalogueStatus{}, err
		}
		found := false
		for _, existing := range bindings {
			if existing.RoutingKey == b.RoutingKey {
				if len(existing.Arguments) != len(b.Arguments) {
					continue
				}
				argsMatch := true
				for key, expected := range b.Arguments {
					actual, ok := existing.Arguments[key]
					if !ok || !equivalentArg(actual, expected) {
						argsMatch = false
						break
					}
				}
				if argsMatch {
					found = true
					break
				}
			}
		}
		if !found {
			hasSameRoute := false
			for _, existing := range bindings {
				if existing.RoutingKey == b.RoutingKey {
					hasSameRoute = true
					break
				}
			}
			if hasSameRoute {
				status.State = "drifted"
				status.Drift = append(status.Drift, DriftDetail{Resource: "binding:" + b.Exchange + "->" + b.Queue, Field: "arguments", Expected: fmt.Sprintf("%v", b.Arguments), Actual: "non-matching arguments for same route"})
				continue
			}
			status.State = "missing"
		}
	}

	exchangeSet := map[string]bool{c.Exchange.Name: true, c.DLX.Name: true}
	allowlist := allowedSourceBindings(c)
	for exchangeName := range exchangeSet {
		sourceBindings, err := s.rabbit.ListSourceBindings(ctx, exchangeName)
		if err != nil {
			if errors.Is(err, rabbitmq.ErrNotFound) {
				status.State = "missing"
				continue
			}
			return CatalogueStatus{}, fmt.Errorf("%w: read source bindings", ErrUnavailable)
		}
		for _, sb := range sourceBindings {
			if sb.Source != exchangeName {
				continue
			}
			if sb.Destination == "" || sb.DestinationType == "" {
				continue
			}
			if len(sb.Arguments) != 0 {
				status.State = "drifted"
				status.Drift = append(status.Drift, DriftDetail{Resource: "binding-source:" + exchangeName, Field: "arguments", Expected: "{}", Actual: fmt.Sprintf("%v", sb.Arguments)})
				continue
			}
			key := sourceBindingKey(sb.Source, sb.DestinationType, sb.Destination, sb.RoutingKey)
			if !allowlist[key] {
				status.State = "drifted"
				status.Drift = append(status.Drift, DriftDetail{Resource: "binding-source:" + exchangeName, Field: "route", Expected: "allowlisted source binding", Actual: key})
			}
		}
	}

	if len(status.Drift) > 0 {
		status.State = "drifted"
	}
	return status, nil
}

type missingSummary struct {
	exchange map[string]bool
	queue    map[string]bool
	binding  map[string]bool
}

func (s *Service) missingParts(ctx context.Context, c Catalogue) (missingSummary, error) {
	out := missingSummary{exchange: map[string]bool{}, queue: map[string]bool{}, binding: map[string]bool{}}

	if _, err := s.rabbit.GetExchange(ctx, c.Exchange.Name); err != nil {
		if errors.Is(err, rabbitmq.ErrNotFound) {
			out.exchange[c.Exchange.Name] = true
		} else {
			return out, fmt.Errorf("%w: read exchange", ErrUnavailable)
		}
	}
	if _, err := s.rabbit.GetExchange(ctx, c.DLX.Name); err != nil {
		if errors.Is(err, rabbitmq.ErrNotFound) {
			out.exchange[c.DLX.Name] = true
		} else {
			return out, fmt.Errorf("%w: read exchange", ErrUnavailable)
		}
	}
	if _, err := s.rabbit.GetQueue(ctx, c.MainQueue.Name); err != nil {
		if errors.Is(err, rabbitmq.ErrNotFound) {
			out.queue[c.MainQueue.Name] = true
		} else {
			return out, fmt.Errorf("%w: read queue", ErrUnavailable)
		}
	}
	if _, err := s.rabbit.GetQueue(ctx, c.DLQ.Name); err != nil {
		if errors.Is(err, rabbitmq.ErrNotFound) {
			out.queue[c.DLQ.Name] = true
		} else {
			return out, fmt.Errorf("%w: read queue", ErrUnavailable)
		}
	}

	for _, b := range c.Bindings {
		bindings, err := s.rabbit.ListBindings(ctx, b.Exchange, b.Queue)
		if err != nil {
			if errors.Is(err, rabbitmq.ErrNotFound) {
				out.binding[b.Exchange+"->"+b.Queue+"->"+b.RoutingKey+"->"+normalizedArgsKey(b.Arguments)] = true
				continue
			}
			return out, fmt.Errorf("%w: read bindings", ErrUnavailable)
		}
		found := false
		for _, existing := range bindings {
			if existing.RoutingKey != b.RoutingKey {
				continue
			}
			if len(existing.Arguments) != len(b.Arguments) {
				continue
			}
			argsMatch := true
			for k, expected := range b.Arguments {
				actual, ok := existing.Arguments[k]
				if !ok || !equivalentArg(actual, expected) {
					argsMatch = false
					break
				}
			}
			if argsMatch {
				found = true
				break
			}
		}
		if !found {
			hasSameRoute := false
			for _, existing := range bindings {
				if existing.RoutingKey == b.RoutingKey {
					hasSameRoute = true
					break
				}
			}
			if hasSameRoute {
				continue
			}
			out.binding[b.Exchange+"->"+b.Queue+"->"+b.RoutingKey+"->"+normalizedArgsKey(b.Arguments)] = true
		}
	}

	exchangeSet := map[string]bool{c.Exchange.Name: true, c.DLX.Name: true}
	allowlist := allowedSourceBindings(c)
	for exchangeName := range exchangeSet {
		sourceBindings, err := s.rabbit.ListSourceBindings(ctx, exchangeName)
		if err != nil {
			if errors.Is(err, rabbitmq.ErrNotFound) {
				continue
			}
			return out, fmt.Errorf("%w: read source bindings", ErrUnavailable)
		}
		for _, sb := range sourceBindings {
			if sb.Source != exchangeName {
				continue
			}
			if sb.Destination == "" || sb.DestinationType == "" {
				continue
			}
			if len(sb.Arguments) != 0 {
				return out, &DriftError{Details: []DriftDetail{{Resource: "binding-source:" + exchangeName, Field: "arguments", Expected: "{}", Actual: fmt.Sprintf("%v", sb.Arguments)}}}
			}
			key := sourceBindingKey(sb.Source, sb.DestinationType, sb.Destination, sb.RoutingKey)
			if !allowlist[key] {
				return out, &DriftError{Details: []DriftDetail{{Resource: "binding-source:" + exchangeName, Field: "route", Expected: "allowlisted source binding", Actual: key}}}
			}
		}
	}

	return out, nil
}

func (s *Service) ensureExchange(ctx context.Context, spec ExchangeSpec) error {
	ex, err := s.rabbit.GetExchange(ctx, spec.Name)
	if errors.Is(err, rabbitmq.ErrNotFound) {
		if err := s.rabbit.EnsureExchange(ctx, spec.Name, spec.Type, spec.Durable); err != nil {
			return fmt.Errorf("%w: create exchange %s", ErrUnavailable, spec.Name)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read exchange %s", ErrUnavailable, spec.Name)
	}
	if ex.Type != spec.Type || ex.Durable != spec.Durable {
		return &DriftError{Details: []DriftDetail{{Resource: "exchange:" + spec.Name, Field: "type_or_durable", Expected: fmt.Sprintf("type=%s durable=%t", spec.Type, spec.Durable), Actual: fmt.Sprintf("type=%s durable=%t", ex.Type, ex.Durable)}}}
	}
	return nil
}

func (s *Service) ensureQueue(ctx context.Context, spec QueueSpec) error {
	q, err := s.rabbit.GetQueue(ctx, spec.Name)
	if errors.Is(err, rabbitmq.ErrNotFound) {
		if err := s.rabbit.EnsureQueue(ctx, spec.Name, spec.Durable, spec.Arguments); err != nil {
			return fmt.Errorf("%w: create queue %s", ErrUnavailable, spec.Name)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read queue %s", ErrUnavailable, spec.Name)
	}
	if q.Durable != spec.Durable {
		return &DriftError{Details: []DriftDetail{{Resource: "queue:" + spec.Name, Field: "durable", Expected: fmt.Sprintf("%t", spec.Durable), Actual: fmt.Sprintf("%t", q.Durable)}}}
	}
	for k, expected := range spec.Arguments {
		actual, ok := q.Arguments[k]
		if !ok || !equivalentArg(actual, expected) {
			act := "<missing>"
			if ok {
				act = fmt.Sprintf("%v", actual)
			}
			return &DriftError{Details: []DriftDetail{{Resource: "queue:" + spec.Name, Field: "arguments." + k, Expected: fmt.Sprintf("%v", expected), Actual: act}}}
		}
	}
	return nil
}

func equivalentArg(actual, expected any) bool {
	switch e := expected.(type) {
	case float64:
		a, ok := toFloat64(actual)
		return ok && a == e
	case string:
		a, ok := actual.(string)
		return ok && a == e
	case bool:
		a, ok := actual.(bool)
		return ok && a == e
	default:
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
	}
}

func normalizedArgsKey(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "<invalid>"
	}
	return string(encoded)
}

func sourceBindingKey(source, destinationType, destination, routingKey string) string {
	return source + "|" + destinationType + "|" + destination + "|" + routingKey
}

func allowedSourceBindings(c Catalogue) map[string]bool {
	out := map[string]bool{}
	add := func(b BindingSpec) {
		out[sourceBindingKey(b.Exchange, "queue", b.Queue, b.RoutingKey)] = true
	}
	for _, b := range c.Bindings {
		add(b)
	}
	for _, b := range c.LegacyExpectedBindings {
		add(b)
	}
	return out
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

type templateSpec struct {
	ID                   string
	Name                 string
	Description          string
	CatalogueID          string
	Exchange             string
	RequiresConfirmation bool
	AllowedRoutingKeys   []string
	DefaultRoutingKey    string
	DefaultPayload       map[string]any
	Validate             func(PublishRequest) ([]byte, string, error)
}

func templates() map[string]templateSpec {
	return map[string]templateSpec{
		"watering-water": {
			ID:                   "watering-water",
			Name:                 "Water plant",
			Description:          "Publish a physical watering command",
			CatalogueID:          "watering",
			Exchange:             Catalogues()["watering"].Exchange.Name,
			RequiresConfirmation: true,
			DefaultPayload:       map[string]any{"plant": "basil", "action": "water", "value": 35.2},
			Validate:             validateWateringTemplate("water", true),
		},
		"watering-skip": {
			ID:             "watering-skip",
			Name:           "Skip watering",
			Description:    "Publish a watering skip command",
			CatalogueID:    "watering",
			Exchange:       Catalogues()["watering"].Exchange.Name,
			DefaultPayload: map[string]any{"plant": "basil", "action": "skip", "value": 81.73},
			Validate:       validateWateringTemplate("skip", false),
		},
		"plant-health-json": {
			ID:                 "plant-health-json",
			Name:               "Plant health JSON",
			Description:        "Publish generic JSON to a curated plant-health route",
			CatalogueID:        "plant-health",
			Exchange:           Catalogues()["plant-health"].Exchange.Name,
			AllowedRoutingKeys: []string{"plant.health.left", "plant.health.middle", "plant.health.right"},
			DefaultRoutingKey:  "plant.health.left",
			DefaultPayload:     map[string]any{"source": "manager", "observation": "leaf edge yellowing"},
			Validate:           validatePlantHealthTemplate,
		},
	}
}

func templateInfos() []TemplateInfo {
	out := make([]TemplateInfo, 0, len(templates()))
	ids := []string{"watering-water", "watering-skip", "plant-health-json"}
	for _, id := range ids {
		t := templates()[id]
		out = append(out, TemplateInfo{
			ID: t.ID, Name: t.Name, CatalogueID: t.CatalogueID, Description: t.Description,
			RequiresConfirmation: t.RequiresConfirmation,
			AllowedRoutingKeys:   append([]string(nil), t.AllowedRoutingKeys...),
			DefaultRoutingKey:    t.DefaultRoutingKey,
			DefaultPayload:       copyMap(t.DefaultPayload),
		})
	}
	return out
}

func copyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func validateTemplateRequest(spec templateSpec, req PublishRequest) ([]byte, string, error) {
	if len(req.Payload) == 0 {
		return nil, "", fmt.Errorf("%w: payload is required", ErrValidation)
	}
	return spec.Validate(req)
}

func validateWateringTemplate(action string, requireConfirm bool) func(PublishRequest) ([]byte, string, error) {
	return func(req PublishRequest) ([]byte, string, error) {
		if requireConfirm && !req.Confirmed {
			return nil, "", ErrConfirmationNeeded
		}
		dec := json.NewDecoder(strings.NewReader(string(req.Payload)))
		dec.DisallowUnknownFields()
		var in struct {
			Plant  string   `json:"plant"`
			Action string   `json:"action"`
			Value  *float64 `json:"value"`
		}
		if err := dec.Decode(&in); err != nil {
			return nil, "", fmt.Errorf("%w: invalid payload", ErrValidation)
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, "", fmt.Errorf("%w: invalid payload", ErrValidation)
		}
		if in.Value == nil {
			return nil, "", fmt.Errorf("%w: value is required", ErrValidation)
		}
		plant := strings.ToLower(strings.TrimSpace(in.Plant))
		switch plant {
		case "basil", "chilli", "oregano":
		default:
			return nil, "", fmt.Errorf("%w: unknown plant", ErrValidation)
		}
		act := strings.ToLower(strings.TrimSpace(in.Action))
		if act != action {
			return nil, "", fmt.Errorf("%w: action must be %s", ErrValidation, action)
		}
		if math.IsNaN(*in.Value) || math.IsInf(*in.Value, 0) || *in.Value < 0 || *in.Value > 100 {
			return nil, "", fmt.Errorf("%w: value must be in range [0,100]", ErrValidation)
		}
		route := "watering." + plant
		if strings.TrimSpace(req.RoutingKey) != "" && strings.TrimSpace(req.RoutingKey) != route {
			return nil, "", fmt.Errorf("%w: routing_key does not match derived plant route", ErrValidation)
		}
		out := map[string]any{"plant": plant, "action": action, "value": *in.Value}
		encoded, err := json.Marshal(out)
		if err != nil {
			return nil, "", fmt.Errorf("%w: payload encoding failed", ErrValidation)
		}
		return encoded, route, nil
	}
}

func validatePlantHealthTemplate(req PublishRequest) ([]byte, string, error) {
	var obj map[string]any
	if err := json.Unmarshal(req.Payload, &obj); err != nil {
		return nil, "", fmt.Errorf("%w: payload must be JSON object", ErrValidation)
	}
	if obj == nil {
		return nil, "", fmt.Errorf("%w: payload must be JSON object", ErrValidation)
	}
	route := strings.TrimSpace(req.RoutingKey)
	if route == "" {
		route = "plant.health.left"
	}
	allowed := map[string]bool{"plant.health.left": true, "plant.health.middle": true, "plant.health.right": true}
	if !allowed[route] {
		return nil, "", fmt.Errorf("%w: routing_key not allowlisted", ErrValidation)
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, "", fmt.Errorf("%w: payload encoding failed", ErrValidation)
	}
	if len(encoded) > maxGenericJSONBytes {
		return nil, "", fmt.Errorf("%w: payload exceeds 64KiB", ErrValidation)
	}
	return encoded, route, nil
}

func newMessageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	hexv := hex.EncodeToString(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexv[0:8], hexv[8:12], hexv[12:16], hexv[16:20], hexv[20:32]), nil
}

func (s *Service) prometheusSummary(ctx context.Context) *PrometheusSummaryStatus {
	if s.prom == nil || !s.prom.Enabled() {
		return &PrometheusSummaryStatus{Available: false, Error: "disabled"}
	}

	result := &PrometheusSummaryStatus{Available: true, Queues: map[string]QueueCounters{}}
	queries := []struct {
		queue string
		field string
		expr  string
	}{
		{queue: "watering.queue", field: "ready", expr: s.queueQuery("rabbitmq_queue_messages_ready", "watering.queue")},
		{queue: "watering.queue", field: "unacked", expr: s.queueQuery("rabbitmq_queue_messages_unacked", "watering.queue")},
		{queue: "watering.queue", field: "consumers", expr: s.queueQuery("rabbitmq_queue_consumers", "watering.queue")},
		{queue: "plant.health.analysis", field: "ready", expr: s.queueQuery("rabbitmq_queue_messages_ready", "plant.health.analysis")},
		{queue: "plant.health.analysis", field: "unacked", expr: s.queueQuery("rabbitmq_queue_messages_unacked", "plant.health.analysis")},
		{queue: "plant.health.analysis", field: "consumers", expr: s.queueQuery("rabbitmq_queue_consumers", "plant.health.analysis")},
	}

	for _, q := range queries {
		value, err := s.prom.InstantQuery(ctx, q.expr)
		if err != nil {
			if errors.Is(err, prometheus.ErrNoSeries) {
				result.Available = false
				result.Error = "prometheus unavailable"
				return result
			}
			result.Available = false
			result.Error = "prometheus unavailable"
			return result
		}
		current := result.Queues[q.queue]
		switch q.field {
		case "ready":
			current.Ready = int(value)
		case "unacked":
			current.Unacked = int(value)
		case "consumers":
			current.Consumers = int(value)
		}
		result.Queues[q.queue] = current
	}

	return result
}

func (s *Service) queueQuery(metric, queue string) string {
	vhost := s.vhost
	if strings.TrimSpace(vhost) == "" {
		vhost = "/"
	}
	return fmt.Sprintf(`sum(%s{queue="%s",vhost="%s"})`, metric, escapePromQLLabelValue(queue), escapePromQLLabelValue(vhost))
}

func escapePromQLLabelValue(input string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\\"`)
	return replacer.Replace(input)
}
