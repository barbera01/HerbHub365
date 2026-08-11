package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	if c == nil || c.baseURL == "" {
		return 0, fmt.Errorf("prometheus: disabled")
	}
	u := c.baseURL + "/api/v1/query?query=" + url.QueryEscape(expr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("prometheus: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prometheus: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, fmt.Errorf("prometheus: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}

	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("prometheus: decode response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus: non-success status %q", parsed.Status)
	}
	if parsed.Data.ResultType != "vector" {
		return 0, fmt.Errorf("prometheus: unsupported result type %q", parsed.Data.ResultType)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, ErrNoSeries
	}
	first := parsed.Data.Result[0]
	if len(first.Value) < 2 {
		return 0, fmt.Errorf("prometheus: malformed vector value")
	}
	v, ok := first.Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("prometheus: vector sample is not string")
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("prometheus: parse sample: %w", err)
	}
	return n, nil
}
