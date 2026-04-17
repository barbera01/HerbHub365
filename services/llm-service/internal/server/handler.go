package server

import (
	"context"
	"encoding/json"
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
}

func NewHandler(client *llm.Client, requestTimeout time.Duration) *Handler {
	return &Handler{client: client, requestTimeout: requestTimeout}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /generate", h.handleGenerate)
	mux.HandleFunc("POST /warm", h.handleWarm)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode request: "+err.Error())
		return
	}
	if req.SystemPrompt == "" || req.UserPrompt == "" {
		writeError(w, http.StatusBadRequest, "system_prompt and user_prompt are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	content, err := h.client.Generate(ctx, req.SystemPrompt, req.UserPrompt)
	if err != nil {
		log.Printf("generate error: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, generateResponse{Content: content})
}

func (h *Handler) handleWarm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	if err := h.client.WarmModel(ctx); err != nil {
		log.Printf("warm error: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
