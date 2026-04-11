// Package video provides a client that proxies video generation requests
// through the video-narrator API, which handles the full pipeline:
// TTS preprocessing → MuseTalk → ffmpeg concat with chroma-key.
package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"HerbHub365/services/herbhub-video/internal/config"
)

// ── Job tracking (mirrors video-narrator server.Job) ─────────────────────────

// JobStatus represents the current state of a video generation job.
type JobStatus struct {
	ID               string  `json:"id"`
	Slug             string  `json:"slug"`
	AvatarID         string  `json:"avatar_id"`
	Phase            string  `json:"phase"` // queued, preprocessing, submitting, generating, downloading, stitching, completed, failed
	Progress         float64 `json:"progress"`
	Error            string  `json:"error,omitempty"`
	VideoFile        string  `json:"video_file,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	ConcatEnabled    bool    `json:"concat_enabled"`
	ConcatIntro      string  `json:"concat_intro,omitempty"`
	ConcatOutro      string  `json:"concat_outro,omitempty"`
	ChromaKeyEnabled bool    `json:"chroma_key_enabled"`
	ChromaKeyBG      string  `json:"chroma_key_bg,omitempty"`
}

// GenerateRequest is the payload sent to the video-narrator API.
type GenerateRequest struct {
	Slug             string `json:"slug"`
	AvatarID         string `json:"avatar_id,omitempty"`
	Text             string `json:"text,omitempty"`
	ConcatEnabled    *bool  `json:"concat_enabled,omitempty"`
	ConcatIntro      string `json:"concat_intro,omitempty"`
	ConcatOutro      string `json:"concat_outro,omitempty"`
	ChromaKeyEnabled *bool  `json:"chroma_key_enabled,omitempty"`
	ChromaKeyBG      string `json:"chroma_key_bg,omitempty"`
}

// NarratorConfig is returned by the video-narrator /api/config endpoint.
type NarratorConfig struct {
	MuseTalkURL      string   `json:"musetalk_url"`
	DefaultAvatar    string   `json:"default_avatar"`
	Avatars          []string `json:"avatars"`
	PostsDir         string   `json:"posts_dir"`
	OutputDir        string   `json:"output_dir"`
	ConcatEnabled    bool     `json:"concat_enabled"`
	ChromaKeyEnabled bool     `json:"chroma_key_enabled"`
	PollInterval     string   `json:"poll_interval"`
	MaxWait          string   `json:"max_wait"`
}

// Resources is returned by the video-narrator /api/resources endpoint.
type Resources struct {
	Intros      []string `json:"intros"`
	Outros      []string `json:"outros"`
	Backgrounds []string `json:"backgrounds"`
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client proxies requests through the video-narrator API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Client targeting the video-narrator API.
func NewClient(cfg config.Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.NarratorURL, "/"),
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// SubmitJob sends a generation request to the video-narrator API.
func (c *Client) SubmitJob(req GenerateRequest) (string, error) {
	encoded, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/generate",
		"application/json",
		bytes.NewReader(encoded),
	)
	if err != nil {
		return "", fmt.Errorf("POST /api/generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("POST /api/generate returned %d: %s", resp.StatusCode, string(snippet))
	}

	var result struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.JobID, nil
}

// Job fetches the status of a specific job from the video-narrator API.
func (c *Client) Job(id string) (*JobStatus, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/jobs/" + id)
	if err != nil {
		return nil, fmt.Errorf("GET /api/jobs/%s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET /api/jobs/%s returned %d: %s", id, resp.StatusCode, string(snippet))
	}

	var job JobStatus
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode job: %w", err)
	}
	return &job, nil
}

// Jobs fetches all jobs from the video-narrator API.
func (c *Client) Jobs() ([]JobStatus, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/jobs")
	if err != nil {
		return nil, fmt.Errorf("GET /api/jobs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET /api/jobs returned %d: %s", resp.StatusCode, string(snippet))
	}

	var result struct {
		Jobs []JobStatus `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode jobs: %w", err)
	}
	return result.Jobs, nil
}

// Config fetches the narrator's configuration.
func (c *Client) Config() (*NarratorConfig, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/config")
	if err != nil {
		return nil, fmt.Errorf("GET /api/config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET /api/config returned %d: %s", resp.StatusCode, string(snippet))
	}

	var cfg NarratorConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &cfg, nil
}

// Resources fetches available intros, outros, and backgrounds.
func (c *Client) Resources() (*Resources, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/resources")
	if err != nil {
		return nil, fmt.Errorf("GET /api/resources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET /api/resources returned %d: %s", resp.StatusCode, string(snippet))
	}

	var res Resources
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode resources: %w", err)
	}
	return &res, nil
}

// Health checks if the video-narrator API is reachable.
func (c *Client) Health() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(c.baseURL + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
