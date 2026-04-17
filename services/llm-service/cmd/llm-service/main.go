package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"HerbHub365/services/llm-service/internal/config"
	"HerbHub365/services/llm-service/internal/llm"
	"HerbHub365/services/llm-service/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	client := llm.NewClient(cfg.LLM)
	handler := server.NewHandler(client, cfg.LLM.RequestTimeout)

	mux := http.NewServeMux()
	handler.Register(mux)

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Printf("shutting down")
		_ = httpServer.Shutdown(context.Background())
	}()

	log.Printf("llm-service listening on %s (model: %s provider: %s)", cfg.ListenAddr, cfg.LLM.Model, cfg.LLM.Provider)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
