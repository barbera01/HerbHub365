package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"HerbHub365/services/video-narrator/internal/concat"
	"HerbHub365/services/video-narrator/internal/config"
	"HerbHub365/services/video-narrator/internal/post"
	"HerbHub365/services/video-narrator/internal/preprocess"
	"HerbHub365/services/video-narrator/internal/video"
)

// ── Job phases and their overall progress ranges ─────────────────────────────
//
//	queued         0.00
//	preprocessing  0.00 - 0.05
//	submitting     0.05 - 0.10
//	generating     0.10 - 0.70  (maps MuseTalk 0-1 to this range)
//	downloading    0.70 - 0.80
//	stitching      0.80 - 0.95
//	completed      1.00
//	failed         (retains last progress)

// Job represents a video generation job tracked by the server.
type Job struct {
	ID        string  `json:"id"`
	Slug      string  `json:"slug"`
	AvatarID  string  `json:"avatar_id"`
	Phase     string  `json:"phase"` // queued, preprocessing, submitting, generating, downloading, stitching, completed, failed
	Progress  float64 `json:"progress"`
	Error     string  `json:"error,omitempty"`
	VideoFile string  `json:"video_file,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`

	// Generation options (per-request overrides).
	ConcatEnabled    bool   `json:"concat_enabled"`
	ConcatIntro      string `json:"concat_intro,omitempty"`
	ConcatOutro      string `json:"concat_outro,omitempty"`
	ChromaKeyEnabled bool   `json:"chroma_key_enabled"`
	ChromaKeyBG      string `json:"chroma_key_bg,omitempty"`
}

// GenerateRequest is the JSON body for POST /api/generate.
type GenerateRequest struct {
	Slug             string `json:"slug"`
	AvatarID         string `json:"avatar_id,omitempty"`
	Text             string `json:"text,omitempty"` // optional: override post content
	ConcatEnabled    *bool  `json:"concat_enabled,omitempty"`
	ConcatIntro      string `json:"concat_intro,omitempty"` // filename only
	ConcatOutro      string `json:"concat_outro,omitempty"` // filename only
	ChromaKeyEnabled *bool  `json:"chroma_key_enabled,omitempty"`
	ChromaKeyBG      string `json:"chroma_key_bg,omitempty"` // filename only
}

// JobManager tracks all generation jobs.
type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewJobManager creates an empty job manager.
func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*Job),
	}
}

// Jobs returns a snapshot of all jobs.
func (jm *JobManager) Jobs() []Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	result := make([]Job, 0, len(jm.jobs))
	for _, j := range jm.jobs {
		result = append(result, *j)
	}
	return result
}

// Job returns a snapshot of a specific job.
func (jm *JobManager) Job(id string) (*Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	j, ok := jm.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	return &cp, true
}

func (jm *JobManager) update(id, phase string, progress float64, errMsg string) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if j, ok := jm.jobs[id]; ok {
		j.Phase = phase
		j.Progress = progress
		j.Error = errMsg
		j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (jm *JobManager) setVideoFile(id, filename string) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if j, ok := jm.jobs[id]; ok {
		j.VideoFile = filename
	}
}

// SubmitJob creates a new job and starts the pipeline in the background.
func (jm *JobManager) SubmitJob(
	cfg config.Config,
	rs *preprocess.RuleSet,
	vc *video.Client,
	req GenerateRequest,
	postContent string, // raw post content (already resolved)
) (string, error) {
	id := generateID()
	now := time.Now().UTC().Format(time.RFC3339)

	avatarID := req.AvatarID
	if avatarID == "" {
		avatarID = cfg.Video.AvatarID
	}

	concatEnabled := cfg.Concat.Enabled
	if req.ConcatEnabled != nil {
		concatEnabled = *req.ConcatEnabled
	}

	chromaKeyEnabled := cfg.Concat.ChromaKey.Enabled
	if req.ChromaKeyEnabled != nil {
		chromaKeyEnabled = *req.ChromaKeyEnabled
	}

	job := &Job{
		ID:               id,
		Slug:             req.Slug,
		AvatarID:         avatarID,
		Phase:            "queued",
		Progress:         0.0,
		CreatedAt:        now,
		UpdatedAt:        now,
		ConcatEnabled:    concatEnabled,
		ConcatIntro:      req.ConcatIntro,
		ConcatOutro:      req.ConcatOutro,
		ChromaKeyEnabled: chromaKeyEnabled,
		ChromaKeyBG:      req.ChromaKeyBG,
	}

	jm.mu.Lock()
	jm.jobs[id] = job
	jm.mu.Unlock()

	go jm.runPipeline(id, cfg, rs, vc, req, postContent)
	return id, nil
}

