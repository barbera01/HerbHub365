package main

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"context"
	"os/signal"
	"syscall"

	"HerbHub365/services/herbhub-manager/internal/api"
	"HerbHub365/services/herbhub-manager/internal/auth"
	"HerbHub365/services/herbhub-manager/internal/blogpost"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/messaging"
	"HerbHub365/services/herbhub-manager/internal/publisher"
	"HerbHub365/services/herbhub-manager/internal/queue"
	"HerbHub365/services/herbhub-manager/internal/rabbitmq"
	"HerbHub365/services/herbhub-manager/internal/timelapse"
	"HerbHub365/services/herbhub-manager/internal/video"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	log.Printf("herbhub-manager starting on %s", cfg.ListenAddr)
	log.Printf("  posts dir:    %s", cfg.Post.PostsDir)
	log.Printf("  output dir:   %s", cfg.OutputDir)
	log.Printf("  narrator API: %s", cfg.NarratorURL)
	log.Printf("  llm-service:  %s", cfg.Blog.LLMServiceURL)
	log.Printf("  timelapse:    %s", cfg.Timelapse.ServiceURL)
	log.Printf("  auth enabled: %t", !cfg.Auth.Disabled)
	log.Printf("  messaging mgmt enabled flag: %t", cfg.Messaging.Management.Enabled)

	if err := validateAuthConfig(cfg.Auth); err != nil {
		log.Fatalf("invalid auth configuration: %v", err)
	}

	videoClient := video.NewClient(cfg)
	blogClient := blogpost.NewClient(cfg.Blog)
	timelapseClient := timelapse.NewClient(cfg.Timelapse.ServiceURL, cfg.Timelapse.Timeout)
	verifier := auth.NewVerifier(cfg.Auth, nil)

	var pubClient *publisher.Client
	if publisher.Enabled(cfg.RabbitMQ.URL) {
		var err error
		pubClient, err = publisher.NewClient(cfg.RabbitMQ.URL, cfg.RabbitMQ.Queue)
		if err != nil {
			log.Printf("publisher client: %v (YouTube publishing disabled)", err)
		} else {
			log.Printf("  rabbitmq:     configured (queue: %s)", cfg.RabbitMQ.Queue)
		}
	}

	queueManager := queue.NewManager(videoClient, cfg.Post.PostsDir)
	go queueManager.Run(ctx)

	var messagingSvc *messaging.Service
	if cfg.Messaging.Management.Enabled {
		rabbitClient, err := rabbitmq.NewClient(rabbitmq.Config{
			URL:      cfg.Messaging.Management.URL,
			VHost:    cfg.Messaging.Management.VHost,
			User:     cfg.Messaging.Management.User,
			Password: cfg.Messaging.Management.Password,
			Timeout:  cfg.Messaging.Management.Timeout,
		})
		if err != nil {
			log.Printf("messaging management client init failed: %v", err)
		} else {
			messagingSvc = messaging.NewService(cfg.Messaging, rabbitClient)
			log.Printf("  rabbitmq management url: %s", cfg.Messaging.Management.URL)
		}
	}

	router := api.NewRouter(cfg, verifier, videoClient, blogClient, timelapseClient, pubClient, queueManager, messagingSvc)
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.Blog.LLMTimeout + (2 * time.Minute),
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func validateAuthConfig(cfg config.AuthConfig) error {
	if cfg.Disabled {
		return nil
	}
	if strings.TrimSpace(cfg.IssuerURL) == "" {
		return errors.New("AUTH_ISSUER_URL is required unless AUTH_DISABLED=true")
	}
	if strings.TrimSpace(cfg.Audience) == "" {
		return errors.New("AUTH_AUDIENCE is required unless AUTH_DISABLED=true")
	}
	if strings.TrimSpace(cfg.RequiredRole) == "" {
		return errors.New("AUTH_REQUIRED_ROLE cannot be empty")
	}
	return nil
}
