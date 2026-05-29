package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
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
	var workerDone <-chan struct{}
	if env("EX_MARKETING_OUTBOX_WORKER", "false") == "true" {
		outboxWorker := worker.NewOutboxWorker(repository, logger)
		mqConfig, mqConfigured, err := worker.LoadMQRuntimeConfigFromEnv()
		if err != nil {
			logger.Error("mq sender config invalid", "error", err)
			os.Exit(1)
		}
		if mqConfigured {
			mqSender, err := worker.NewRuntimeMQSender(mqConfig, nil)
			if err != nil {
				logger.Error("mq sender init failed", "error", err)
				os.Exit(1)
			}
			outboxWorker.SetMQSender(mqSender)
			logger.Info("mq sender runtime config loaded")
		}
		done := make(chan struct{})
		workerDone = done
		go func() {
			defer close(done)
			outboxWorker.Run(ctx)
		}()
		logger.Info("outbox worker enabled")
	}

	server := &http.Server{
		Addr:              env("EX_MARKETING_ADDR", ":8080"),
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		logger.Error("server listen failed", "addr", server.Addr, "error", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("exmarketing listening", "addr", listener.Addr().String(), "configuredAddr", server.Addr)
		errCh <- server.Serve(listener)
	}()

	shutdownRequested := false
	select {
	case <-ctx.Done():
		shutdownRequested = true
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
	if workerDone != nil && shutdownRequested {
		<-workerDone
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
