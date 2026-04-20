package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"HerbHub365/services/video-narrator/internal/concat"
	"HerbHub365/services/video-narrator/internal/config"
	"HerbHub365/services/video-narrator/internal/notify"
	"HerbHub365/services/video-narrator/internal/post"
	"HerbHub365/services/video-narrator/internal/preprocess"
	"HerbHub365/services/video-narrator/internal/server"
	"HerbHub365/services/video-narrator/internal/tts"
	"HerbHub365/services/video-narrator/internal/video"

	"github.com/robfig/cron/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	mode := resolveMode(cfg.Mode)

	switch mode {
	case "server":
		if err := runServer(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "generate", "once":
		if err := runGenerate(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	case "backfill":
		if err := runBackfill(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	case "dry-run":
		if err := runDryRun(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	case "daemon":
		if err := runDaemon(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unsupported VIDEO_MODE %q (valid: server, daemon, generate, backfill, dry-run)", mode)
	}
}

// ─── mode: server ─────────────────────────────────────────────────────────────

func runServer(ctx context.Context, cfg config.Config) error {
	rs, videoClient, ttsClient, err := buildDeps(cfg)
	if err != nil {
		return err
	}

	srv := server.New(cfg, rs, videoClient, ttsClient)
	return srv.ListenAndServe(ctx)
}

// ─── mode: generate ───────────────────────────────────────────────────────────

func runGenerate(ctx context.Context, cfg config.Config) error {
	rs, videoClient, _, err := buildDeps(cfg)
	if err != nil {
		return err
	}

	var posts []*post.Post

	if cfg.TargetSlug != "" {
		all, err := post.FindAllPosts(cfg.Post.PostsDir)
		if err != nil {
			return err
		}
		for _, p := range all {
			if strings.Contains(p.Slug, cfg.TargetSlug) {
				posts = append(posts, p)
			}
		}
		if len(posts) == 0 {
			return fmt.Errorf("no posts found matching slug fragment %q", cfg.TargetSlug)
		}
	} else {
		day, err := cfg.ResolveTargetDate(time.Now())
		if err != nil {
			return err
		}
		posts, err = post.FindPostsForDate(cfg.Post.PostsDir, day)
		if err != nil {
			return err
		}
		if len(posts) == 0 {
			log.Printf("no posts found for %s", day.Format("2006-01-02"))
			return nil
		}
	}

	for _, p := range posts {
		if err := generatePost(ctx, cfg, p, rs, videoClient, ""); err != nil {
			log.Printf("generate %s: %v", p.Path, err)
		}
	}
	return nil
}

// ─── mode: backfill ───────────────────────────────────────────────────────────

func runBackfill(ctx context.Context, cfg config.Config) error {
	rs, videoClient, _, err := buildDeps(cfg)
	if err != nil {
		return err
	}

	allPosts, err := post.FindAllPosts(cfg.Post.PostsDir)
	if err != nil {
		return err
	}

	// Avatar rotation: VIDEO_AVATARS takes precedence, else fall back to single VIDEO_AVATAR.
	avatars := cfg.Video.AvatarIDs
	if len(avatars) == 0 {
		avatars = []string{cfg.Video.AvatarID}
	}

	var skipped, generated int
	for _, p := range allPosts {
		base := filepath.Base(p.Path)

		if matchesSkipPattern(base, cfg.SkipPatterns) {
			log.Printf("backfill: skipping %s (matches skip pattern)", base)
			skipped++
			continue
		}

		if p.OutputExists(cfg.Concat.OutputDir) {
			log.Printf("backfill: skipping %s (output already exists)", base)
			skipped++
			continue
		}

		avatar := avatars[generated%len(avatars)]
		if err := generatePost(ctx, cfg, p, rs, videoClient, avatar); err != nil {
			log.Printf("backfill: generate %s: %v", p.Path, err)
			continue
		}
		generated++

		// Small pause between posts to be kind to the video API.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	log.Printf("backfill complete: %d generated, %d skipped", generated, skipped)
	return nil
}

// ─── mode: dry-run ────────────────────────────────────────────────────────────

func runDryRun(_ context.Context, cfg config.Config) error {
	rs, err := preprocess.LoadRules(cfg.RulesFile)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	var posts []*post.Post

	if cfg.TargetSlug != "" {
		all, err := post.FindAllPosts(cfg.Post.PostsDir)
		if err != nil {
			return err
		}
		for _, p := range all {
			if strings.Contains(p.Slug, cfg.TargetSlug) {
				posts = append(posts, p)
			}
		}
		if len(posts) == 0 {
			return fmt.Errorf("dry-run: no posts found matching slug fragment %q", cfg.TargetSlug)
		}
	} else {
		day, err := cfg.ResolveTargetDate(time.Now())
		if err != nil {
			return err
		}
		posts, err = post.FindPostsForDate(cfg.Post.PostsDir, day)
		if err != nil {
			return err
		}
		if len(posts) == 0 {
			log.Printf("dry-run: no posts found for %s", day.Format("2006-01-02"))
			return nil
		}
	}

	for _, p := range posts {
		processed := preprocess.Process(p.RawContent, rs)
		fmt.Printf("\n── %s ──\n\n%s\n", filepath.Base(p.Path), processed)
	}
	return nil
}

// ─── mode: daemon ─────────────────────────────────────────────────────────────

func runDaemon(ctx context.Context, cfg config.Config) error {
	scheduler := cron.New()

	_, err := scheduler.AddFunc(cfg.GenerateSchedule, func() {
		jobCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := runGenerate(jobCtx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("daemon: scheduled generation failed: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("configure scheduler: %w", err)
	}

	scheduler.Start()
	defer scheduler.Stop()

	log.Printf("video-narrator daemon started, schedule: %s", cfg.GenerateSchedule)

	if cfg.RunOnStart {
		log.Printf("daemon: running generation on start")
		if err := runGenerate(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("daemon: on-start generation failed: %v", err)
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

// ─── core generation logic ────────────────────────────────────────────────────

func generatePost(
	ctx context.Context,
	cfg config.Config,
	p *post.Post,
	rs *preprocess.RuleSet,
	videoClient *video.Client,
	avatarOverride string, // "" → use client's default (cfg.Video.AvatarID)
) error {
	// 1. Preprocess text.
	text := preprocess.Process(p.RawContent, rs)
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("post %s produced empty text after preprocessing", filepath.Base(p.Path))
	}

	avatarLabel := avatarOverride
	if avatarLabel == "" {
		avatarLabel = cfg.Video.AvatarID
	}
	log.Printf("generating video for %s (%d chars, avatar: %s)…",
		filepath.Base(p.Path), len(text), avatarLabel)

	// 2. Call video API — use avatar override if supplied.
	var (
		mp4Bytes []byte
		err      error
	)
	if avatarOverride != "" {
		mp4Bytes, err = videoClient.GenerateAs(ctx, text, avatarOverride)
	} else {
		mp4Bytes, err = videoClient.Generate(ctx, text)
	}
	if err != nil {
		return fmt.Errorf("video generate: %w", err)
	}
	log.Printf("video API returned %d bytes for %s", len(mp4Bytes), filepath.Base(p.Path))

	// 3. Ensure output directory exists.
	if err := os.MkdirAll(cfg.Concat.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	finalPath := filepath.Join(cfg.Concat.OutputDir, p.VideoFilename())

	if !cfg.Concat.Enabled {
		// No concat — write the raw avatar video directly.
		if err := os.WriteFile(finalPath, mp4Bytes, 0644); err != nil {
			return fmt.Errorf("write video %s: %w", finalPath, err)
		}
		log.Printf("wrote %s (%d bytes)", finalPath, len(mp4Bytes))
		if err := notify.PublishCompletion(ctx, cfg, p.Slug, p.VideoFilename()); err != nil {
			log.Printf("publish completion for %s: %v", p.Slug, err)
		}
		return nil
	}

	// 4. Write avatar video to a temp file for ffmpeg.
	tmpFile, err := os.CreateTemp("", "avatar-*.mp4")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // clean up regardless of outcome

	if _, err := tmpFile.Write(mp4Bytes); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp avatar video: %w", err)
	}
	tmpFile.Close()

	// 5. Stitch intro + avatar + outro → final MP4.
	log.Printf("stitching intro + avatar + outro → %s", filepath.Base(finalPath))
	if err := concat.Stitch(ctx, cfg.Concat, tmpPath, finalPath); err != nil {
		return fmt.Errorf("concat stitch: %w", err)
	}

	info, _ := os.Stat(finalPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	log.Printf("wrote %s (%.1f MB)", finalPath, float64(size)/(1024*1024))
	if err := notify.PublishCompletion(ctx, cfg, p.Slug, p.VideoFilename()); err != nil {
		log.Printf("publish completion for %s: %v", p.Slug, err)
	}
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func buildDeps(cfg config.Config) (*preprocess.RuleSet, *video.Client, *tts.Client, error) {
	rs, err := preprocess.LoadRules(cfg.RulesFile)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load rules: %w", err)
	}
	videoClient := video.NewClient(cfg.Video, cfg.RequestTimeout)
	ttsClient := tts.NewClient(cfg.TTS, cfg.RequestTimeout)
	return rs, videoClient, ttsClient, nil
}

func resolveMode(defaultMode string) string {
	if len(os.Args) > 1 {
		return strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	return strings.ToLower(strings.TrimSpace(defaultMode))
}

func matchesSkipPattern(name string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}
