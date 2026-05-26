package marketing

import (
	"context"
	"time"
)

type Repository interface {
	CreateActivity(ctx context.Context, activity Activity) (Activity, error)
	GetActivity(ctx context.Context, id string) (Activity, error)
	ListActivities(ctx context.Context) ([]Activity, error)
	UpdateActivity(ctx context.Context, activity Activity) (Activity, error)
	FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]Activity, error)
	SaveFactProvider(ctx context.Context, provider FactProvider) (FactProvider, error)
	FindEnabledFactProvider(ctx context.Context, tenantID string, factCode string) (FactProvider, error)

	CreateInbox(ctx context.Context, inbox EventInbox) (EventInbox, bool, error)
	UpdateInbox(ctx context.Context, inbox EventInbox) error

	CreateParticipation(ctx context.Context, participation Participation) (Participation, error)
	CreateGrantOrder(ctx context.Context, order GrantOrder) (GrantOrder, error)
	UpdateGrantOrderByCommandID(ctx context.Context, tenantID string, commandID string, status GrantStatus, message string) error
	ApplyRewardCallback(ctx context.Context, callback RewardCallback) (RewardCallbackResult, error)
	ReleaseExpiredQuotaReservations(ctx context.Context, cmd QuotaTimeoutReleaseCommand) (QuotaTimeoutReleaseResult, error)
	ListRewardReconciliation(ctx context.Context, query RewardReconciliationQuery) ([]RewardReconciliationItem, error)
	CreateOutbox(ctx context.Context, message OutboxMessage) (OutboxMessage, error)
	CreateParticipationGrantOutboxesWithQuota(ctx context.Context, cmd AtomicGrantCommand) (AtomicGrantResult, error)

	CreateQuota(ctx context.Context, quota Quota) (Quota, error)
	ListActivityQuotas(ctx context.Context, activityID string) ([]Quota, error)
	ReserveQuota(ctx context.Context, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) (bool, error)

	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error)
	ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]OutboxMessage, error)
	MarkOutboxSent(ctx context.Context, id string) error
	MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error
	MarkOutboxDLQ(ctx context.Context, id string, retryCount int, message string) error
	ReplayOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, replayedAt time.Time) (OutboxMessage, error)
	CloseOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, closedAt time.Time) (OutboxMessage, error)

	CreateAudit(ctx context.Context, record AuditRecord) error
	ListAudit(ctx context.Context, query AuditQuery) ([]AuditRecord, error)
}
