package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"HerbHub365/services/video-narrator/internal/concat"
	"HerbHub365/services/video-narrator/internal/config"
	"HerbHub365/services/video-narrator/internal/preprocess"
	"HerbHub365/services/video-narrator/internal/queue"
	"HerbHub365/services/video-narrator/internal/video"
)

// TimelapseNarrateRequest is the JSON body for POST /api/timelapse/narrate.
type TimelapseNarrateRequest struct {
	Text          string `json:"text"`           // TTS narration script
	TimelapseFile string `json:"timelapse_file"` // filename of the built timelapse MP4
	Intro         string `json:"intro"`          // intro filename (in resources dir)
	Outro         string `json:"outro"`          // outro filename (in resources dir)
	Slug          string `json:"slug"`           // used for output filename and notification
	Date          string `json:"date"`           // YYYY-MM-DD for the RabbitMQ message
}

// TimelapseJob represents a timelapse narration job.
type TimelapseJob struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Phase       string  `json:"phase"` // queued, preprocessing, submitting, generating, narrating, stitching, completed, failed
	Progress    float64 `json:"progress"`
	Error       string  `json:"error,omitempty"`
	VideoFile   string  `json:"video_file,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	CompletedAt string  `json:"completed_at,omitempty"`
}

// TimelapseJobManager tracks timelapse narration jobs.
type TimelapseJobManager struct {
	mu   sync.RWMutex
	jobs map[string]*TimelapseJob
}

func NewTimelapseJobManager() *TimelapseJobManager {
	return &TimelapseJobManager{jobs: make(map[string]*TimelapseJob)}
}

func (m *TimelapseJobManager) Jobs() []TimelapseJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TimelapseJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		result = append(result, *j)
	}
	return result
}

func (m *TimelapseJobManager) Job(id string) (*TimelapseJob, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	return &cp, true
}

func (m *TimelapseJobManager) update(id, phase string, progress float64, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if j, ok := m.jobs[id]; ok {
		j.Phase = phase
		j.Progress = progress
		j.Error = errMsg
		j.UpdatedAt = now
		if phase == "completed" || phase == "failed" {
			j.CompletedAt = now
		}
	}
}

func (m *TimelapseJobManager) setVideoFile(id, filename string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		j.VideoFile = filename
	}
}

// SubmitJob creates a new timelapse narration job and starts it in the background.
func (m *TimelapseJobManager) SubmitJob(
	cfg config.Config,
	rs *preprocess.RuleSet,
	vc *video.Client,
	req TimelapseNarrateRequest,
) (string, error) {
	if strings.TrimSpace(req.Text) == "" {
		return "", fmt.Errorf("text is required")
	}
	if strings.TrimSpace(req.TimelapseFile) == "" {
		return "", fmt.Errorf("timelapse_file is required")
	}
	if strings.TrimSpace(req.Slug) == "" {
		return "", fmt.Errorf("slug is required")
	}

	id := generateTLID()
	now := time.Now().UTC().Format(time.RFC3339)

	job := &TimelapseJob{
		ID:        id,
		Slug:      req.Slug,
		Phase:     "queued",
		Progress:  0.0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()

	go m.runPipeline(id, cfg, rs, vc, req)
	return id, nil
}

func (m *TimelapseJobManager) runPipeline(
	id string,
	cfg config.Config,
	rs *preprocess.RuleSet,
	vc *video.Client,
	req TimelapseNarrateRequest,
) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Video.MaxWait+cfg.Video.PipelineBuffer)
	defer cancel()

	ffmpeg := cfg.Concat.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}

	// ── Phase 1: Preprocess TTS text ────────────────────────────────────────
	m.update(id, "preprocessing", 0.02, "")
	log.Printf("[tl-job %s] preprocessing text for slug=%s", id[:8], req.Slug)

	processed := preprocess.Process(req.Text, rs)
	if strings.TrimSpace(processed) == "" {
		m.update(id, "failed", 0.02, "text produced empty result after preprocessing")
		return
	}
	m.update(id, "preprocessing", 0.05, "")

	// ── Phase 2: Submit to MuseTalk for TTS audio ────────────────────────────
	m.update(id, "submitting", 0.07, "")
	log.Printf("[tl-job %s] submitting TTS to MuseTalk (%d chars)", id[:8], len(processed))

	museJobID, err := vc.Submit(ctx, vc.Cfg().AvatarID, processed)
	if err != nil {
		m.update(id, "failed", 0.10, fmt.Sprintf("submit to MuseTalk: %v", err))
		return
	}
	m.update(id, "generating", 0.10, "")

	// ── Phase 3: Poll MuseTalk ───────────────────────────────────────────────
	ticker := time.NewTicker(cfg.Video.PollInterval)
	defer ticker.Stop()
	deadline := time.Now().Add(cfg.Video.MaxWait)

	for {
		select {
		case <-ctx.Done():
			m.update(id, "failed", 0.0, "timed out waiting for MuseTalk")
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				m.update(id, "failed", 0.0, fmt.Sprintf("timed out after %s", cfg.Video.MaxWait))
				return
			}
			status, err := vc.PollOnce(ctx, museJobID)
			if err != nil {
				continue
			}
			switch strings.ToLower(status.Status) {
			case "completed", "done", "success":
				goto downloadPhase
			case "failed", "error":
				msg := status.Error
				if msg == "" {
					msg = "MuseTalk job failed"
				}
				m.update(id, "failed", 0.10, msg)
				return
			default:
				mp := status.Progress
				if mp < 0 {
					mp = 0
				}
				if mp > 1 {
					mp = 1
				}
				m.update(id, "generating", 0.10+mp*0.60, "")
			}
		}
	}

downloadPhase:
	// ── Phase 4: Download avatar MP4 (for audio track) ──────────────────────
	m.update(id, "narrating", 0.72, "")
	log.Printf("[tl-job %s] downloading avatar MP4 for audio extraction", id[:8])

	avatarBytes, err := vc.Download(ctx, museJobID)
	if err != nil {
		m.update(id, "failed", 0.72, fmt.Sprintf("download avatar: %v", err))
		return
	}

	avatarTmp, err := os.CreateTemp("", "tl-avatar-*.mp4")
	if err != nil {
		m.update(id, "failed", 0.72, fmt.Sprintf("create avatar temp: %v", err))
		return
	}
	avatarTmpPath := avatarTmp.Name()
	defer os.Remove(avatarTmpPath)

	if _, err := avatarTmp.Write(avatarBytes); err != nil {
		avatarTmp.Close()
		m.update(id, "failed", 0.72, fmt.Sprintf("write avatar temp: %v", err))
		return
	}
	avatarTmp.Close()

	// ── Phase 5: Extract TTS audio from avatar MP4 ──────────────────────────
	ttsAudioPath := avatarTmpPath + ".aac"
	defer os.Remove(ttsAudioPath)

	extractOut, err := exec.CommandContext(ctx, ffmpeg,
		"-y", "-i", avatarTmpPath,
		"-vn", "-acodec", "aac", "-ar", "48000", "-ac", "2",
		ttsAudioPath,
	).CombinedOutput()
	if err != nil {
		m.update(id, "failed", 0.75, fmt.Sprintf("extract TTS audio: %v\n%s", err, tlTrimOutput(string(extractOut))))
		return
	}
	m.update(id, "narrating", 0.76, "")

	// ── Phase 6: Overlay TTS audio on timelapse video ───────────────────────
	timelapsePath, err := resolveTimelapsePath(cfg.TimelapseOutputDir, req.TimelapseFile)
	if err != nil {
		m.update(id, "failed", 0.76, err.Error())
		return
	}

	narratedTmp, err := os.CreateTemp("", "tl-narrated-*.mp4")
	if err != nil {
		m.update(id, "failed", 0.76, fmt.Sprintf("create narrated temp: %v", err))
		return
	}
	narratedTmpPath := narratedTmp.Name()
	narratedTmp.Close()
	defer os.Remove(narratedTmpPath)

	// Map video from timelapse, audio from TTS. Duration is driven by the
	// timelapse video stream; TTS audio is trimmed if longer, silent if shorter.
	overlayOut, err := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-i", timelapsePath,
		"-i", ttsAudioPath,
		"-map", "0:v",
		"-map", "1:a",
		"-c:v", "copy",
		"-c:a", "aac", "-ar", "48000", "-ac", "2",
		narratedTmpPath,
	).CombinedOutput()
	if err != nil {
		m.update(id, "failed", 0.78, fmt.Sprintf("overlay TTS audio: %v\n%s", err, tlTrimOutput(string(overlayOut))))
		return
	}
	m.update(id, "narrating", 0.80, "")

	// ── Phase 7: Stitch intro + narrated_timelapse + outro ──────────────────
	m.update(id, "stitching", 0.82, "")
	log.Printf("[tl-job %s] stitching intro + narrated timelapse + outro", id[:8])

	if err := os.MkdirAll(cfg.Concat.OutputDir, 0755); err != nil {
		m.update(id, "failed", 0.82, fmt.Sprintf("create output dir: %v", err))
		return
	}

	outputFilename := req.Slug + ".mp4"
	outputPath := filepath.Join(cfg.Concat.OutputDir, outputFilename)

	concatCfg := cfg.Concat
	concatCfg.ChromaKey.Enabled = false // no chroma key for timelapse narration
	if req.Intro != "" {
		concatCfg.IntroPath = filepath.Join(cfg.Server.ResourcesDir, req.Intro)
	}
	if req.Outro != "" {
		concatCfg.OutroPath = filepath.Join(cfg.Server.ResourcesDir, req.Outro)
	}

	stitchCtx, stitchCancel := context.WithTimeout(context.Background(), concatCfg.StitchTimeout)
	defer stitchCancel()

	if err := concat.Stitch(stitchCtx, concatCfg, narratedTmpPath, outputPath); err != nil {
		m.update(id, "failed", 0.88, fmt.Sprintf("stitch: %v", err))
		return
	}

	info, _ := os.Stat(outputPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	m.setVideoFile(id, outputFilename)
	m.update(id, "completed", 1.0, "")
	log.Printf("[tl-job %s] completed: %s (%.1f MB)", id[:8], outputPath, float64(size)/(1024*1024))

	// ── Notify via RabbitMQ ──────────────────────────────────────────────────
	date := req.Date
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	msgData, _ := json.Marshal(map[string]any{
		"slug":        req.Slug,
		"date":        date,
		"output_file": outputFilename,
		"status":      "completed",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})

	markerPath := filepath.Join(cfg.Concat.OutputDir, strings.TrimSuffix(outputFilename, ".mp4")+".json")
	if err := os.WriteFile(markerPath, msgData, 0644); err != nil {
		log.Printf("[tl-job %s] write marker: %v", id[:8], err)
	}

	pub := queue.NewPublisher(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if pub != nil && pub.Enabled() {
		if err := pub.Publish(ctx, msgData); err != nil {
			log.Printf("[tl-job %s] publish to queue: %v", id[:8], err)
		}
	}
}

func tlTrimOutput(s string) string {
	if len(s) <= 1000 {
		return s
	}
	return "…" + s[len(s)-1000:]
}

func resolveTimelapsePath(outputDir, requestedFile string) (string, error) {
	requestedFile = strings.TrimSpace(requestedFile)
	if requestedFile == "" {
		return "", fmt.Errorf("timelapse file is empty")
	}

	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	addCandidate := func(path string) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		candidates = append(candidates, cleaned)
	}

	if filepath.IsAbs(requestedFile) {
		addCandidate(requestedFile)
	} else {
		addCandidate(filepath.Join(outputDir, requestedFile))
		if filepath.Base(filepath.Clean(outputDir)) == "timelapse" {
			// Older defaults pointed at /output/timelapse while builder writes to /output.
			addCandidate(filepath.Join(filepath.Dir(filepath.Clean(outputDir)), requestedFile))
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("timelapse file not found; checked: %s", strings.Join(candidates, ", "))
}

func generateTLID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tl-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
