package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// imageTimestamp tries to extract a timestamp from the filename (YYYYMMDD_HHMMSS)
// and falls back to the file's modification time.
var filenameTS = regexp.MustCompile(`(\d{8})_(\d{6})`)

func imageTimestamp(path string, info os.FileInfo) time.Time {
	base := filepath.Base(path)
	if m := filenameTS.FindStringSubmatch(base); m != nil {
		t, err := time.ParseInLocation("20060102_150405", m[1]+"_"+m[2], time.Local)
		if err == nil {
			return t
		}
	}
	return info.ModTime()
}

func (h *handlers) runBuild(jobID string, p job.Params) {
	defer func() { <-h.sem }()

	h.tracker.MarkRunning(jobID)
	log.Printf("timelapse build start (job=%s)", jobID[:8])

	if err := os.MkdirAll(h.cfg.OutputDir, 0755); err != nil {
		msg := fmt.Sprintf("create output dir: %v", err)
		log.Printf("timelapse (job=%s): %s", jobID[:8], msg)
		h.tracker.MarkFailed(jobID, msg, "")
		return
	}

	// Parse optional time range.
	var fromT, toT time.Time
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	parseTime := func(s string) (time.Time, error) {
		for _, l := range layouts {
			if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot parse time %q", s)
	}
	if p.From != "" {
		t, err := parseTime(p.From)
		if err != nil {
			h.tracker.MarkFailed(jobID, err.Error(), "")
			return
		}
		fromT = t
	}
	if p.To != "" {
		t, err := parseTime(p.To)
		if err != nil {
			h.tracker.MarkFailed(jobID, err.Error(), "")
			return
		}
		toT = t
	}

	// Collect and sort image files.
	var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	type frame struct {
		path string
		ts   time.Time
	}
	var frames []frame

	err := filepath.Walk(h.cfg.InputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !imageExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		ts := imageTimestamp(path, info)
		if !fromT.IsZero() && ts.Before(fromT) {
			return nil
		}
		if !toT.IsZero() && ts.After(toT) {
			return nil
		}
		frames = append(frames, frame{path: path, ts: ts})
		return nil
	})
	if err != nil {
		msg := fmt.Sprintf("walk input dir: %v", err)
		log.Printf("timelapse (job=%s): %s", jobID[:8], msg)
		h.tracker.MarkFailed(jobID, msg, "")
		return
	}

	if len(frames) == 0 {
		msg := fmt.Sprintf("no images found in %s (range: %q – %q)", h.cfg.InputDir, p.From, p.To)
		log.Printf("timelapse (job=%s): %s", jobID[:8], msg)
		h.tracker.MarkFailed(jobID, msg, "")
		return
	}

	sort.Slice(frames, func(i, j int) bool { return frames[i].ts.Before(frames[j].ts) })
	log.Printf("timelapse (job=%s): %d frames selected", jobID[:8], len(frames))

	// Write ffmpeg concat list to a temp file.
	tmp, err := os.CreateTemp("", "timelapse-*.txt")
	if err != nil {
		h.tracker.MarkFailed(jobID, fmt.Sprintf("create temp file: %v", err), "")
		return
	}
	defer os.Remove(tmp.Name())

	frameDuration := 1.0 / float64(p.InputFPS)
	for _, f := range frames {
		fmt.Fprintf(tmp, "file '%s'\nduration %f\n", f.path, frameDuration)
	}
	// ffmpeg concat demuxer requires the last file listed twice.
	fmt.Fprintf(tmp, "file '%s'\n", frames[len(frames)-1].path)
	tmp.Close()

	outputName, err := sanitizeOutputName(p.OutputName)
	if err != nil {
		h.tracker.MarkFailed(jobID, err.Error(), "")
		return
	}
	outputPath := filepath.Join(h.cfg.OutputDir, outputName)

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.BuildTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "concat", "-safe", "0", "-i", tmp.Name(),
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p",
		"-r", fmt.Sprintf("%d", p.OutputFPS),
		"-c:v", "libx264",
		"-crf", fmt.Sprintf("%d", p.CRF),
		"-movflags", "+faststart",
		outputPath,
	)

	log.Printf("timelapse (job=%s): running ffmpeg → %s", jobID[:8], outputName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("timelapse (job=%s): ffmpeg failed: %v", jobID[:8], err)
		h.tracker.MarkFailed(jobID, fmt.Sprintf("ffmpeg: %v\n%s", err, trimOutput(string(out))), string(out))
		return
	}

	log.Printf("timelapse build ok (job=%s): %s", jobID[:8], outputName)
	h.tracker.MarkCompleted(jobID, outputName, "")
}

func sanitizeOutputName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("timelapse-%s.mp4", time.Now().UTC().Format("20060102-150405")), nil
	}

	cleaned := filepath.Clean(name)
	if cleaned == "." || cleaned == ".." || filepath.Base(cleaned) != cleaned {
		return "", fmt.Errorf("output_name must be a filename, got %q", name)
	}
	if filepath.Ext(cleaned) == "" {
		cleaned += ".mp4"
	}
	return cleaned, nil
}

func trimOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 1000 {
		return s
	}
	return "…" + s[len(s)-1000:]
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
