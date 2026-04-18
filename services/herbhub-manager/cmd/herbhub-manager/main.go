package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"HerbHub365/services/herbhub-manager/internal/api"
	"HerbHub365/services/herbhub-manager/internal/blogpost"
	"HerbHub365/services/herbhub-manager/internal/config"
	"HerbHub365/services/herbhub-manager/internal/timelapse"
	"HerbHub365/services/herbhub-manager/internal/video"
)

//go:embed all:web
var webContent embed.FS

func main() {
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

	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		log.Fatalf("embedded web fs: %v", err)
	}

	router := api.NewRouter(cfg, videoClient, blogClient, timelapseClient, http.FS(webFS))

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
