package api

import (
	"net/http"

	"HerbHub365/services/herbhub-video/internal/config"
	"HerbHub365/services/herbhub-video/internal/video"
)

// NewRouter builds the HTTP mux with all API routes and static file serving.
func NewRouter(cfg config.Config, videoClient *video.Client, webFS http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{cfg: cfg, videoClient: videoClient}

	// API routes.
	mux.HandleFunc("/api/posts", h.handlePosts)
	mux.HandleFunc("/api/posts/", h.handlePostBySlug)
	mux.HandleFunc("/api/generate", h.handleGenerate)
	mux.HandleFunc("/api/jobs", h.handleJobs)
	mux.HandleFunc("/api/jobs/", h.handleJobByID)
	mux.HandleFunc("/api/videos", h.handleVideos)
	mux.HandleFunc("/api/videos/", h.handleVideoFile)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/resources", h.handleResources)
	mux.HandleFunc("/api/health", h.handleHealth)

	// Static frontend files — serve index.html for all non-API routes (SPA).
	fileServer := http.FileServer(webFS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly first.
		if r.URL.Path != "/" {
			// Check if the file exists in the embedded FS.
			f, err := webFS.Open(r.URL.Path)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Fall back to index.html for SPA routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return withCORS(mux)
}

// withCORS wraps a handler with permissive CORS headers for development.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
