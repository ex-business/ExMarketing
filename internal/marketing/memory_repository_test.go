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

func TestFactProviderValidateRejectsInvalidMappingContract(t *testing.T) {
	base := FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		CacheTTLSeconds: 0,
		Enabled:         true,
		TimeoutMS:       5000,
	}
	for name, mutate := range map[string]func(*FactProvider){
		"facts prefix": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"facts.USER_PROFILE.vipLevel": {Source: "$.data.vipLevel", Type: "string"}}
		},
		"trailing dot": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"risk.": {Source: "$.risk", Type: "object"}}
		},
		"wildcard source": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data.*", Type: "string"}}
		},
		"filter source": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data[?(@.vip)]", Type: "string"}}
		},
		"invalid type": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "decimal"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := base
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("expected invalid provider")
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid provider rejected: %v", err)
	}
}

func TestExtractFactReferencesValidatedRejectsInvalidFactPaths(t *testing.T) {
	for _, field := range []string{"facts", "facts.USER_PROFILE", "facts..vipLevel", "facts.USER_PROFILE.", "facts.user_profile.vipLevel"} {
		t.Run(field, func(t *testing.T) {
			_, err := ExtractFactReferencesValidated([]Rule{{
				Type:   RuleTypeCondition,
				Config: []byte(`{"logic":"AND","conditions":[{"field":"` + field + `","op":"=","value":"VIP3"}]}`),
			}})
			if err == nil || !strings.Contains(err.Error(), "FACT_FIELD_INVALID") {
				t.Fatalf("expected FACT_FIELD_INVALID, got %v", err)
			}
		})
	}
}

