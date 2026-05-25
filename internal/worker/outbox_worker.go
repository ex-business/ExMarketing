package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
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
	now        func() time.Time
	jitter     func() float64
}

func NewOutboxWorker(repository marketing.Repository, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{
		repository: repository,
		client:     &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
		interval:   2 * time.Second,
		maxRetries: 5,
		now:        func() time.Time { return time.Now().UTC() },
		jitter:     func() float64 { return 0.8 + rand.Float64()*0.4 },
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
			deliveryErr := classifyDeliveryError(err)
			retryCount := message.RetryCount + 1
			errorMessage := sanitizeError(deliveryErr.Error())
			if !deliveryErr.retryable || retryCount >= w.maxRetries {
				_ = w.repository.MarkOutboxDLQ(ctx, message.ID, retryCount, errorMessage)
				w.logger.Warn("outbox moved to dlq", "id", message.ID, "error", err)
				continue
			}
			nextRetry := w.nextRetryAt(retryCount, deliveryErr.retryAfter)
			_ = w.repository.MarkOutboxFailed(ctx, message.ID, retryCount, nextRetry, errorMessage)
			w.logger.Warn("outbox delivery failed", "id", message.ID, "retryCount", retryCount, "error", err)
			continue
		}
		_ = w.repository.MarkOutboxSent(ctx, message.ID)
	}
}

func (w *OutboxWorker) deliver(ctx context.Context, message marketing.OutboxMessage) error {
	switch message.TargetChannel {
	case marketing.GrantHTTP, marketing.GrantWebhook:
		if !json.Valid(message.Payload) {
			return deliveryError{retryable: false, message: "payload_json is invalid JSON"}
		}
		if !validHTTPURL(message.TargetRef) {
			return deliveryError{retryable: false, message: fmt.Sprintf("targetRef must be http(s) URL for %s", message.TargetChannel)}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, message.TargetRef, bytes.NewReader(message.Payload))
		if err != nil {
			return deliveryError{retryable: false, message: err.Error()}
		}
		req.Header.Set("content-type", "application/json")
		resp, err := w.client.Do(req)
		if err != nil {
			return deliveryError{retryable: true, message: err.Error()}
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err := deliveryError{retryable: retryableStatus(resp.StatusCode), message: fmt.Sprintf("target returned status %d", resp.StatusCode)}
			if resp.StatusCode == http.StatusTooManyRequests {
				err.retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), w.now())
			}
			return err
		}
		return nil
	case marketing.GrantMQ, marketing.GrantManualExport:
		return nil
	default:
		return deliveryError{retryable: false, message: fmt.Sprintf("unsupported target channel %s", message.TargetChannel)}
	}
}

func (w *OutboxWorker) nextRetryAt(retryCount int, retryAfter *time.Duration) time.Time {
	now := w.now()
	if retryAfter != nil {
		return now.Add(*retryAfter)
	}
	delay := 30 * time.Second
	for i := 1; i < retryCount; i++ {
		delay *= 2
		if delay >= 30*time.Minute {
			delay = 30 * time.Minute
			break
		}
	}
	jitter := w.jitter()
	if jitter <= 0 {
		jitter = 1
	}
	return now.Add(time.Duration(float64(delay) * jitter))
}

type deliveryError struct {
	retryable  bool
	retryAfter *time.Duration
	message    string
}

func (e deliveryError) Error() string {
	return e.message
}

func classifyDeliveryError(err error) deliveryError {
	if err == nil {
		return deliveryError{}
	}
	if deliveryErr, ok := err.(deliveryError); ok {
		return deliveryErr
	}
	return deliveryError{retryable: true, message: err.Error()}
}

func validHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func retryableStatus(status int) bool {
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status <= 599
}

func parseRetryAfter(value string, now time.Time) *time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		duration := time.Duration(seconds) * time.Second
		return &duration
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		duration := at.Sub(now)
		return &duration
	}
	return nil
}

func sanitizeError(message string) string {
	message = strings.TrimSpace(message)
	fields := strings.Fields(message)
	for i, field := range fields {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "signature") {
			fields[i] = "<redacted>"
		}
	}
	message = strings.Join(fields, " ")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
