package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"HerbHub365/services/llm-service/internal/config"
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
	Temperature   float64 `json:"temperature,omitempty"`
	NumPredict    int     `json:"num_predict,omitempty"`
	TopP          float64 `json:"top_p,omitempty"`
	RepeatPenalty float64 `json:"repeat_penalty,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

type geminiRequest struct {
	SystemInstruction geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func NewClient(cfg config.LLMConfig) *Client {
	// No Timeout on the http.Client — long Ollama streaming responses are
	// cancelled via the request context instead. ResponseHeaderTimeout on the
	// transport catches a dead/unresponsive LLM host without cutting off a
	// legitimate slow generation mid-stream.
	transport := &http.Transport{
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &Client{
		httpClient: &http.Client{Transport: transport},
		config:     cfg,
	}
}

func (c *Client) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	content, err := c.generatePrimary(ctx, systemPrompt, userPrompt)
	if err == nil {
		return content, nil
	}
	if !c.shouldTryConfiguredFallback(ctx, err) {
		return "", err
	}

	log.Printf("primary LLM unavailable, trying %s fallback: %v", c.config.FallbackProvider, err)
	fallbackContent, fallbackErr := c.generateFallback(ctx, systemPrompt, userPrompt)
	if fallbackErr != nil {
		return "", fmt.Errorf("primary LLM failed: %v; fallback %s failed: %w", err, c.config.FallbackProvider, fallbackErr)
	}
	return fallbackContent, nil
}

func (c *Client) generatePrimary(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(c.config.Provider)) {
	case "", "auto":
		content, err := c.generateOpenAICompatible(ctx, systemPrompt, userPrompt)
		if err == nil {
			return content, nil
		}
		if !shouldTryOllamaFallback(ctx, err) {
			return "", err
		}

		content, ollamaErr := c.generateOllamaStreaming(ctx, systemPrompt, userPrompt)
		if ollamaErr != nil {
			return "", fmt.Errorf("openai-compatible call failed: %v; ollama fallback failed: %w", err, ollamaErr)
		}

		return content, nil
	case "ollama":
		return c.generateOllamaStreaming(ctx, systemPrompt, userPrompt)
	case "openai", "openai-compatible":
		return c.generateOpenAICompatible(ctx, systemPrompt, userPrompt)
	case "gemini", "google", "google-gemini":
		return c.generateGemini(ctx, systemPrompt, userPrompt, c.config.BaseURL, c.config.APIKey, c.config.Model)
	default:
		return "", fmt.Errorf("unsupported LLM_PROVIDER %q", c.config.Provider)
	}
}

func (c *Client) shouldTryConfiguredFallback(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(c.config.FallbackProvider))
	if provider == "" || provider == "none" || provider == "disabled" {
		return false
	}
	if provider == strings.ToLower(strings.TrimSpace(c.config.Provider)) {
		return false
	}
	if isGeminiProvider(provider) && c.config.FallbackAPIKey == "" {
		log.Printf("Gemini fallback configured but LLM_FALLBACK_API_KEY/GEMINI_API_KEY is not set")
		return false
	}
	return IsAvailabilityError(err) || c.primaryIsLocalProvider()
}

func (c *Client) primaryIsLocalProvider() bool {
	switch strings.ToLower(strings.TrimSpace(c.config.Provider)) {
	case "", "auto", "ollama":
		return true
	default:
		return false
	}
}

func (c *Client) generateFallback(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	switch provider := strings.ToLower(strings.TrimSpace(c.config.FallbackProvider)); provider {
	case "gemini", "google", "google-gemini":
		return c.generateGemini(ctx, systemPrompt, userPrompt, c.config.FallbackBaseURL, c.config.FallbackAPIKey, c.config.FallbackModel)
	case "openai", "openai-compatible":
		fallback := *c
		fallback.config.Provider = provider
		fallback.config.BaseURL = c.config.FallbackBaseURL
		fallback.config.APIKey = c.config.FallbackAPIKey
		fallback.config.Model = c.config.FallbackModel
		return fallback.generateOpenAICompatible(ctx, systemPrompt, userPrompt)
	case "ollama":
		fallback := *c
		fallback.config.Provider = provider
		fallback.config.BaseURL = c.config.FallbackBaseURL
		fallback.config.APIKey = c.config.FallbackAPIKey
		fallback.config.Model = c.config.FallbackModel
		return fallback.generateOllamaStreaming(ctx, systemPrompt, userPrompt)
	default:
		return "", fmt.Errorf("unsupported LLM_FALLBACK_PROVIDER %q", c.config.FallbackProvider)
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

func (c *Client) generateGemini(ctx context.Context, systemPrompt, prompt, baseURL, apiKey, model string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("Gemini API key is required")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("Gemini model is required")
	}

	body, err := json.Marshal(geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: prompt}}},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     c.config.Temperature,
			TopP:            c.config.TopP,
			MaxOutputTokens: c.config.MaxTokens,
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiGenerateURL(baseURL, model, apiKey), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call gemini: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if c.config.Debug {
		log.Printf("llm gemini response: %s", truncateForLog(responseBody))
	}

	var parsed geminiResponse
	decodeErr := json.Unmarshal(responseBody, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decodeErr != nil {
			return "", fmt.Errorf("gemini returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("gemini returned %d: %s", resp.StatusCode, parsed.Error.Message)
		}
		return "", fmt.Errorf("gemini returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if decodeErr != nil {
		return "", fmt.Errorf("decode gemini response: %w", decodeErr)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("gemini error: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	parts := parsed.Candidates[0].Content.Parts
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		texts = append(texts, part.Text)
	}
	content := extractMarkdownContent(strings.Join(texts, ""))
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("gemini returned empty content")
	}

	return strings.TrimSpace(content), nil
}

func (c *Client) WarmModel(ctx context.Context) error {
	provider := strings.ToLower(strings.TrimSpace(c.config.Provider))
	if provider == "openai" || provider == "openai-compatible" || isGeminiProvider(provider) {
		return nil
	}
	body, err := json.Marshal(ollamaRequest{
		Model: c.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: "Warm the model and return a short response."},
			{Role: "user", Content: "hi"},
		},
		Stream: false,
		Options: ollamaOptions{
			NumPredict: 1,
		},
		KeepAlive: "10m",
		Think:     boolPtr(false),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaChatURL(c.config.BaseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("warm model: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("warm model returned %d", resp.StatusCode)
	}
	log.Printf("model %s warmed up", c.config.Model)
	return nil
}

func (c *Client) generateOllamaStreaming(ctx context.Context, systemPrompt, prompt string) (string, error) {
	body, err := json.Marshal(ollamaRequest{
		Model: c.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: true,
		Options: ollamaOptions{
			Temperature:   c.config.Temperature,
			NumPredict:    c.config.MaxTokens,
			TopP:          c.config.TopP,
			RepeatPenalty: c.config.RepeatPenalty,
		},
		KeepAlive: "10m",
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var rawBuilder strings.Builder
	var contentBuilder strings.Builder
	var thinkingBuilder strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if c.config.Debug {
			rawBuilder.Write(line)
		}
		var chunk ollamaResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return "", fmt.Errorf("decode ollama stream chunk: %w", err)
		}
		if chunk.Error != "" {
			return "", fmt.Errorf("ollama stream error: %s", chunk.Error)
		}
		contentBuilder.WriteString(chunk.Message.Content)
		thinkingBuilder.WriteString(chunk.Message.Thinking)
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read ollama stream: %w", err)
	}
	if c.config.Debug && rawBuilder.Len() > 0 {
		log.Printf("llm ollama response: %s", truncateForLog([]byte(rawBuilder.String())))
	}
	content := extractMarkdownContent(contentBuilder.String(), thinkingBuilder.String())
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

func geminiGenerateURL(base, model, apiKey string) string {
	trimmedBase := strings.TrimRight(strings.TrimSpace(base), "/")
	trimmedModel := strings.Trim(strings.TrimSpace(model), "/")
	if !strings.HasPrefix(trimmedModel, "models/") {
		trimmedModel = "models/" + trimmedModel
	}
	endpoint := trimmedBase + "/" + trimmedModel + ":generateContent"
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint + "?key=" + url.QueryEscape(apiKey)
	}
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isGeminiProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gemini", "google", "google-gemini":
		return true
	default:
		return false
	}
}

func shouldTryOllamaFallback(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	// Never fall back if the context is already done — Ollama will fail
	// immediately for the same reason and produce confusing duplicate errors.
	if ctx.Err() != nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

// IsAvailabilityError returns true for errors that indicate the LLM host is
// unreachable or overloaded (connection refused, network timeout, etc.) as
// opposed to errors caused by the request content or response parsing.
func IsAvailabilityError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false // timeout is reported separately
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "dial ") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "returned 502") ||
		strings.Contains(msg, "returned 503") ||
		strings.Contains(msg, "returned 504")
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
	content = stripThinkTags(content)
	content = stripPreambleBeforeTitle(content)
	content = stripFencedCodeBlocks(content)
	content = ensureMarkdownTitle(content)
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

func stripThinkTags(content string) string {
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	cleaned := re.ReplaceAllString(content, "")
	return strings.TrimSpace(cleaned)
}

func stripPreambleBeforeTitle(content string) string {
	idx := strings.Index(content, "\n# ")
	if idx > 0 {
		return strings.TrimSpace(content[idx+1:])
	}
	if strings.HasPrefix(strings.TrimSpace(content), "# ") {
		return strings.TrimSpace(content)
	}
	return content
}

func stripFencedCodeBlocks(content string) string {
	re := regexp.MustCompile("(?m)^```[a-zA-Z]*\\n?")
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}

func ensureMarkdownTitle(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
		return trimmed
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	first := strings.TrimSpace(lines[0])
	for strings.HasPrefix(first, "#") {
		first = strings.TrimSpace(strings.TrimPrefix(first, "#"))
	}
	if len(lines) == 2 {
		return "# " + first + "\n" + lines[1]
	}
	return "# " + first
}

func boolPtr(value bool) *bool {
	return &value
}
