// Mega Magic Project - Main Entry Point
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"mega-magic/internal/config"
	"mega-magic/internal/server"
	"mega-magic/pkg/logger"
)

func main() {
	// Initialize structured logger
	log := logger.New("mega-magic", "development")
	defer log.Sync()
	sugar := log.Sugar()

	sugar.Info("Starting Mega Magic Project SaaS Backend")

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		sugar.Fatalw("Failed to load configuration", "error", err)
	}

	// Create application server
	srv, err := server.New(cfg, log)
	if err != nil {
		sugar.Fatalw("Failed to create server", "error", err)
	}

	// Start server in goroutine
	go func() {
		if err := srv.Start(); err != nil {
			sugar.Errorw("Server error", "error", err)
		}
	}()

	sugar.Infow("Server started", "port", cfg.Server.Port)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	sugar.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		sugar.Errorw("Server forced to shutdown", "error", err)
	}

	sugar.Info("Server exited gracefully")
}