package rabbitmq

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPathEscapingAndPublish(t *testing.T) {
	var gotEscapedPath string
	var gotAuth string
	var gotProps map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscapedPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		props, _ := body["properties"].(map[string]any)
		gotProps = props
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"routed": true})
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, VHost: "/", User: "u", Password: "p", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.Publish(context.Background(), PublishRequest{Exchange: "herbhub.watering", RoutingKey: "watering.basil", Payload: "{}", DeliveryMode: 2, ContentType: "application/json", AppID: "herbhub-manager", MessageID: "m", Timestamp: time.Unix(100, 0), Expiration: "300000"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if want := "/api/exchanges/%2F/herbhub.watering/publish"; gotEscapedPath != want {
		t.Fatalf("escaped path = %q want %q", gotEscapedPath, want)
	}
	if gotAuth == "" || !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("expected basic auth header")
	}
	if gotAuth != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
		t.Fatalf("unexpected auth header")
	}
	if gotProps["expiration"] != "300000" {
		t.Fatalf("expected expiration property 300000, got %v", gotProps["expiration"])
	}
}

func TestNotFoundMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, VHost: "/", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.GetQueue(context.Background(), "missing")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound got %v", err)
	}
}

func TestPublishRoutedFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"routed": false})
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, VHost: "/", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Publish(context.Background(), PublishRequest{Exchange: "e", RoutingKey: "k", Payload: "{}", DeliveryMode: 2, ContentType: "application/json", AppID: "a", MessageID: "m", Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if resp.Routed {
		t.Fatalf("expected routed false")
	}
}

func TestExchangeAndQueueExpandedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/exchanges/"):
			_, _ = w.Write([]byte(`{"name":"x","type":"topic","durable":true,"auto_delete":false,"internal":false}`))
		case strings.Contains(r.URL.Path, "/queues/"):
			_, _ = w.Write([]byte(`{"name":"q","durable":true,"auto_delete":false,"exclusive":false,"arguments":{"x-message-ttl":86400000},"messages_ready":1,"messages_unacknowledged":2,"consumers":3}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, VHost: "/", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ex, err := c.GetExchange(context.Background(), "x")
	if err != nil {
		t.Fatalf("GetExchange: %v", err)
	}
	if ex.AutoDelete || ex.Internal {
		t.Fatalf("unexpected exchange fields")
	}
	q, err := c.GetQueue(context.Background(), "q")
	if err != nil {
		t.Fatalf("GetQueue: %v", err)
	}
	if q.AutoDelete || q.Exclusive || q.MessagesReady != 1 || q.MessagesUnacknowledged != 2 || q.Consumers != 3 {
		t.Fatalf("unexpected queue fields: %+v", q)
	}
}

func TestListSourceBindings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/bindings/source") {
			_, _ = w.Write([]byte(`[{"source":"herbhub.watering","destination":"watering.queue","destination_type":"queue","routing_key":"watering.#","arguments":{},"properties_key":"watering.%23","vhost":"/"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, VHost: "/", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	b, err := c.ListSourceBindings(context.Background(), "herbhub.watering")
	if err != nil {
		t.Fatalf("ListSourceBindings: %v", err)
	}
	if len(b) != 1 {
		t.Fatalf("expected one binding, got %d", len(b))
	}
	if b[0].DestinationType != "queue" || b[0].Destination != "watering.queue" || b[0].RoutingKey != "watering.#" {
		t.Fatalf("unexpected binding: %+v", b[0])
	}
}
