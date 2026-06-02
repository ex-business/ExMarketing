package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestOutboxWorkerSendsHTTPAndMarksSent(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_sent", marketing.GrantHTTP, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if got := repo.messages[0].SendStatus; got != marketing.OutboxSent {
		t.Fatalf("expected SENT, got %s", got)
	}
}

func TestOutboxWorkerSendsWebhookAndMarksSent(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_webhook_sent", marketing.GrantWebhook, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("content-type"); got != "application/json" {
			t.Fatalf("expected JSON content-type, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if got := repo.messages[0].SendStatus; got != marketing.OutboxSent {
		t.Fatalf("expected SENT, got %s", got)
	}
}

func TestOutboxWorkerDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, server.URL+"/redirected", http.StatusFound)
	}))
	defer server.Close()

	repo := newWorkerTestRepo(outboxMessage("outbox_redirect", marketing.GrantHTTP, "{}"))
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	message := repo.messages[0]
	if redirected {
		t.Fatal("expected worker not to follow redirect")
	}
	if message.SendStatus != marketing.OutboxDLQ || !strings.Contains(message.ErrorMessage, "302") {
		t.Fatalf("expected 302 to move to DLQ with status error, got %+v", message)
	}
	if len(repo.audits) != 1 || repo.audits[0].Action != "OUTBOX_ENTER_DLQ" || repo.audits[0].Resource != "OUTBOX" {
		t.Fatalf("expected OUTBOX_ENTER_DLQ audit, got %+v", repo.audits)
	}
}

func TestOutboxWorkerAdvancesGrantOrderState(t *testing.T) {
	message := outboxMessage("outbox_grant_state", marketing.GrantHTTP, `{"commandId":"cmd_grant_state"}`)
	repo := newWorkerTestRepo(message)
	repo.grantStatuses["cmd_grant_state"] = marketing.GrantPending
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if got := repo.grantStatuses["cmd_grant_state"]; got != marketing.GrantRetry {
		t.Fatalf("expected grant order RETRY after delivery failure, got %s", got)
	}
	if got := repo.grantRetryCounts["cmd_grant_state"]; got != 1 {
		t.Fatalf("expected grant retry_count=1, got %d", got)
	}
}

func TestOutboxWorkerReleasesClaimWhenGrantCannotAdvance(t *testing.T) {
	message := outboxMessage("outbox_grant_conflict", marketing.GrantHTTP, `{"commandId":"cmd_grant_conflict"}`)
	repo := newWorkerTestRepo(message)
	repo.grantStatuses["cmd_grant_conflict"] = marketing.GrantSending

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	got := repo.messages[0]
	if got.SendStatus != marketing.OutboxFailed {
		t.Fatalf("expected outbox claim to be released as FAILED, got %+v", got)
	}
	if got.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", got.RetryCount)
	}
	if got.NextRetryAt == nil {
		t.Fatal("expected immediate retry timestamp")
	}
	if len(repo.audits) != 1 || repo.audits[0].Action != "OUTBOX_CLAIM_RELEASED" {
		t.Fatalf("expected OUTBOX_CLAIM_RELEASED audit, got %+v", repo.audits)
	}
}

func TestOutboxWorkerTimeoutIsRetryableFailure(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_timeout", marketing.GrantHTTP, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	worker := newTestWorker(repo)
	worker.client.Timeout = 5 * time.Millisecond
	worker.now = func() time.Time { return now }
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxFailed || message.RetryCount != 1 {
		t.Fatalf("expected timeout to be retryable FAILED, got %+v", message)
	}
	if !strings.Contains(strings.ToLower(message.ErrorMessage), "timeout") {
		t.Fatalf("expected timeout error message, got %q", message.ErrorMessage)
	}
	if len(repo.audits) != 1 || repo.audits[0].Action != "OUTBOX_DELIVERY_FAILED" || !strings.Contains(string(repo.audits[0].Payload), `"source"`) {
		t.Fatalf("expected OUTBOX_DELIVERY_FAILED audit with source, got %+v", repo.audits)
	}
}

func TestOutboxWorkerNetworkErrorIsRetryableFailure(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_network", marketing.GrantHTTP, "{}"))
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	worker := newTestWorker(repo)
	worker.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	worker.now = func() time.Time { return now }
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxFailed || message.RetryCount != 1 {
		t.Fatalf("expected network error to be retryable FAILED, got %+v", message)
	}
	if !strings.Contains(message.ErrorMessage, "connection refused") {
		t.Fatalf("expected network error message, got %q", message.ErrorMessage)
	}
}

