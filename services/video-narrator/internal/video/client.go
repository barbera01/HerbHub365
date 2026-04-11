package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"HerbHub365/services/video-narrator/internal/config"
)

// Client submits text to the avatar video generation API and returns raw MP4 bytes.
// The API is asynchronous: POST /generate → job_id → poll /job/{id} → GET /download/{id}.
type Client struct {
	cfg        config.VideoConfig
	httpClient *http.Client
}

// NewClient constructs a Client from config.
func NewClient(cfg config.VideoConfig, requestTimeout time.Duration) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// ── request / response shapes ─────────────────────────────────────────────────

// generateRequest is the payload for POST /generate.
// Note: MuseTalk preserves the source avatar resolution — resolution/fps
// params are NOT sent because the API ignores them and always outputs at the
// native resolution of the avatar source file (e.g. 1280×720 for the 720p
// green-screen loop). Upscaling to 1920×1080 is handled by ffmpeg in concat.
type generateRequest struct {
	AvatarID string `json:"avatar_id"`
	Text     string `json:"text"`
}

type generateResponse struct {
	JobID string `json:"job_id"`
}

type jobStatusResponse struct {
	JobID    string  `json:"job_id"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// ── public API ────────────────────────────────────────────────────────────────

// Generate submits text to the video API using the client's configured avatar,
// waits for the job to complete, and returns the raw MP4 bytes.
func (c *Client) Generate(ctx context.Context, text string) ([]byte, error) {
	return c.GenerateAs(ctx, text, c.cfg.AvatarID)
}

// GenerateAs submits text using a specific avatar ID.
func (c *Client) GenerateAs(ctx context.Context, text, avatarID string) ([]byte, error) {
	// 1. Submit the job.
	jobID, err := c.submit(ctx, avatarID, text)
	if err != nil {
		return nil, fmt.Errorf("submit video job: %w", err)
	}

	// 2. Poll until complete, failed, or timed out.
	if err := c.waitForCompletion(ctx, jobID); err != nil {
		return nil, fmt.Errorf("job %s: %w", jobID, err)
	}

	// 3. Download the finished MP4.
	mp4, err := c.download(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("download job %s: %w", jobID, err)
	}
	return mp4, nil
}

// ── exported wrappers (used by the server package for fine-grained progress) ──

// JobStatusResponse is the exported form of the MuseTalk job status response.
type JobStatusResponse struct {
	JobID    string  `json:"job_id"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// Submit sends a generation request to the MuseTalk API and returns the job ID.
func (c *Client) Submit(ctx context.Context, avatarID, text string) (string, error) {
	return c.submit(ctx, avatarID, text)
}

// PollOnce checks the status of a MuseTalk job once and returns the response.
func (c *Client) PollOnce(ctx context.Context, jobID string) (*JobStatusResponse, error) {
	js, err := c.pollStatus(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return &JobStatusResponse{
		JobID:    js.JobID,
		Status:   js.Status,
		Progress: js.Progress,
		Error:    js.Error,
	}, nil
}

// Download fetches the completed MP4 bytes for a MuseTalk job.
func (c *Client) Download(ctx context.Context, jobID string) ([]byte, error) {
	return c.download(ctx, jobID)
}

// Cfg returns the video configuration (used by the server to access defaults).
func (c *Client) Cfg() config.VideoConfig {
	return c.cfg
}

// ── internal steps ────────────────────────────────────────────────────────────

func (c *Client) submit(ctx context.Context, avatarID, text string) (string, error) {
	body := generateRequest{
		AvatarID: avatarID,
		Text:     text,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST /generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("POST /generate returned %d: %s", resp.StatusCode, string(snippet))
	}

	var gr generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", fmt.Errorf("decode generate response: %w", err)
	}
	if gr.JobID == "" {
		return "", fmt.Errorf("generate response contained no job_id")
	}
	return gr.JobID, nil
}

func (c *Client) waitForCompletion(ctx context.Context, jobID string) error {
	deadline := time.Now().Add(c.cfg.MaxWait)
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timed out after %s waiting for job to complete", c.cfg.MaxWait)
			}

			status, err := c.pollStatus(ctx, jobID)
			if err != nil {
				// Non-fatal: log and keep polling.
				continue
			}

			switch strings.ToLower(status.Status) {
			case "completed", "done", "success":
				return nil
			case "failed", "error":
				msg := status.Error
				if msg == "" {
					msg = "unknown error"
				}
				return fmt.Errorf("job failed: %s", msg)
			default:
				// queued, processing, pending — keep waiting.
			}
		}
	}
}

func (c *Client) pollStatus(ctx context.Context, jobID string) (*jobStatusResponse, error) {
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/job/" + jobID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("GET /job/%s returned %d: %s", jobID, resp.StatusCode, string(snippet))
	}

	var js jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&js); err != nil {
		return nil, fmt.Errorf("decode job status: %w", err)
	}
	return &js, nil
}

func (c *Client) download(ctx context.Context, jobID string) ([]byte, error) {
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/download/" + jobID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /download/%s: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET /download/%s returned %d: %s", jobID, resp.StatusCode, string(snippet))
	}

	mp4, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download body: %w", err)
	}
	if len(mp4) == 0 {
		return nil, fmt.Errorf("download returned empty body")
	}
	return mp4, nil
}
