package api

import (
	"net/http"
	"strings"
	"time"

	"HerbHub365/services/herbhub-manager/internal/auth"

	"HerbHub365/services/herbhub-manager/internal/blogpost"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/publisher"
	"HerbHub365/services/herbhub-manager/internal/queue"
	"HerbHub365/services/herbhub-manager/internal/timelapse"
	"HerbHub365/services/herbhub-manager/internal/video"
)

// NewRouter builds the HTTP mux with API routes only.
func NewRouter(cfg config.Config, verifier *auth.Verifier, videoClient *video.Client, blogClient *blogpost.Client, timelapseClient *timelapse.Client, pubClient *publisher.Client, queueManager *queue.Manager) http.Handler {
	root := http.NewServeMux()
	apiMux := http.NewServeMux()
	h := &handlers{cfg: cfg, videoClient: videoClient, blogClient: blogClient, timelapseClient: timelapseClient, pubClient: pubClient, queueManager: queueManager}

	root.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "timestamp": time.Now().UTC().Format(time.RFC3339)})
	})

	// Video API routes.
	apiMux.HandleFunc("/api/posts", h.handlePosts)
	apiMux.HandleFunc("/api/posts/", h.handlePostBySlug)
	apiMux.HandleFunc("/api/generate", h.handleGenerate)
	apiMux.HandleFunc("/api/jobs", h.handleJobs)
	apiMux.HandleFunc("/api/jobs/", h.handleJobByID)
	apiMux.HandleFunc("/api/videos", h.handleVideos)
	apiMux.HandleFunc("/api/videos/", h.handleVideoFile)
	apiMux.HandleFunc("/api/config", h.handleConfig)
	apiMux.HandleFunc("/api/resources", h.handleResources)
	apiMux.HandleFunc("/api/health", h.handleHealth)

	// YouTube publish route.
	apiMux.HandleFunc("/api/publish", h.handlePublish)

	// Sequential generation queue.
	apiMux.HandleFunc("/api/queue", h.handleQueue)
	apiMux.HandleFunc("/api/queue/", h.handleQueueCancel)

	// Blog poster API routes.
	apiMux.HandleFunc("/api/blog/generate", h.handleBlogGenerate)
	apiMux.HandleFunc("/api/blog/save", h.handleBlogSave)
	apiMux.HandleFunc("/api/blog/config", h.handleBlogConfig)

	// Timelapse publish routes (must be registered before the catch-all proxy).
	apiMux.HandleFunc("/api/timelapse/publish", h.handleTimelapsePublish)
	apiMux.HandleFunc("/api/timelapse/narrate/", h.handleTimelapseNarrateJob)

	// Timelapse proxy routes.
	apiMux.HandleFunc("/api/timelapse/", h.handleTimelapseProxy)
	apiMux.HandleFunc("/api/timelapse/build", h.handleTimelapseProxy)
	apiMux.HandleFunc("/api/timelapse/jobs", h.handleTimelapseProxy)
	apiMux.HandleFunc("/api/timelapse/videos", h.handleTimelapseProxy)
	apiMux.HandleFunc("/api/timelapse/config", h.handleTimelapseProxy)
	apiMux.HandleFunc("/api/timelapse/health", h.handleTimelapseProxy)
	apiMux.HandleFunc("/internal/timelapse/videos/", h.handleInternalTimelapseVideo)

	authenticated := withAuth(cfg.Auth, verifier, apiMux)
	root.Handle("/api/", authenticated)
	root.Handle("/internal/", apiMux)
	root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
	})

	return withMiddleware(cfg, root)
}