func TestOutboxWorkerRetryableFailureUsesExponentialBackoff(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_retry", marketing.GrantWebhook, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	worker := newTestWorker(repo)
	worker.now = func() time.Time { return now }
	worker.jitter = func() float64 { return 1 }
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxFailed || message.RetryCount != 1 {
		t.Fatalf("expected FAILED retry_count=1, got status=%s retry=%d", message.SendStatus, message.RetryCount)
	}
	if message.NextRetryAt == nil || !message.NextRetryAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("expected next retry at +30s, got %v", message.NextRetryAt)
	}
}

func TestOutboxWorkerRetryAfterOverridesBackoff(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_429", marketing.GrantHTTP, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	worker := newTestWorker(repo)
	worker.now = func() time.Time { return now }
	worker.jitter = func() float64 { return 1 }
	worker.tick(context.Background())

	if got := repo.messages[0].NextRetryAt; got == nil || !got.Equal(now.Add(45*time.Second)) {
		t.Fatalf("expected retry-after +45s, got %v", got)
	}
}

func TestOutboxWorkerNonRetryableFailureMovesToDLQ(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_bad_request", marketing.GrantHTTP, "{}"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxDLQ || message.RetryCount != 1 {
		t.Fatalf("expected DLQ retry_count=1, got status=%s retry=%d", message.SendStatus, message.RetryCount)
	}
}

func TestOutboxWorkerMaxRetriesMovesToDLQ(t *testing.T) {
	message := outboxMessage("outbox_exhausted", marketing.GrantHTTP, "{}")
	message.RetryCount = 4
	repo := newWorkerTestRepo(message)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if repo.messages[0].SendStatus != marketing.OutboxDLQ || repo.messages[0].RetryCount != 5 {
		t.Fatalf("expected exhausted message in DLQ with retry_count=5, got %+v", repo.messages[0])
	}
}

func TestOutboxWorkerInvalidTargetMovesToDLQ(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_invalid_target", marketing.GrantHTTP, "{}"))
	repo.messages[0].TargetRef = "reward.commands"

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxDLQ || !strings.Contains(message.ErrorMessage, "http(s) URL") {
		t.Fatalf("expected invalid target to move to DLQ, got %+v", message)
	}
}

