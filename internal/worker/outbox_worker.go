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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/mqtarget"
)

type OutboxWorker struct {
	repository marketing.Repository
	client     *http.Client
	mqSender   MQSender
	logger     *slog.Logger
	interval   time.Duration
	batchSize  int
	maxRetries int
	workerID   string

	drainTimeout        time.Duration
	staleClaimThreshold time.Duration
	now                 func() time.Time
	jitter              func() float64
	statsMu             sync.Mutex
	stats               OutboxWorkerStats
}

func NewOutboxWorker(repository marketing.Repository, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{
		repository: repository,
		client: &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		logger:     logger,
		interval:   2 * time.Second,
		batchSize:  50,
		maxRetries: 5,
		workerID:   defaultWorkerID(),

		drainTimeout:        30 * time.Second,
		staleClaimThreshold: 5 * time.Minute,
		now:                 func() time.Time { return time.Now().UTC() },
		jitter:              func() float64 { return 0.8 + rand.Float64()*0.4 },
	}
}

type OutboxWorkerStats struct {
	Started              bool
	Draining             bool
	InFlight             int
	ClaimAttempts        int64
	ClaimSuccesses       int64
	ClaimEmptyBatches    int64
	ClaimConflicts       int64
	DeliverySuccesses    int64
	RetryableFailures    int64
	NonRetryableFailures int64
	DLQEntries           int64
	StaleRecoveries      int64
	StaleRecoveryErrors  int64
	DLQCounts            map[string]int
	ClaimDelaySamples    int64
	ClaimDelayTotal      time.Duration
	ClaimDelayMax        time.Duration
	LastError            string
}

type outboxProductionRepository interface {
	RecoverStaleOutbox(ctx context.Context, staleBefore time.Time, limit int) ([]marketing.OutboxMessage, error)
	CountOutboxDLQ(ctx context.Context) ([]marketing.OutboxDLQCount, error)
}

type MQSender interface {
	Send(ctx context.Context, message MQMessage) error
}

type MQMessage struct {
	Provider    string
	Destination string
	TargetRef   string
	Key         string
	Headers     map[string]string
	Payload     []byte
}

func (w *OutboxWorker) SetMQSender(sender MQSender) {
	w.mqSender = sender
}

func (w *OutboxWorker) Run(ctx context.Context) {
	if w.logger == nil {
		w.logger = slog.Default()
	}
	processCtx, cancelProcess := context.WithCancel(context.Background())
	defer cancelProcess()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.runLoop(processCtx, ctx)
	}()

	select {
	case <-ctx.Done():
		w.setDraining(true)
		w.logger.Info("outbox worker shutdown requested", "worker_id", w.workerID, "event", "shutdown_requested", "drain_timeout_ms", w.drainTimeout.Milliseconds())
	case <-done:
		return
	}

	timer := time.NewTimer(w.drainTimeout)
	defer timer.Stop()
	select {
	case <-done:
		w.setDraining(false)
		w.logger.Info("outbox worker drain completed", "worker_id", w.workerID, "event", "drain_completed")
	case <-timer.C:
		w.logger.Warn("outbox worker drain timeout", "worker_id", w.workerID, "event", "drain_timeout", "in_flight", w.Snapshot().InFlight)
		cancelProcess()
		<-done
	}
	w.setDraining(false)
	w.logger.Info("outbox worker stopped", "worker_id", w.workerID, "event", "worker_stopped")
}

func (w *OutboxWorker) runLoop(processCtx context.Context, stopCtx context.Context) {
	w.setStarted(true)
	w.logger.Info("outbox worker started",
		"worker_id", w.workerID,
		"event", "worker_started",
		"poll_interval_ms", w.interval.Milliseconds(),
		"batch_size", w.batchSize,
		"stale_claim_threshold_ms", w.staleClaimThreshold.Milliseconds(),
		"drain_timeout_ms", w.drainTimeout.Milliseconds(),
	)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-processCtx.Done():
			return
		default:
		}
		w.tick(processCtx)
		select {
		case <-stopCtx.Done():
			w.logger.Info("outbox worker claim stopped", "worker_id", w.workerID, "event", "claim_stopped")
			return
		case <-processCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *OutboxWorker) tick(ctx context.Context) {
	w.recoverStaleOutbox(ctx)
	w.refreshDLQCounts(ctx)
	w.addClaimAttempt()
	messages, err := w.repository.ListPendingOutbox(ctx, w.batchSize)
	if err != nil {
		w.recordError(err)
		w.logger.Warn("list outbox failed", "worker_id", w.workerID, "event", "claim_failed", "error", err)
		return
	}
	if len(messages) == 0 {
		w.addEmptyBatch()
		w.logger.Debug("outbox worker empty batch", "worker_id", w.workerID, "event", "claim_empty")
		return
	}
	w.addClaimSuccesses(len(messages))
	for _, message := range messages {
		w.recordClaimDelay(message)
		w.addInFlight(1)
		w.logger.Info("outbox claimed",
			"worker_id", w.workerID,
			"event", "outbox_claimed",
			"tenant_id", message.TenantID,
			"outbox_id", message.ID,
			"target_channel", message.TargetChannel,
			"target_ref", redactedTargetRef(message.TargetRef),
			"message_type", message.MessageType,
			"send_status_after", marketing.OutboxSending,
			"retry_count", message.RetryCount,
			"next_retry_at", message.NextRetryAt,
			"claim_delay_ms", claimDelay(message, w.now()).Milliseconds(),
		)
		w.processMessage(ctx, message)
		w.addInFlight(-1)
		if ctx.Err() != nil {
			return
		}
	}
}

