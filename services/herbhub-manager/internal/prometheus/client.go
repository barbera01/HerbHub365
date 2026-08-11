package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

var ErrNoSeries = errors.New("prometheus: no series")

type Client struct {
	baseURL string
	http    *http.Client
}

type Sample struct {
	Metric    map[string]string
	Value     float64
	Timestamp time.Time
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *Client) InstantQuery(ctx context.Context, expr string) (float64, error) {
	samples, err := c.QueryVector(ctx, expr)
	if err != nil {
		return 0, err
	}
	if len(samples) == 0 {
		return 0, ErrNoSeries
	}
	return samples[0].Value, nil
}

func (c *Client) QueryVector(ctx context.Context, expr string) ([]Sample, error) {
	return c.QueryVectorAt(ctx, expr, time.Time{})
}

func (c *Client) QueryVectorAt(ctx context.Context, expr string, evaluationTime time.Time) ([]Sample, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("prometheus: disabled")
	}
	query := url.Values{}
	query.Set("query", expr)
	if !evaluationTime.IsZero() {
		query.Set("time", evaluationTime.UTC().Format(time.RFC3339Nano))
	}
	u := c.baseURL + "/api/v1/query?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("prometheus: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("prometheus: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}

	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]any `json:"metric"`
				Value  []any          `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("prometheus: decode response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("prometheus: non-success status %q", parsed.Status)
	}
	if parsed.Data.ResultType != "vector" {
		return nil, fmt.Errorf("prometheus: unsupported result type %q", parsed.Data.ResultType)
	}
	if len(parsed.Data.Result) == 0 {
		return nil, ErrNoSeries
	}
	out := make([]Sample, 0, len(parsed.Data.Result))
	for _, item := range parsed.Data.Result {
		if len(item.Value) < 2 {
			return nil, fmt.Errorf("prometheus: malformed vector value")
		}
		ts, err := parseTimestamp(item.Value[0])
		if err != nil {
			return nil, err
		}
		v, ok := item.Value[1].(string)
		if !ok {
			return nil, fmt.Errorf("prometheus: vector sample is not string")
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("prometheus: parse sample: %w", err)
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("prometheus: non-finite sample")
		}
		labels := make(map[string]string, len(item.Metric))
		for k, raw := range item.Metric {
			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("prometheus: metric label %q not string", k)
			}
			labels[k] = s
		}
		out = append(out, Sample{Metric: labels, Value: n, Timestamp: ts})
	}
	return out, nil
}

func parseTimestamp(raw any) (time.Time, error) {
	var f float64
	switch v := raw.(type) {
	case float64:
		f = v
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("prometheus: parse timestamp: %w", err)
		}
		f = parsed
	default:
		return time.Time{}, fmt.Errorf("prometheus: timestamp is not number")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return time.Time{}, fmt.Errorf("prometheus: invalid timestamp")
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * float64(time.Second))
	ts := time.Unix(sec, nsec).UTC()
	if ts.IsZero() {
		return time.Time{}, fmt.Errorf("prometheus: invalid timestamp")
	}
	return ts, nil
}
