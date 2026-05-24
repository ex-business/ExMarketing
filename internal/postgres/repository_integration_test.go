package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestPostgresAtomicGrantRollsBackOnStockExceeded(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_atomic_rollback"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	for _, quota := range []marketing.Quota{
		{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_user", ActivityID: activityID, Type: marketing.QuotaUserLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock_a", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock_b", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_B", TotalValue: "0", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repo.CreateQuota(ctx, quota); err != nil {
			t.Fatalf("create quota %s: %v", quota.ID, err)
		}
	}

	_, err = repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation: marketing.Participation{
			ID:             prefix + "_part",
			TenantID:       "site_001",
			ActivityID:     activityID,
			EventInboxID:   prefix + "_inbox",
			Subject:        marketing.EventSubject{Type: "USER", ID: "user_1"},
			Biz:            marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
			Status:         marketing.ParticipationRewardable,
			Snapshot:       []byte(`{}`),
			IdempotencyKey: prefix + "_part_idem",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_B", Amount: "1"},
		},
		GrantOrders: []marketing.GrantOrder{
			{ID: prefix + "_grant_1", TenantID: "site_001", ActivityID: activityID, ParticipationID: prefix + "_part", Subject: marketing.EventSubject{Type: "USER", ID: "user_1"}, RewardType: marketing.RewardBonus, RewardAmount: "10", RewardPayload: []byte(`{}`), BillNo: prefix + "_cmd_1", GrantStatus: marketing.GrantPending, GrantChannel: marketing.GrantHTTP, IdempotencyKey: prefix + "_grant_1", CreatedAt: now, UpdatedAt: now},
			{ID: prefix + "_grant_2", TenantID: "site_001", ActivityID: activityID, ParticipationID: prefix + "_part", Subject: marketing.EventSubject{Type: "USER", ID: "user_1"}, RewardType: marketing.RewardBonus, RewardAmount: "10", RewardPayload: []byte(`{}`), BillNo: prefix + "_cmd_2", GrantStatus: marketing.GrantPending, GrantChannel: marketing.GrantHTTP, IdempotencyKey: prefix + "_grant_2", CreatedAt: now, UpdatedAt: now},
		},
		OutboxMessages: []marketing.OutboxMessage{
			{ID: prefix + "_outbox_1", TenantID: "site_001", MessageType: "REWARD_COMMAND", TargetChannel: marketing.GrantHTTP, TargetRef: "https://reward.example.com", Payload: []byte(`{}`), SendStatus: marketing.OutboxPending, CreatedAt: now, UpdatedAt: now},
			{ID: prefix + "_outbox_2", TenantID: "site_001", MessageType: "REWARD_COMMAND", TargetChannel: marketing.GrantHTTP, TargetRef: "https://reward.example.com", Payload: []byte(`{}`), SendStatus: marketing.OutboxPending, CreatedAt: now, UpdatedAt: now},
		},
	})
	if !errors.Is(err, marketing.ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}

	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value <> 0`, activityID, 0)
}

func cleanupPostgresAtomicTest(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM marketing_outbox WHERE id LIKE $1`,
		`DELETE FROM marketing_grant_order WHERE id LIKE $1`,
		`DELETE FROM marketing_participation WHERE id LIKE $1`,
		`DELETE FROM marketing_quota WHERE id LIKE $1`,
	} {
		if _, err := repo.db.ExecContext(ctx, stmt, prefix+"_%"); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
}

func assertPostgresCount(t *testing.T, repo *Repository, query string, arg any, want int) {
	t.Helper()
	var got int
	if err := repo.db.QueryRowContext(context.Background(), query, arg).Scan(&got); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if got != want {
		t.Fatalf("expected count %d, got %d for %s", want, got, query)
	}
}
