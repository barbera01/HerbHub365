package api

import (
	"net/http"
	"strings"

	"HerbHub365/services/herbhub-manager/internal/auth"
	"HerbHub365/services/herbhub-manager/internal/config"
)

func withMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return withSecurityHeaders(withCORS(cfg.AllowedOrigin, next))
}

func withCORS(allowedOrigin string, next http.Handler) http.Handler {
	allowedOrigin = strings.TrimSpace(allowedOrigin)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if allowedOrigin != "" && origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}

		if r.Method == http.MethodOptions {
			if allowedOrigin != "" && origin == allowedOrigin {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func withAuth(cfg config.AuthConfig, verifier *auth.Verifier, next http.Handler) http.Handler {
	authDisabled := cfg.Disabled

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authDisabled {
			next.ServeHTTP(w, r)
			return
		}

		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		token := strings.TrimSpace(header[len("Bearer "):])
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		_, err := verifier.Verify(r.Context(), token)
		if err == auth.ErrForbidden {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}
