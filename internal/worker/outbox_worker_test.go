package worker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestOutboxWorkerSkipsMQMessages(t *testing.T) {
	repo := newWorkerTestRepo(outboxMessage("outbox_mq", marketing.GrantMQ, "{}"))
	worker := newTestWorker(repo)
	worker.tick(context.Background())

	if repo.messages[0].SendStatus != marketing.OutboxPending {
		t.Fatalf("expected MQ message to remain pending, got %s", repo.messages[0].SendStatus)
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
		target = "reward.commands"
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
	messages []marketing.OutboxMessage
}

func newWorkerTestRepo(messages ...marketing.OutboxMessage) *workerTestRepo {
	return &workerTestRepo{messages: messages}
}

func (r *workerTestRepo) ListPendingOutbox(ctx context.Context, limit int) ([]marketing.OutboxMessage, error) {
	var out []marketing.OutboxMessage
	now := time.Now().UTC()
	for i := range r.messages {
		message := r.messages[i]
		if message.TargetChannel != marketing.GrantHTTP && message.TargetChannel != marketing.GrantWebhook {
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
	for i := range r.messages {
		if r.messages[i].ID == id {
			update(&r.messages[i])
			return nil
		}
	}
	return marketing.ErrNotFound
}

func (r *workerTestRepo) CreateActivity(context.Context, marketing.Activity) (marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) ListActivities(context.Context) ([]marketing.Activity, error) {
	panic("not implemented")
}
func (r *workerTestRepo) FindMatchingActivities(context.Context, string, string) ([]marketing.Activity, error) {
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
func (r *workerTestRepo) UpdateGrantOrderByCommandID(context.Context, string, string, marketing.GrantStatus, string) error {
	panic("not implemented")
}
func (r *workerTestRepo) ApplyRewardCallback(context.Context, marketing.RewardCallback) (marketing.RewardCallbackResult, error) {
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
func (r *workerTestRepo) CreateAudit(context.Context, marketing.AuditRecord) error {
	panic("not implemented")
}
func (r *workerTestRepo) ListAudit(context.Context, int) ([]marketing.AuditRecord, error) {
	panic("not implemented")
}
