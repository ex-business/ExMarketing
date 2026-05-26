package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/httpapi"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/postgres"
	"github.com/ex-stack/ExMarketing/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repository, closeRepository := buildRepository(ctx, logger)
	defer closeRepository()

	service := engine.NewService(repository)
	openAPIEventConfig, err := httpapi.LoadOpenAPIEventConfigFromEnv(logger)
	if err != nil {
		logger.Error("openapi event config invalid", "error", err)
		os.Exit(1)
	}
	webhookConfig, err := httpapi.LoadWebhookReceiverConfigFromEnv()
	if err != nil {
		logger.Error("webhook receiver config invalid", "error", err)
		os.Exit(1)
	}
	handler := httpapi.NewHandlerWithConfigs(service, openAPIEventConfig, webhookConfig)
	if env("EX_MARKETING_OUTBOX_WORKER", "false") == "true" {
		go worker.NewOutboxWorker(repository, logger).Run(ctx)
		logger.Info("outbox worker enabled")
	}

	server := &http.Server{
		Addr:              env("EX_MARKETING_ADDR", ":8080"),
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("exmarketing listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}

func buildRepository(ctx context.Context, logger *slog.Logger) (marketing.Repository, func()) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		logger.Info("using in-memory repository")
		return marketing.NewMemoryRepository(), func() {}
	}

	repository, err := postgres.NewRepository(ctx, databaseURL)
	if err != nil {
		logger.Error("postgres repository init failed", "error", err)
		os.Exit(1)
	}
	logger.Info("using postgres repository")
	return repository, func() {
		if err := repository.Close(); err != nil {
			logger.Warn("postgres close failed", "error", err)
		}
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
