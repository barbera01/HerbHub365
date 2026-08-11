package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"HerbHub365/services/herbhub-manager/internal/autowatering"
)

func (h *handlers) handleAutomaticWatering(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	subject := authSubjectFromContext(r.Context())
	if subject == "" {
		subject = "anonymous"
	}
	outcome := "failed"
	defer func() {
		log.Printf("autowatering audit: action=api subject=%s method=%s outcome=%s duration_ms=%d", subject, r.Method, outcome, time.Since(start).Milliseconds())
	}()

	if h.autoManager == nil {
		outcome = "unavailable"
		writeError(w, http.StatusServiceUnavailable, "automatic watering unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		view := h.autoManager.View()
		w.Header().Set("ETag", autowatering.ETagForRevision(view.ConfigRevision))
		outcome = "success"
		writeJSON(w, http.StatusOK, view)
	case http.MethodPut:
		if h.autoManager.Unsafe() {
			outcome = "unsafe"
			writeError(w, http.StatusServiceUnavailable, "automatic watering store faulted")
			return
		}
		rev, err := autowatering.ParseIfMatchRevision(r.Header.Get("If-Match"))
		if err != nil {
			if autowatering.IsMissingIfMatch(err) {
				outcome = "missing_if_match"
				writeError(w, http.StatusPreconditionRequired, "If-Match revision header required")
				return
			}
			outcome = "invalid_if_match"
			writeError(w, http.StatusBadRequest, "invalid If-Match revision")
			return
		}
		var req struct {
			Config        autowatering.Config `json:"config"`
			ConfirmEnable bool                `json:"confirm_enable"`
		}
		if err := decodeStrictJSONWithLimitAndNoDuplicates(r.Body, &req, 64*1024); err != nil {
			outcome = "validation"
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := autowatering.ValidateConfig(req.Config); err != nil {
			outcome = "validation"
			writeError(w, http.StatusBadRequest, sanitizeAPIError(err))
			return
		}
		view, changed, err := h.autoManager.ReplaceConfig(rev, req.Config, subject, req.ConfirmEnable)
		if err != nil {
			switch {
			case errors.Is(err, autowatering.ErrRevisionMismatch):
				outcome = "stale_revision"
				writeError(w, http.StatusPreconditionFailed, "stale config revision")
			case errors.Is(err, autowatering.ErrStoreFaulted):
				outcome = "faulted"
				writeError(w, http.StatusServiceUnavailable, "automatic watering store faulted")
			default:
				outcome = "validation"
				writeError(w, http.StatusBadRequest, sanitizeAPIError(err))
			}
			return
		}
		w.Header().Set("ETag", autowatering.ETagForRevision(view.ConfigRevision))
		log.Printf("autowatering audit: action=config-update subject=%s from_revision=%d to_revision=%d changed=%v", subject, rev, view.ConfigRevision, changed)
		outcome = "success"
		writeJSON(w, http.StatusOK, view)
	default:
		outcome = "method_not_allowed"
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func sanitizeAPIError(err error) string {
	if err == nil {
		return "invalid request"
	}
	msg := err.Error()
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return fmt.Sprintf("invalid request: %s", msg)
}