func (w *OutboxWorker) processMessage(ctx context.Context, message marketing.OutboxMessage) {
	startedAt := w.now()
	commandID := rewardCommandID(message)
	if commandID != "" {
		if err := w.repository.UpdateGrantOrderByCommandID(ctx, message.TenantID, commandID, marketing.GrantSending, ""); err != nil {
			w.addClaimConflict()
			w.logger.Warn("advance grant order to sending failed", "worker_id", w.workerID, "event", "grant_sending_failed", "tenant_id", message.TenantID, "outbox_id", message.ID, "command_id", commandID, "error", err)
			return
		}
	}
	if err := w.deliver(ctx, message); err != nil {
		deliveryErr := classifyDeliveryError(err)
		retryCount := message.RetryCount + 1
		errorMessage := sanitizeError(deliveryErr.Error())
		if !deliveryErr.retryable || retryCount >= w.maxRetries {
			_ = w.repository.MarkOutboxDLQ(ctx, message.ID, retryCount, errorMessage)
			_ = w.recordOutboxAudit(ctx, message, "OUTBOX_ENTER_DLQ", retryCount, errorMessage)
			if commandID != "" {
				_ = w.repository.UpdateGrantOrderByCommandID(ctx, message.TenantID, commandID, marketing.GrantRetry, errorMessage)
			}
			w.addDLQEntry()
			w.addNonRetryableFailure()
			w.logger.Warn("outbox moved to dlq", w.logAttrs(message, marketing.OutboxDLQ, retryCount, startedAt, errorMessage, "outbox_dlq")...)
			return
		}
		nextRetry := w.nextRetryAt(retryCount, deliveryErr.retryAfter)
		_ = w.repository.MarkOutboxFailed(ctx, message.ID, retryCount, nextRetry, errorMessage)
		_ = w.recordOutboxAudit(ctx, message, "OUTBOX_DELIVERY_FAILED", retryCount, errorMessage)
		if commandID != "" {
			_ = w.repository.UpdateGrantOrderByCommandID(ctx, message.TenantID, commandID, marketing.GrantRetry, errorMessage)
		}
		w.addRetryableFailure()
		w.logger.Warn("outbox delivery failed", w.logAttrs(message, marketing.OutboxFailed, retryCount, startedAt, errorMessage, "outbox_failed")...)
		return
	}
	_ = w.repository.MarkOutboxSent(ctx, message.ID)
	w.addDeliverySuccess()
	w.logger.Info("outbox sent", w.logAttrs(message, marketing.OutboxSent, message.RetryCount, startedAt, "", "outbox_sent")...)
}

