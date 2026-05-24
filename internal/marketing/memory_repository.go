package marketing

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

type MemoryRepository struct {
	mu                 sync.RWMutex
	activities         []Activity
	inboxByIdempotency map[string]EventInbox
	participations     []Participation
	grantOrders        []GrantOrder
	outboxMessages     []OutboxMessage
	quotas             []Quota
	auditRecords       []AuditRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		inboxByIdempotency: make(map[string]EventInbox),
	}
}

func (r *MemoryRepository) CreateActivity(ctx context.Context, activity Activity) (Activity, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activities = append(r.activities, activity)
	return activity, nil
}

func (r *MemoryRepository) ListActivities(ctx context.Context) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.activities)
	return out, nil
}

func (r *MemoryRepository) FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Activity
	for _, activity := range r.activities {
		if activity.TenantID != tenantID {
			continue
		}
		if !slices.Contains(activity.TriggerEvents, eventType) {
			continue
		}
		out = append(out, activity)
	}
	slices.SortFunc(out, func(a, b Activity) int {
		return b.Priority - a.Priority
	})
	return out, nil
}

func (r *MemoryRepository) CreateInbox(ctx context.Context, inbox EventInbox) (EventInbox, bool, error) {
	if err := ctx.Err(); err != nil {
		return EventInbox{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.inboxByIdempotency[inbox.IdempotencyKey]
	if ok {
		existing.ProcessStatus = InboxDuplicate
		return existing, true, nil
	}
	r.inboxByIdempotency[inbox.IdempotencyKey] = inbox
	return inbox, false, nil
}

func (r *MemoryRepository) UpdateInbox(ctx context.Context, inbox EventInbox) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inboxByIdempotency[inbox.IdempotencyKey] = inbox
	return nil
}

func (r *MemoryRepository) CreateParticipation(ctx context.Context, participation Participation) (Participation, error) {
	if err := ctx.Err(); err != nil {
		return Participation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.participations = append(r.participations, participation)
	return participation, nil
}

func (r *MemoryRepository) CreateGrantOrder(ctx context.Context, order GrantOrder) (GrantOrder, error) {
	if err := ctx.Err(); err != nil {
		return GrantOrder{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grantOrders = append(r.grantOrders, order)
	return order, nil
}

func (r *MemoryRepository) UpdateGrantOrderByCommandID(ctx context.Context, tenantID string, commandID string, status GrantStatus, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.grantOrders {
		if r.grantOrders[i].TenantID == tenantID && r.grantOrders[i].BillNo == commandID {
			r.grantOrders[i].GrantStatus = status
			r.grantOrders[i].ErrorMessage = message
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) CreateGrantOutboxesWithQuota(ctx context.Context, activityID string, subjectID string, totalRewardAmount string, reserveUserLimit bool, orders []GrantOrder, messages []OutboxMessage) ([]GrantOrder, []OutboxMessage, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.reserveQuotaLocked(activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", totalRewardAmount) {
		return nil, nil, false, nil
	}
	if reserveUserLimit && !r.reserveQuotaLocked(activityID, QuotaUserLimit, QuotaScopeUser, subjectID, "1") {
		r.releaseQuotaLocked(activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", totalRewardAmount)
		return nil, nil, false, nil
	}
	r.grantOrders = append(r.grantOrders, orders...)
	r.outboxMessages = append(r.outboxMessages, messages...)
	return orders, messages, true, nil
}

func (r *MemoryRepository) CreateOutbox(ctx context.Context, message OutboxMessage) (OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return OutboxMessage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outboxMessages = append(r.outboxMessages, message)
	return message, nil
}

func (r *MemoryRepository) CreateQuota(ctx context.Context, quota Quota) (Quota, error) {
	if err := ctx.Err(); err != nil {
		return Quota{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotas = append(r.quotas, quota)
	return quota, nil
}

func (r *MemoryRepository) ReserveQuota(ctx context.Context, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		total := decimal.MustParse(q.TotalValue)
		used := decimal.MustParse(q.UsedValue)
		increment := decimal.MustParse(amount)
		next := used.Add(increment)
		if next.Cmp(total) > 0 {
			return false, nil
		}
		q.UsedValue = next.String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return true, nil
	}
	return true, nil
}

func (r *MemoryRepository) ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []OutboxMessage
	for _, message := range r.outboxMessages {
		if len(out) >= limit {
			break
		}
		if message.SendStatus == OutboxPending || message.SendStatus == OutboxFailed {
			if message.NextRetryAt == nil || !message.NextRetryAt.After(time.Now().UTC()) {
				out = append(out, message)
			}
		}
	}
	for _, claimed := range out {
		for i := range r.outboxMessages {
			if r.outboxMessages[i].ID == claimed.ID {
				r.outboxMessages[i].SendStatus = OutboxSending
				r.outboxMessages[i].UpdatedAt = time.Now().UTC()
			}
		}
	}
	return out, nil
}

func (r *MemoryRepository) MarkOutboxSent(ctx context.Context, id string) error {
	return r.updateOutbox(ctx, id, func(message *OutboxMessage) {
		message.SendStatus = OutboxSent
		message.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error {
	return r.updateOutbox(ctx, id, func(item *OutboxMessage) {
		item.SendStatus = OutboxFailed
		item.RetryCount = retryCount
		item.NextRetryAt = &nextRetryAt
		item.ErrorMessage = message
		item.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) MarkOutboxDLQ(ctx context.Context, id string, message string) error {
	return r.updateOutbox(ctx, id, func(item *OutboxMessage) {
		item.SendStatus = OutboxDLQ
		item.ErrorMessage = message
		item.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) CreateAudit(ctx context.Context, record AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditRecords = append(r.auditRecords, record)
	return nil
}

func (r *MemoryRepository) ListAudit(ctx context.Context, limit int) ([]AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > len(r.auditRecords) {
		limit = len(r.auditRecords)
	}
	out := slices.Clone(r.auditRecords[:limit])
	return out, nil
}

func (r *MemoryRepository) updateOutbox(ctx context.Context, id string, update func(*OutboxMessage)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outboxMessages {
		if r.outboxMessages[i].ID == id {
			update(&r.outboxMessages[i])
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) reserveQuotaLocked(activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) bool {
	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		total := decimal.MustParse(q.TotalValue)
		used := decimal.MustParse(q.UsedValue)
		increment := decimal.MustParse(amount)
		next := used.Add(increment)
		if next.Cmp(total) > 0 {
			return false
		}
		q.UsedValue = next.String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return true
	}
	return true
}

func (r *MemoryRepository) releaseQuotaLocked(activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) {
	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		used := decimal.MustParse(q.UsedValue)
		decrement := decimal.MustParse("-" + amount)
		q.UsedValue = used.Add(decrement).String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return
	}
}
