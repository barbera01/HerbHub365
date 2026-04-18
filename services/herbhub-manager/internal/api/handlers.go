package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"HerbHub365/services/herbhub-manager/internal/blogpost"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/post"
	"HerbHub365/services/herbhub-manager/internal/timelapse"
	"HerbHub365/services/herbhub-manager/internal/video"
)

type handlers struct {
	cfg              config.Config
	videoClient      *video.Client
	blogClient       *blogpost.Client
	timelapseClient  *timelapse.Client
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
// Post content is resolved locally and sent inline so that the narrator
// does not need access to the posts directory (supports remote/GPU deployments).

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

	// Resolve post content locally so the remote narrator never needs the
	// posts directory. Skip if the caller already supplied text.
	if req.Text == "" && req.Slug != "" {
		posts, err := post.FindAllPosts(h.cfg.Post.PostsDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("read posts: %v", err))
			return
		}
		for _, p := range posts {
			if p.Slug == req.Slug || strings.Contains(p.Slug, req.Slug) {
				req.Text = p.RawContent
				break
			}
		}
		if req.Text == "" {
			writeError(w, http.StatusNotFound, fmt.Sprintf("post not found for slug %q", req.Slug))
			return
		}
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

// ── POST /api/blog/generate ───────────────────────────────────────────────────
// Calls llm-service to generate blog post content and returns it for preview.

type blogGenerateRequest struct {
	UserPrompt   string `json:"user_prompt"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Categories   string `json:"categories,omitempty"`
}

type blogGenerateResponse struct {
	Content  string `json:"content"`
	Filename string `json:"filename"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
}

func (h *handlers) handleBlogGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req blogGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if strings.TrimSpace(req.UserPrompt) == "" {
		writeError(w, http.StatusBadRequest, "user_prompt is required")
		return
	}

	log.Printf("blog generate start (prompt_len=%d)", len(req.UserPrompt))

	content, err := h.blogClient.Generate(req.SystemPrompt, req.UserPrompt)
	if err != nil {
		log.Printf("blog generate error: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("generate: %v", err))
		return
	}

	meta := blogpost.DeriveFilename(content, time.Now().UTC())
	content = blogpost.InjectFrontMatter(content, meta, req.Categories)

	log.Printf("blog generate ok (filename=%s)", meta.Filename)
	writeJSON(w, http.StatusOK, blogGenerateResponse{
		Content:  content,
		Filename: meta.Filename,
		Slug:     meta.Slug,
		Title:    meta.Title,
	})
}

// ── POST /api/blog/save ───────────────────────────────────────────────────────
// Saves provided content as a Jekyll post file in the posts directory.

type blogSaveRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func (h *handlers) handleBlogSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req blogSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if strings.TrimSpace(req.Filename) == "" || strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "filename and content are required")
		return
	}

	// Safety: only allow simple Jekyll-style filenames, no path traversal.
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "..") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	if !strings.HasSuffix(req.Filename, ".markdown") && !strings.HasSuffix(req.Filename, ".md") {
		writeError(w, http.StatusBadRequest, "filename must end in .markdown or .md")
		return
	}

	dest := filepath.Join(h.cfg.Post.PostsDir, req.Filename)
	if err := os.WriteFile(dest, []byte(req.Content), 0644); err != nil {
		log.Printf("blog save error: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save post: %v", err))
		return
	}

	log.Printf("blog post saved: %s", dest)
	writeJSON(w, http.StatusOK, map[string]string{
		"filename": req.Filename,
		"path":     dest,
	})
}

// ── GET /api/blog/config ──────────────────────────────────────────────────────
// Returns blog generation config so the frontend can pre-fill prompts.

func (h *handlers) handleBlogConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"llm_service_url": h.cfg.Blog.LLMServiceURL,
		"site_name":       h.cfg.Blog.SiteName,
		"site_url":        h.cfg.Blog.SiteURL,
		"plant_name":      h.cfg.Blog.PlantName,
		"system_prompt":   h.cfg.Blog.SystemPrompt,
	})
}

// ── Timelapse proxy handlers ──────────────────────────────────────────────────
// These are thin proxies to the timelapse-builder HTTP API.

