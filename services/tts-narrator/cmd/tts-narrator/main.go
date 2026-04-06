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

	"HerbHub365/services/tts-narrator/internal/config"
	"HerbHub365/services/tts-narrator/internal/gitpublish"
	"HerbHub365/services/tts-narrator/internal/post"
	"HerbHub365/services/tts-narrator/internal/preprocess"
	"HerbHub365/services/tts-narrator/internal/tts"

	"github.com/robfig/cron/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	mode := resolveMode(cfg.Mode)

	switch mode {
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
		log.Fatalf("unsupported TTS_MODE %q (valid: daemon, generate, backfill, dry-run)", mode)
	}
}

// ─── mode: generate ──────────────────────────────────────────────────────────

func runGenerate(ctx context.Context, cfg config.Config) error {
	rs, ttsClient, publisher, err := buildDeps(cfg)
	if err != nil {
		return err
	}

	var posts []*post.Post

	if cfg.TargetSlug != "" {
		// Find by slug fragment.
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
		if err := narratePost(ctx, cfg, p, rs, ttsClient, publisher, ""); err != nil {
			log.Printf("narrate %s: %v", p.Path, err)
		}
	}
	return nil
}

// ─── mode: backfill ──────────────────────────────────────────────────────────

func runBackfill(ctx context.Context, cfg config.Config) error {
	rs, ttsClient, publisher, err := buildDeps(cfg)
	if err != nil {
		return err
	}

	allPosts, err := post.FindAllPosts(cfg.Post.PostsDir)
	if err != nil {
		return err
	}

	// Build the rotation list: TTS_VOICES takes precedence, else fall back to single TTS_VOICE.
	voices := cfg.TTS.Voices
	if len(voices) == 0 {
		voices = []string{cfg.TTS.Voice}
	}

	var skipped, narrated int
	for _, p := range allPosts {
		base := filepath.Base(p.Path)

		// Skip posts whose filename matches any skip pattern.
		if matchesSkipPattern(base, cfg.SkipPatterns) {
			log.Printf("backfill: skipping %s (matches skip pattern)", base)
			skipped++
			continue
		}

		if p.AudioURL != "" {
			skipped++
			continue
		}

		// Rotate through presets: eve for even index, rowan for odd (or whatever order supplied).
		voice := voices[narrated%len(voices)]
		if err := narratePost(ctx, cfg, p, rs, ttsClient, publisher, voice); err != nil {
			log.Printf("backfill: narrate %s: %v", p.Path, err)
			continue
		}
		narrated++
		// Small pause between posts to be kind to the TTS API.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	log.Printf("backfill complete: %d narrated, %d already had audio", narrated, skipped)
	return nil
}

// ─── mode: dry-run ───────────────────────────────────────────────────────────

func runDryRun(_ context.Context, cfg config.Config) error {
	rs, err := preprocess.LoadRules(cfg.RulesFile)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	day, err := cfg.ResolveTargetDate(time.Now())
	if err != nil {
		return err
	}

	posts, err := post.FindPostsForDate(cfg.Post.PostsDir, day)
	if err != nil {
		return err
	}
	if len(posts) == 0 {
		log.Printf("dry-run: no posts found for %s", day.Format("2006-01-02"))
		return nil
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

	_, err := scheduler.AddFunc(cfg.NarrateSchedule, func() {
		jobCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := runGenerate(jobCtx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("daemon: scheduled narration failed: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("configure scheduler: %w", err)
	}

	scheduler.Start()
	defer scheduler.Stop()

	log.Printf("tts-narrator daemon started, schedule: %s", cfg.NarrateSchedule)

	if cfg.RunOnStart {
		log.Printf("daemon: running narration on start")
		if err := runGenerate(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("daemon: on-start narration failed: %v", err)
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

// ─── core narration logic ─────────────────────────────────────────────────────

func narratePost(
	ctx context.Context,
	cfg config.Config,
	p *post.Post,
	rs *preprocess.RuleSet,
	ttsClient *tts.Client,
	publisher *gitpublish.Publisher,
	voiceOverride string, // "" → use client's default (cfg.TTS.Voice)
) error {
	// 1. Preprocess text.
	text := preprocess.Process(p.RawContent, rs)
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("post %s produced empty text after preprocessing", filepath.Base(p.Path))
	}

	// 2. Call TTS API — use voice override if supplied, else client default.
	log.Printf("narrating %s (%d chars, preset: %s)…", filepath.Base(p.Path), len(text), resolveVoiceLabel(voiceOverride, cfg.TTS.Voice))
	var (
		audioBytes []byte
		err        error
	)
	if voiceOverride != "" {
		audioBytes, err = ttsClient.SpeakAs(ctx, text, voiceOverride)
	} else {
		audioBytes, err = ttsClient.Speak(ctx, text)
	}
	if err != nil {
		return fmt.Errorf("TTS speak: %w", err)
	}

	// 3. Write MP3 to assets/audio/blog/YYYY-MM-DD-slug.mp3.
	if err := os.MkdirAll(cfg.Post.AudioDir, 0755); err != nil {
		return fmt.Errorf("create audio dir: %w", err)
	}
	audioFilename := p.AudioFilename()
	audioPath := filepath.Join(cfg.Post.AudioDir, audioFilename)
	if err := os.WriteFile(audioPath, audioBytes, 0644); err != nil {
		return fmt.Errorf("write MP3 %s: %w", audioPath, err)
	}
	log.Printf("wrote %s (%d bytes)", audioPath, len(audioBytes))

	// 4. Patch audio_url into post front matter.
	audioURL := cfg.Post.AudioPublicPath + "/" + audioFilename
	if _, err := post.PatchAudioURL(p, audioURL); err != nil {
		return fmt.Errorf("patch audio_url: %w", err)
	}
	log.Printf("patched audio_url: %s", audioURL)

	// 5. Git commit + push.
	result := gitpublish.Result{
		PostPath:  p.Path,
		AudioPath: audioPath,
	}
	if err := publisher.PublishNarration(ctx, result, p.Date); err != nil {
		return fmt.Errorf("git publish: %w", err)
	}

	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func buildDeps(cfg config.Config) (*preprocess.RuleSet, *tts.Client, *gitpublish.Publisher, error) {
	rs, err := preprocess.LoadRules(cfg.RulesFile)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load rules: %w", err)
	}
	ttsClient := tts.NewClient(cfg.TTS, cfg.RequestTimeout)
	publisher := gitpublish.NewPublisher(cfg.Git)
	return rs, ttsClient, publisher, nil
}

func resolveMode(defaultMode string) string {
	if len(os.Args) > 1 {
		return strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	return strings.ToLower(strings.TrimSpace(defaultMode))
}

// matchesSkipPattern returns true if name contains any of the given substrings.
func matchesSkipPattern(name string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// resolveVoiceLabel returns override if non-empty, otherwise fallback.
func resolveVoiceLabel(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}
