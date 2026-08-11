package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"HerbHub365/services/herbhub-manager/internal/messaging"
)

const maxMessagingPublishBodyBytes = 70 * 1024

func (h *handlers) handleMessagingOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.messagingSvc == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":       false,
			"broker_status": "disabled",
			"catalogues":    []any{},
			"templates":     []any{},
		})
		return
	}

	overview := h.messagingSvc.Overview(r.Context())
	writeJSON(w, http.StatusOK, overview)
}

func (h *handlers) handleMessagingProvision(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	subject := authSubjectFromContext(r.Context())
	outcome := "failed"
	if r.Method != http.MethodPost {
		outcome = "method_not_allowed"
		defer func() {
			log.Printf("messaging audit: action=provision subject=%s catalogue=%s outcome=%s duration_ms=%d", subject, "", outcome, time.Since(start).Milliseconds())
		}()
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.messagingSvc == nil {
		defer func() {
			log.Printf("messaging audit: action=provision subject=%s catalogue=%s outcome=%s duration_ms=%d", subject, "", "disabled", time.Since(start).Milliseconds())
		}()
		writeError(w, http.StatusServiceUnavailable, "messaging disabled")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/messaging/catalogues/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "provision" || strings.TrimSpace(parts[0]) == "" {
		defer func() {
			log.Printf("messaging audit: action=provision subject=%s catalogue=%s outcome=%s duration_ms=%d", subject, "", "not_found", time.Since(start).Milliseconds())
		}()
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	catalogueID := parts[0]
	defer func() {
		log.Printf("messaging audit: action=provision subject=%s catalogue=%s outcome=%s duration_ms=%d", subject, catalogueID, outcome, time.Since(start).Milliseconds())
	}()

	status, err := h.messagingSvc.ProvisionCatalogue(r.Context(), catalogueID)
	if err != nil {
		switch {
		case errors.Is(err, messaging.ErrUnknownCatalogue):
			outcome = "unknown"
			writeError(w, http.StatusNotFound, "unknown catalogue")
		case errors.Is(err, messaging.ErrDrift):
			outcome = "drifted"
			writeJSON(w, http.StatusConflict, map[string]any{"error": "catalogue drift detected", "status": status})
		case errors.Is(err, messaging.ErrTopologyNotReady):
			outcome = "not_ready"
			writeJSON(w, http.StatusConflict, map[string]any{"error": "catalogue not ready", "status": status})
		case errors.Is(err, messaging.ErrDisabled), errors.Is(err, messaging.ErrUnavailable):
			outcome = "unavailable"
			writeError(w, http.StatusServiceUnavailable, "messaging unavailable")
		default:
			var drift *messaging.DriftError
			if errors.As(err, &drift) {
				outcome = "drifted"
				writeJSON(w, http.StatusConflict, map[string]any{"error": "catalogue drift detected", "status": status})
				return
			}
			outcome = "unavailable"
			writeError(w, http.StatusServiceUnavailable, "messaging unavailable")
		}
		return
	}

	outcome = "success"
	writeJSON(w, http.StatusOK, status)
}

func (h *handlers) handleMessagingTemplatePublish(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	subject := authSubjectFromContext(r.Context())
	msgID := ""
	route := ""
	outcome := "failed"
	templateID := ""
	defer func() {
		log.Printf("messaging audit: action=publish subject=%s template=%s route=%s message_id=%s outcome=%s duration_ms=%d", subject, templateID, route, msgID, outcome, time.Since(start).Milliseconds())
	}()

	if r.Method != http.MethodPost {
		outcome = "method_not_allowed"
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.messagingSvc == nil {
		outcome = "disabled"
		writeError(w, http.StatusServiceUnavailable, "messaging disabled")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/messaging/templates/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "publish" || strings.TrimSpace(parts[0]) == "" {
		outcome = "not_found"
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	templateID = parts[0]

	var req messaging.PublishRequest
	if err := decodeStrictJSONWithLimit(r, &req, maxMessagingPublishBodyBytes); err != nil {
		outcome = "validation"
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	result, err := h.messagingSvc.PublishTemplate(r.Context(), templateID, req)
	if err != nil {
		switch {
		case errors.Is(err, messaging.ErrUnknownTemplate):
			outcome = "unknown"
			writeError(w, http.StatusNotFound, "unknown template")
		case errors.Is(err, messaging.ErrValidation):
			outcome = "validation"
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, messaging.ErrConfirmationNeeded):
			outcome = "confirmation_required"
			writeError(w, http.StatusConflict, "confirmed=true is required for physical watering")
		case errors.Is(err, messaging.ErrTopologyNotReady):
			outcome = "not_ready"
			writeError(w, http.StatusConflict, "catalogue topology is not ready")
		case errors.Is(err, messaging.ErrDrift):
			outcome = "drifted"
			writeError(w, http.StatusConflict, "catalogue topology is drifted")
		case errors.Is(err, messaging.ErrDisabled), errors.Is(err, messaging.ErrUnavailable):
			outcome = "unavailable"
			writeError(w, http.StatusServiceUnavailable, "messaging unavailable")
		default:
			var drift *messaging.DriftError
			if errors.As(err, &drift) {
				outcome = "drifted"
				writeError(w, http.StatusConflict, "catalogue topology is drifted")
				return
			}
			outcome = "unavailable"
			writeError(w, http.StatusServiceUnavailable, "messaging unavailable")
		}
		return
	}

	msgID = result.MessageID
	route = result.RoutingKey
	outcome = "success"
	writeJSON(w, http.StatusOK, result)
}

func decodeStrictJSONWithLimit(r *http.Request, dst any, limit int64) error {
	defer r.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(buf)) > limit {
		return fmt.Errorf("request body exceeds %d bytes", limit)
	}
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing data after JSON object")
	}
	return nil
}
