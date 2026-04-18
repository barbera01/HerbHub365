package api

import (
	"net/http"

	"HerbHub365/services/timelapse-builder/internal/config"
	"HerbHub365/services/timelapse-builder/internal/job"
)

func NewRouter(cfg config.Config, tracker *job.Tracker) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{
		cfg:     cfg,
		tracker: tracker,
		sem:     make(chan struct{}, 1),
	}

	mux.HandleFunc("/api/build", h.handleBuild)
	mux.HandleFunc("/api/jobs", h.handleJobs)
	mux.HandleFunc("/api/jobs/", h.handleJobByID)
	mux.HandleFunc("/api/videos", h.handleVideos)
	mux.HandleFunc("/api/videos/", h.handleVideoFile)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/health", h.handleHealth)

	return mux
}
