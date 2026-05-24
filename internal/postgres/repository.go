package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(ctx context.Context, databaseURL string) (*Repository, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) CreateActivity(ctx context.Context, activity marketing.Activity) (marketing.Activity, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.Activity{}, err
	}
	defer rollback(tx)

	triggerEvents, err := json.Marshal(activity.TriggerEvents)
	if err != nil {
		return marketing.Activity{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO marketing_activity (
			id, tenant_id, activity_code, activity_name, activity_type, status,
			trigger_events, start_time, end_time, timezone, priority, exclusive_group,
			stackable, version, config_json, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17
		)
	`, activity.ID, activity.TenantID, activity.Code, activity.Name, activity.Type, activity.Status,
		triggerEvents, activity.StartTime, activity.EndTime, activity.Timezone, activity.Priority, activity.ExclusiveGroup,
		activity.Stackable, activity.Version, nullJSON(activity.Config), activity.CreatedAt, activity.UpdatedAt)
	if err != nil {
		return marketing.Activity{}, err
	}

	for _, rule := range activity.Rules {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_rule (
				id, activity_id, rule_code, rule_phase, rule_type, rule_config_json,
				sort, enabled, version, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, rule.ID, activity.ID, rule.Code, rule.Phase, rule.Type, rule.Config,
			rule.Sort, rule.Enabled, rule.Version, rule.CreatedAt, rule.UpdatedAt)
		if err != nil {
			return marketing.Activity{}, err
		}
	}

	for _, reward := range activity.Rewards {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_reward (
				id, activity_id, reward_code, reward_type, calculate_type, reward_config_json,
				grant_mode, grant_channel, flow_multiple, enabled, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, reward.ID, activity.ID, reward.Code, reward.Type, reward.CalculateType, reward.Config,
			reward.GrantMode, reward.GrantChannel, emptyDecimal(reward.FlowMultiple), reward.Enabled, reward.CreatedAt, reward.UpdatedAt)
		if err != nil {
			return marketing.Activity{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return marketing.Activity{}, err
	}
	return activity, nil
}

func (r *Repository) ListActivities(ctx context.Context) ([]marketing.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_code, activity_name, activity_type, status,
		       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		       stackable, version, config_json, created_at, updated_at
		FROM marketing_activity
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []marketing.Activity
	for rows.Next() {
		activity, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return activities, rows.Err()
}

func (r *Repository) FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]marketing.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_code, activity_name, activity_type, status,
		       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		       stackable, version, config_json, created_at, updated_at
		FROM marketing_activity
		WHERE tenant_id = $1
		  AND EXISTS (
		    SELECT 1
		    FROM json_array_elements_text(trigger_events) AS trigger_event
		    WHERE trigger_event = $2
		  )
		ORDER BY priority DESC, created_at ASC
	`, tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []marketing.Activity
	for rows.Next() {
		activity, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return activities, rows.Err()
}

func (r *Repository) CreateInbox(ctx context.Context, inbox marketing.EventInbox) (marketing.EventInbox, bool, error) {
	payload, err := json.Marshal(inbox.Event)
	if err != nil {
		return marketing.EventInbox{}, false, err
	}

	var id string
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_event_inbox (
			id, tenant_id, event_id, event_type, source_system, subject_type, subject_id,
			biz_type, biz_id, occurred_at, payload_json, idempotency_key, process_status,
			error_message, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16
		)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id
	`, inbox.ID, inbox.Event.TenantID, inbox.Event.EventID, inbox.Event.EventType, inbox.Event.SourceSystem,
		inbox.Event.Subject.Type, inbox.Event.Subject.ID, inbox.Event.Biz.BizType, inbox.Event.Biz.BizID,
		inbox.Event.OccurredAt, payload, inbox.IdempotencyKey, inbox.ProcessStatus, nullString(inbox.ErrorMessage),
		inbox.CreatedAt, inbox.UpdatedAt).Scan(&id)
	if err == nil {
		return inbox, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return marketing.EventInbox{}, false, err
	}

	existing, err := r.inboxByIdempotency(ctx, inbox.IdempotencyKey)
	if err != nil {
		return marketing.EventInbox{}, false, err
	}
	existing.ProcessStatus = marketing.InboxDuplicate
	return existing, true, nil
}

func (r *Repository) UpdateInbox(ctx context.Context, inbox marketing.EventInbox) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_event_inbox
		SET process_status = $1, error_message = $2, updated_at = $3
		WHERE idempotency_key = $4
	`, inbox.ProcessStatus, nullString(inbox.ErrorMessage), inbox.UpdatedAt, inbox.IdempotencyKey)
	return err
}

func (r *Repository) CreateParticipation(ctx context.Context, participation marketing.Participation) (marketing.Participation, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_participation (
			id, tenant_id, activity_id, event_inbox_id, subject_type, subject_id,
			biz_type, biz_id, status, reject_reason, snapshot_json, idempotency_key,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, participation.ID, participation.TenantID, participation.ActivityID, participation.EventInboxID,
		participation.Subject.Type, participation.Subject.ID, participation.Biz.BizType, participation.Biz.BizID,
		participation.Status, nullString(participation.RejectReason), participation.Snapshot,
		participation.IdempotencyKey, participation.CreatedAt, participation.UpdatedAt)
	return participation, err
}

func (r *Repository) CreateGrantOrder(ctx context.Context, order marketing.GrantOrder) (marketing.GrantOrder, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_grant_order (
			id, tenant_id, activity_id, participation_id, subject_type, subject_id,
			reward_type, reward_amount, reward_payload_json, bill_no, grant_status,
			grant_channel, retry_count, error_message, idempotency_key, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, order.ID, order.TenantID, order.ActivityID, order.ParticipationID, order.Subject.Type, order.Subject.ID,
		order.RewardType, emptyDecimal(order.RewardAmount), order.RewardPayload, order.BillNo, order.GrantStatus,
		order.GrantChannel, order.RetryCount, nullString(order.ErrorMessage), order.IdempotencyKey, order.CreatedAt, order.UpdatedAt)
	return order, err
}

func (r *Repository) UpdateGrantOrderByCommandID(ctx context.Context, tenantID string, commandID string, status marketing.GrantStatus, message string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE marketing_grant_order
		SET grant_status = $1, error_message = $2, updated_at = NOW()
		WHERE tenant_id = $3 AND bill_no = $4
	`, status, nullString(message), tenantID, commandID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return marketing.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateGrantOutboxesWithQuota(ctx context.Context, activityID string, subjectID string, totalRewardAmount string, reserveUserLimit bool, orders []marketing.GrantOrder, messages []marketing.OutboxMessage) ([]marketing.GrantOrder, []marketing.OutboxMessage, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, err
	}
	defer rollback(tx)

	ok, err := reserveQuotaTx(ctx, tx, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", totalRewardAmount)
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	if reserveUserLimit {
		ok, err = reserveQuotaTx(ctx, tx, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, subjectID, "1")
		if err != nil || !ok {
			return nil, nil, ok, err
		}
	}

	for _, order := range orders {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_grant_order (
				id, tenant_id, activity_id, participation_id, subject_type, subject_id,
				reward_type, reward_amount, reward_payload_json, bill_no, grant_status,
				grant_channel, retry_count, error_message, idempotency_key, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		`, order.ID, order.TenantID, order.ActivityID, order.ParticipationID, order.Subject.Type, order.Subject.ID,
			order.RewardType, emptyDecimal(order.RewardAmount), order.RewardPayload, order.BillNo, order.GrantStatus,
			order.GrantChannel, order.RetryCount, nullString(order.ErrorMessage), order.IdempotencyKey, order.CreatedAt, order.UpdatedAt)
		if err != nil {
			return nil, nil, false, err
		}
	}
	for _, message := range messages {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_outbox (
				id, tenant_id, message_type, target_channel, target_ref, payload_json,
				send_status, retry_count, next_retry_at, error_message, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, message.ID, message.TenantID, message.MessageType, message.TargetChannel, message.TargetRef,
			message.Payload, message.SendStatus, message.RetryCount, message.NextRetryAt,
			nullString(message.ErrorMessage), message.CreatedAt, message.UpdatedAt)
		if err != nil {
			return nil, nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, err
	}
	return orders, messages, true, nil
}

