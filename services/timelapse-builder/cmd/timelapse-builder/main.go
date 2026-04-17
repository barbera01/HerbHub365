package main

import (
	"log"
	"net/http"
	"os"
	"syscall"

	"HerbHub365/services/timelapse-builder/internal/api"
	"HerbHub365/services/timelapse-builder/internal/config"
	"HerbHub365/services/timelapse-builder/internal/job"
)

func main() {
	mode := os.Getenv("TIMELAPSE_MODE")
	if mode == "" {
		mode = "server"
	}

	// Legacy batch modes: replace this process with the bash entrypoint.
	if mode == "once" || mode == "loop" {
		log.Printf("TIMELAPSE_MODE=%s — delegating to bash entrypoint", mode)
		if err := syscall.Exec("/usr/local/bin/timelapse-entrypoint.sh", []string{"timelapse-entrypoint.sh"}, os.Environ()); err != nil {
			log.Fatalf("exec entrypoint: %v", err)
		}
		return
	}

	cfg := config.Load()

	log.Printf("timelapse-builder server starting on %s", cfg.ListenAddr)
	log.Printf("  input dir:  %s", cfg.InputDir)
	log.Printf("  output dir: %s", cfg.OutputDir)

	tracker := job.NewTracker()
	router := api.NewRouter(cfg, tracker)

	if err := http.ListenAndServe(cfg.ListenAddr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}