func TestExtractFactReferencesValidatedKeepsNestedFieldExact(t *testing.T) {
	refs, err := ExtractFactReferencesValidated([]Rule{{
		Type:   RuleTypeCondition,
		Config: []byte(`{"logic":"AND","conditions":[{"field":"facts.USER_PROFILE.risk.score","op":">=","value":80}]}`),
	}})
	if err != nil {
		t.Fatalf("extract refs: %v", err)
	}
	if _, ok := refs["USER_PROFILE"]["risk.score"]; !ok {
		t.Fatalf("expected exact nested path, got %#v", refs)
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

func TestMemoryCallbackFinalizesConfiguredLimitReservations(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_limit_callback"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_daily", ActivityID: activityID, Type: QuotaDailyLimit, Scope: QuotaScopeUser, ScopeKey: "user_1:2026-05-24", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation:      testAtomicParticipation("part_limit_callback", activityID, "part_idem_limit_callback", now),
		BudgetAmount:       "10",
		ReserveUserLimit:   true,
		UserScopeKey:       "user_1",
		ReserveDailyLimit:  true,
		DailyLimitScopeKey: "user_1:2026-05-24",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_limit_callback", activityID, "part_limit_callback", "grant_idem_limit_callback", now)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_limit_callback", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if len(repo.quotaReservations) != 3 {
		t.Fatalf("expected budget, user and daily reservations, got %d", len(repo.quotaReservations))
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_limit_callback", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_limit_callback",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_limit_callback",
		FinishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("expected participation granted, got %s", result.ParticipationStatus)
	}
	for _, reservation := range repo.quotaReservations {
		if reservation.Status != QuotaReservationConfirmed {
			t.Fatalf("expected reservation %s confirmed, got %s", reservation.Type, reservation.Status)
		}
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "10.0000", "0.0000")
	assertMemoryQuotaValues(t, repo, activityID, QuotaUserLimit, QuotaScopeUser, "user_1", "1.0000", "0.0000")
	assertMemoryQuotaValues(t, repo, activityID, QuotaDailyLimit, QuotaScopeUser, "user_1:2026-05-24", "1.0000", "0.0000")
}

func TestMemoryCallbackTreatsMissingReservationsAsUnlimitedWhenNoQuotaWasConfigured(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_unlimited_callback"
	repo.participations = append(repo.participations, testAtomicParticipation("part_unlimited_callback", activityID, "part_idem_unlimited_callback", now))
	repo.grantOrders = append(repo.grantOrders, func() GrantOrder {
		order := testAtomicGrant("grant_unlimited_callback", activityID, "part_unlimited_callback", "grant_idem_unlimited_callback", now)
		order.GrantStatus = GrantSending
		return order
	}())

	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_unlimited_callback",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_unlimited",
		FinishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if result.GrantStatus != GrantSuccess || result.ParticipationStatus != ParticipationGranted || len(result.Reservations) != 0 {
		t.Fatalf("expected unlimited callback to succeed without reservations, got %+v", result)
	}
}

func TestMemoryCloseOutboxDLQOnlyClosesOutbox(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_dlq_close"
	repo.quotas = []Quota{
		{ID: "quota_dlq_close_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: testAtomicParticipation("part_dlq_close", activityID, "part_idem_dlq_close", now),
		BudgetAmount:  "10",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_dlq_close", activityID, "part_dlq_close", "grant_idem_dlq_close", now)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{func() OutboxMessage {
			message := testAtomicOutbox("outbox_dlq_close", now)
			message.Payload = []byte(`{"commandId":"cmd_grant_dlq_close"}`)
			return message
		}()},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_dlq_close", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_dlq_close", GrantRetry, "target returned status 400"); err != nil {
		t.Fatalf("mark retry: %v", err)
	}
	if err := repo.MarkOutboxDLQ(context.Background(), "outbox_dlq_close", 5, "target returned status 400"); err != nil {
		t.Fatalf("mark outbox dlq: %v", err)
	}

	closed, err := repo.CloseOutboxDLQ(context.Background(), "site_001", "outbox_dlq_close", "ops", "business rejected", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("close dlq: %v", err)
	}
	if closed.SendStatus != OutboxDLQClosed {
		t.Fatalf("expected DLQ_CLOSED, got %s", closed.SendStatus)
	}
	order := repo.grantOrderByCommandIDLocked("site_001", "cmd_grant_dlq_close")
	if order == nil || order.GrantStatus != GrantRetry {
		t.Fatalf("expected grant to remain RETRY, got %+v", order)
	}
	if got := repo.participationStatusLocked("part_dlq_close"); got != ParticipationRewardable {
		t.Fatalf("expected participation to remain REWARDABLE, got %s", got)
	}
	for _, reservation := range repo.quotaReservations {
		if reservation.Status != QuotaReservationFrozen {
			t.Fatalf("expected reservation to remain frozen, got %+v", reservation)
		}
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "10.0000")
}

func TestMemoryCallbackConfirmsParticipationLimitOnceWhenAnyGrantSucceeds(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_multi_grant_limit"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation:    testAtomicParticipation("part_multi_grant_limit", activityID, "part_idem_multi_grant_limit", now),
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		GrantOrders: []GrantOrder{
			func() GrantOrder {
				order := testAtomicGrant("grant_multi_success", activityID, "part_multi_grant_limit", "grant_idem_multi_success", now)
				order.RewardAmount = "10"
				return order
			}(),
			func() GrantOrder {
				order := testAtomicGrant("grant_multi_failed", activityID, "part_multi_grant_limit", "grant_idem_multi_failed", now)
				order.RewardAmount = "10"
				return order
			}(),
		},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_multi_success", now), testAtomicOutbox("outbox_multi_failed", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	for _, commandID := range []string{"cmd_grant_multi_success", "cmd_grant_multi_failed"} {
		if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", commandID, GrantSending, "sending"); err != nil {
			t.Fatalf("mark sending %s: %v", commandID, err)
		}
	}
	if _, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_multi_success",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_multi_success",
		FinishedAt:      now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("success callback: %v", err)
	}
	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:   "site_001",
		CommandID:  "cmd_grant_multi_failed",
		Status:     GrantFailed,
		Message:    "provider rejected",
		FinishedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed callback: %v", err)
	}
	if result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("expected participation granted because one grant succeeded, got %s", result.ParticipationStatus)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaUserLimit, QuotaScopeUser, "user_1", "1.0000", "0.0000")
}