func (r *Repository) CreateOutbox(ctx context.Context, message marketing.OutboxMessage) (marketing.OutboxMessage, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_outbox (
			id, tenant_id, message_type, target_channel, target_ref, payload_json,
			send_status, retry_count, next_retry_at, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, message.ID, message.TenantID, message.MessageType, message.TargetChannel, message.TargetRef,
		message.Payload, message.SendStatus, message.RetryCount, message.NextRetryAt,
		nullString(message.ErrorMessage), message.CreatedAt, message.UpdatedAt)
	return message, err
}

func (r *Repository) CreateQuota(ctx context.Context, quota marketing.Quota) (marketing.Quota, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_quota (
			id, activity_id, quota_type, scope, scope_key, total_value, used_value,
			frozen_value, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, quota.ID, quota.ActivityID, quota.Type, quota.Scope, quota.ScopeKey,
		emptyDecimal(quota.TotalValue), emptyDecimal(quota.UsedValue), emptyDecimal(quota.FrozenValue),
		quota.Version, quota.CreatedAt, quota.UpdatedAt)
	return quota, err
}

func (r *Repository) ReserveQuota(ctx context.Context, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, amount string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE marketing_quota
		SET used_value = used_value + $5::numeric,
		    version = version + 1,
		    updated_at = NOW()
		WHERE activity_id = $1
		  AND quota_type = $2
		  AND scope = $3
		  AND scope_key = $4
		  AND used_value + $5::numeric <= total_value
	`, activityID, quotaType, scope, scopeKey, emptyDecimal(amount))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	var exists bool
	err = r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM marketing_quota
			WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
		)
	`, activityID, quotaType, scope, scopeKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	return false, nil
}

