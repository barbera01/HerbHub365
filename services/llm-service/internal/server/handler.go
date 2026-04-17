package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"HerbHub365/services/llm-service/internal/llm"
)

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

	log.Printf("generate start (prompt_len=%d)", len(req.UserPrompt))

	content, err := h.client.Generate(ctx, req.SystemPrompt, req.UserPrompt)
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