func (w *OutboxWorker) recordOutboxAudit(ctx context.Context, message marketing.OutboxMessage, action string, retryCount int, errMessage string) error {
	payload, _ := json.Marshal(map[string]any{
		"outboxId":      message.ID,
		"beforeStatus":  message.SendStatus,
		"targetChannel": message.TargetChannel,
		"targetRef":     message.TargetRef,
		"retryCount":    retryCount,
		"errorMessage":  errMessage,
		"commandId":     rewardCommandID(message),
		"source": map[string]any{
			"component": "outbox_worker",
		},
	})
	return w.repository.CreateAudit(ctx, marketing.AuditRecord{
		ID:         marketing.NewID("audit"),
		TenantID:   message.TenantID,
		Actor:      "outbox_worker",
		Action:     action,
		Resource:   "OUTBOX",
		ResourceID: message.ID,
		Payload:    payload,
		CreatedAt:  w.now(),
	})
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
	case marketing.GrantMQ:
		if !json.Valid(message.Payload) {
			return deliveryError{retryable: false, message: "payload_json is invalid JSON"}
		}
		target, err := mqtarget.Parse(message.TargetRef)
		if err != nil {
			return deliveryError{retryable: false, message: fmt.Sprintf("targetRef is invalid for MQ: %v", err)}
		}
		if w.mqSender == nil {
			return deliveryError{retryable: true, message: fmt.Sprintf("mq sender is not configured for provider %s destination %s", target.Provider, target.Destination)}
		}
		mqMessage := buildMQMessage(message, target)
		if err := w.mqSender.Send(ctx, mqMessage); err != nil {
			return err
		}
		return nil
	case marketing.GrantManualFile:
		return deliveryError{retryable: false, message: fmt.Sprintf("unsupported target channel %s", message.TargetChannel)}
	default:
		return deliveryError{retryable: false, message: fmt.Sprintf("unsupported target channel %s", message.TargetChannel)}
	}
}

func buildMQMessage(message marketing.OutboxMessage, target mqtarget.Target) MQMessage {
	headers := map[string]string{
		"tenantId":      message.TenantID,
		"messageType":   message.MessageType,
		"targetChannel": string(message.TargetChannel),
	}
	var command marketing.RewardCommand
	if err := json.Unmarshal(message.Payload, &command); err == nil {
		if command.CommandID != "" {
			headers["commandId"] = command.CommandID
		}
		if command.IdempotencyKey != "" {
			headers["idempotencyKey"] = command.IdempotencyKey
		}
		if command.TenantID != "" {
			headers["tenantId"] = command.TenantID
		}
	}
	key := firstNonEmpty(headers["commandId"], headers["idempotencyKey"], message.ID)
	return MQMessage{
		Provider:    target.Provider,
		Destination: target.Destination,
		TargetRef:   message.TargetRef,
		Key:         key,
		Headers:     headers,
		Payload:     append([]byte(nil), message.Payload...),
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

func (w *OutboxWorker) Snapshot() OutboxWorkerStats {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	snapshot := w.stats
	if w.stats.DLQCounts != nil {
		snapshot.DLQCounts = make(map[string]int, len(w.stats.DLQCounts))
		for key, value := range w.stats.DLQCounts {
			snapshot.DLQCounts[key] = value
		}
	}
	return snapshot
}

func (w *OutboxWorker) recoverStaleOutbox(ctx context.Context) {
	repository, ok := w.repository.(outboxProductionRepository)
	if !ok {
		return
	}
	staleBefore := w.now().Add(-w.staleClaimThreshold)
	messages, err := repository.RecoverStaleOutbox(ctx, staleBefore, w.batchSize)
	if err != nil {
		w.addStaleRecoveryError(err)
		w.logger.Warn("stale outbox recovery failed", "worker_id", w.workerID, "event", "stale_recovery_failed", "error", err)
		return
	}
	if len(messages) == 0 {
		return
	}
	w.addStaleRecoveries(len(messages))
	for _, message := range messages {
		w.logger.Warn("stale outbox recovered",
			"worker_id", w.workerID,
			"event", "stale_recovered",
			"tenant_id", message.TenantID,
			"outbox_id", message.ID,
			"target_channel", message.TargetChannel,
			"send_status_before", marketing.OutboxSending,
			"send_status_after", marketing.OutboxPending,
			"stale_duration_ms", w.now().Sub(message.UpdatedAt).Milliseconds(),
		)
	}
}

func (w *OutboxWorker) refreshDLQCounts(ctx context.Context) {
	repository, ok := w.repository.(outboxProductionRepository)
	if !ok {
		return
	}
	counts, err := repository.CountOutboxDLQ(ctx)
	if err != nil {
		w.recordError(err)
		w.logger.Warn("dlq count failed", "worker_id", w.workerID, "event", "dlq_count_failed", "error", err)
		return
	}
	next := make(map[string]int, len(counts))
	for _, count := range counts {
		key := dlqCountKey(count.TenantID, count.TargetChannel)
		next[key] = count.Count
		if count.Count > 0 {
			severity := "warning"
			if count.Count > 10 {
				severity = "high"
			}
			w.logger.Warn("outbox dlq backlog",
				"worker_id", w.workerID,
				"event", "dlq_backlog",
				"tenant_id", count.TenantID,
				"target_channel", count.TargetChannel,
				"dlq_count", count.Count,
				"severity", severity,
			)
		}
	}
	w.statsMu.Lock()
	w.stats.DLQCounts = next
	w.statsMu.Unlock()
}

func (w *OutboxWorker) logAttrs(message marketing.OutboxMessage, statusAfter marketing.OutboxStatus, retryCount int, startedAt time.Time, errMessage string, event string) []any {
	return []any{
		"worker_id", w.workerID,
		"event", event,
		"tenant_id", message.TenantID,
		"outbox_id", message.ID,
		"target_channel", message.TargetChannel,
		"target_ref", redactedTargetRef(message.TargetRef),
		"message_type", message.MessageType,
		"send_status_before", message.SendStatus,
		"send_status_after", statusAfter,
		"retry_count", retryCount,
		"next_retry_at", message.NextRetryAt,
		"claim_delay_ms", claimDelay(message, startedAt).Milliseconds(),
		"delivery_duration_ms", w.now().Sub(startedAt).Milliseconds(),
		"error_class", errorClass(errMessage),
		"error_message", errMessage,
	}
}

func (w *OutboxWorker) setStarted(started bool) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.stats.Started = started
}

func (w *OutboxWorker) setDraining(draining bool) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.stats.Draining = draining
}

