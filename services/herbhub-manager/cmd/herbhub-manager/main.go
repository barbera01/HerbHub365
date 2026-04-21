package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"context"
	"os/signal"
	"syscall"

	"HerbHub365/services/herbhub-manager/internal/api"
	"HerbHub365/services/herbhub-manager/internal/blogpost"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/publisher"
	"HerbHub365/services/herbhub-manager/internal/queue"
	"HerbHub365/services/herbhub-manager/internal/timelapse"
	"HerbHub365/services/herbhub-manager/internal/video"
)

//go:embed all:web
var webContent embed.FS

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

	videoClient := video.NewClient(cfg)
	blogClient := blogpost.NewClient(cfg.Blog)
	timelapseClient := timelapse.NewClient(cfg.Timelapse.ServiceURL, cfg.Timelapse.Timeout)

	var pubClient *publisher.Client
	if publisher.Enabled(cfg.RabbitMQ.URL) {
		var err error
		pubClient, err = publisher.NewClient(cfg.RabbitMQ.URL, cfg.RabbitMQ.Queue)
		if err != nil {
			log.Printf("publisher client: %v (YouTube publishing disabled)", err)
		} else {
			log.Printf("  rabbitmq:     %s (queue: %s)", cfg.RabbitMQ.URL, cfg.RabbitMQ.Queue)
		}
	}

	queueManager := queue.NewManager(videoClient, cfg.Post.PostsDir)
	go queueManager.Run(ctx)

	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		log.Fatalf("embedded web fs: %v", err)
	}

	router := api.NewRouter(cfg, videoClient, blogClient, timelapseClient, pubClient, queueManager, http.FS(webFS))

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