func (h *handlers) handleTimelapseProxy(w http.ResponseWriter, r *http.Request) {
	if h.timelapseClient == nil {
		writeError(w, http.StatusServiceUnavailable, "timelapse-builder not configured")
		return
	}

	// Route to the appropriate client method.
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/timelapse/build":
		var req timelapse.BuildRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		jobID, err := h.timelapseClient.Build(req)
		if err != nil {
			log.Printf("timelapse build error: %v", err)
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "queued"})

	case r.Method == http.MethodGet && r.URL.Path == "/api/timelapse/jobs":
		raw, err := h.timelapseClient.Jobs()
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/timelapse/jobs/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/timelapse/jobs/")
		raw, err := h.timelapseClient.Job(id)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/timelapse/videos/"):
		filename := strings.TrimPrefix(r.URL.Path, "/api/timelapse/videos/")
		if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}
		if err := h.timelapseClient.ProxyVideoFile(w, filename); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
		}

	case r.Method == http.MethodGet && r.URL.Path == "/api/timelapse/videos":
		raw, err := h.timelapseClient.Videos()
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)

	case r.Method == http.MethodGet && r.URL.Path == "/api/timelapse/config":
		raw, err := h.timelapseClient.Config()
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"online": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)

	case r.Method == http.MethodGet && r.URL.Path == "/api/timelapse/health":
		online := h.timelapseClient.Health()
		writeJSON(w, http.StatusOK, map[string]any{"online": online})

	default:
		writeError(w, http.StatusNotFound, "unknown timelapse route")
	}
}

// ── POST /api/timelapse/publish ───────────────────────────────────────────────

type timelapsePublishRequest struct {
	TimelapseFile string `json:"timelapse_file"` // filename of the built timelapse MP4
	TTSText       string `json:"tts_text"`       // narration script
	Title         string `json:"title"`          // YouTube / blog post title
	FromDate      string `json:"from_date"`      // YYYY-MM-DD, for blog post body
	ToDate        string `json:"to_date"`        // YYYY-MM-DD, used for slug + post date
	Intro         string `json:"intro"`          // intro filename
	Outro         string `json:"outro"`          // outro filename
}

func (h *handlers) handleTimelapsePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req timelapsePublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if strings.TrimSpace(req.TimelapseFile) == "" {
		writeError(w, http.StatusBadRequest, "timelapse_file is required")
		return
	}
	if strings.TrimSpace(req.TTSText) == "" {
		writeError(w, http.StatusBadRequest, "tts_text is required")
		return
	}
	if strings.TrimSpace(req.ToDate) == "" {
		writeError(w, http.StatusBadRequest, "to_date is required")
		return
	}

	slug := "timelapse-" + req.ToDate

	if err := h.createTimelapsePost(req, slug); err != nil {
		log.Printf("timelapse publish: create post failed (non-fatal): %v", err)
	}

	jobID, err := h.videoClient.NarrateTimelapse(video.TimelapseNarrateRequest{
		Text:          req.TTSText,
		TimelapseFile: req.TimelapseFile,
		Intro:         req.Intro,
		Outro:         req.Outro,
		Slug:          slug,
		Date:          req.ToDate,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("narrate timelapse: %v", err))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id": jobID,
		"slug":   slug,
		"phase":  "queued",
	})
}

func (h *handlers) createTimelapsePost(req timelapsePublishRequest, slug string) error {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Timelapse " + req.ToDate
	}

	datePrefix := req.ToDate

	var body strings.Builder
	if req.FromDate != "" {
		fmt.Fprintf(&body, "Timelapse from %s to %s.\n\n", req.FromDate, req.ToDate)
	}
	body.WriteString(req.TTSText)

	content := fmt.Sprintf("---\nlayout: post\ntitle: %q\ndate: %s 12:00:00 +0000\ncategories: [timelapse]\ntags: [timelapse, herbhub365]\n---\n\n%s\n",
		title, datePrefix, body.String())

	filename := datePrefix + "-" + slug + ".markdown"
	dest := filepath.Join(h.cfg.Post.PostsDir, filename)
	return os.WriteFile(dest, []byte(content), 0644)
}

// ── GET /api/timelapse/narrate/{id} ──────────────────────────────────────────

func (h *handlers) handleTimelapseNarrateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/timelapse/narrate/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	job, err := h.videoClient.GetTimelapseNarrateJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// publishCompletedJobs emits RabbitMQ messages for completed jobs.