func TestOutboxWorkerInvalidPayloadMovesToDLQ(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_invalid_payload", marketing.GrantHTTP, "{"))

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxDLQ || !strings.Contains(message.ErrorMessage, "invalid JSON") {
		t.Fatalf("expected invalid payload to move to DLQ, got %+v", message)
	}
}

func TestOutboxWorkerSendsMQAndMarksSent(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_mq", marketing.GrantMQ, `{"tenantId":"tenant_001","commandId":"cmd_mq","idempotencyKey":"idem_mq"}`))
	repo.grantStatuses["cmd_mq"] = marketing.GrantPending
	worker := newTestWorker(repo)
	sender := &recordingMQSender{}
	worker.mqSender = sender
	worker.tick(context.Background())

	if repo.messages[0].SendStatus != marketing.OutboxSent {
		t.Fatalf("expected MQ message SENT, got %s", repo.messages[0].SendStatus)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one MQ message, got %d", len(sender.messages))
	}
	message := sender.messages[0]
	if message.Provider != "kafka" || message.Destination != "reward.commands" {
		t.Fatalf("expected kafka reward.commands target, got %+v", message)
	}
	if message.Key != "cmd_mq" || message.Headers["tenantId"] != "tenant_001" || message.Headers["idempotencyKey"] != "idem_mq" {
		t.Fatalf("expected command headers and key, got %+v", message)
	}
}

func TestOutboxWorkerMQSendFailureKeepsOutboxRetryable(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_mq_failure", marketing.GrantMQ, `{"commandId":"cmd_mq_failure"}`))
	repo.grantStatuses["cmd_mq_failure"] = marketing.GrantPending
	worker := newTestWorker(repo)
	worker.mqSender = &recordingMQSender{err: errors.New("broker unavailable token=secret")}
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxFailed || message.RetryCount != 1 {
		t.Fatalf("expected MQ failure to keep retryable outbox, got %+v", message)
	}
	if strings.Contains(message.ErrorMessage, "token=secret") {
		t.Fatalf("expected sensitive error to be redacted, got %q", message.ErrorMessage)
	}
}

func TestOutboxWorkerMQInvalidTargetMovesToDLQ(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_mq_invalid", marketing.GrantMQ, "{}"))
	repo.messages[0].TargetRef = "kafka://"
	worker := newTestWorker(repo)
	worker.mqSender = &recordingMQSender{}
	worker.tick(context.Background())

	message := repo.messages[0]
	if message.SendStatus != marketing.OutboxDLQ || !strings.Contains(message.ErrorMessage, "targetRef") {
		t.Fatalf("expected invalid MQ target to move to DLQ, got %+v", message)
	}
}

func TestOutboxWorkerSingleFailureDoesNotStopNextMessage(t *testing.T) {
	first := outboxMessage("outbox_first_failure", marketing.GrantHTTP, "{}")
	first.TargetRef = "http://127.0.0.1/fail"
	second := outboxMessage("outbox_second_success", marketing.GrantHTTP, "{}")
	second.TargetRef = "http://127.0.0.1/success"
	repo := newWorkerTestRepo(first, second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL + "/fail"
	repo.messages[1].TargetRef = server.URL + "/success"

	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if got := repo.messageStatus("outbox_first_failure"); got != marketing.OutboxFailed {
		t.Fatalf("expected first outbox FAILED, got %s", got)
	}
	if got := repo.messageStatus("outbox_second_success"); got != marketing.OutboxSent {
		t.Fatalf("expected second outbox SENT after first failure, got %s", got)
	}
	stats := worker.Snapshot()
	if stats.RetryableFailures != 1 || stats.DeliverySuccesses != 1 {
		t.Fatalf("expected one retryable failure and one success, got %+v", stats)
	}
}

func TestOutboxWorkerRunContinuesAfterEmptyBatch(t *testing.T) {
	repo := newWorkerTestRepo()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	worker := newTestWorker(repo)
	worker.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	waitFor(t, time.Second, func() bool {
		return worker.Snapshot().ClaimEmptyBatches > 0
	})
	message := outboxMessage("outbox_after_empty", marketing.GrantHTTP, "{}")
	message.TargetRef = server.URL
	repo.appendMessage(message)

	waitFor(t, time.Second, func() bool {
		return repo.messageStatus("outbox_after_empty") == marketing.OutboxSent
	})
	cancel()
	<-done
}

func TestOutboxWorkerShutdownDrainsInFlight(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_drain", marketing.GrantHTTP, "{}"))
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.interval = time.Hour
	worker.drainTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	<-requestStarted
	cancel()
	close(releaseRequest)
	<-done

	if got := repo.messageStatus("outbox_drain"); got != marketing.OutboxSent {
		t.Fatalf("expected in-flight outbox to be drained and marked SENT, got %s", got)
	}
	if stats := worker.Snapshot(); stats.InFlight != 0 || stats.DeliverySuccesses != 1 || stats.Draining {
		t.Fatalf("expected drained stats, got %+v", stats)
	}
}

func TestOutboxWorkerShutdownStopsClaimingNewMessages(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_stop_inflight", marketing.GrantHTTP, "{}"))
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.interval = time.Hour
	worker.drainTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	<-requestStarted
	cancel()
	newMessage := outboxMessage("outbox_stop_new", marketing.GrantHTTP, "{}")
	newMessage.TargetRef = server.URL
	repo.appendMessage(newMessage)
	close(releaseRequest)
	<-done

	if got := repo.messageStatus("outbox_stop_inflight"); got != marketing.OutboxSent {
		t.Fatalf("expected in-flight outbox SENT, got %s", got)
	}
	if got := repo.messageStatus("outbox_stop_new"); got != marketing.OutboxPending {
		t.Fatalf("expected post-stop outbox to remain PENDING, got %s", got)
	}
}

func TestOutboxWorkerRecoversStaleClaimsAndSnapshotsDLQ(t *testing.T) {
	stale := outboxMessage("outbox_stale", marketing.GrantHTTP, "{}")
	stale.SendStatus = marketing.OutboxSending
	stale.UpdatedAt = stale.UpdatedAt.Add(-10 * time.Minute)
	dlq := outboxMessage("outbox_dlq_count", marketing.GrantHTTP, "{}")
	dlq.SendStatus = marketing.OutboxDLQ
	repo := newWorkerTestRepo(stale, dlq)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repo.messages[0].TargetRef = server.URL

	worker := newTestWorker(repo)
	worker.staleClaimThreshold = 5 * time.Minute
	worker.tick(context.Background())

	if got := repo.messageStatus("outbox_stale"); got != marketing.OutboxSent {
		t.Fatalf("expected stale SENDING outbox to recover and send, got %s", got)
	}
	stats := worker.Snapshot()
	if stats.StaleRecoveries != 1 {
		t.Fatalf("expected one stale recovery, got %+v", stats)
	}
	if got := stats.DLQCounts["tenant_001|HTTP"]; got != 1 {
		t.Fatalf("expected tenant/channel DLQ count in snapshot, got %+v", stats.DLQCounts)
	}
	if stats.ClaimDelaySamples != 1 || stats.ClaimDelayMax <= 0 {
		t.Fatalf("expected claim delay sample, got %+v", stats)
	}
}

func TestOutboxWorkerStructuredLogsRedactSensitiveFields(t *testing.T) {
	var logs bytes.Buffer
	repo := newWorkerTestRepo(outboxMessage("outbox_sensitive_logs", marketing.GrantMQ, `{"commandId":"cmd_sensitive"}`))
	repo.grantStatuses["cmd_sensitive"] = marketing.GrantPending
	worker := NewOutboxWorker(repo, slog.New(slog.NewJSONHandler(&logs, nil)))
	worker.mqSender = &recordingMQSender{err: errors.New("Authorization=Bearer abc Cookie=session token=secret signature=raw")}
	worker.tick(context.Background())

	var sawFailure bool
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		if record["event"] != "outbox_failed" {
			continue
		}
		sawFailure = true
		for _, field := range []string{"worker_id", "event", "tenant_id", "outbox_id", "target_channel", "target_ref", "message_type", "send_status_before", "send_status_after", "retry_count", "next_retry_at", "claim_delay_ms", "delivery_duration_ms", "error_class", "error_message"} {
			if _, ok := record[field]; !ok {
				t.Fatalf("expected structured log field %q in %+v", field, record)
			}
		}
		errorMessage, _ := record["error_message"].(string)
		lowerError := strings.ToLower(errorMessage)
		if strings.Contains(lowerError, "authorization") || strings.Contains(lowerError, "cookie") || strings.Contains(lowerError, "token=secret") || strings.Contains(lowerError, "signature=raw") {
			t.Fatalf("expected sensitive error details to be redacted, got %q", errorMessage)
		}
		targetRef, _ := record["target_ref"].(string)
		if strings.Contains(targetRef, "?") || strings.Contains(strings.ToLower(targetRef), "secret") {
			t.Fatalf("expected target_ref to be redacted, got %q", targetRef)
		}
	}
	if !sawFailure {
		t.Fatalf("expected outbox_failed log, got %s", logs.String())
	}
}

func newTestWorker(repo *workerTestRepo) *OutboxWorker {
	worker := NewOutboxWorker(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker.jitter = func() float64 { return 1 }
	return worker
}

func outboxMessage(id string, channel marketing.GrantChannel, payload string) marketing.OutboxMessage {
	now := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)
	target := "http://127.0.0.1"
	if channel == marketing.GrantMQ {
		target = "kafka://reward.commands"
	}
	return marketing.OutboxMessage{
		ID:            id,
		TenantID:      "tenant_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: channel,
		TargetRef:     target,
		Payload:       []byte(payload),
		SendStatus:    marketing.OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

type workerTestRepo struct {
	mu               sync.Mutex
	messages         []marketing.OutboxMessage
	grantStatuses    map[string]marketing.GrantStatus
	grantRetryCounts map[string]int
	audits           []marketing.AuditRecord
}

func newWorkerTestRepo(messages ...marketing.OutboxMessage) *workerTestRepo {
	return &workerTestRepo{
		messages:         messages,
		grantStatuses:    map[string]marketing.GrantStatus{},
		grantRetryCounts: map[string]int{},
	}
}

func (r *workerTestRepo) ListPendingOutbox(ctx context.Context, limit int) ([]marketing.OutboxMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []marketing.OutboxMessage
	now := time.Now().UTC()
	for i := range r.messages {
		message := r.messages[i]
		if message.TargetChannel != marketing.GrantHTTP && message.TargetChannel != marketing.GrantWebhook && message.TargetChannel != marketing.GrantMQ {
			continue
		}
		if message.SendStatus != marketing.OutboxPending && message.SendStatus != marketing.OutboxFailed {
			continue
		}
		if message.NextRetryAt != nil && message.NextRetryAt.After(now) {
			continue
		}
		r.messages[i].SendStatus = marketing.OutboxSending
		out = append(out, message)
	}
	return out, nil
}

func (r *workerTestRepo) MarkOutboxSent(ctx context.Context, id string) error {
	return r.update(id, func(message *marketing.OutboxMessage) {
		message.SendStatus = marketing.OutboxSent
	})
}

func (r *workerTestRepo) MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error {
	return r.update(id, func(item *marketing.OutboxMessage) {
		item.SendStatus = marketing.OutboxFailed
		item.RetryCount = retryCount
		item.NextRetryAt = &nextRetryAt
		item.ErrorMessage = message
	})
}

func (r *workerTestRepo) MarkOutboxDLQ(ctx context.Context, id string, retryCount int, message string) error {
	return r.update(id, func(item *marketing.OutboxMessage) {
		item.SendStatus = marketing.OutboxDLQ
		item.RetryCount = retryCount
		item.NextRetryAt = nil
		item.ErrorMessage = message
	})
}

func (r *workerTestRepo) update(id string, update func(*marketing.OutboxMessage)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.messages {
		if r.messages[i].ID == id {
			update(&r.messages[i])
			return nil
		}
	}
	return marketing.ErrNotFound
}

func (r *workerTestRepo) appendMessage(message marketing.OutboxMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, message)
}

func (r *workerTestRepo) messageStatus(id string) marketing.OutboxStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, message := range r.messages {
		if message.ID == id {
			return message.SendStatus
		}
	}
	return ""
}

func (r *workerTestRepo) RecoverStaleOutbox(ctx context.Context, staleBefore time.Time, limit int) ([]marketing.OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var recovered []marketing.OutboxMessage
	for i := range r.messages {
		if len(recovered) >= limit {
			break
		}
		if r.messages[i].SendStatus != marketing.OutboxSending || !r.messages[i].UpdatedAt.Before(staleBefore) {
			continue
		}
		before := r.messages[i]
		r.messages[i].SendStatus = marketing.OutboxPending
		r.messages[i].NextRetryAt = nil
		r.messages[i].UpdatedAt = time.Now().UTC()
		recovered = append(recovered, before)
	}
	return recovered, nil
}

func (r *workerTestRepo) CountOutboxDLQ(ctx context.Context) ([]marketing.OutboxDLQCount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := map[string]marketing.OutboxDLQCount{}
	for _, message := range r.messages {
		if message.SendStatus != marketing.OutboxDLQ {
			continue
		}
		key := message.TenantID + "|" + string(message.TargetChannel)
		count := counts[key]
		count.TenantID = message.TenantID
		count.TargetChannel = message.TargetChannel
		count.Count++
		counts[key] = count
	}
	out := make([]marketing.OutboxDLQCount, 0, len(counts))
	for _, count := range counts {
		out = append(out, count)
	}
	return out, nil
}

func (r *workerTestRepo) CreateActivity(context.Context, marketing.Activity) (marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) GetActivity(context.Context, string) (marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListActivities(context.Context) ([]marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) UpdateActivity(context.Context, marketing.Activity) (marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) FindMatchingActivities(context.Context, string, string) ([]marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) SaveWorkflowDraft(context.Context, marketing.WorkflowDraft) (marketing.WorkflowDraft, error) {
	panic("not implemented")
}
func (r *workerTestRepo) GetWorkflowDraft(context.Context, string) (marketing.WorkflowDraft, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateWorkflowVersion(context.Context, marketing.WorkflowVersion) (marketing.WorkflowVersion, error) {
	panic("not implemented")
}
func (r *workerTestRepo) GetActiveWorkflowVersion(context.Context, string) (marketing.WorkflowVersion, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListWorkflowVersions(context.Context, string) ([]marketing.WorkflowVersion, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateWorkflowExecution(context.Context, marketing.WorkflowExecution) (marketing.WorkflowExecution, error) {
	panic("not implemented")
}
func (r *workerTestRepo) UpdateWorkflowExecution(context.Context, marketing.WorkflowExecution) error {
	panic("not implemented")
}
func (r *workerTestRepo) CreateWorkflowNodeExecution(context.Context, marketing.WorkflowNodeExecution) (marketing.WorkflowNodeExecution, error) {
	panic("not implemented")
}
func (r *workerTestRepo) UpdateWorkflowNodeExecution(context.Context, marketing.WorkflowNodeExecution) error {
	panic("not implemented")
}
func (r *workerTestRepo) ListWorkflowExecutions(context.Context, marketing.WorkflowExecutionQuery) ([]marketing.WorkflowExecution, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListWorkflowNodeExecutions(context.Context, string) ([]marketing.WorkflowNodeExecution, error) {
	panic("not implemented")
}
func (r *workerTestRepo) SaveFactProvider(context.Context, marketing.FactProvider) (marketing.FactProvider, error) {
	panic("not implemented")
}
func (r *workerTestRepo) FindEnabledFactProvider(context.Context, string, string) (marketing.FactProvider, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateInbox(context.Context, marketing.EventInbox) (marketing.EventInbox, bool, error) {
	panic("not implemented")
}
func (r *workerTestRepo) UpdateInbox(context.Context, marketing.EventInbox) error {
	panic("not implemented")
}
func (r *workerTestRepo) CreateParticipation(context.Context, marketing.Participation) (marketing.Participation, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateGrantOrder(context.Context, marketing.GrantOrder) (marketing.GrantOrder, error) {
	panic("not implemented")
}
func (r *workerTestRepo) UpdateGrantOrderByCommandID(_ context.Context, _ string, commandID string, status marketing.GrantStatus, _ string) error {
	current, ok := r.grantStatuses[commandID]
	if !ok {
		return marketing.ErrNotFound
	}
	if status == marketing.GrantSending && current != marketing.GrantPending && current != marketing.GrantRetry {
		return marketing.ErrCallbackConflict
	}
	if status == marketing.GrantRetry && current != marketing.GrantSending {
		return marketing.ErrCallbackConflict
	}
	r.grantStatuses[commandID] = status
	if status == marketing.GrantRetry {
		r.grantRetryCounts[commandID]++
	}
	return nil
}
func (r *workerTestRepo) ApplyRewardCallback(context.Context, marketing.RewardCallback) (marketing.RewardCallbackResult, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ApplyManualGrantDecision(context.Context, marketing.ManualGrantDecisionCommand) (marketing.RewardCallbackResult, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateManualGrantExport(context.Context, marketing.ManualGrantExportCreateCommand) (marketing.ManualGrantExportCreateResult, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ReleaseExpiredQuotaReservations(context.Context, marketing.QuotaTimeoutReleaseCommand) (marketing.QuotaTimeoutReleaseResult, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListRewardReconciliation(context.Context, marketing.RewardReconciliationQuery) ([]marketing.RewardReconciliationItem, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateOutbox(context.Context, marketing.OutboxMessage) (marketing.OutboxMessage, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateParticipationGrantOutboxesWithQuota(context.Context, marketing.AtomicGrantCommand) (marketing.AtomicGrantResult, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateQuota(context.Context, marketing.Quota) (marketing.Quota, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListActivityQuotas(context.Context, string) ([]marketing.Quota, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ReserveQuota(context.Context, string, marketing.QuotaType, marketing.QuotaScope, string, string) (bool, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListOutboxDLQ(context.Context, string, bool, int) ([]marketing.OutboxMessage, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ReplayOutboxDLQ(context.Context, string, string, string, string, time.Time) (marketing.OutboxMessage, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CloseOutboxDLQ(context.Context, string, string, string, string, time.Time) (marketing.OutboxMessage, error) {
	panic("not implemented")
}
func (r *workerTestRepo) CreateAudit(_ context.Context, record marketing.AuditRecord) error {
	r.audits = append(r.audits, record)
	return nil
}
func (r *workerTestRepo) ListAudit(context.Context, marketing.AuditQuery) ([]marketing.AuditRecord, error) {
	panic("not implemented")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type recordingMQSender struct {
	messages []MQMessage
	err      error
}

func (s *recordingMQSender) Send(_ context.Context, message MQMessage) error {
	s.messages = append(s.messages, message)
	return s.err
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
