package marketing

import (
	"context"
	"errors"
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
		if quota.Type == QuotaBudget && quota.UsedValue != "20.0000" {
			t.Fatalf("expected budget used 20, got %s", quota.UsedValue)
		}
		if quota.Type == QuotaStock && quota.UsedValue != "1.0000" {
			t.Fatalf("expected stock used 1, got %s", quota.UsedValue)
		}
	}
}
