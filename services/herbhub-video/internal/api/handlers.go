package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"HerbHub365/services/herbhub-video/internal/config"
	"HerbHub365/services/herbhub-video/internal/post"
	"HerbHub365/services/herbhub-video/internal/video"
)

type handlers struct {
	cfg         config.Config
	videoClient *video.Client
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── GET /api/posts ────────────────────────────────────────────────────────────

type postListItem struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	Excerpt    string `json:"excerpt"`
	Filename   string `json:"filename"`
	HasVideo   bool   `json:"has_video"`
	Published  bool   `json:"published"`
	VideoFile  string `json:"video_file,omitempty"`
	YouTubeURL string `json:"youtube_url,omitempty"`
}

func (h *handlers) handlePosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	posts, err := post.FindAllPosts(h.cfg.Post.PostsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read posts: %v", err))
		return
	}

	items := make([]postListItem, 0, len(posts))
	for _, p := range posts {
		status := p.OutputStatus(h.cfg.OutputDir)
		videoFile := ""
		if status.HasVideo {
			videoFile = status.Filename
		}
		items = append(items, postListItem{
			Slug:       p.Slug,
			Title:      p.Title,
			Date:       p.Date.Format("2006-01-02"),
			Excerpt:    p.Excerpt,
			Filename:   p.Filename,
			HasVideo:   status.HasVideo,
			Published:  status.IsPublished,
			VideoFile:  videoFile,
			YouTubeURL: status.YouTubeURL,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"posts": items,
		"total": len(items),
	})
}

// ── GET /api/posts/{slug} ─────────────────────────────────────────────────────

func (h *handlers) handlePostBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/api/posts/")
	slug = strings.TrimSuffix(slug, "/")

	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}

	posts, err := post.FindAllPosts(h.cfg.Post.PostsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read posts: %v", err))
		return
	}

	for _, p := range posts {
		if p.Slug == slug {
			status := p.OutputStatus(h.cfg.OutputDir)
			writeJSON(w, http.StatusOK, map[string]any{
				"slug":        p.Slug,
				"title":       p.Title,
				"date":        p.Date.Format("2006-01-02"),
				"excerpt":     p.Excerpt,
				"filename":    p.Filename,
				"has_video":   status.HasVideo,
				"published":   status.IsPublished,
				"youtube_url": status.YouTubeURL,
				"content":     p.RawContent,
			})
			return
		}
	}

	writeError(w, http.StatusNotFound, "post not found")
}

// ── POST /api/generate ────────────────────────────────────────────────────────
// Proxies the request to the video-narrator API.

func (h *handlers) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req video.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.Slug == "" && req.Text == "" {
		writeError(w, http.StatusBadRequest, "slug or text is required")
		return
	}

	jobID, err := h.videoClient.SubmitJob(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("submit job: %v", err))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id": jobID,
		"phase":  "queued",
	})
}

// ── GET /api/jobs ─────────────────────────────────────────────────────────────

func (h *handlers) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	jobs, err := h.videoClient.Jobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("fetch jobs: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":  jobs,
		"total": len(jobs),
	})
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
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	job, err := h.videoClient.Job(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, job)
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
		// Directory might not exist yet — return empty list.
		writeJSON(w, http.StatusOK, map[string]any{
			"videos": []videoFile{},
			"total":  0,
		})
		return
	}

	var videos []videoFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") {
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

	writeJSON(w, http.StatusOK, map[string]any{
		"videos": videos,
		"total":  len(videos),
	})
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

	if !strings.HasSuffix(filename, ".mp4") {
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
// Proxies config from the video-narrator API and adds local config.

func (h *handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg, err := h.videoClient.Config()
	if err != nil {
		// Fallback: return minimal local config.
		writeJSON(w, http.StatusOK, map[string]any{
			"narrator_url":    h.cfg.NarratorURL,
			"narrator_online": false,
			"posts_dir":       h.cfg.Post.PostsDir,
			"output_dir":      h.cfg.OutputDir,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"narrator_url":       h.cfg.NarratorURL,
		"narrator_online":    true,
		"musetalk_url":       cfg.MuseTalkURL,
		"default_avatar":     cfg.DefaultAvatar,
		"avatars":            cfg.Avatars,
		"posts_dir":          h.cfg.Post.PostsDir,
		"output_dir":         h.cfg.OutputDir,
		"concat_enabled":     cfg.ConcatEnabled,
		"chroma_key_enabled": cfg.ChromaKeyEnabled,
		"poll_interval":      cfg.PollInterval,
		"max_wait":           cfg.MaxWait,
	})
}

// ── GET /api/resources ────────────────────────────────────────────────────────
// Proxies resources list from the video-narrator API.

func (h *handlers) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	res, err := h.videoClient.Resources()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("fetch resources: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// ── GET /api/health ───────────────────────────────────────────────────────────

func (h *handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	narratorOK := h.videoClient.Health()
	status := "ok"
	if !narratorOK {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          status,
		"narrator_online": narratorOK,
	})
}

// publishCompletedJobs emits RabbitMQ messages for completed jobs.
