// Package timelapse provides an HTTP client for the timelapse-builder service.
package timelapse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

type BuildRequest struct {
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	OutputName    string   `json:"output_name,omitempty"`
	InputFPS      *int     `json:"input_fps,omitempty"`
	OutputFPS     *int     `json:"output_fps,omitempty"`
	CRF           *int     `json:"crf,omitempty"`
	MinBrightness *float64 `json:"min_brightness,omitempty"`
}

func (c *Client) Build(req BuildRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Post(c.baseURL+"/api/build", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("timelapse build: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct{ Error string `json:"error"` }
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return "", fmt.Errorf("timelapse-builder %d: %s", resp.StatusCode, e.Error)
		}
		return "", fmt.Errorf("timelapse-builder returned %d", resp.StatusCode)
	}
	var r struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return r.JobID, nil
}

func (c *Client) Jobs() (json.RawMessage, error) {
	return c.get("/api/jobs")
}

func (c *Client) Job(id string) (json.RawMessage, error) {
	return c.get("/api/jobs/" + id)
}

func (c *Client) Videos() (json.RawMessage, error) {
	return c.get("/api/videos")
}

func (c *Client) Config() (json.RawMessage, error) {
	return c.get("/api/config")
}

func (c *Client) ProxyVideoFile(w http.ResponseWriter, filename string) error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/videos/" + filename)
	if err != nil {
		return fmt.Errorf("fetch video: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("timelapse-builder returned %d", resp.StatusCode)
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if ct := resp.Header.Get("Content-Length"); ct != "" {
		w.Header().Set("Content-Length", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *Client) Health() bool {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(c.baseURL + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *Client) get(path string) (json.RawMessage, error) {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("timelapse-builder %s returned %d", path, resp.StatusCode)
	}
	return raw, nil
}