// runPipeline executes the full video generation pipeline for a job.
func (jm *JobManager) runPipeline(
	id string,
	cfg config.Config,
	rs *preprocess.RuleSet,
	vc *video.Client,
	req GenerateRequest,
	postContent string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Video.MaxWait+10*time.Minute)
	defer cancel()

	avatarID := req.AvatarID
	if avatarID == "" {
		avatarID = cfg.Video.AvatarID
	}

	// ── Phase 1: Preprocessing ──────────────────────────────────────────────
	jm.update(id, "preprocessing", 0.02, "")
	log.Printf("[job %s] preprocessing text for slug=%s", id[:8], req.Slug)

	text := postContent
	if req.Text != "" {
		text = req.Text
	}

	processed := preprocess.Process(text, rs)
	if strings.TrimSpace(processed) == "" {
		jm.update(id, "failed", 0.05, "post produced empty text after preprocessing")
		return
	}
	jm.update(id, "preprocessing", 0.05, "")

	// ── Phase 2: Submit to MuseTalk ─────────────────────────────────────────
	jm.update(id, "submitting", 0.07, "")
	log.Printf("[job %s] submitting to MuseTalk (avatar: %s, %d chars)", id[:8], avatarID, len(processed))

	museJobID, err := vc.Submit(ctx, avatarID, processed)
	if err != nil {
		jm.update(id, "failed", 0.10, fmt.Sprintf("submit to MuseTalk: %v", err))
		return
	}
	jm.update(id, "generating", 0.10, "")
	log.Printf("[job %s] MuseTalk job: %s", id[:8], museJobID)

	// ── Phase 3: Poll MuseTalk ──────────────────────────────────────────────
	ticker := time.NewTicker(cfg.Video.PollInterval)
	defer ticker.Stop()
	deadline := time.Now().Add(cfg.Video.MaxWait)

	for {
		select {
		case <-ctx.Done():
			jm.update(id, "failed", 0.0, "timed out waiting for MuseTalk")
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				jm.update(id, "failed", 0.0, fmt.Sprintf("timed out after %s", cfg.Video.MaxWait))
				return
			}

			status, err := vc.PollOnce(ctx, museJobID)
			if err != nil {
				continue // transient error, keep polling
			}

			switch strings.ToLower(status.Status) {
			case "completed", "done", "success":
				goto downloadPhase
			case "failed", "error":
				msg := status.Error
				if msg == "" {
					msg = "MuseTalk job failed"
				}
				jm.update(id, "failed", 0.10, msg)
				return
			default:
				// Map MuseTalk progress (0-1) to overall range (0.10-0.70).
				museProgress := status.Progress
				if museProgress < 0 {
					museProgress = 0
				}
				if museProgress > 1 {
					museProgress = 1
				}
				overall := 0.10 + museProgress*0.60
				jm.update(id, "generating", overall, "")
			}
		}
	}

downloadPhase:
	// ── Phase 4: Download MP4 ───────────────────────────────────────────────
	jm.update(id, "downloading", 0.72, "")
	log.Printf("[job %s] downloading MP4 from MuseTalk", id[:8])

	mp4Bytes, err := vc.Download(ctx, museJobID)
	if err != nil {
		jm.update(id, "failed", 0.70, fmt.Sprintf("download: %v", err))
		return
	}
	jm.update(id, "downloading", 0.80, "")
	log.Printf("[job %s] downloaded %d bytes", id[:8], len(mp4Bytes))

	// ── Ensure output directory ─────────────────────────────────────────────
	if err := os.MkdirAll(cfg.Concat.OutputDir, 0755); err != nil {
		jm.update(id, "failed", 0.80, fmt.Sprintf("create output dir: %v", err))
		return
	}

	// Build output filename.
	outputFilename := req.Slug + ".mp4"
	outputPath := filepath.Join(cfg.Concat.OutputDir, outputFilename)

	// Resolve concat config with per-request overrides.
	concatCfg := cfg.Concat

	// Check concat enabled override.
	concatEnabled := cfg.Concat.Enabled
	if req.ConcatEnabled != nil {
		concatEnabled = *req.ConcatEnabled
	}

	if !concatEnabled {
		// No concat — write raw MP4 directly.
		if err := os.WriteFile(outputPath, mp4Bytes, 0644); err != nil {
			jm.update(id, "failed", 0.80, fmt.Sprintf("write video: %v", err))
			return
		}
		jm.setVideoFile(id, outputFilename)
		jm.update(id, "completed", 1.0, "")
		log.Printf("[job %s] wrote raw video: %s (%d bytes)", id[:8], outputPath, len(mp4Bytes))
		return
	}

	// ── Phase 5: Stitch with ffmpeg ─────────────────────────────────────────
	jm.update(id, "stitching", 0.82, "")
	log.Printf("[job %s] stitching intro + avatar + outro", id[:8])

	// Resolve intro/outro paths from filenames.
	if req.ConcatIntro != "" {
		concatCfg.IntroPath = filepath.Join(cfg.Server.ResourcesDir, req.ConcatIntro)
	}
	if req.ConcatOutro != "" {
		concatCfg.OutroPath = filepath.Join(cfg.Server.ResourcesDir, req.ConcatOutro)
	}

	// Resolve chroma-key overrides.
	if req.ChromaKeyEnabled != nil {
		concatCfg.ChromaKey.Enabled = *req.ChromaKeyEnabled
	}
	if req.ChromaKeyBG != "" {
		concatCfg.ChromaKey.BGPath = filepath.Join(cfg.Server.BGDir, req.ChromaKeyBG)
	}

	// Write avatar MP4 to temp file for ffmpeg.
	tmpFile, err := os.CreateTemp("", "avatar-*.mp4")
	if err != nil {
		jm.update(id, "failed", 0.82, fmt.Sprintf("create temp file: %v", err))
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(mp4Bytes); err != nil {
		tmpFile.Close()
		jm.update(id, "failed", 0.82, fmt.Sprintf("write temp file: %v", err))
		return
	}
	tmpFile.Close()

	jm.update(id, "stitching", 0.88, "")

	if err := concat.Stitch(ctx, concatCfg, tmpPath, outputPath); err != nil {
		jm.update(id, "failed", 0.88, fmt.Sprintf("ffmpeg stitch: %v", err))
		return
	}

	info, _ := os.Stat(outputPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	jm.setVideoFile(id, outputFilename)
	jm.update(id, "completed", 1.0, "")
	log.Printf("[job %s] completed: %s (%.1f MB)", id[:8], outputPath, float64(size)/(1024*1024))
}

// findPostBySlug searches postsDir for a post matching the slug fragment.
func findPostBySlug(postsDir, slug string) (*post.Post, error) {
	all, err := post.FindAllPosts(postsDir)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p.Slug == slug || strings.Contains(p.Slug, slug) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no post found matching slug %q", slug)
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
