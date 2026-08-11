package rabbitmq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBodyBytes = 1 << 20

var ErrNotFound = errors.New("rabbitmq: not found")

type Config struct {
	URL      string
	VHost    string
	User     string
	Password string
	Timeout  time.Duration
}

type Client struct {
	baseURL  string
	vhost    string
	user     string
	password string
	http     *http.Client
}

type Exchange struct {
	Name       string
	Type       string
	Durable    bool
	AutoDelete bool
	Internal   bool
}

type Queue struct {
	Name                   string
	Durable                bool
	AutoDelete             bool
	Exclusive              bool
	Arguments              map[string]any
	MessagesReady          int
	MessagesUnacknowledged int
	Consumers              int
}

type Binding struct {
	Source           string
	Destination      string
	DestinationType  string
	RoutingKey       string
	Arguments        map[string]any
	PropertiesKey    string
	VHost            string
	ExplicitlyDecode bool
}

type PublishRequest struct {
	Exchange     string
	RoutingKey   string
	Payload      string
	DeliveryMode int
	ContentType  string
	AppID        string
	MessageID    string
	Timestamp    time.Time
	Expiration   string
}

type PublishResponse struct {
	Routed bool
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if base == "" {
		return nil, errors.New("rabbitmq: management url is required")
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("rabbitmq: parse management url: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	vhost := cfg.VHost
	if strings.TrimSpace(vhost) == "" {
		vhost = "/"
	}
	return &Client{
		baseURL:  base,
		vhost:    vhost,
		user:     cfg.User,
		password: cfg.Password,
		http:     &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.requestJSON(ctx, http.MethodGet, c.path("overview"), nil)
	return err
}

func (c *Client) GetExchange(ctx context.Context, name string) (Exchange, error) {
	body, err := c.requestJSON(ctx, http.MethodGet, c.path("exchanges", c.vhost, name), nil)
	if err != nil {
		return Exchange{}, err
	}
	var raw struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Durable    bool   `json:"durable"`
		AutoDelete bool   `json:"auto_delete"`
		Internal   bool   `json:"internal"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Exchange{}, fmt.Errorf("rabbitmq: decode exchange: %w", err)
	}
	return Exchange{
		Name:       raw.Name,
		Type:       raw.Type,
		Durable:    raw.Durable,
		AutoDelete: raw.AutoDelete,
		Internal:   raw.Internal,
	}, nil
}

func (c *Client) EnsureExchange(ctx context.Context, name, kind string, durable bool) error {
	payload := map[string]any{
		"type":        kind,
		"durable":     durable,
		"auto_delete": false,
		"internal":    false,
		"arguments":   map[string]any{},
	}
	_, err := c.requestJSON(ctx, http.MethodPut, c.path("exchanges", c.vhost, name), payload)
	return err
}

func (c *Client) GetQueue(ctx context.Context, name string) (Queue, error) {
	body, err := c.requestJSON(ctx, http.MethodGet, c.path("queues", c.vhost, name), nil)
	if err != nil {
		return Queue{}, err
	}
	var raw struct {
		Name                   string         `json:"name"`
		Durable                bool           `json:"durable"`
		AutoDelete             bool           `json:"auto_delete"`
		Exclusive              bool           `json:"exclusive"`
		Arguments              map[string]any `json:"arguments"`
		MessagesReady          int            `json:"messages_ready"`
		MessagesUnacknowledged int            `json:"messages_unacknowledged"`
		Consumers              int            `json:"consumers"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Queue{}, fmt.Errorf("rabbitmq: decode queue: %w", err)
	}
	if raw.Arguments == nil {
		raw.Arguments = map[string]any{}
	}
	return Queue{
		Name:                   raw.Name,
		Durable:                raw.Durable,
		AutoDelete:             raw.AutoDelete,
		Exclusive:              raw.Exclusive,
		Arguments:              raw.Arguments,
		MessagesReady:          raw.MessagesReady,
		MessagesUnacknowledged: raw.MessagesUnacknowledged,
		Consumers:              raw.Consumers,
	}, nil
}

func (c *Client) EnsureQueue(ctx context.Context, name string, durable bool, arguments map[string]any) error {
	args := map[string]any{}
	for k, v := range arguments {
		args[k] = v
	}
	payload := map[string]any{
		"durable":     durable,
		"auto_delete": false,
		"exclusive":   false,
		"arguments":   args,
	}
	_, err := c.requestJSON(ctx, http.MethodPut, c.path("queues", c.vhost, name), payload)
	return err
}

func (c *Client) ListBindings(ctx context.Context, exchange, queue string) ([]Binding, error) {
	body, err := c.requestJSON(ctx, http.MethodGet, c.path("bindings", c.vhost, "e", exchange, "q", queue), nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Source          string         `json:"source"`
		Destination     string         `json:"destination"`
		DestinationType string         `json:"destination_type"`
		RoutingKey      string         `json:"routing_key"`
		Arguments       map[string]any `json:"arguments"`
		PropertiesKey   string         `json:"properties_key"`
		VHost           string         `json:"vhost"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("rabbitmq: decode bindings: %w", err)
	}
	out := make([]Binding, 0, len(raw))
	for _, item := range raw {
		if item.Arguments == nil {
			item.Arguments = map[string]any{}
		}
		out = append(out, Binding{
			Source:          item.Source,
			Destination:     item.Destination,
			DestinationType: item.DestinationType,
			RoutingKey:      item.RoutingKey,
			Arguments:       item.Arguments,
			PropertiesKey:   item.PropertiesKey,
			VHost:           item.VHost,
		})
	}
	return out, nil
}

func (c *Client) EnsureBinding(ctx context.Context, exchange, queue, routingKey string, arguments map[string]any) error {
	args := map[string]any{}
	for k, v := range arguments {
		args[k] = v
	}
	payload := map[string]any{
		"routing_key": routingKey,
		"arguments":   args,
	}
	_, err := c.requestJSON(ctx, http.MethodPost, c.path("bindings", c.vhost, "e", exchange, "q", queue), payload)
	return err
}

func (c *Client) ListSourceBindings(ctx context.Context, exchange string) ([]Binding, error) {
	body, err := c.requestJSON(ctx, http.MethodGet, c.path("exchanges", c.vhost, exchange, "bindings", "source"), nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Source          string         `json:"source"`
		Destination     string         `json:"destination"`
		DestinationType string         `json:"destination_type"`
		RoutingKey      string         `json:"routing_key"`
		Arguments       map[string]any `json:"arguments"`
		PropertiesKey   string         `json:"properties_key"`
		VHost           string         `json:"vhost"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("rabbitmq: decode source bindings: %w", err)
	}
	out := make([]Binding, 0, len(raw))
	for _, item := range raw {
		if item.Arguments == nil {
			item.Arguments = map[string]any{}
		}
		out = append(out, Binding{
			Source:          item.Source,
			Destination:     item.Destination,
			DestinationType: item.DestinationType,
			RoutingKey:      item.RoutingKey,
			Arguments:       item.Arguments,
			PropertiesKey:   item.PropertiesKey,
			VHost:           item.VHost,
		})
	}
	return out, nil
}

func (c *Client) Publish(ctx context.Context, req PublishRequest) (PublishResponse, error) {
	properties := map[string]any{
		"content_type":  req.ContentType,
		"delivery_mode": req.DeliveryMode,
		"app_id":        req.AppID,
		"message_id":    req.MessageID,
		"timestamp":     req.Timestamp.Unix(),
	}
	if strings.TrimSpace(req.Expiration) != "" {
		properties["expiration"] = req.Expiration
	}
	body := map[string]any{
		"properties":       properties,
		"routing_key":      req.RoutingKey,
		"payload":          req.Payload,
		"payload_encoding": "string",
	}
	resp, err := c.requestJSON(ctx, http.MethodPost, c.path("exchanges", c.vhost, req.Exchange, "publish"), body)
	if err != nil {
		return PublishResponse{}, err
	}
	var parsed PublishResponse
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return PublishResponse{}, fmt.Errorf("rabbitmq: decode publish response: %w", err)
	}
	return parsed, nil
}

func (c *Client) path(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return c.baseURL + "/api/" + strings.Join(escaped, "/")
}

func (c *Client) requestJSON(ctx context.Context, method, target string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("rabbitmq: encode request body: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.user) != "" || c.password != "" {
		req.SetBasicAuth(c.user, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBodyBytes)
	data, readErr := io.ReadAll(limited)
	if readErr != nil {
		return nil, fmt.Errorf("rabbitmq: read response: %w", readErr)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rabbitmq: status %d", resp.StatusCode)
	}
	return data, nil
}
