package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"HerbHub365/services/video-narrator/internal/post"
)

type handlers struct {
	server *Server
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

// ── POST /api/generate ────────────────────────────────────────────────────────

func (h *handlers) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.Slug == "" && req.Text == "" {
		writeError(w, http.StatusBadRequest, "slug or text is required")
		return
	}

	// Resolve the post content.
	var postContent string
	if req.Text != "" {
		postContent = req.Text
	} else {
		p, err := findPostBySlug(h.server.cfg.Post.PostsDir, req.Slug)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("post not found: %v", err))
			return
		}
		postContent = p.RawContent
	}

	jobID, err := h.server.jobManager.SubmitJob(
		h.server.cfg,
		h.server.ruleSet,
		h.server.videoClient,
		req,
		postContent,
	)
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

	jobs := h.server.jobManager.Jobs()
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

	job, ok := h.server.jobManager.Job(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// ── GET /api/posts ────────────────────────────────────────────────────────────

type postListItem struct {
	Slug     string `json:"slug"`
	Date     string `json:"date"`
	Filename string `json:"filename"`
}

func (h *handlers) handlePosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	posts, err := post.FindAllPosts(h.server.cfg.Post.PostsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read posts: %v", err))
		return
	}

	items := make([]postListItem, 0, len(posts))
	for _, p := range posts {
		items = append(items, postListItem{
			Slug:     p.Slug,
			Date:     p.Date.Format("2006-01-02"),
			Filename: filepath.Base(p.Path),
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

	p, err := findPostBySlug(h.server.cfg.Post.PostsDir, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":     p.Slug,
		"date":     p.Date.Format("2006-01-02"),
		"filename": filepath.Base(p.Path),
		"content":  p.RawContent,
	})
}

// ── GET /api/config ───────────────────────────────────────────────────────────

func (h *handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg := h.server.cfg

	writeJSON(w, http.StatusOK, map[string]any{
		"musetalk_url":       cfg.Video.BaseURL,
		"default_avatar":     cfg.Video.AvatarID,
		"avatars":            cfg.Video.AvatarIDs,
		"posts_dir":          cfg.Post.PostsDir,
		"output_dir":         cfg.Concat.OutputDir,
		"concat_enabled":     cfg.Concat.Enabled,
		"chroma_key_enabled": cfg.Concat.ChromaKey.Enabled,
		"poll_interval":      cfg.Video.PollInterval.String(),
		"max_wait":           cfg.Video.MaxWait.String(),
	})
}

// ── GET /api/resources ────────────────────────────────────────────────────────
// Returns available intros, outros, and backgrounds.

func (h *handlers) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg := h.server.cfg

	intros := listMP4s(cfg.Server.ResourcesDir, "Intro")
	outros := listMP4s(cfg.Server.ResourcesDir, "Outro")
	backgrounds := listImages(cfg.Server.BGDir)

	writeJSON(w, http.StatusOK, map[string]any{
		"intros":      intros,
		"outros":      outros,
		"backgrounds": backgrounds,
	})
}

// ── GET /api/health ───────────────────────────────────────────────────────────

func (h *handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// listMP4s lists .mp4 files in dir whose name contains the given substring.
func listMP4s(dir, nameFilter string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mp4") {
			continue
		}
		if nameFilter != "" && !strings.Contains(name, nameFilter) {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// listImages lists image files in dir.
func listImages(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	imageExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".bmp": true, ".webp": true,
	}
	var result []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if imageExts[ext] {
			result = append(result, e.Name())
		}
	}
	sort.Strings(result)
	return result
}
