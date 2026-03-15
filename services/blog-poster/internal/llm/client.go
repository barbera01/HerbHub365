package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"HerbHub365/services/blog-poster/internal/config"
)

type Client struct {
	httpClient *http.Client
	config     config.LLMConfig
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Think       *bool         `json:"think,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          any    `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
}

type ollamaRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	Options   ollamaOptions `json:"options,omitempty"`
	KeepAlive string        `json:"keep_alive,omitempty"`
	Think     *bool         `json:"think,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
}

func NewClient(cfg config.LLMConfig) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.RequestTimeout},
		config:     cfg,
	}
}

func (c *Client) GenerateMarkdown(ctx context.Context, prompt string) (string, error) {
	return c.GenerateMarkdownWithSystemPrompt(ctx, c.config.SystemPrompt, prompt)
}

func (c *Client) GenerateMarkdownWithSystemPrompt(ctx context.Context, systemPrompt, prompt string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(c.config.Provider)) {
	case "", "auto":
		content, err := c.generateOpenAICompatible(ctx, systemPrompt, prompt)
		if err == nil {
			return content, nil
		}
		if !shouldTryOllamaFallback(err) {
			return "", err
		}

		content, ollamaErr := c.generateOllamaNative(ctx, systemPrompt, prompt)
		if ollamaErr != nil {
			return "", fmt.Errorf("openai-compatible call failed: %v; ollama fallback failed: %w", err, ollamaErr)
		}

		return content, nil
	case "ollama":
		return c.generateOllamaNative(ctx, systemPrompt, prompt)
	case "openai", "openai-compatible":
		return c.generateOpenAICompatible(ctx, systemPrompt, prompt)
	default:
		return "", fmt.Errorf("unsupported LLM_PROVIDER %q", c.config.Provider)
	}
}

func (c *Client) generateOpenAICompatible(ctx context.Context, systemPrompt, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Think:       boolPtr(false),
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionsURL(c.config.BaseURL), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call llm: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if c.config.Debug {
		log.Printf("llm openai-compatible response: %s", truncateForLog(responseBody))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}

	content := extractMarkdownContent(
		parsed.Choices[0].Message.Content,
		parsed.Choices[0].Message.ReasoningContent,
		parsed.Choices[0].Message.Reasoning,
	)
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("llm returned empty content")
	}

	return strings.TrimSpace(content), nil
}

func (c *Client) generateOllamaNative(ctx context.Context, systemPrompt, prompt string) (string, error) {
	body, err := json.Marshal(ollamaRequest{
		Model: c.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
		Options: ollamaOptions{
			Temperature: c.config.Temperature,
			NumPredict:  c.config.MaxTokens,
		},
		KeepAlive: "5m",
		Think:     boolPtr(false),
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaChatURL(c.config.BaseURL), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if c.config.Debug {
		log.Printf("llm ollama response: %s", truncateForLog(responseBody))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed ollamaResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	content := extractMarkdownContent(parsed.Message.Content, parsed.Message.Thinking)
	if content == "" {
		return "", fmt.Errorf("ollama returned empty content")
	}

	return content, nil
}

func completionsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(trimmed, "/api/chat") {
		return strings.TrimSuffix(trimmed, "/api/chat") + "/v1/chat/completions"
	}
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/v1/chat/completions"
}

func ollamaChatURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(trimmed, "/v1/chat/completions") {
		return strings.TrimSuffix(trimmed, "/v1/chat/completions") + "/api/chat"
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return strings.TrimSuffix(trimmed, "/v1") + "/api/chat"
	}
	if strings.HasSuffix(trimmed, "/api/chat") {
		return trimmed
	}
	return trimmed + "/api/chat"
}

func shouldTryOllamaFallback(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found") || strings.Contains(message, "deadline exceeded") || errors.Is(err, context.DeadlineExceeded)
}

func flattenContent(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			piece, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := piece["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func truncateForLog(body []byte) string {
	const max = 4000
	text := strings.TrimSpace(string(body))
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func extractMarkdownContent(primary any, fallbacks ...string) string {
	content := strings.TrimSpace(flattenContent(primary))
	if content != "" && !looksLikeThinking(content) {
		return content
	}

	for _, fallback := range fallbacks {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" {
			continue
		}
		if !looksLikeThinking(fallback) {
			return fallback
		}
		if extracted := extractDraftFromReasoning(fallback); extracted != "" {
			return extracted
		}
	}

	if content != "" {
		if extracted := extractDraftFromReasoning(content); extracted != "" {
			return extracted
		}
	}

	return ""
}

func looksLikeThinking(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	return strings.HasPrefix(trimmed, "thinking process:") || strings.Contains(trimmed, "analyze the request:") || strings.Contains(trimmed, "drafting - paragraph by paragraph:")
}

func extractDraftFromReasoning(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	headingPattern := regexp.MustCompile(`^\s*(?:[-*]\s*)?(?:draft:\s*)?#\s+(.+?)\s*$`)
	titlePattern := regexp.MustCompile(`^\s*(?:[-*]\s*)?(?:\*+)?title(?:\*+)?:\s*(.+?)\s*$`)
	draftPattern := regexp.MustCompile(`^\s*(?:[-*]\s*)?draft:\s*(.+?)\s*$`)

	title := ""
	paragraphs := make([]string, 0, 4)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if matches := headingPattern.FindStringSubmatch(trimmed); len(matches) == 2 && title == "" {
			title = strings.TrimSpace(matches[1])
			continue
		}

		if matches := titlePattern.FindStringSubmatch(trimmed); len(matches) == 2 && title == "" {
			candidate := strings.TrimSpace(matches[1])
			candidate = strings.Trim(candidate, "\"'")
			candidate = strings.TrimPrefix(candidate, "#")
			candidate = strings.TrimSpace(candidate)
			if candidate != "" && !strings.Contains(strings.ToLower(candidate), "needs to be") {
				title = candidate
			}
			continue
		}

		if matches := draftPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
			paragraph := cleanDraftLine(matches[1])
			if paragraph != "" {
				paragraphs = append(paragraphs, paragraph)
			}
		}
	}

	if len(paragraphs) == 0 {
		return ""
	}

	body := strings.Join(paragraphs, "\n\n")
	if title != "" {
		return "# " + title + "\n\n" + body
	}
	return body
}

func cleanDraftLine(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "*")
	trimmed = strings.TrimSpace(trimmed)
	return trimmed
}

func boolPtr(value bool) *bool {
	return &value
}
