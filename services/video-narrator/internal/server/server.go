// Package server provides an HTTP API for the video-narrator pipeline.
// It allows external clients (e.g. herbhub-video) to submit generation
// requests and track progress through the full pipeline: TTS preprocessing →
// MuseTalk API → ffmpeg concat with chroma-key stitching.
package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"HerbHub365/services/video-narrator/internal/config"
	"HerbHub365/services/video-narrator/internal/preprocess"
	"HerbHub365/services/video-narrator/internal/video"
)

// Server is the HTTP server for the video-narrator API.
type Server struct {
	cfg                 config.Config
	ruleSet             *preprocess.RuleSet
	videoClient         *video.Client
	jobManager          *JobManager
	timelapseJobManager *TimelapseJobManager
	httpServer          *http.Server
}

// New creates a new Server with the given configuration.
func New(cfg config.Config, rs *preprocess.RuleSet, vc *video.Client) *Server {
	s := &Server{
		cfg:                 cfg,
		ruleSet:             rs,
		videoClient:         vc,
		jobManager:          NewJobManager(),
		timelapseJobManager: NewTimelapseJobManager(),
	}

	mux := http.NewServeMux()
	h := &handlers{server: s}

	mux.HandleFunc("/api/generate", h.handleGenerate)
	mux.HandleFunc("/api/jobs", h.handleJobs)
	mux.HandleFunc("/api/jobs/", h.handleJobByID)
	mux.HandleFunc("/api/posts", h.handlePosts)
	mux.HandleFunc("/api/posts/", h.handlePostBySlug)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/resources", h.handleResources)
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/api/timelapse/narrate", h.handleTimelapseNarrate)
	mux.HandleFunc("/api/timelapse/narrate/", h.handleTimelapseNarrateJob)

	s.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      withCORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // long-running responses for large downloads
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// ListenAndServe starts the HTTP server, blocking until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Start server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("video-narrator server listening on %s", s.cfg.Server.ListenAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// withCORS wraps a handler with permissive CORS headers for development.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
