package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"HerbHub365/services/llm-service/internal/llm"
)

type generateRequest struct {
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	ImageData    string `json:"image_data,omitempty"`      // base64-encoded image bytes (with or without data URI prefix)
	ImageMime    string `json:"image_mime_type,omitempty"` // e.g. image/jpeg; optional if embedded in ImageData URI
}

type generateResponse struct {
	Content string `json:"content"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Handler holds the LLM client and serves HTTP requests.
type Handler struct {
	client         *llm.Client
	requestTimeout time.Duration
	sem            chan struct{} // limits concurrent generations
}

func NewHandler(client *llm.Client, requestTimeout time.Duration, maxConcurrent int) *Handler {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Handler{
		client:         client,
		requestTimeout: requestTimeout,
		sem:            make(chan struct{}, maxConcurrent),
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /generate", h.handleGenerate)
	mux.HandleFunc("POST /warm", h.handleWarm)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode request: "+err.Error())
		return
	}
	if req.SystemPrompt == "" || req.UserPrompt == "" {
		writeError(w, http.StatusBadRequest, "system_prompt and user_prompt are required")
		return
	}

	imageBytes, imageMime, err := decodeImageData(req.ImageData, req.ImageMime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "decode image: "+err.Error())
		return
	}
	if len(imageBytes) > 0 {
		log.Printf("generate start (prompt_len=%d image_bytes=%d mime=%s)", len(req.UserPrompt), len(imageBytes), imageMime)
	} else {
		log.Printf("generate start (prompt_len=%d)", len(req.UserPrompt))
	}

	// Reject immediately if already at capacity rather than queuing — the
	// caller (blog-poster) has its own retry logic and a long timeout, so
	// queuing here would just create a hidden backlog.
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		log.Printf("generate rejected: already at max concurrent generations")
		writeError(w, http.StatusTooManyRequests, "generation already in progress, try again shortly")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	content, err := h.client.Generate(ctx, req.SystemPrompt, req.UserPrompt, imageBytes, imageMime)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		status := statusForError(err)
		log.Printf("generate error after %s (status=%d): %v", elapsed, status, err)
		writeError(w, status, err.Error())
		return
	}

	log.Printf("generate ok in %s (content_len=%d)", elapsed, len(content))
	writeJSON(w, http.StatusOK, generateResponse{Content: content})
}

func (h *Handler) handleWarm(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	if err := h.client.WarmModel(ctx); err != nil {
		log.Printf("warm error after %s: %v", time.Since(start).Round(time.Millisecond), err)
		writeError(w, statusForError(err), err.Error())
		return
	}

	log.Printf("warm ok in %s", time.Since(start).Round(time.Millisecond))
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// statusForError maps an LLM error to the most appropriate HTTP status code.
// 503 = LLM host unreachable (caller can retry later)
// 504 = LLM took too long (context deadline / timeout)
// 500 = anything else (bad response, parse failure, etc.)
func statusForError(err error) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return http.StatusGatewayTimeout
	}
	if llm.IsAvailabilityError(err) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// decodeImageData extracts raw image bytes and MIME type from a client payload.
// It supports both raw base64 strings and data URI prefixes like "data:image/jpeg;base64,/9j/...".
func decodeImageData(imageData, imageMime string) ([]byte, string, error) {
	imageData = strings.TrimSpace(imageData)
	if imageData == "" {
		return nil, "", nil
	}

	if strings.HasPrefix(imageData, "data:") {
		idx := strings.Index(imageData, ",")
		if idx == -1 {
			return nil, "", errors.New("invalid data URI: missing comma separator")
		}
		meta := imageData[5:idx]
		data := imageData[idx+1:]
		// meta is "<mime>;base64" or just ";base64"
		parts := strings.SplitN(meta, ";", 2)
		if imageMime == "" && parts[0] != "" {
			imageMime = parts[0]
		}
		imageData = data
	}

	decoded, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return nil, "", fmt.Errorf("decode base64: %w", err)
	}
	if len(decoded) == 0 {
		return nil, "", nil
	}
	if imageMime == "" {
		imageMime = http.DetectContentType(decoded)
	}
	return decoded, imageMime, nil
}
