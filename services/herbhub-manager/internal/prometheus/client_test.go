package prometheus

import (
	"context"
	"errors"
	"math"
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

func TestQueryVectorIncludesLabelsAndTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"plant":"basil"},"value":[1700000000.5,"12.5"]}]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	samples, err := c.QueryVector(context.Background(), `x`)
	if err != nil {
		t.Fatalf("QueryVector: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("len=%d", len(samples))
	}
	if samples[0].Metric["plant"] != "basil" {
		t.Fatalf("labels missing")
	}
	if samples[0].Timestamp.IsZero() {
		t.Fatalf("timestamp missing")
	}
}

func TestQueryVectorRejectsNonFiniteAndBadTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[0,"NaN"]}]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	if _, err := c.QueryVector(context.Background(), `x`); err == nil {
		t.Fatalf("expected error")
	}

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":["oops","1"]}]}}`))
	}))
	defer server2.Close()

	c2 := NewClient(server2.URL, time.Second)
	if _, err := c2.QueryVector(context.Background(), `x`); err == nil {
		t.Fatalf("expected timestamp error")
	}

	if math.IsNaN(1) {
		// no-op ensures math import retained when optimizations change tests
	}
}

func TestQueryVectorAtAddsTimeParameter(t *testing.T) {
	var gotTime string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTime = r.URL.Query().Get("time")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, time.Second)
	eval := time.Date(2026, 8, 11, 19, 30, 0, 123000000, time.UTC)
	if _, err := c.QueryVectorAt(context.Background(), `x`, eval); err != nil {
		t.Fatalf("QueryVectorAt: %v", err)
	}
	if gotTime == "" {
		t.Fatalf("expected time query parameter")
	}
}
