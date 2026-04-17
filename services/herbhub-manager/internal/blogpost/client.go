// Package blogpost provides an HTTP client for llm-service and helpers for
// saving generated content as Jekyll posts.
package blogpost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"HerbHub365/services/herbhub-manager/internal/config"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	cfg        config.BlogConfig
}

func NewClient(cfg config.BlogConfig) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.LLMTimeout},
		baseURL:    strings.TrimRight(cfg.LLMServiceURL, "/"),
		cfg:        cfg,
	}
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

// Generate calls llm-service and returns the raw markdown content.
func (c *Client) Generate(systemPrompt, userPrompt string) (string, error) {
	if systemPrompt == "" {
		systemPrompt = c.cfg.SystemPrompt
	}
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt(c.cfg)
	}

	body, err := json.Marshal(generateRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("call llm-service: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp errorResponse
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
			return "", fmt.Errorf("llm-service %d: %s", resp.StatusCode, errResp.Error)
		}
		return "", fmt.Errorf("llm-service returned %d", resp.StatusCode)
	}

	var genResp generateResponse
	if err := json.Unmarshal(raw, &genResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(genResp.Content) == "" {
		return "", fmt.Errorf("llm-service returned empty content")
	}

	return genResp.Content, nil
}

// PostMeta holds the derived metadata for a Jekyll post file.
type PostMeta struct {
	Filename string
	Slug     string
	Title    string
	Date     time.Time
}

// DeriveFilename builds a Jekyll-style filename and slug from content and a
// requested date (if zero, today UTC is used).
func DeriveFilename(content string, date time.Time) PostMeta {
	if date.IsZero() {
		date = time.Now().UTC()
	}

	title := extractTitle(content)
	slug := slugify(title)
	filename := fmt.Sprintf("%s-%s.markdown", date.Format("2006-01-02"), slug)

	return PostMeta{
		Filename: filename,
		Slug:     slug,
		Title:    title,
		Date:     date,
	}
}

// InjectFrontMatter prepends Jekyll front matter to raw LLM content that
// doesn't already have it.
func InjectFrontMatter(content string, meta PostMeta, categories string) string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "---") {
		return content
	}

	if categories == "" {
		categories = "Daily Update"
	}

	fm := fmt.Sprintf(`---
layout: post
title: "%s"
date: %s +0000
categories: %s
---

`,
		strings.ReplaceAll(meta.Title, `"`, `\"`),
		meta.Date.Format("2006-01-02 15:04:05"),
		categories,
	)

	// Strip the markdown H1 if present — it's redundant with the title tag.
	body := strings.TrimSpace(content)
	if strings.HasPrefix(body, "# ") {
		body = strings.TrimSpace(body[strings.Index(body, "\n"):])
	}

	return fm + body
}

// ── helpers ──────────────────────────────────────────────────────────────────

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(title)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "post"
	}
	return s
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if strings.HasPrefix(line, "title:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		}
	}
	return "Herb Hub Update"
}

func defaultSystemPrompt(cfg config.BlogConfig) string {
	return fmt.Sprintf(
		"You are a helpful gardening writer for %s (%s). "+
			"Write engaging, informative blog posts about %s. "+
			"Use Markdown formatting. Start with a level-1 heading as the title. "+
			"Keep posts between 300 and 600 words. Do not include front matter.",
		cfg.SiteName, cfg.SiteURL, cfg.PlantName,
	)
}