func (r *Repository) ListPendingOutbox(ctx context.Context, limit int) ([]marketing.OutboxMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)

	rows, err := tx.QueryContext(ctx, `
		WITH claimed AS (
			SELECT id
			FROM marketing_outbox
			WHERE send_status IN ('PENDING', 'FAILED')
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE marketing_outbox o
		SET send_status = 'SENDING', updated_at = NOW()
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING o.id, o.tenant_id, o.message_type, o.target_channel, o.target_ref, o.payload_json,
		          o.send_status, o.retry_count, o.next_retry_at, COALESCE(o.error_message, ''), o.created_at, o.updated_at
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []marketing.OutboxMessage
	for rows.Next() {
		message, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *Repository) MarkOutboxSent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_outbox
		SET send_status = 'SENT', error_message = NULL, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_outbox
		SET send_status = 'FAILED', retry_count = $2, next_retry_at = $3, error_message = $4, updated_at = NOW()
		WHERE id = $1
	`, id, retryCount, nextRetryAt, message)
	return err
}

func (r *Repository) MarkOutboxDLQ(ctx context.Context, id string, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_outbox
		SET send_status = 'DLQ', error_message = $2, updated_at = NOW()
		WHERE id = $1
	`, id, message)
	return err
}

func (r *Repository) CreateAudit(ctx context.Context, record marketing.AuditRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_audit (
			id, tenant_id, actor, action, resource, resource_id, payload_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, record.ID, record.TenantID, record.Actor, record.Action, record.Resource,
		record.ResourceID, record.Payload, record.CreatedAt)
	return err
}

func (r *Repository) ListAudit(ctx context.Context, limit int) ([]marketing.AuditRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, actor, action, resource, resource_id, payload_json, created_at
		FROM marketing_audit
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []marketing.AuditRecord
	for rows.Next() {
		var record marketing.AuditRecord
		if err := rows.Scan(&record.ID, &record.TenantID, &record.Actor, &record.Action,
			&record.Resource, &record.ResourceID, &record.Payload, &record.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *Repository) rulesForActivity(ctx context.Context, activityID string) ([]marketing.Rule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, rule_code, rule_phase, rule_type, rule_config_json, sort, enabled, version, created_at, updated_at
		FROM marketing_rule
		WHERE activity_id = $1
		ORDER BY sort ASC, created_at ASC
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []marketing.Rule
	for rows.Next() {
		var item marketing.Rule
		if err := rows.Scan(&item.ID, &item.Code, &item.Phase, &item.Type, &item.Config,
			&item.Sort, &item.Enabled, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, item)
	}
	return rules, rows.Err()
}

func (r *Repository) rewardsForActivity(ctx context.Context, activityID string) ([]marketing.Reward, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, reward_code, reward_type, calculate_type, reward_config_json, grant_mode,
		       grant_channel, flow_multiple::text, enabled, created_at, updated_at
		FROM marketing_reward
		WHERE activity_id = $1
		ORDER BY created_at ASC
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rewards []marketing.Reward
	for rows.Next() {
		var item marketing.Reward
		if err := rows.Scan(&item.ID, &item.Code, &item.Type, &item.CalculateType, &item.Config,
			&item.GrantMode, &item.GrantChannel, &item.FlowMultiple, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		rewards = append(rewards, item)
	}
	return rewards, rows.Err()
}

func (r *Repository) inboxByIdempotency(ctx context.Context, key string) (marketing.EventInbox, error) {
	var inbox marketing.EventInbox
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, payload_json, idempotency_key, process_status, COALESCE(error_message, ''), created_at, updated_at
		FROM marketing_event_inbox
		WHERE idempotency_key = $1
	`, key).Scan(&inbox.ID, &payload, &inbox.IdempotencyKey, &inbox.ProcessStatus, &inbox.ErrorMessage, &inbox.CreatedAt, &inbox.UpdatedAt)
	if err != nil {
		return marketing.EventInbox{}, err
	}
	if err := json.Unmarshal(payload, &inbox.Event); err != nil {
		return marketing.EventInbox{}, err
	}
	return inbox, nil
}