func (w *OutboxWorker) addInFlight(delta int) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.stats.InFlight += delta
	if w.stats.InFlight < 0 {
		w.stats.InFlight = 0
	}
}

func (w *OutboxWorker) addClaimAttempt() {
	w.statsMu.Lock()
	w.stats.ClaimAttempts++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addClaimSuccesses(count int) {
	w.statsMu.Lock()
	w.stats.ClaimSuccesses += int64(count)
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addEmptyBatch() {
	w.statsMu.Lock()
	w.stats.ClaimEmptyBatches++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addClaimConflict() {
	w.statsMu.Lock()
	w.stats.ClaimConflicts++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addDeliverySuccess() {
	w.statsMu.Lock()
	w.stats.DeliverySuccesses++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addRetryableFailure() {
	w.statsMu.Lock()
	w.stats.RetryableFailures++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addNonRetryableFailure() {
	w.statsMu.Lock()
	w.stats.NonRetryableFailures++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addDLQEntry() {
	w.statsMu.Lock()
	w.stats.DLQEntries++
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addStaleRecoveries(count int) {
	w.statsMu.Lock()
	w.stats.StaleRecoveries += int64(count)
	w.statsMu.Unlock()
}

func (w *OutboxWorker) addStaleRecoveryError(err error) {
	w.statsMu.Lock()
	w.stats.StaleRecoveryErrors++
	w.stats.LastError = sanitizeError(err.Error())
	w.statsMu.Unlock()
}

func (w *OutboxWorker) recordError(err error) {
	w.statsMu.Lock()
	w.stats.LastError = sanitizeError(err.Error())
	w.statsMu.Unlock()
}

func (w *OutboxWorker) recordClaimDelay(message marketing.OutboxMessage) {
	delay := claimDelay(message, w.now())
	w.statsMu.Lock()
	w.stats.ClaimDelaySamples++
	w.stats.ClaimDelayTotal += delay
	if delay > w.stats.ClaimDelayMax {
		w.stats.ClaimDelayMax = delay
	}
	w.statsMu.Unlock()
}

func claimDelay(message marketing.OutboxMessage, now time.Time) time.Duration {
	eligibleAt := message.CreatedAt
	if message.NextRetryAt != nil && !message.NextRetryAt.After(now) {
		eligibleAt = *message.NextRetryAt
	}
	if eligibleAt.IsZero() || eligibleAt.After(now) {
		return 0
	}
	return now.Sub(eligibleAt)
}

func redactedTargetRef(targetRef string) string {
	if parsed, err := url.Parse(targetRef); err == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	if len(targetRef) > 128 {
		return targetRef[:128]
	}
	return targetRef
}

func errorClass(message string) string {
	if message == "" {
		return ""
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "status"):
		return "http_status"
	case strings.Contains(lower, "json"):
		return "payload"
	default:
		return "delivery"
	}
}

func dlqCountKey(tenantID string, channel marketing.GrantChannel) string {
	return tenantID + "|" + string(channel)
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
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
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "signature") {
			fields[i] = "<redacted>"
		}
	}
	message = strings.Join(fields, " ")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func rewardCommandID(message marketing.OutboxMessage) string {
	if message.MessageType != "REWARD_COMMAND" {
		return ""
	}
	var command marketing.RewardCommand
	if err := json.Unmarshal(message.Payload, &command); err != nil {
		return ""
	}
	return strings.TrimSpace(command.CommandID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
