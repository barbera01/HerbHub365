package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestInstantQueryParsesVector(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1700000000,"12"]}]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	v, err := c.InstantQuery(context.Background(), `sum(x)`)
	if err != nil {
		t.Fatalf("InstantQuery: %v", err)
	}
	if v != 12 {
		t.Fatalf("value = %v want 12", v)
	}
	if rawQuery != `sum(x)` {
		t.Fatalf("unexpected query=%q", rawQuery)
	}
}

func TestInstantQueryUnavailable(t *testing.T) {
	c := NewClient("", time.Second)
	if c != nil {
		t.Fatalf("expected nil client when url empty")
	}
}

func TestInstantQueryNoSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	_, err := c.InstantQuery(context.Background(), `sum(y)`)
	if !errors.Is(err, ErrNoSeries) {
		t.Fatalf("expected ErrNoSeries got %v", err)
	}
}

func TestInstantQueryEncodesQuery(t *testing.T) {
	var gotRaw string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1700000000,"1"]}]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	_, err := c.InstantQuery(context.Background(), `sum(metric{queue="a",vhost="/"})`)
	if err != nil {
		t.Fatalf("InstantQuery: %v", err)
	}
	vals, _ := url.ParseQuery(gotRaw)
	if vals.Get("query") == "" {
		t.Fatalf("query parameter missing")
	}
}