type activityScanner interface {
	Scan(dest ...any) error
}

type outboxScanner interface {
	Scan(dest ...any) error
}

func scanActivity(scanner activityScanner) (marketing.Activity, error) {
	var activity marketing.Activity
	var triggerEvents []byte
	var config sql.NullString
	err := scanner.Scan(
		&activity.ID, &activity.TenantID, &activity.Code, &activity.Name, &activity.Type, &activity.Status,
		&triggerEvents, &activity.StartTime, &activity.EndTime, &activity.Timezone, &activity.Priority, &activity.ExclusiveGroup,
		&activity.Stackable, &activity.Version, &config, &activity.CreatedAt, &activity.UpdatedAt,
	)
	if err != nil {
		return marketing.Activity{}, err
	}
	if err := json.Unmarshal(triggerEvents, &activity.TriggerEvents); err != nil {
		return marketing.Activity{}, err
	}
	if config.Valid {
		activity.Config = json.RawMessage(config.String)
	}
	return activity, nil
}

func scanOutbox(scanner outboxScanner) (marketing.OutboxMessage, error) {
	var message marketing.OutboxMessage
	var nextRetry sql.NullTime
	err := scanner.Scan(&message.ID, &message.TenantID, &message.MessageType, &message.TargetChannel,
		&message.TargetRef, &message.Payload, &message.SendStatus, &message.RetryCount,
		&nextRetry, &message.ErrorMessage, &message.CreatedAt, &message.UpdatedAt)
	if err != nil {
		return marketing.OutboxMessage{}, err
	}
	if nextRetry.Valid {
		message.NextRetryAt = &nextRetry.Time
	}
	return message, nil
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func nullJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func emptyDecimal(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func reserveQuotaTx(ctx context.Context, tx *sql.Tx, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, amount string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE marketing_quota
		SET used_value = used_value + $5::numeric,
		    version = version + 1,
		    updated_at = NOW()
		WHERE activity_id = $1
		  AND quota_type = $2
		  AND scope = $3
		  AND scope_key = $4
		  AND used_value + $5::numeric <= total_value
	`, activityID, quotaType, scope, scopeKey, emptyDecimal(amount))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	var exists bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM marketing_quota
			WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
		)
	`, activityID, quotaType, scope, scopeKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	return false, nil
}

var _ marketing.Repository = (*Repository)(nil)

func sortActivitiesByPriority(activities []marketing.Activity) {
	slices.SortFunc(activities, func(a, b marketing.Activity) int {
		return b.Priority - a.Priority
	})
}
