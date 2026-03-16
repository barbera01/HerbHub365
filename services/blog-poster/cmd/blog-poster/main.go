package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"HerbHub365/services/blog-poster/internal/archive"
	"HerbHub365/services/blog-poster/internal/blog"
	"HerbHub365/services/blog-poster/internal/config"
	"HerbHub365/services/blog-poster/internal/gitpublish"
	"HerbHub365/services/blog-poster/internal/llm"
	"HerbHub365/services/blog-poster/internal/model"
	"HerbHub365/services/blog-poster/internal/rabbitmq"
	"HerbHub365/services/blog-poster/internal/repopost"
	"github.com/robfig/cron/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	mode := resolveMode(cfg.Mode)
	store := archive.NewStore(cfg.DataDir)
	client := llm.NewClient(cfg.LLM)
	generator := blog.NewGenerator(cfg.Blog, cfg.LLM, store, client)
	publisher := gitpublish.NewPublisher(cfg.Git)

	switch mode {
	case "collect":
		if err := runCollectorLoop(ctx, cfg, store); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "draft":
		if err := runDraft(ctx, cfg, generator); err != nil {
			log.Fatal(err)
		}
	case "generate", "once":
		if err := runGenerate(ctx, cfg, generator, publisher); err != nil {
			log.Fatal(err)
		}
	case "repo-post":
		if err := runRepoPost(ctx, cfg, generator, publisher); err != nil {
			log.Fatal(err)
		}
	case "daemon":
		if err := runDaemon(ctx, cfg, store, generator, publisher); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unsupported BLOG_POSTER_MODE %q", mode)
	}
}

func resolveMode(defaultMode string) string {
	if len(os.Args) > 1 {
		return strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	return strings.ToLower(strings.TrimSpace(defaultMode))
}

func runDaemon(ctx context.Context, cfg config.Config, store *archive.Store, generator *blog.Generator, publisher *gitpublish.Publisher) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- runCollectorLoop(ctx, cfg, store)
	}()

	scheduler := cron.New()
	_, err := scheduler.AddFunc(cfg.GenerateSchedule, func() {
		jobCtx, cancel := context.WithTimeout(context.Background(), cfg.GenerateTimeout)
		defer cancel()

		targetDate, resolveErr := cfg.ResolveTargetDate(time.Now())
		if resolveErr != nil {
			log.Printf("generate skipped: %v", resolveErr)
			return
		}

		result, genErr := generator.Generate(jobCtx, targetDate)
		if genErr != nil {
			if errors.Is(genErr, blog.ErrNoSnapshots) {
				log.Printf("generate skipped for %s: %v", targetDate.Format("2006-01-02"), genErr)
				return
			}
			log.Printf("generate failed for %s: %v", targetDate.Format("2006-01-02"), genErr)
			return
		}

		if publishErr := publisher.PublishPost(jobCtx, result, targetDate); publishErr != nil {
			log.Printf("publish failed for %s: %v", targetDate.Format("2006-01-02"), publishErr)
			return
		}

		log.Printf("generated post %s", result.Path)
	})
	if err != nil {
		return fmt.Errorf("configure scheduler: %w", err)
	}

	scheduler.Start()
	defer scheduler.Stop()

	if cfg.RunGenerateOnStart {
		if err := runGenerate(ctx, cfg, generator, publisher); err != nil && !errors.Is(err, blog.ErrNoSnapshots) {
			return err
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func runGenerate(ctx context.Context, cfg config.Config, generator *blog.Generator, publisher *gitpublish.Publisher) error {
	targetDate, err := cfg.ResolveTargetDate(time.Now())
	if err != nil {
		return err
	}

	jobCtx, cancel := context.WithTimeout(ctx, cfg.GenerateTimeout)
	defer cancel()

	result, err := generator.Generate(jobCtx, targetDate)
	if err != nil {
		return err
	}

	if err := publisher.PublishPost(jobCtx, result, targetDate); err != nil {
		return err
	}

	log.Printf("generated post %s", result.Path)
	return nil
}

func runDraft(ctx context.Context, cfg config.Config, generator *blog.Generator) error {
	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQ)
	if err != nil {
		return err
	}
	defer consumer.Close()

	snapshot, _, err := consumer.FetchSnapshot(true)
	if err != nil {
		return err
	}

	targetDate := snapshot.Timestamp.UTC()
	if targetDate.IsZero() && snapshot.CollectedAt != nil {
		targetDate = snapshot.CollectedAt.UTC()
	}
	if targetDate.IsZero() {
		targetDate = time.Now().UTC()
	}
	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)

	jobCtx, cancel := context.WithTimeout(ctx, cfg.GenerateTimeout)
	defer cancel()

	result, err := generator.GenerateDraft(jobCtx, targetDate, []model.Snapshot{*snapshot})
	if err != nil {
		return err
	}

	log.Printf("generated draft %s", result.Path)
	return nil
}

func runRepoPost(ctx context.Context, cfg config.Config, generator *blog.Generator, publisher *gitpublish.Publisher) error {
	jobCtx, cancel := context.WithTimeout(ctx, cfg.GenerateTimeout)
	defer cancel()

	request, err := repopost.BuildPrompt(cfg)
	if err != nil {
		return err
	}

	targetDate, err := cfg.ResolveTargetDate(time.Now())
	if err != nil {
		return err
	}

	result, err := generator.GenerateRepoPost(jobCtx, targetDate, request.Prompt, cfg.RepoPost.Title, cfg.RepoPost.Draft, cfg.RepoPost.Categories)
	if err != nil {
		return err
	}

	log.Printf("repo-post used sources: %s", strings.Join(request.SourcePaths, ", "))
	if cfg.RepoPost.Draft {
		log.Printf("generated repo post draft %s", result.Path)
		return nil
	}

	if err := publisher.PublishPost(jobCtx, result, targetDate); err != nil {
		return err
	}

	log.Printf("generated repo post %s", result.Path)
	return nil
}

func runCollectorLoop(ctx context.Context, cfg config.Config, store *archive.Store) error {
	for {
		consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQ)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("rabbitmq connect failed: %v", err)
		} else {
			err = consumer.Run(ctx, store)
			_ = consumer.Close()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				log.Printf("rabbitmq consumer stopped: %v", err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.ReconnectDelay):
		}
	}
}
