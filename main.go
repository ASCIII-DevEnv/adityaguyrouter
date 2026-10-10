package main

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/server"
	"antigravity-gateway/internal/storage"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := config.LoadConfig()
	log.Printf("[Antigravity Gateway] Starting on port %d...", cfg.Port)

	st, err := storage.NewStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	client := provider.NewAntigravityClient(cfg)
	pool := provider.NewAccountPool(cfg, st, client)

	// Background ticker to refresh quotas every 60 seconds
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			pool.RefreshAllQuotas(ctx)
			cancel()
		}
	}()

	router := server.NewRouter(cfg, st, pool, client)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	log.Printf("[Antigravity Gateway] Ready at http://localhost:%d", cfg.Port)
	log.Printf("[Antigravity Gateway] OpenAI API: http://localhost:%d/v1/chat/completions", cfg.Port)
	log.Printf("[Antigravity Gateway] Anthropic API: http://localhost:%d/v1/messages", cfg.Port)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Antigravity Gateway] Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}
	log.Println("[Antigravity Gateway] Stopped cleanly.")
}
