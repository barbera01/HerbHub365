package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"HerbHub365/services/timelapse-builder/internal/config"
	"HerbHub365/services/timelapse-builder/internal/job"
)

type handlers struct {
	cfg     config.Config
	tracker *job.Tracker
	sem     chan struct{} // only one build at a time
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── POST /api/build ───────────────────────────────────────────────────────────

type buildRequest struct {
	From          string  `json:"from"`
	To            string  `json:"to"`
	OutputName    string  `json:"output_name"`
	InputFPS      *int    `json:"input_fps"`
	OutputFPS     *int    `json:"output_fps"`
	CRF           *int    `json:"crf"`
	MinBrightness *float64 `json:"min_brightness"`
}

func (h *handlers) handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req buildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Reject if already building.
	select {
	case h.sem <- struct{}{}:
	default:
		writeError(w, http.StatusTooManyRequests, "a build is already in progress")
		return
	}

	p := job.Params{
		From:          req.From,
		To:            req.To,
		OutputName:    req.OutputName,
		InputFPS:      orInt(req.InputFPS, h.cfg.InputFPS),
		OutputFPS:     orInt(req.OutputFPS, h.cfg.OutputFPS),
		CRF:           orInt(req.CRF, h.cfg.CRF),
		MinBrightness: orFloat(req.MinBrightness, h.cfg.MinBrightness),
	}
	j := h.tracker.Create(p)

	go h.runBuild(j.ID, p)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id": j.ID,
		"status": string(job.StatusQueued),
	})
}

func (h *handlers) runBuild(jobID string, p job.Params) {
	defer func() { <-h.sem }()

	h.tracker.MarkRunning(jobID)
	log.Printf("timelapse build start (job=%s)", jobID[:8])

	outputName := p.OutputName
	if outputName == "" {
		outputName = fmt.Sprintf("timelapse-%s.mp4", time.Now().UTC().Format("20060102-150405"))
	}
	if err := os.MkdirAll(h.cfg.OutputDir, 0755); err != nil {
		log.Printf("timelapse: cannot create output dir: %v", err)
		h.tracker.MarkFailed(jobID, fmt.Sprintf("create output dir: %v", err), "")
		return
	}

	outputPath := filepath.Join(h.cfg.OutputDir, outputName)

	scriptArgs := []string{"/usr/local/bin/make-timelapse.sh"}
	if p.From != "" {
		scriptArgs = append(scriptArgs, "--from", p.From)
	}
	if p.To != "" {
		scriptArgs = append(scriptArgs, "--to", p.To)
	}
	scriptArgs = append(scriptArgs, h.cfg.InputDir, outputPath)

	log.Printf("timelapse exec: bash %v", scriptArgs)

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.BuildTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", scriptArgs...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("INPUT_FPS=%d", p.InputFPS),
		fmt.Sprintf("OUTPUT_FPS=%d", p.OutputFPS),
		fmt.Sprintf("CRF=%d", p.CRF),
		fmt.Sprintf("MIN_BRIGHTNESS=%g", p.MinBrightness),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	err := cmd.Run()
	logOutput := stdout.String() + stderr.String()

	if err != nil {
		log.Printf("timelapse build failed (job=%s): %v", jobID[:8], err)
		h.tracker.MarkFailed(jobID, err.Error(), logOutput)
		return
	}

	log.Printf("timelapse build ok (job=%s output=%s)", jobID[:8], outputName)
	h.tracker.MarkCompleted(jobID, outputName, logOutput)
}

// ── GET /api/jobs ─────────────────────────────────────────────────────────────

func (h *handlers) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jobs := h.tracker.All()
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "total": len(jobs)})
}

// ── GET /api/jobs/{id} ────────────────────────────────────────────────────────

func (h *handlers) handleJobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "job id required")
		return
	}
	j, ok := h.tracker.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// ── GET /api/videos ───────────────────────────────────────────────────────────

type videoFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	SizeMB   string `json:"size_mb"`
	Modified string `json:"modified"`
}

func (h *handlers) handleVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	entries, err := os.ReadDir(h.cfg.OutputDir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []videoFile{}, "total": 0})
		return
	}
	var videos []videoFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		videos = append(videos, videoFile{
			Name:     e.Name(),
			Size:     info.Size(),
			SizeMB:   fmt.Sprintf("%.1f MB", float64(info.Size())/(1024*1024)),
			Modified: info.ModTime().UTC().Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(videos, func(i, j int) bool { return videos[i].Modified > videos[j].Modified })
	writeJSON(w, http.StatusOK, map[string]any{"videos": videos, "total": len(videos)})
}

// ── GET /api/videos/{filename} ────────────────────────────────────────────────

func (h *handlers) handleVideoFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filename := strings.TrimPrefix(r.URL.Path, "/api/videos/")
	filename = strings.TrimSuffix(filename, "/")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".mp4") {
		writeError(w, http.StatusBadRequest, "only .mp4 files can be served")
		return
	}
	path := filepath.Join(h.cfg.OutputDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "video not found")
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	http.ServeFile(w, r, path)
}

// ── GET /api/config ───────────────────────────────────────────────────────────

func (h *handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"input_dir":      h.cfg.InputDir,
		"output_dir":     h.cfg.OutputDir,
		"input_fps":      h.cfg.InputFPS,
		"output_fps":     h.cfg.OutputFPS,
		"crf":            h.cfg.CRF,
		"min_brightness": h.cfg.MinBrightness,
		"build_timeout":  h.cfg.BuildTimeout.String(),
	})
}

// ── GET /api/health ───────────────────────────────────────────────────────────

func (h *handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	busy := len(h.sem) > 0
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "building": busy})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func orInt(v *int, fallback int) int {
	if v != nil {
		return *v
	}
	return fallback
}

func orFloat(v *float64, fallback float64) float64 {
	if v != nil {
		return *v
	}
	return fallback
}