func TestMemoryTimeoutReleaseDryRunReleaseAndIdempotency(t *testing.T) {
	repo := NewMemoryRepository()
	old := time.Now().UTC().Add(-3 * time.Hour)
	activityID := "act_timeout_release"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: old, UpdatedAt: old},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: testAtomicParticipation("part_timeout_release", activityID, "part_idem_timeout_release", old),
		BudgetAmount:  "10",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_timeout_release", activityID, "part_timeout_release", "grant_idem_timeout_release", old)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_timeout_release", old)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_timeout_release", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	repo.grantOrders[0].UpdatedAt = old

	dryRun, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		DryRun:    true,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(dryRun.Items) != 1 || dryRun.Items[0].Result != "DRY_RUN" {
		t.Fatalf("expected one dry-run candidate, got %+v", dryRun)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "10.0000")

	released, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(released.Items) != 1 || released.Items[0].GrantStatus != GrantFailed || released.Items[0].Result != "RELEASED" {
		t.Fatalf("expected released item, got %+v", released)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "0.0000")

	repeated, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("repeat release: %v", err)
	}
	if len(repeated.Items) != 0 {
		t.Fatalf("expected repeated release to be idempotent with no candidates, got %+v", repeated)
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

func TestMemoryManualExportRequiresReadyFileBeforeSuccess(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_manual_export"
	repo.participations = append(repo.participations, testAtomicParticipation("part_manual_export", activityID, "part_idem_manual_export", now))
	repo.grantOrders = append(repo.grantOrders, testManualGrant("grant_manual_export", activityID, "part_manual_export", "grant_idem_manual_export", "REWARD_EXPORT", "5", GrantManualExport, now))

	_, err := repo.ApplyManualGrantDecision(context.Background(), ManualGrantDecisionCommand{TenantID: "site_001", CommandID: "cmd_grant_manual_export", Status: GrantSuccess, ExternalGrantID: "offline_export", Operator: "ops", Reason: "paid", FinishedAt: now.Add(time.Minute)})
	if !errors.Is(err, ErrManualGrantExportRequired) {
		t.Fatalf("expected export required, got %v", err)
	}
	exportResult, err := repo.CreateManualGrantExport(context.Background(), ManualGrantExportCreateCommand{TenantID: "site_001", Operator: "ops", Reason: "daily export", ActivityID: activityID})
	if err != nil {
		t.Fatalf("create export: %v", err)
	}
	if exportResult.ExportFile.Status != ManualGrantExportReady || len(exportResult.Details) != 1 {
		t.Fatalf("unexpected export result: %+v", exportResult)
	}
	if repo.grantOrders[0].GrantStatus != GrantSending {
		t.Fatalf("expected exported grant sending, got %s", repo.grantOrders[0].GrantStatus)
	}
	result, err := repo.ApplyManualGrantDecision(context.Background(), ManualGrantDecisionCommand{TenantID: "site_001", CommandID: "cmd_grant_manual_export", Status: GrantSuccess, ExternalGrantID: "offline_export", Operator: "ops", Reason: "paid", FinishedAt: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("manual export success: %v", err)
	}
	if result.GrantStatus != GrantSuccess || result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMemoryTimeoutReleaseSkipsManualGrantOrders(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC().Add(-3 * time.Hour)
	activityID := "act_manual_timeout"
	repo.quotas = []Quota{{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "10", Version: 1, CreatedAt: now, UpdatedAt: now}}
	repo.participations = append(repo.participations, testAtomicParticipation("part_manual_timeout", activityID, "part_idem_manual_timeout", now))
	repo.grantOrders = append(repo.grantOrders, testManualGrant("grant_manual_timeout", activityID, "part_manual_timeout", "grant_idem_manual_timeout", "REWARD_TIMEOUT", "10", GrantManual, now))
	repo.quotaReservations = append(repo.quotaReservations, QuotaReservation{ID: "reservation_manual_timeout", TenantID: "site_001", ActivityID: activityID, GrantOrderID: "grant_manual_timeout", CommandID: "cmd_grant_manual_timeout", Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", Amount: "10", Status: QuotaReservationFrozen, CreatedAt: now, UpdatedAt: now})
	result, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{TenantID: "site_001", OlderThan: time.Now().UTC().Add(-2 * time.Hour), Operator: "ops", Reason: "timeout"})
	if err != nil {
		t.Fatalf("timeout release: %v", err)
	}
	if len(result.Items) != 0 || repo.grantOrders[0].GrantStatus != GrantPending {
		t.Fatalf("manual order should be skipped, result=%+v status=%s", result, repo.grantOrders[0].GrantStatus)
	}
}

func testManualGrant(id string, activityID string, participationID string, idempotencyKey string, rewardCode string, amount string, mode GrantMode, now time.Time) GrantOrder {
	payload, _ := json.Marshal(map[string]any{"rewardCode": rewardCode, "rewardType": RewardBonus, "calculateType": CalculateFixed, "grantMode": mode, "grantChannel": GrantManualFile})
	order := testAtomicGrant(id, activityID, participationID, idempotencyKey, now)
	order.RewardAmount = amount
	order.RewardPayload = payload
	order.GrantChannel = GrantManualFile
	return order
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

func assertMemoryQuotaValues(t *testing.T, repo *MemoryRepository, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, wantUsed string, wantFrozen string) {
	t.Helper()
	for _, quota := range repo.quotas {
		if quota.ActivityID == activityID && quota.Type == quotaType && quota.Scope == scope && quota.ScopeKey == scopeKey {
			if quota.UsedValue != wantUsed || quota.FrozenValue != wantFrozen {
				t.Fatalf("expected quota %s/%s/%s used=%s frozen=%s, got used=%s frozen=%s", quotaType, scope, scopeKey, wantUsed, wantFrozen, quota.UsedValue, quota.FrozenValue)
			}
			return
		}
	}
	t.Fatalf("quota not found: %s/%s/%s", quotaType, scope, scopeKey)
}
