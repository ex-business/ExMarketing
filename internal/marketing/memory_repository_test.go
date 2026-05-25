package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_1"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock_a", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock_b", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_B", TotalValue: "0", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: Participation{
			ID:             "part_1",
			TenantID:       "site_001",
			ActivityID:     activityID,
			EventInboxID:   "inbox_1",
			Subject:        EventSubject{Type: "USER", ID: "user_1"},
			Biz:            EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
			Status:         ParticipationRewardable,
			Snapshot:       []byte(`{}`),
			IdempotencyKey: "idem_1",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		StockReservations: []QuotaReservation{
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_B", Amount: "1"},
		},
		GrantOrders: []GrantOrder{
			{ID: "grant_1", ActivityID: activityID, ParticipationID: "part_1"},
			{ID: "grant_2", ActivityID: activityID, ParticipationID: "part_1"},
		},
		OutboxMessages: []OutboxMessage{
			{ID: "outbox_1"},
			{ID: "outbox_2"},
		},
	})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	if len(repo.participations) != 0 || len(repo.grantOrders) != 0 || len(repo.outboxMessages) != 0 {
		t.Fatalf("expected rollback, got participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	for _, quota := range repo.quotas {
		if quota.UsedValue != "0" {
			t.Fatalf("expected quota %s to remain unused, got %s", quota.ID, quota.UsedValue)
		}
	}
}

func TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_1"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	inbox := EventInbox{ID: "inbox_1", IdempotencyKey: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1"}
	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || duplicate {
		t.Fatalf("create inbox duplicate=%v err=%v", duplicate, err)
	}
	if _, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: Participation{ID: "part_1", ActivityID: activityID, Status: ParticipationRewardable, IdempotencyKey: "part_idem_1", CreatedAt: now, UpdatedAt: now},
		BudgetAmount:  "20",
		StockReservations: []QuotaReservation{
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
		GrantOrders:    []GrantOrder{{ID: "grant_1"}},
		OutboxMessages: []OutboxMessage{{ID: "outbox_1"}},
	}); err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || !duplicate {
		t.Fatalf("expected duplicate inbox, duplicate=%v err=%v", duplicate, err)
	}
	if len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected no duplicate grants/outbox, grants=%d outbox=%d", len(repo.grantOrders), len(repo.outboxMessages))
	}
	for _, quota := range repo.quotas {
		if quota.Type == QuotaBudget && quota.FrozenValue != "20.0000" {
			t.Fatalf("expected budget frozen 20, got %s", quota.FrozenValue)
		}
		if quota.Type == QuotaStock && quota.FrozenValue != "1.0000" {
			t.Fatalf("expected stock frozen 1, got %s", quota.FrozenValue)
		}
	}
}

func TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	participation := Participation{
		ID:             "part_1",
		TenantID:       "site_001",
		ActivityID:     "act_1",
		EventInboxID:   "inbox_1",
		Status:         ParticipationRejected,
		Snapshot:       []byte(`{}`),
		IdempotencyKey: "site_001:act_1:site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateParticipation(context.Background(), participation); err != nil {
		t.Fatalf("create participation: %v", err)
	}
	participation.ID = "part_2"
	if _, err := repo.CreateParticipation(context.Background(), participation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected duplicate participation idempotency conflict, got %v", err)
	}

	order := GrantOrder{
		ID:             "grant_1",
		IdempotencyKey: "site_001:act_1:site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1:REWARD_A",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateGrantOrder(context.Background(), order); err != nil {
		t.Fatalf("create grant order: %v", err)
	}
	order.ID = "grant_2"
	if _, err := repo.CreateGrantOrder(context.Background(), order); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected duplicate grant idempotency conflict, got %v", err)
	}
}

func TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_concurrent_duplicate"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	const attempts = 24
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
				Participation:  testAtomicParticipation("part_concurrent_duplicate", activityID, "part_idem_concurrent_duplicate", now),
				BudgetAmount:   "10",
				GrantOrders:    []GrantOrder{testAtomicGrant("grant_concurrent_duplicate", activityID, "part_concurrent_duplicate", "grant_idem_concurrent_duplicate", now)},
				OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_concurrent_duplicate", now)},
				StockReservations: []QuotaReservation{
					{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
				},
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var success, conflicts int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent duplicate error: %v", err)
		}
	}
	if success != 1 || conflicts != attempts-1 {
		t.Fatalf("expected one success and %d conflicts, got success=%d conflicts=%d", attempts-1, success, conflicts)
	}
	if len(repo.participations) != 1 || len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected one persisted grant chain, participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "10.0000")
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaStock, QuotaScopeActivity, "REWARD_A", "1.0000")
}

func TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_concurrent_quota"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	const attempts = 24
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			suffix := strings.ReplaceAll(time.Unix(0, int64(i)).Format("150405.000000000"), ".", "_")
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
				Participation:  testAtomicParticipation("part_quota_"+suffix, activityID, "part_idem_quota_"+suffix, now),
				BudgetAmount:   "1",
				GrantOrders:    []GrantOrder{testAtomicGrant("grant_quota_"+suffix, activityID, "part_quota_"+suffix, "grant_idem_quota_"+suffix, now)},
				OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_quota_"+suffix, now)},
				StockReservations: []QuotaReservation{
					{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
				},
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	var success, quotaExceeded int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrQuotaExceeded):
			quotaExceeded++
		default:
			t.Fatalf("unexpected concurrent quota error: %v", err)
		}
	}
	if success != 1 || quotaExceeded != attempts-1 {
		t.Fatalf("expected one success and %d quota failures, got success=%d quotaExceeded=%d", attempts-1, success, quotaExceeded)
	}
	if len(repo.participations) != 1 || len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected one persisted grant chain, participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "1.0000")
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaStock, QuotaScopeActivity, "REWARD_A", "1.0000")
}

func TestMemoryCreateInboxDetectsDuplicateAndConflict(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	event := testInboxEvent(now)
	inbox := EventInbox{
		ID:             "inbox_1",
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || duplicate {
		t.Fatalf("create inbox duplicate=%v err=%v", duplicate, err)
	}

	same := inbox
	same.ID = "inbox_2"
	same.Event.Attributes = map[string]any{"count": 2, "amount": "12.34", "nested": map[string]any{"ok": true}}
	existing, duplicate, err := repo.CreateInbox(context.Background(), same)
	if err != nil || !duplicate {
		t.Fatalf("expected duplicate inbox, duplicate=%v err=%v", duplicate, err)
	}
	if existing.ProcessStatus != InboxDuplicate {
		t.Fatalf("expected duplicate status, got %s", existing.ProcessStatus)
	}

	conflict := inbox
	conflict.ID = "inbox_3"
	conflict.Event.EventID = "evt_conflict"
	if _, duplicate, err := repo.CreateInbox(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) || duplicate {
		t.Fatalf("expected idempotency conflict, duplicate=%v err=%v", duplicate, err)
	}
}

func TestActivityEventSameStandardPayloadCanonicalizesNumbers(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	left := testInboxEvent(now)
	left.Attributes = map[string]any{"amount": jsonNumber("12.3400"), "count": jsonNumber("2")}
	right := testInboxEvent(now)
	right.Attributes = map[string]any{"count": jsonNumber("2.0"), "amount": 12.34}

	if !left.SameStandardPayload(right) {
		t.Fatal("expected canonical payload comparison to treat equivalent numeric attributes as equal")
	}
}

func testInboxEvent(now time.Time) ActivityEvent {
	return ActivityEvent{
		EventID:      "evt_1",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      EventSubject{Type: "USER", ID: "10001"},
		Biz:          EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "12.34", "count": 2, "nested": map[string]any{"ok": true}},
	}
}

func jsonNumber(value string) any {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		panic(err)
	}
	return out
}

func testAtomicParticipation(id string, activityID string, idempotencyKey string, now time.Time) Participation {
	return Participation{
		ID:             id,
		TenantID:       "site_001",
		ActivityID:     activityID,
		EventInboxID:   "inbox_" + id,
		Subject:        EventSubject{Type: "USER", ID: "user_1"},
		Biz:            EventBiz{BizType: "DEPOSIT", BizID: "pay_" + id},
		Status:         ParticipationRewardable,
		Snapshot:       []byte(`{}`),
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testAtomicGrant(id string, activityID string, participationID string, idempotencyKey string, now time.Time) GrantOrder {
	return GrantOrder{
		ID:              id,
		TenantID:        "site_001",
		ActivityID:      activityID,
		ParticipationID: participationID,
		Subject:         EventSubject{Type: "USER", ID: "user_1"},
		RewardType:      RewardBonus,
		RewardAmount:    "1",
		RewardPayload:   []byte(`{}`),
		BillNo:          "cmd_" + id,
		GrantStatus:     GrantPending,
		GrantChannel:    GrantHTTP,
		IdempotencyKey:  idempotencyKey,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func testAtomicOutbox(id string, now time.Time) OutboxMessage {
	return OutboxMessage{
		ID:            id,
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: GrantHTTP,
		TargetRef:     "https://reward.example.com",
		Payload:       []byte(`{}`),
		SendStatus:    OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func assertMemoryQuotaFrozen(t *testing.T, repo *MemoryRepository, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, want string) {
	t.Helper()
	for _, quota := range repo.quotas {
		if quota.ActivityID == activityID && quota.Type == quotaType && quota.Scope == scope && quota.ScopeKey == scopeKey {
			if quota.FrozenValue != want {
				t.Fatalf("expected quota %s/%s/%s frozen %s, got %s", quotaType, scope, scopeKey, want, quota.FrozenValue)
			}
			return
		}
	}
	t.Fatalf("quota not found: %s/%s/%s", quotaType, scope, scopeKey)
}
