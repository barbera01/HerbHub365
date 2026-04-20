// Package publisher provides a client that queues videos for YouTube publishing
// via the RabbitMQ management HTTP API. This avoids an AMQP dependency in the
// manager; the management plugin ships with rabbitmq:3-management.
package publisher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Message mirrors video-publisher's ProducedMessage.
type Message struct {
	Slug       string `json:"slug"`
	Date       string `json:"date"`
	OutputFile string `json:"output_file"`
	Status     string `json:"status"`
	Timestamp  string `json:"timestamp"`
}

// Client publishes messages to RabbitMQ via the management HTTP API.
type Client struct {
	mgmtURL  string
	user     string
	password string
	queue    string
	http     *http.Client
}

// NewClient builds a Client from an AMQP URL (amqp://user:pass@host:5672/) and
// a queue name. It derives the management API base URL by replacing port 5672
// with 15672 on the same host.
func NewClient(amqpURL, queue string) (*Client, error) {
	u, err := url.Parse(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("parse amqp url: %w", err)
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()

	mgmtURL := "http://" + host + ":15672"

	return &Client{
		mgmtURL:  mgmtURL,
		user:     user,
		password: pass,
		queue:    queue,
		http:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Publish enqueues msg on the configured queue via the management API.
func (c *Client) Publish(msg Message) error {
	if msg.Status == "" {
		msg.Status = "completed"
	}
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	body := map[string]any{
		"properties":       map[string]any{"delivery_mode": 2},
		"routing_key":      c.queue,
		"payload":          string(payload),
		"payload_encoding": "string",
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal publish body: %w", err)
	}

	apiURL := c.mgmtURL + "/api/exchanges/%2F/amq.default/publish"
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.user, c.password)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST management API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("management API returned %d", resp.StatusCode)
	}

	var result struct {
		Routed bool `json:"routed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !result.Routed {
		return fmt.Errorf("message not routed — queue %q may not exist yet", c.queue)
	}
	return nil
}

// Enabled reports whether the client has a valid AMQP URL configured.
func Enabled(amqpURL string) bool {
	return strings.TrimSpace(amqpURL) != ""
}
