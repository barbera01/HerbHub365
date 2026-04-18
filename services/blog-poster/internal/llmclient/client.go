// Package llmclient provides an HTTP client for the llm-service, implementing
// the markdownGenerator interface expected by the blog.Generator.
package llmclient

import (
	"bytes"
	"context"
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

type generateRequest struct {
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
}

type generateResponse struct {
	Content string `json:"content"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

// GenerateMarkdown calls llm-service using the default system prompt already
// configured in that service.  The blog-poster always supplies an explicit
// system prompt, so this path is only here to satisfy the interface.
func (c *Client) GenerateMarkdown(ctx context.Context, prompt string) (string, error) {
	return c.GenerateMarkdownWithSystemPrompt(ctx, "", prompt)
}

// GenerateMarkdownWithSystemPrompt sends a generation request to llm-service.
func (c *Client) GenerateMarkdownWithSystemPrompt(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body, err := json.Marshal(generateRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call llm-service: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp errorResponse
		if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Error != "" {
			return "", fmt.Errorf("llm-service returned %d: %s", resp.StatusCode, errResp.Error)
		}
		return "", fmt.Errorf("llm-service returned %d", resp.StatusCode)
	}

	var genResp generateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return "", fmt.Errorf("decode llm-service response: %w", err)
	}
	if strings.TrimSpace(genResp.Content) == "" {
		return "", fmt.Errorf("llm-service returned empty content")
	}

	return genResp.Content, nil
}

// WarmModel asks llm-service to warm the underlying model.
func (c *Client) WarmModel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/warm", http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("warm llm-service: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("llm-service warm returned %d", resp.StatusCode)
	}
	return nil
}
