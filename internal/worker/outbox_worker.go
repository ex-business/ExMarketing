package worker

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type OutboxWorker struct {
	repository marketing.Repository
	client     *http.Client
	logger     *slog.Logger
	interval   time.Duration
	maxRetries int
}

func NewOutboxWorker(repository marketing.Repository, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{
		repository: repository,
		client:     &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
		interval:   2 * time.Second,
		maxRetries: 3,
	}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *OutboxWorker) tick(ctx context.Context) {
	messages, err := w.repository.ListPendingOutbox(ctx, 50)
	if err != nil {
		w.logger.Warn("list outbox failed", "error", err)
		return
	}
	for _, message := range messages {
		if err := w.deliver(ctx, message); err != nil {
			retryCount := message.RetryCount + 1
			if retryCount >= w.maxRetries {
				_ = w.repository.MarkOutboxDLQ(ctx, message.ID, err.Error())
				w.logger.Warn("outbox moved to dlq", "id", message.ID, "error", err)
				continue
			}
			nextRetry := time.Now().UTC().Add(time.Duration(retryCount*retryCount) * time.Second)
			_ = w.repository.MarkOutboxFailed(ctx, message.ID, retryCount, nextRetry, err.Error())
			w.logger.Warn("outbox delivery failed", "id", message.ID, "retryCount", retryCount, "error", err)
			continue
		}
		_ = w.repository.MarkOutboxSent(ctx, message.ID)
	}
}

func (w *OutboxWorker) deliver(ctx context.Context, message marketing.OutboxMessage) error {
	switch message.TargetChannel {
	case marketing.GrantHTTP, marketing.GrantWebhook:
		if !strings.HasPrefix(message.TargetRef, "http://") && !strings.HasPrefix(message.TargetRef, "https://") {
			return fmt.Errorf("targetRef must be http(s) URL for %s", message.TargetChannel)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, message.TargetRef, bytes.NewReader(message.Payload))
		if err != nil {
			return err
		}
		req.Header.Set("content-type", "application/json")
		resp, err := w.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("target returned status %d", resp.StatusCode)
		}
		return nil
	case marketing.GrantMQ, marketing.GrantManualExport:
		return fmt.Errorf("%s delivery is not configured", message.TargetChannel)
	default:
		return fmt.Errorf("unsupported target channel %s", message.TargetChannel)
	}
}
