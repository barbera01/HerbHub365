package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"HerbHub365/services/video-narrator/internal/config"
)

type Client struct {
	cfg        config.TTSConfig
	httpClient *http.Client
}

func NewClient(cfg config.TTSConfig, timeout time.Duration) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: timeout},
	}
}

type request struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	Speed          float64 `json:"speed"`
	ResponseFormat string  `json:"response_format"`
}

func (c *Client) Speak(ctx context.Context, text string) ([]byte, error) {
	voice := ResolveVoice(c.cfg.Voice, c.cfg.Speed)
	body := request{
		Model:          c.cfg.Model,
		Input:          text,
		Voice:          voice.VoiceString,
		Speed:          voice.Speed,
		ResponseFormat: c.cfg.ResponseFormat,
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal TTS request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build TTS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TTS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("TTS API returned %d: %s", resp.StatusCode, string(snippet))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TTS response: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("TTS API returned empty audio body")
	}
	return audio, nil
}
