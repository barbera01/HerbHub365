package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"HerbHub365/services/herbhub-video/internal/api"
	"HerbHub365/services/herbhub-video/internal/config"
	"HerbHub365/services/herbhub-video/internal/video"
)

//go:embed all:web
var webContent embed.FS

func main() {
	cfg := config.Load()

	log.Printf("herbhub-video starting on %s", cfg.ListenAddr)
	log.Printf("  posts dir:    %s", cfg.Post.PostsDir)
	log.Printf("  output dir:   %s", cfg.OutputDir)
	log.Printf("  narrator API: %s", cfg.NarratorURL)

	videoClient := video.NewClient(cfg)

	// Serve embedded frontend files from web/ subdirectory.
	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		log.Fatalf("embedded web fs: %v", err)
	}

	router := api.NewRouter(cfg, videoClient, http.FS(webFS))

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
