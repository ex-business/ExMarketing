package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/jackc/pgx/v5/pgconn"
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
	if err := ensureActivityUniqueConstraint(ctx, db); err != nil {
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
		return marketing.Activity{}, mapPostgresActivityError(err)
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
			stackable, plugin_code, version, config_json, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18
		)
	`, activity.ID, activity.TenantID, activity.Code, activity.Name, activity.Type, activity.Status,
		triggerEvents, activity.StartTime, activity.EndTime, activity.Timezone, activity.Priority, activity.ExclusiveGroup,
		activity.Stackable, activity.PluginCode, activity.Version, nullJSON(activity.Config), activity.CreatedAt, activity.UpdatedAt)
	if err != nil {
		return marketing.Activity{}, mapPostgresActivityError(err)
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
				grant_mode, grant_channel, flow_multiple, enabled, version, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`, reward.ID, activity.ID, reward.Code, reward.Type, reward.CalculateType, reward.Config,
			reward.GrantMode, reward.GrantChannel, emptyDecimal(reward.FlowMultiple), reward.Enabled, reward.Version, reward.CreatedAt, reward.UpdatedAt)
		if err != nil {
			return marketing.Activity{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return marketing.Activity{}, err
	}
	return activity, nil
}

func (r *Repository) GetActivity(ctx context.Context, id string) (marketing.Activity, error) {
	activity, err := r.activityByID(ctx, r.db, id)
	if err != nil {
		return marketing.Activity{}, err
	}
	activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
	if err != nil {
		return marketing.Activity{}, err
	}
	activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
	if err != nil {
		return marketing.Activity{}, err
	}
	return activity, nil
}

func (r *Repository) ListActivities(ctx context.Context) ([]marketing.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_code, activity_name, activity_type, status,
		       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		       stackable, plugin_code, version, config_json, created_at, updated_at
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

func (r *Repository) UpdateActivity(ctx context.Context, activity marketing.Activity) (marketing.Activity, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE marketing_activity
		SET activity_name = $2,
		    status = $3,
		    start_time = $4,
		    end_time = $5,
		    timezone = $6,
		    priority = $7,
		    exclusive_group = $8,
		    stackable = $9,
		    version = $10,
		    config_json = $11,
		    updated_at = $12
		WHERE id = $1
		RETURNING id, tenant_id, activity_code, activity_name, activity_type, status,
		          trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		          stackable, plugin_code, version, config_json, created_at, updated_at
	`, activity.ID, activity.Name, activity.Status, activity.StartTime, activity.EndTime, activity.Timezone,
		activity.Priority, activity.ExclusiveGroup, activity.Stackable, activity.Version, nullJSON(activity.Config), activity.UpdatedAt)
	updated, err := scanActivity(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.Activity{}, marketing.ErrNotFound
		}
		return marketing.Activity{}, mapPostgresActivityError(err)
	}
	updated.Rules = activity.Rules
	updated.Rewards = activity.Rewards
	return updated, nil
}

func (r *Repository) FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]marketing.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_code, activity_name, activity_type, status,
		       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		       stackable, plugin_code, version, config_json, created_at, updated_at
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

func (r *Repository) SaveFactProvider(ctx context.Context, provider marketing.FactProvider) (marketing.FactProvider, error) {
	if err := provider.Validate(); err != nil {
		return marketing.FactProvider{}, err
	}
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = time.Now().UTC()
	}
	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = time.Now().UTC()
	}
	mapping, err := json.Marshal(provider.Mapping)
	if err != nil {
		return marketing.FactProvider{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO marketing_fact_provider (
			tenant_id, fact_code, provider_type, endpoint, method, auth_ref,
			mapping_json, cache_ttl_seconds, enabled, timeout_ms, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (tenant_id, fact_code) DO UPDATE SET
			provider_type = EXCLUDED.provider_type,
			endpoint = EXCLUDED.endpoint,
			method = EXCLUDED.method,
			auth_ref = EXCLUDED.auth_ref,
			mapping_json = EXCLUDED.mapping_json,
			cache_ttl_seconds = EXCLUDED.cache_ttl_seconds,
			enabled = EXCLUDED.enabled,
			timeout_ms = EXCLUDED.timeout_ms,
			updated_at = EXCLUDED.updated_at
	`, provider.TenantID, provider.FactCode, provider.ProviderType, provider.Endpoint, provider.Method, nullString(provider.AuthRef),
		mapping, provider.CacheTTLSeconds, provider.Enabled, provider.TimeoutMS, provider.CreatedAt, provider.UpdatedAt)
	if err != nil {
		return marketing.FactProvider{}, err
	}
	return provider, nil
}

func (r *Repository) FindEnabledFactProvider(ctx context.Context, tenantID string, factCode string) (marketing.FactProvider, error) {
	var provider marketing.FactProvider
	var mapping []byte
	var authRef sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, fact_code, provider_type, endpoint, method, auth_ref,
		       mapping_json, cache_ttl_seconds, enabled, timeout_ms, created_at, updated_at
		FROM marketing_fact_provider
		WHERE tenant_id = $1 AND fact_code = $2
	`, tenantID, factCode).Scan(&provider.TenantID, &provider.FactCode, &provider.ProviderType, &provider.Endpoint,
		&provider.Method, &authRef, &mapping, &provider.CacheTTLSeconds, &provider.Enabled, &provider.TimeoutMS,
		&provider.CreatedAt, &provider.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.FactProvider{}, marketing.ErrNotFound
		}
		return marketing.FactProvider{}, err
	}
	provider.AuthRef = authRef.String
	if err := json.Unmarshal(mapping, &provider.Mapping); err != nil {
		return marketing.FactProvider{}, err
	}
	if !provider.Enabled {
		return marketing.FactProvider{}, marketing.ErrFactProviderDisabled
	}
	if err := provider.Validate(); err != nil {
		return marketing.FactProvider{}, err
	}
	return provider, nil
}

func (r *Repository) CreateInbox(ctx context.Context, inbox marketing.EventInbox) (marketing.EventInbox, bool, error) {
	inbox.Event = inbox.Event.Normalized()
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
	if !existing.Event.SameStandardPayload(inbox.Event) {
		return marketing.EventInbox{}, false, marketing.ErrIdempotencyConflict
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
		SET grant_status = $1,
		    error_message = $2,
		    retry_count = CASE WHEN $1 = 'RETRY' THEN retry_count + 1 ELSE retry_count END,
		    updated_at = NOW()
		WHERE tenant_id = $3 AND bill_no = $4
		  AND grant_status NOT IN ('SUCCESS', 'FAILED', 'CLOSED')
		  AND (
		    ($1 = 'SENDING' AND grant_status IN ('PENDING', 'RETRY')) OR
		    ($1 = 'RETRY' AND grant_status = 'SENDING')
		  )
	`, status, nullString(message), tenantID, commandID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists bool
		err = r.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM marketing_grant_order WHERE tenant_id = $1 AND bill_no = $2
			)
		`, tenantID, commandID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			return marketing.ErrCallbackConflict
		}
		return marketing.ErrNotFound
	}
	return nil
}

func (r *Repository) ApplyRewardCallback(ctx context.Context, callback marketing.RewardCallback) (marketing.RewardCallbackResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	defer rollback(tx)

	var order marketing.GrantOrder
	var externalGrantID sql.NullString
	var finishedAt sql.NullTime
	rows, err := tx.QueryContext(ctx, `
		SELECT id, tenant_id, activity_id, participation_id, subject_type, subject_id,
		       reward_type, reward_amount::text, reward_payload_json, bill_no, grant_status, grant_channel, COALESCE(error_message, ''),
		       COALESCE(external_grant_id, ''), finished_at, idempotency_key, created_at, updated_at
		FROM marketing_grant_order
		WHERE tenant_id = $1
		  AND (($2 <> '' AND bill_no = $2) OR ($3 <> '' AND idempotency_key = $3))
		FOR UPDATE
	`, callback.TenantID, callback.CommandID, callback.IdempotencyKey)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		found++
		if found > 1 {
			return marketing.RewardCallbackResult{}, marketing.ErrCallbackLocatorConflict
		}
		if err := rows.Scan(
			&order.ID, &order.TenantID, &order.ActivityID, &order.ParticipationID,
			&order.Subject.Type, &order.Subject.ID, &order.RewardType, &order.RewardAmount, &order.RewardPayload,
			&order.BillNo, &order.GrantStatus, &order.GrantChannel, &order.ErrorMessage, &externalGrantID, &finishedAt,
			&order.IdempotencyKey, &order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if found == 0 {
		return marketing.RewardCallbackResult{}, marketing.ErrNotFound
	}
	if callback.CommandID != "" && order.BillNo != callback.CommandID {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackLocatorConflict
	}
	if callback.IdempotencyKey != "" && order.IdempotencyKey != callback.IdempotencyKey {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackLocatorConflict
	}
	order.ExternalGrantID = externalGrantID.String
	if finishedAt.Valid {
		order.FinishedAt = &finishedAt.Time
	}
	if mode := grantModeFromPayload(order.RewardPayload); mode == marketing.GrantManual || mode == marketing.GrantManualExport {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
	}
	if order.GrantStatus == marketing.GrantClosed {
		return marketing.RewardCallbackResult{}, marketing.ErrGrantOrderClosed
	}
	reservations, err := quotaReservationsForOrderTx(ctx, tx, order.ID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if order.GrantStatus == marketing.GrantSuccess || order.GrantStatus == marketing.GrantFailed {
		if order.GrantStatus != callback.Status || (order.ExternalGrantID != "" && callback.ExternalGrantID != "" && order.ExternalGrantID != callback.ExternalGrantID) {
			return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
		}
		status, err := participationStatusTx(ctx, tx, order.ParticipationID)
		if err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		return marketing.RewardCallbackResult{
			CommandID:           order.BillNo,
			BillNo:              order.BillNo,
			IdempotencyKey:      order.IdempotencyKey,
			TenantID:            order.TenantID,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			GrantStatus:         order.GrantStatus,
			Status:              order.GrantStatus,
			ParticipationStatus: status,
			Duplicate:           true,
			Reservations:        reservations,
		}, nil
	}
	if order.GrantStatus != marketing.GrantSending && order.GrantStatus != marketing.GrantRetry {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
	}
	if len(reservations) == 0 {
		hadQuota, err := activityHadQuotaAtTx(ctx, tx, order.ActivityID, order.CreatedAt)
		if err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		if hadQuota {
			return marketing.RewardCallbackResult{}, marketing.ErrQuotaReservationMissing
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE marketing_grant_order
			SET grant_status = $1, external_grant_id = $2, error_message = $3, finished_at = $4, updated_at = NOW()
			WHERE id = $5
		`, callback.Status, nullString(callback.ExternalGrantID), nullString(callback.Message), callback.FinishedAt, order.ID); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		status, err := updateParticipationAfterCallbackTx(ctx, tx, order.ParticipationID)
		if err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		return marketing.RewardCallbackResult{
			CommandID:           order.BillNo,
			BillNo:              order.BillNo,
			IdempotencyKey:      order.IdempotencyKey,
			TenantID:            order.TenantID,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			GrantStatus:         callback.Status,
			Status:              callback.Status,
			ParticipationStatus: status,
		}, nil
	}
	for _, reservation := range reservations {
		if reservation.Status != marketing.QuotaReservationFrozen {
			return marketing.RewardCallbackResult{}, marketing.ErrQuotaReservationConflict
		}
		if isParticipationQuotaReservation(reservation) {
			continue
		}
		if err := finishQuotaReservationTx(ctx, tx, reservation, callback.Status); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE marketing_grant_order
		SET grant_status = $1, external_grant_id = $2, error_message = $3, finished_at = $4, updated_at = NOW()
		WHERE id = $5
	`, callback.Status, nullString(callback.ExternalGrantID), nullString(callback.Message), callback.FinishedAt, order.ID); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	status, err := updateParticipationAfterCallbackTx(ctx, tx, order.ParticipationID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if status == marketing.ParticipationGranted || status == marketing.ParticipationFailed {
		finalStatus := marketing.GrantSuccess
		if status == marketing.ParticipationFailed {
			finalStatus = marketing.GrantFailed
		}
		if err := finishParticipationQuotaReservationsTx(ctx, tx, order.ParticipationID, finalStatus); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
	}
	reservations, err = quotaReservationsForOrderTx(ctx, tx, order.ID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	return marketing.RewardCallbackResult{
		CommandID:           order.BillNo,
		BillNo:              order.BillNo,
		IdempotencyKey:      order.IdempotencyKey,
		TenantID:            order.TenantID,
		GrantOrderID:        order.ID,
		ParticipationID:     order.ParticipationID,
		GrantStatus:         callback.Status,
		Status:              callback.Status,
		ParticipationStatus: status,
		Reservations:        reservations,
	}, nil
}

func (r *Repository) ApplyManualGrantDecision(ctx context.Context, cmd marketing.ManualGrantDecisionCommand) (marketing.RewardCallbackResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	defer rollback(tx)

	order, err := manualGrantOrderForUpdateTx(ctx, tx, cmd)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	mode := grantModeFromPayload(order.RewardPayload)
	if mode != marketing.GrantManual && mode != marketing.GrantManualExport {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
	}
	if marketingIsFinalGrantStatus(order.GrantStatus) {
		if order.GrantStatus == cmd.Status && (order.ExternalGrantID == "" || cmd.ExternalGrantID == "" || order.ExternalGrantID == cmd.ExternalGrantID) {
			status, err := participationStatusTx(ctx, tx, order.ParticipationID)
			if err != nil {
				return marketing.RewardCallbackResult{}, err
			}
			reservations, err := quotaReservationsForOrderTx(ctx, tx, order.ID)
			if err != nil {
				return marketing.RewardCallbackResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return marketing.RewardCallbackResult{}, err
			}
			return marketing.RewardCallbackResult{
				CommandID:           order.BillNo,
				BillNo:              order.BillNo,
				IdempotencyKey:      order.IdempotencyKey,
				TenantID:            order.TenantID,
				GrantOrderID:        order.ID,
				ParticipationID:     order.ParticipationID,
				GrantStatus:         order.GrantStatus,
				Status:              order.GrantStatus,
				ParticipationStatus: status,
				Duplicate:           true,
				Reservations:        reservations,
			}, nil
		}
		if order.GrantStatus == marketing.GrantClosed {
			return marketing.RewardCallbackResult{}, marketing.ErrGrantOrderClosed
		}
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
	}
	if mode == marketing.GrantManualExport && cmd.Status == marketing.GrantSuccess && (order.GrantStatus != marketing.GrantSending || !hasReadyManualExportDetailTx(ctx, tx, order.ID)) {
		return marketing.RewardCallbackResult{}, marketing.ErrManualGrantExportRequired
	}
	if order.GrantStatus != marketing.GrantPending && order.GrantStatus != marketing.GrantSending && order.GrantStatus != marketing.GrantRetry {
		return marketing.RewardCallbackResult{}, marketing.ErrCallbackConflict
	}
	reservations, err := quotaReservationsForOrderTx(ctx, tx, order.ID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if len(reservations) == 0 {
		hadQuota, err := activityHadQuotaAtTx(ctx, tx, order.ActivityID, order.CreatedAt)
		if err != nil {
			return marketing.RewardCallbackResult{}, err
		}
		if hadQuota {
			return marketing.RewardCallbackResult{}, marketing.ErrQuotaReservationMissing
		}
	}
	for _, reservation := range reservations {
		if reservation.Status != marketing.QuotaReservationFrozen && !isParticipationQuotaReservation(reservation) {
			return marketing.RewardCallbackResult{}, marketing.ErrQuotaReservationConflict
		}
		if isParticipationQuotaReservation(reservation) {
			continue
		}
		if err := finishQuotaReservationTx(ctx, tx, reservation, cmd.Status); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE marketing_grant_order
		SET grant_status = $1, external_grant_id = $2, error_message = $3, finished_at = $4, updated_at = NOW()
		WHERE id = $5
	`, cmd.Status, nullString(cmd.ExternalGrantID), nullString(cmd.Reason), cmd.FinishedAt, order.ID); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	status, err := updateParticipationAfterCallbackTx(ctx, tx, order.ParticipationID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if status == marketing.ParticipationGranted || status == marketing.ParticipationFailed {
		finalStatus := marketing.GrantSuccess
		if status == marketing.ParticipationFailed {
			finalStatus = marketing.GrantFailed
		}
		if err := finishParticipationQuotaReservationsTx(ctx, tx, order.ParticipationID, finalStatus); err != nil {
			return marketing.RewardCallbackResult{}, err
		}
	}
	reservations, err = quotaReservationsForOrderTx(ctx, tx, order.ID)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	return marketing.RewardCallbackResult{
		CommandID:           order.BillNo,
		BillNo:              order.BillNo,
		IdempotencyKey:      order.IdempotencyKey,
		TenantID:            order.TenantID,
		GrantOrderID:        order.ID,
		ParticipationID:     order.ParticipationID,
		GrantStatus:         cmd.Status,
		Status:              cmd.Status,
		ParticipationStatus: status,
		Reservations:        reservations,
	}, nil
}

func (r *Repository) CreateManualGrantExport(ctx context.Context, cmd marketing.ManualGrantExportCreateCommand) (marketing.ManualGrantExportCreateResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	defer rollback(tx)

	candidates, err := manualGrantExportCandidatesTx(ctx, tx, cmd)
	if err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	if len(candidates) == 0 {
		return marketing.ManualGrantExportCreateResult{}, marketing.ErrNotFound
	}
	exportID := marketing.NewID("manual_export")
	rows, checksum, err := manualGrantExportCSV(exportID, candidates)
	now := time.Now().UTC()
	filterSnapshot, _ := json.Marshal(cmd)
	status := marketing.ManualGrantExportReady
	errorMessage := ""
	if err != nil {
		status = marketing.ManualGrantExportFailed
		errorMessage = err.Error()
	}
	_ = rows
	file := marketing.ManualGrantExportFile{
		ID:             exportID,
		TenantID:       cmd.TenantID,
		FileName:       exportID + ".csv",
		FileFormat:     "CSV",
		Status:         status,
		Operator:       cmd.Operator,
		Reason:         cmd.Reason,
		FilterSnapshot: filterSnapshot,
		RowCount:       len(candidates),
		FileChecksum:   checksum,
		StorageRef:     "postgres://manual-grant-exports/" + exportID + ".csv",
		ErrorMessage:   errorMessage,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO marketing_manual_grant_export_file (
			id, tenant_id, file_name, file_format, status, operator, reason, filter_snapshot_json,
			row_count, file_checksum, storage_ref, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, file.ID, file.TenantID, file.FileName, file.FileFormat, file.Status, file.Operator, file.Reason,
		file.FilterSnapshot, file.RowCount, nullString(file.FileChecksum), nullString(file.StorageRef), nullString(file.ErrorMessage),
		file.CreatedAt, file.UpdatedAt); err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	if status == marketing.ManualGrantExportFailed {
		if err := tx.Commit(); err != nil {
			return marketing.ManualGrantExportCreateResult{}, err
		}
		return marketing.ManualGrantExportCreateResult{ExportFile: file}, nil
	}
	details := make([]marketing.ManualGrantExportDetail, 0, len(candidates))
	for i, order := range candidates {
		detail := marketing.ManualGrantExportDetail{
			ID:                marketing.NewID("manual_export_detail"),
			ExportID:          exportID,
			TenantID:          cmd.TenantID,
			GrantOrderID:      order.ID,
			BillNo:            order.BillNo,
			RowNumber:         i + 1,
			ExportGrantStatus: order.GrantStatus,
			DetailStatus:      marketing.ManualGrantExportDetailReady,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO marketing_manual_grant_export_detail (
				id, export_id, tenant_id, grant_order_id, bill_no, row_number,
				export_grant_status, detail_status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, detail.ID, detail.ExportID, detail.TenantID, detail.GrantOrderID, detail.BillNo, detail.RowNumber,
			detail.ExportGrantStatus, detail.DetailStatus, detail.CreatedAt, detail.UpdatedAt); err != nil {
			return marketing.ManualGrantExportCreateResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE marketing_grant_order
			SET grant_status = 'SENDING', updated_at = NOW()
			WHERE id = $1 AND grant_status IN ('PENDING', 'RETRY')
		`, order.ID); err != nil {
			return marketing.ManualGrantExportCreateResult{}, err
		}
		details = append(details, detail)
	}
	if err := tx.Commit(); err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	return marketing.ManualGrantExportCreateResult{ExportFile: file, Details: details}, nil
}

func (r *Repository) ReleaseExpiredQuotaReservations(ctx context.Context, cmd marketing.QuotaTimeoutReleaseCommand) (marketing.QuotaTimeoutReleaseResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	defer rollback(tx)

	limit := cmd.Limit
	if limit <= 0 {
		limit = 100
	}
	result := marketing.QuotaTimeoutReleaseResult{
		TenantID:  cmd.TenantID,
		OlderThan: cmd.OlderThan,
		Limit:     limit,
		DryRun:    cmd.DryRun,
		Operator:  cmd.Operator,
		Reason:    cmd.Reason,
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT g.id, g.tenant_id, g.activity_id, g.participation_id, g.bill_no, g.grant_status, g.reward_payload_json
		FROM marketing_grant_order g
		WHERE g.tenant_id = $1
		  AND g.grant_status NOT IN ('SUCCESS', 'FAILED', 'CLOSED')
		  AND COALESCE(g.updated_at, g.created_at) < $2
		  AND EXISTS (
		    SELECT 1 FROM marketing_quota_reservation r
		    WHERE r.grant_order_id = g.id AND r.reservation_status = 'FROZEN'
		  )
		ORDER BY COALESCE(g.updated_at, g.created_at) ASC, g.id ASC
		LIMIT $3
		FOR UPDATE OF g
	`, cmd.TenantID, cmd.OlderThan, limit)
	if err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	defer rows.Close()

	var orders []marketing.GrantOrder
	for rows.Next() {
		var order marketing.GrantOrder
		if err := rows.Scan(&order.ID, &order.TenantID, &order.ActivityID, &order.ParticipationID, &order.BillNo, &order.GrantStatus, &order.RewardPayload); err != nil {
			return marketing.QuotaTimeoutReleaseResult{}, err
		}
		mode := grantModeFromPayload(order.RewardPayload)
		if mode == marketing.GrantManual || mode == marketing.GrantManualExport {
			continue
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	if err := rows.Close(); err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}

	for _, order := range orders {
		beforeStatus := order.GrantStatus
		reservations, err := quotaReservationsForOrderTx(ctx, tx, order.ID)
		if err != nil {
			return marketing.QuotaTimeoutReleaseResult{}, err
		}
		if cmd.DryRun {
			status, err := participationStatusTx(ctx, tx, order.ParticipationID)
			if err != nil {
				return marketing.QuotaTimeoutReleaseResult{}, err
			}
			result.Items = append(result.Items, marketing.QuotaTimeoutReleaseItem{
				CommandID:           order.BillNo,
				GrantOrderID:        order.ID,
				ParticipationID:     order.ParticipationID,
				ActivityID:          order.ActivityID,
				PreviousGrantStatus: beforeStatus,
				GrantStatus:         order.GrantStatus,
				ParticipationStatus: status,
				Result:              "DRY_RUN",
				Reservations:        reservations,
			})
			continue
		}
		for _, reservation := range reservations {
			if reservation.Status != marketing.QuotaReservationFrozen || isParticipationQuotaReservation(reservation) {
				continue
			}
			if err := finishQuotaReservationTx(ctx, tx, reservation, marketing.GrantFailed); err != nil {
				return marketing.QuotaTimeoutReleaseResult{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE marketing_grant_order
			SET grant_status = 'FAILED',
			    error_message = $2,
			    finished_at = NOW(),
			    updated_at = NOW()
			WHERE id = $1
		`, order.ID, nullString("TIMEOUT_RELEASE: "+cmd.Reason)); err != nil {
			return marketing.QuotaTimeoutReleaseResult{}, err
		}
		status, err := updateParticipationAfterCallbackTx(ctx, tx, order.ParticipationID)
		if err != nil {
			return marketing.QuotaTimeoutReleaseResult{}, err
		}
		if status == marketing.ParticipationGranted || status == marketing.ParticipationFailed {
			finalStatus := marketing.GrantSuccess
			if status == marketing.ParticipationFailed {
				finalStatus = marketing.GrantFailed
			}
			if err := finishParticipationQuotaReservationsTx(ctx, tx, order.ParticipationID, finalStatus); err != nil {
				return marketing.QuotaTimeoutReleaseResult{}, err
			}
		}
		reservations, err = quotaReservationsForOrderTx(ctx, tx, order.ID)
		if err != nil {
			return marketing.QuotaTimeoutReleaseResult{}, err
		}
		result.Items = append(result.Items, marketing.QuotaTimeoutReleaseItem{
			CommandID:           order.BillNo,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			ActivityID:          order.ActivityID,
			PreviousGrantStatus: beforeStatus,
			GrantStatus:         marketing.GrantFailed,
			ParticipationStatus: status,
			Result:              "RELEASED",
			Reservations:        reservations,
		})
	}
	if err := tx.Commit(); err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	return result, nil
}

func (r *Repository) ListRewardReconciliation(ctx context.Context, query marketing.RewardReconciliationQuery) ([]marketing.RewardReconciliationItem, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT go.id, go.tenant_id, go.activity_id, go.participation_id, go.subject_type, go.subject_id,
		       go.reward_type, go.reward_amount::text, go.bill_no, go.grant_status,
		       COALESCE(go.error_message, ''), COALESCE(go.external_grant_id, ''), go.finished_at, go.updated_at,
		       p.status,
		       ei.id, ei.payload_json, ei.idempotency_key, ei.process_status, COALESCE(ei.error_message, ''), ei.created_at, ei.updated_at
		FROM marketing_grant_order go
		JOIN marketing_participation p ON p.id = go.participation_id
		JOIN marketing_event_inbox ei ON ei.id = p.event_inbox_id
		WHERE go.tenant_id = $1
		  AND ($2 = '' OR go.bill_no = $2)
		  AND ($3 = '' OR go.id = $3)
		  AND ($4 = '' OR go.activity_id = $4)
		  AND ($5 = '' OR go.subject_type = $5)
		  AND ($6 = '' OR go.subject_id = $6)
		  AND ($7 = '' OR ei.event_id = $7)
		  AND ($8 = '' OR ei.idempotency_key = $8)
		  AND ($9 = '' OR p.id = $9)
		  AND ($10 = '' OR EXISTS (
		    SELECT 1 FROM marketing_outbox mo
		    WHERE mo.tenant_id = go.tenant_id
		      AND mo.id = $10
		      AND (mo.payload_json::text LIKE '%' || go.bill_no || '%' OR mo.payload_json::text LIKE '%' || go.participation_id || '%')
		  ))
		ORDER BY go.updated_at DESC
		LIMIT $11
	`, query.TenantID, query.CommandID, query.GrantOrderID, query.ActivityID, query.SubjectType, query.SubjectID, query.EventID, query.IdempotencyKey, query.ParticipationID, query.OutboxID, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []marketing.RewardReconciliationItem
	for rows.Next() {
		var item marketing.RewardReconciliationItem
		var finishedAt sql.NullTime
		var inbox marketing.EventInbox
		var eventPayload []byte
		if err := rows.Scan(&item.GrantOrderID, &item.TenantID, &item.ActivityID, &item.ParticipationID,
			&item.Subject.Type, &item.Subject.ID, &item.RewardType, &item.RewardAmount,
			&item.CommandID, &item.GrantStatus, &item.CallbackMessage, &item.ExternalGrantID,
			&finishedAt, &item.ServerUpdatedAt, &item.ParticipationStatus,
			&inbox.ID, &eventPayload, &inbox.IdempotencyKey, &inbox.ProcessStatus, &inbox.ErrorMessage, &inbox.CreatedAt, &inbox.UpdatedAt); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			item.FinishedAt = &finishedAt.Time
		}
		if err := json.Unmarshal(eventPayload, &inbox.Event); err != nil {
			return nil, err
		}
		item.EventInbox = &inbox
		item.Reservations, err = quotaReservationsForOrder(ctx, r.db, item.GrantOrderID)
		if err != nil {
			return nil, err
		}
		item.OutboxMessages, err = r.outboxesForReward(ctx, item.TenantID, item.CommandID, item.ParticipationID)
		if err != nil {
			return nil, err
		}
		item.AuditTimeline, err = r.auditTimeline(ctx, item.TenantID, []string{item.GrantOrderID, item.CommandID, item.ParticipationID, item.ActivityID, inbox.ID, inbox.Event.EventID, inbox.IdempotencyKey})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateParticipationGrantOutboxesWithQuota(ctx context.Context, cmd marketing.AtomicGrantCommand) (marketing.AtomicGrantResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.AtomicGrantResult{}, err
	}
	defer rollback(tx)

	if err := insertParticipationTx(ctx, tx, cmd.Participation); err != nil {
		return marketing.AtomicGrantResult{}, err
	}
	if err := reserveQuotaOrExceeded(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", cmd.BudgetAmount); err != nil {
		return marketing.AtomicGrantResult{}, err
	}
	if cmd.ReserveUserLimit {
		if err := reserveQuotaOrExceeded(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, cmd.UserScopeKey, "1"); err != nil {
			return marketing.AtomicGrantResult{}, err
		}
	}
	if cmd.ReserveDailyLimit {
		if err := reserveQuotaOrExceeded(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, cmd.DailyLimitScopeKey, "1"); err != nil {
			return marketing.AtomicGrantResult{}, err
		}
	}
	for _, reservation := range cmd.StockReservations {
		if err := reserveQuotaOrExceeded(ctx, tx, cmd.Participation.ActivityID, reservation.Type, reservation.Scope, reservation.ScopeKey, reservation.Amount); err != nil {
			return marketing.AtomicGrantResult{}, err
		}
	}

	for _, order := range cmd.GrantOrders {
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
			return marketing.AtomicGrantResult{}, err
		}
	}
	reservations, err := postgresQuotaReservationsForAtomicCommand(ctx, tx, cmd, time.Now().UTC())
	if err != nil {
		return marketing.AtomicGrantResult{}, err
	}
	for _, reservation := range reservations {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_quota_reservation (
				id, tenant_id, activity_id, grant_order_id, command_id, quota_type,
				scope, scope_key, reserved_amount, reservation_status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::numeric, $10, $11, $12)
		`, reservation.ID, reservation.TenantID, reservation.ActivityID, reservation.GrantOrderID,
			reservation.CommandID, reservation.Type, reservation.Scope, reservation.ScopeKey,
			emptyDecimal(reservation.Amount), reservation.Status, reservation.CreatedAt, reservation.UpdatedAt)
		if err != nil {
			return marketing.AtomicGrantResult{}, err
		}
	}
	for _, message := range cmd.OutboxMessages {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO marketing_outbox (
				id, tenant_id, message_type, target_channel, target_ref, payload_json,
				send_status, retry_count, next_retry_at, error_message, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, message.ID, message.TenantID, message.MessageType, message.TargetChannel, message.TargetRef,
			message.Payload, message.SendStatus, message.RetryCount, message.NextRetryAt,
			nullString(message.ErrorMessage), message.CreatedAt, message.UpdatedAt)
		if err != nil {
			return marketing.AtomicGrantResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return marketing.AtomicGrantResult{}, err
	}
	return marketing.AtomicGrantResult{
		Participation:  cmd.Participation,
		GrantOrders:    cmd.GrantOrders,
		OutboxMessages: cmd.OutboxMessages,
	}, nil
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

func (r *Repository) ListActivityQuotas(ctx context.Context, activityID string) ([]marketing.Quota, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, activity_id, quota_type, scope, scope_key, total_value, used_value,
		       frozen_value, version, created_at, updated_at
		FROM marketing_quota
		WHERE activity_id = $1
		ORDER BY quota_type, scope, scope_key
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotas []marketing.Quota
	for rows.Next() {
		var quota marketing.Quota
		if err := rows.Scan(
			&quota.ID,
			&quota.ActivityID,
			&quota.Type,
			&quota.Scope,
			&quota.ScopeKey,
			&quota.TotalValue,
			&quota.UsedValue,
			&quota.FrozenValue,
			&quota.Version,
			&quota.CreatedAt,
			&quota.UpdatedAt,
		); err != nil {
			return nil, err
		}
		quotas = append(quotas, quota)
	}
	return quotas, rows.Err()
}

func (r *Repository) ReserveQuota(ctx context.Context, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, amount string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE marketing_quota
		SET frozen_value = frozen_value + $5::numeric,
		    version = version + 1,
		    updated_at = NOW()
		WHERE activity_id = $1
		  AND quota_type = $2
		  AND scope = $3
		  AND scope_key = $4
		  AND used_value + frozen_value + $5::numeric <= total_value
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
		  AND target_channel IN ('HTTP', 'WEBHOOK', 'MQ')
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

func (r *Repository) ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]marketing.OutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, message_type, target_channel, target_ref, payload_json,
		       send_status, retry_count, next_retry_at, COALESCE(error_message, ''), created_at, updated_at
		FROM marketing_outbox
		WHERE ($1 = '' OR tenant_id = $1)
		  AND (send_status = 'DLQ' OR ($2 AND send_status = 'DLQ_CLOSED'))
		ORDER BY updated_at DESC
		LIMIT $3
	`, tenantID, includeClosed, limit)
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
	return messages, rows.Err()
}

func (r *Repository) MarkOutboxSent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_outbox
		SET send_status = 'SENT', next_retry_at = NULL, error_message = NULL, updated_at = NOW()
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

func (r *Repository) MarkOutboxDLQ(ctx context.Context, id string, retryCount int, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_outbox
		SET send_status = 'DLQ', retry_count = $2, next_retry_at = NULL, error_message = $3, updated_at = NOW()
		WHERE id = $1
	`, id, retryCount, message)
	return err
}

func (r *Repository) ReplayOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, replayedAt time.Time) (marketing.OutboxMessage, error) {
	return r.updateOutboxDLQ(ctx, tenantID, id, operator, reason, replayedAt, marketing.OutboxPending, "OUTBOX_DLQ_REPLAY")
}

func (r *Repository) CloseOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, closedAt time.Time) (marketing.OutboxMessage, error) {
	return r.updateOutboxDLQ(ctx, tenantID, id, operator, reason, closedAt, marketing.OutboxDLQClosed, "OUTBOX_DLQ_CLOSE")
}

func (r *Repository) updateOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, at time.Time, nextStatus marketing.OutboxStatus, action string) (marketing.OutboxMessage, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.OutboxMessage{}, err
	}
	defer rollback(tx)

	var before marketing.OutboxMessage
	err = tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, message_type, target_channel, target_ref, payload_json,
		       send_status, retry_count, next_retry_at, COALESCE(error_message, ''), created_at, updated_at
		FROM marketing_outbox
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`, id, tenantID).Scan(
		&before.ID, &before.TenantID, &before.MessageType, &before.TargetChannel, &before.TargetRef,
		&before.Payload, &before.SendStatus, &before.RetryCount, &before.NextRetryAt,
		&before.ErrorMessage, &before.CreatedAt, &before.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.OutboxMessage{}, marketing.ErrNotFound
		}
		return marketing.OutboxMessage{}, err
	}
	if before.SendStatus != marketing.OutboxDLQ {
		return marketing.OutboxMessage{}, marketing.ErrInvalidOperation
	}
	commandID := outboxRewardCommandID(before.Payload)
	if commandID != "" && nextStatus == marketing.OutboxPending {
		var grantStatus marketing.GrantStatus
		err = tx.QueryRowContext(ctx, `
			SELECT grant_status
			FROM marketing_grant_order
			WHERE tenant_id = $1 AND bill_no = $2
			FOR UPDATE
		`, tenantID, commandID).Scan(&grantStatus)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return marketing.OutboxMessage{}, marketing.ErrNotFound
			}
			return marketing.OutboxMessage{}, err
		}
		if grantStatus != marketing.GrantRetry {
			return marketing.OutboxMessage{}, marketing.ErrInvalidOperation
		}
	}

	var row *sql.Row
	if nextStatus == marketing.OutboxPending {
		row = tx.QueryRowContext(ctx, `
			UPDATE marketing_outbox
			SET send_status = $3, next_retry_at = NULL, updated_at = $4
			WHERE id = $1 AND tenant_id = $2 AND send_status = 'DLQ'
			RETURNING id, tenant_id, message_type, target_channel, target_ref, payload_json,
			          send_status, retry_count, next_retry_at, COALESCE(error_message, ''), created_at, updated_at
		`, id, tenantID, nextStatus, at)
	} else {
		row = tx.QueryRowContext(ctx, `
			UPDATE marketing_outbox
			SET send_status = $3, updated_at = $4
			WHERE id = $1 AND tenant_id = $2 AND send_status = 'DLQ'
			RETURNING id, tenant_id, message_type, target_channel, target_ref, payload_json,
			          send_status, retry_count, next_retry_at, COALESCE(error_message, ''), created_at, updated_at
		`, id, tenantID, nextStatus, at)
	}
	message, err := scanOutbox(row)
	if err != nil {
		return marketing.OutboxMessage{}, err
	}
	auditPayload, _ := json.Marshal(map[string]any{
		"outboxId":      id,
		"operator":      operator,
		"reason":        reason,
		"beforeStatus":  before.SendStatus,
		"afterStatus":   nextStatus,
		"targetChannel": before.TargetChannel,
		"targetRef":     before.TargetRef,
		"retryCount":    before.RetryCount,
		"errorMessage":  before.ErrorMessage,
		"commandId":     commandID,
		"operatedAt":    at,
		"source":        auditSourceFromContext(ctx),
	})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO marketing_audit (
			id, tenant_id, actor, action, resource, resource_id, payload_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, marketing.NewID("audit"), tenantID, operator, action, "OUTBOX", id, auditPayload, at)
	if err != nil {
		return marketing.OutboxMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return marketing.OutboxMessage{}, err
	}
	return message, nil
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

func (r *Repository) ListAudit(ctx context.Context, query marketing.AuditQuery) ([]marketing.AuditRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, actor, action, resource, resource_id, payload_json, created_at
		FROM marketing_audit
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR actor = $2)
		  AND ($3 = '' OR action = $3)
		  AND ($4 = '' OR resource = $4)
		  AND ($5 = '' OR resource_id = $5)
		  AND ($6 = '' OR resource_id = $6 OR payload_json::text LIKE '%' || $6 || '%')
		  AND ($7 = '' OR resource_id = $7 OR payload_json::text LIKE '%' || $7 || '%')
		  AND ($8 = '' OR resource_id = $8 OR payload_json::text LIKE '%' || $8 || '%')
		ORDER BY created_at DESC
		LIMIT $9
	`, query.TenantID, query.Actor, query.Action, query.Resource, query.ResourceID,
		query.ActivityID, query.EventID, query.CommandID, query.Limit)
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

func auditSourceFromContext(ctx context.Context) map[string]any {
	source := marketing.AuditSourceFromContext(ctx)
	if len(source) == 0 {
		return map[string]any{"component": "internal"}
	}
	return source
}

func (r *Repository) outboxesForReward(ctx context.Context, tenantID string, commandID string, participationID string) ([]marketing.OutboxMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, message_type, target_channel, target_ref, payload_json,
		       send_status, retry_count, next_retry_at, COALESCE(error_message, ''), created_at, updated_at
		FROM marketing_outbox
		WHERE tenant_id = $1
		  AND (payload_json ->> 'commandId' = $2 OR payload_json ->> 'participationId' = $3)
		ORDER BY created_at ASC
	`, tenantID, commandID, participationID)
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
	return messages, rows.Err()
}

func (r *Repository) auditTimeline(ctx context.Context, tenantID string, relatedIDs []string) ([]marketing.AuditRecord, error) {
	var records []marketing.AuditRecord
	for _, relatedID := range relatedIDs {
		if relatedID == "" {
			continue
		}
		items, err := r.ListAudit(ctx, marketing.AuditQuery{
			TenantID:   tenantID,
			ResourceID: relatedID,
			Limit:      100,
		})
		if err != nil {
			return nil, err
		}
		records = appendUniqueAudit(records, items)
		items, err = r.ListAudit(ctx, marketing.AuditQuery{
			TenantID:   tenantID,
			ActivityID: relatedID,
			Limit:      100,
		})
		if err != nil {
			return nil, err
		}
		records = appendUniqueAudit(records, items)
	}
	slices.SortFunc(records, func(a, b marketing.AuditRecord) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return records, nil
}

func appendUniqueAudit(records []marketing.AuditRecord, items []marketing.AuditRecord) []marketing.AuditRecord {
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		seen[record.ID] = struct{}{}
	}
	for _, item := range items {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		records = append(records, item)
		seen[item.ID] = struct{}{}
	}
	return records
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
		       grant_channel, flow_multiple::text, enabled, version, created_at, updated_at
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
			&item.GrantMode, &item.GrantChannel, &item.FlowMultiple, &item.Enabled, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		rewards = append(rewards, item)
	}
	return rewards, rows.Err()
}

func (r *Repository) activityByID(ctx context.Context, db activityQuerier, id string) (marketing.Activity, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, tenant_id, activity_code, activity_name, activity_type, status,
		       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		       stackable, plugin_code, version, config_json, created_at, updated_at
		FROM marketing_activity
		WHERE id = $1
	`, id)
	activity, err := scanActivity(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.Activity{}, marketing.ErrNotFound
		}
		return marketing.Activity{}, err
	}
	return activity, nil
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
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&inbox.Event); err != nil {
		return marketing.EventInbox{}, err
	}
	inbox.Event = inbox.Event.Normalized()
	return inbox, nil
}

type activityScanner interface {
	Scan(dest ...any) error
}

type activityQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
		&activity.Stackable, &activity.PluginCode, &activity.Version, &config, &activity.CreatedAt, &activity.UpdatedAt,
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

func ensureActivityUniqueConstraint(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint c
				JOIN pg_class t ON t.oid = c.conrelid
				WHERE t.relname = 'marketing_activity'
				  AND c.conname = 'marketing_activity_tenant_code_key'
			) THEN
				ALTER TABLE marketing_activity
				ADD CONSTRAINT marketing_activity_tenant_code_key UNIQUE (tenant_id, activity_code);
			END IF;
		END
		$$;
	`)
	return err
}

func mapPostgresActivityError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "marketing_activity_tenant_code_key" {
		return marketing.ErrActivityConflict
	}
	return err
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
		SET frozen_value = frozen_value + $5::numeric,
		    version = version + 1,
		    updated_at = NOW()
		WHERE activity_id = $1
		  AND quota_type = $2
		  AND scope = $3
		  AND scope_key = $4
		  AND used_value + frozen_value + $5::numeric <= total_value
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

func reserveQuotaOrExceeded(ctx context.Context, tx *sql.Tx, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, amount string) error {
	ok, err := reserveQuotaTx(ctx, tx, activityID, quotaType, scope, scopeKey, amount)
	if err != nil {
		return err
	}
	if !ok {
		return marketing.QuotaExceededError{Type: quotaType, Scope: scope, ScopeKey: scopeKey, Amount: amount}
	}
	return nil
}

func insertParticipationTx(ctx context.Context, tx *sql.Tx, participation marketing.Participation) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO marketing_participation (
			id, tenant_id, activity_id, event_inbox_id, subject_type, subject_id,
			biz_type, biz_id, status, reject_reason, snapshot_json, idempotency_key,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, participation.ID, participation.TenantID, participation.ActivityID, participation.EventInboxID,
		participation.Subject.Type, participation.Subject.ID, participation.Biz.BizType, participation.Biz.BizID,
		participation.Status, nullString(participation.RejectReason), participation.Snapshot,
		participation.IdempotencyKey, participation.CreatedAt, participation.UpdatedAt)
	return err
}

func postgresQuotaReservationsForAtomicCommand(ctx context.Context, tx *sql.Tx, cmd marketing.AtomicGrantCommand, now time.Time) ([]marketing.QuotaReservation, error) {
	var reservations []marketing.QuotaReservation
	if len(cmd.GrantOrders) > 0 {
		firstOrder := cmd.GrantOrders[0]
		exists, err := quotaExistsTx(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, cmd.UserScopeKey)
		if err != nil {
			return nil, err
		}
		if cmd.ReserveUserLimit && exists {
			reservations = append(reservations, marketing.QuotaReservation{
				ID:           marketing.NewID("reservation"),
				TenantID:     firstOrder.TenantID,
				ActivityID:   firstOrder.ActivityID,
				GrantOrderID: firstOrder.ID,
				CommandID:    firstOrder.BillNo,
				Type:         marketing.QuotaUserLimit,
				Scope:        marketing.QuotaScopeUser,
				ScopeKey:     cmd.UserScopeKey,
				Amount:       "1",
				Status:       marketing.QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
		exists, err = quotaExistsTx(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, cmd.DailyLimitScopeKey)
		if err != nil {
			return nil, err
		}
		if cmd.ReserveDailyLimit && exists {
			reservations = append(reservations, marketing.QuotaReservation{
				ID:           marketing.NewID("reservation"),
				TenantID:     firstOrder.TenantID,
				ActivityID:   firstOrder.ActivityID,
				GrantOrderID: firstOrder.ID,
				CommandID:    firstOrder.BillNo,
				Type:         marketing.QuotaDailyLimit,
				Scope:        marketing.QuotaScopeUser,
				ScopeKey:     cmd.DailyLimitScopeKey,
				Amount:       "1",
				Status:       marketing.QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	for _, order := range cmd.GrantOrders {
		exists, err := quotaExistsTx(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL")
		if err != nil {
			return nil, err
		}
		if order.RewardAmount != "" && exists {
			reservations = append(reservations, marketing.QuotaReservation{
				ID:           marketing.NewID("reservation"),
				TenantID:     order.TenantID,
				ActivityID:   order.ActivityID,
				GrantOrderID: order.ID,
				CommandID:    order.BillNo,
				Type:         marketing.QuotaBudget,
				Scope:        marketing.QuotaScopeActivity,
				ScopeKey:     "TOTAL",
				Amount:       order.RewardAmount,
				Status:       marketing.QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
		rewardCode := rewardCodeFromPayload(order.RewardPayload)
		exists, err = quotaExistsTx(ctx, tx, cmd.Participation.ActivityID, marketing.QuotaStock, marketing.QuotaScopeActivity, rewardCode)
		if err != nil {
			return nil, err
		}
		if rewardCode != "" && exists {
			reservations = append(reservations, marketing.QuotaReservation{
				ID:           marketing.NewID("reservation"),
				TenantID:     order.TenantID,
				ActivityID:   order.ActivityID,
				GrantOrderID: order.ID,
				CommandID:    order.BillNo,
				Type:         marketing.QuotaStock,
				Scope:        marketing.QuotaScopeActivity,
				ScopeKey:     rewardCode,
				Amount:       "1",
				Status:       marketing.QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	return reservations, nil
}

func quotaExistsTx(ctx context.Context, tx *sql.Tx, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM marketing_quota
			WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
		)
	`, activityID, quotaType, scope, scopeKey).Scan(&exists)
	return exists, err
}

func isParticipationQuotaReservation(reservation marketing.QuotaReservation) bool {
	return reservation.Type == marketing.QuotaUserLimit || reservation.Type == marketing.QuotaDailyLimit
}

func rewardCodeFromPayload(payload json.RawMessage) string {
	var body struct {
		RewardCode string `json:"rewardCode"`
	}
	_ = json.Unmarshal(payload, &body)
	return body.RewardCode
}

func grantModeFromPayload(payload json.RawMessage) marketing.GrantMode {
	var body struct {
		GrantMode marketing.GrantMode `json:"grantMode"`
	}
	_ = json.Unmarshal(payload, &body)
	if body.GrantMode == "" {
		return marketing.GrantAuto
	}
	return body.GrantMode
}

func marketingIsFinalGrantStatus(status marketing.GrantStatus) bool {
	return status == marketing.GrantSuccess || status == marketing.GrantFailed || status == marketing.GrantClosed
}

func manualGrantOrderForUpdateTx(ctx context.Context, tx *sql.Tx, cmd marketing.ManualGrantDecisionCommand) (marketing.GrantOrder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, tenant_id, activity_id, participation_id, subject_type, subject_id,
		       reward_type, reward_amount::text, reward_payload_json, bill_no, grant_status, grant_channel,
		       COALESCE(error_message, ''), COALESCE(external_grant_id, ''), finished_at, idempotency_key, created_at, updated_at
		FROM marketing_grant_order
		WHERE tenant_id = $1
		  AND (($2 <> '' AND id = $2) OR ($3 <> '' AND bill_no = $3) OR ($4 <> '' AND idempotency_key = $4))
		FOR UPDATE
	`, cmd.TenantID, cmd.GrantOrderID, cmd.CommandID, cmd.IdempotencyKey)
	if err != nil {
		return marketing.GrantOrder{}, err
	}
	defer rows.Close()

	var order marketing.GrantOrder
	var externalGrantID sql.NullString
	var finishedAt sql.NullTime
	found := 0
	for rows.Next() {
		found++
		if found > 1 {
			return marketing.GrantOrder{}, marketing.ErrCallbackLocatorConflict
		}
		if err := rows.Scan(&order.ID, &order.TenantID, &order.ActivityID, &order.ParticipationID,
			&order.Subject.Type, &order.Subject.ID, &order.RewardType, &order.RewardAmount, &order.RewardPayload,
			&order.BillNo, &order.GrantStatus, &order.GrantChannel, &order.ErrorMessage, &externalGrantID, &finishedAt,
			&order.IdempotencyKey, &order.CreatedAt, &order.UpdatedAt); err != nil {
			return marketing.GrantOrder{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return marketing.GrantOrder{}, err
	}
	if found == 0 {
		return marketing.GrantOrder{}, marketing.ErrNotFound
	}
	if cmd.GrantOrderID != "" && order.ID != cmd.GrantOrderID {
		return marketing.GrantOrder{}, marketing.ErrCallbackLocatorConflict
	}
	if cmd.CommandID != "" && order.BillNo != cmd.CommandID {
		return marketing.GrantOrder{}, marketing.ErrCallbackLocatorConflict
	}
	if cmd.IdempotencyKey != "" && order.IdempotencyKey != cmd.IdempotencyKey {
		return marketing.GrantOrder{}, marketing.ErrCallbackLocatorConflict
	}
	order.ExternalGrantID = externalGrantID.String
	if finishedAt.Valid {
		order.FinishedAt = &finishedAt.Time
	}
	return order, nil
}

func hasReadyManualExportDetailTx(ctx context.Context, tx *sql.Tx, grantOrderID string) bool {
	var exists bool
	_ = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM marketing_manual_grant_export_detail d
			JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
			WHERE d.grant_order_id = $1
			  AND d.detail_status = 'READY'
			  AND f.status = 'READY'
		)
	`, grantOrderID).Scan(&exists)
	return exists
}

func manualGrantExportCandidatesTx(ctx context.Context, tx *sql.Tx, cmd marketing.ManualGrantExportCreateCommand) ([]marketing.GrantOrder, error) {
	limit := cmd.Limit
	if limit <= 0 {
		limit = 1000
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT g.id, g.tenant_id, g.activity_id, g.participation_id, g.subject_type, g.subject_id,
		       g.reward_type, g.reward_amount::text, g.reward_payload_json, g.bill_no, g.grant_status,
		       g.grant_channel, COALESCE(g.error_message, ''), COALESCE(g.external_grant_id, ''),
		       g.finished_at, g.idempotency_key, g.created_at, g.updated_at
		FROM marketing_grant_order g
		WHERE g.tenant_id = $1
		  AND g.grant_status IN ('PENDING', 'RETRY')
		  AND NOT EXISTS (
		    SELECT 1
		    FROM marketing_manual_grant_export_detail d
		    JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		    WHERE d.grant_order_id = g.id AND f.status <> 'FAILED'
		  )
		ORDER BY g.created_at ASC, g.id ASC
		LIMIT $2
		FOR UPDATE OF g
	`, cmd.TenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowedIDs := map[string]struct{}{}
	for _, id := range cmd.GrantOrderIDs {
		if strings.TrimSpace(id) != "" {
			allowedIDs[id] = struct{}{}
		}
	}
	var out []marketing.GrantOrder
	for rows.Next() {
		var order marketing.GrantOrder
		var externalGrantID sql.NullString
		var finishedAt sql.NullTime
		if err := rows.Scan(&order.ID, &order.TenantID, &order.ActivityID, &order.ParticipationID,
			&order.Subject.Type, &order.Subject.ID, &order.RewardType, &order.RewardAmount, &order.RewardPayload,
			&order.BillNo, &order.GrantStatus, &order.GrantChannel, &order.ErrorMessage, &externalGrantID, &finishedAt,
			&order.IdempotencyKey, &order.CreatedAt, &order.UpdatedAt); err != nil {
			return nil, err
		}
		if externalGrantID.Valid {
			order.ExternalGrantID = externalGrantID.String
		}
		if finishedAt.Valid {
			order.FinishedAt = &finishedAt.Time
		}
		if grantModeFromPayload(order.RewardPayload) != marketing.GrantManualExport {
			continue
		}
		if len(allowedIDs) > 0 {
			if _, ok := allowedIDs[order.ID]; !ok {
				continue
			}
		}
		if cmd.ActivityID != "" && order.ActivityID != cmd.ActivityID {
			continue
		}
		if cmd.SubjectType != "" && order.Subject.Type != cmd.SubjectType {
			continue
		}
		if cmd.SubjectID != "" && order.Subject.ID != cmd.SubjectID {
			continue
		}
		if !cmd.CreatedFrom.IsZero() && order.CreatedAt.Before(cmd.CreatedFrom) {
			continue
		}
		if !cmd.CreatedTo.IsZero() && !order.CreatedAt.Before(cmd.CreatedTo) {
			continue
		}
		if cmd.RewardCode != "" && rewardCodeFromPayload(order.RewardPayload) != cmd.RewardCode {
			continue
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

func manualGrantExportCSV(exportID string, orders []marketing.GrantOrder) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"tenantId", "exportId", "billNo", "grantOrderId", "idempotencyKey", "activityId", "participationId", "subjectType", "subjectId", "rewardCode", "rewardType", "rewardAmount", "rewardPayload", "grantStatus", "createdAt"}); err != nil {
		return nil, "", err
	}
	for _, order := range orders {
		if err := writer.Write([]string{
			order.TenantID,
			exportID,
			order.BillNo,
			order.ID,
			order.IdempotencyKey,
			order.ActivityID,
			order.ParticipationID,
			order.Subject.Type,
			order.Subject.ID,
			rewardCodeFromPayload(order.RewardPayload),
			string(order.RewardType),
			order.RewardAmount,
			string(order.RewardPayload),
			string(order.GrantStatus),
			order.CreatedAt.Format(time.RFC3339Nano),
		}); err != nil {
			return nil, "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

func quotaReservationsForOrderTx(ctx context.Context, tx *sql.Tx, grantOrderID string) ([]marketing.QuotaReservation, error) {
	return quotaReservationsForOrder(ctx, tx, grantOrderID)
}

type quotaReservationQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func quotaReservationsForOrder(ctx context.Context, db quotaReservationQuerier, grantOrderID string) ([]marketing.QuotaReservation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_id, grant_order_id, command_id, quota_type,
		       scope, scope_key, reserved_amount::text, reservation_status,
		       created_at, updated_at, confirmed_at, released_at
		FROM marketing_quota_reservation
		WHERE grant_order_id = $1
		ORDER BY created_at ASC, id ASC
	`, grantOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reservations []marketing.QuotaReservation
	for rows.Next() {
		var reservation marketing.QuotaReservation
		var confirmedAt sql.NullTime
		var releasedAt sql.NullTime
		if err := rows.Scan(&reservation.ID, &reservation.TenantID, &reservation.ActivityID,
			&reservation.GrantOrderID, &reservation.CommandID, &reservation.Type, &reservation.Scope,
			&reservation.ScopeKey, &reservation.Amount, &reservation.Status,
			&reservation.CreatedAt, &reservation.UpdatedAt, &confirmedAt, &releasedAt); err != nil {
			return nil, err
		}
		if confirmedAt.Valid {
			reservation.ConfirmedAt = &confirmedAt.Time
		}
		if releasedAt.Valid {
			reservation.ReleasedAt = &releasedAt.Time
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func finishQuotaReservationTx(ctx context.Context, tx *sql.Tx, reservation marketing.QuotaReservation, status marketing.GrantStatus) error {
	var result sql.Result
	var err error
	if status == marketing.GrantSuccess {
		result, err = tx.ExecContext(ctx, `
			UPDATE marketing_quota
			SET frozen_value = frozen_value - $5::numeric,
			    used_value = used_value + $5::numeric,
			    version = version + 1,
			    updated_at = NOW()
			WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
			  AND frozen_value >= $5::numeric
		`, reservation.ActivityID, reservation.Type, reservation.Scope, reservation.ScopeKey, emptyDecimal(reservation.Amount))
	} else {
		result, err = tx.ExecContext(ctx, `
			UPDATE marketing_quota
			SET frozen_value = frozen_value - $5::numeric,
			    version = version + 1,
			    updated_at = NOW()
			WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
			  AND frozen_value >= $5::numeric
		`, reservation.ActivityID, reservation.Type, reservation.Scope, reservation.ScopeKey, emptyDecimal(reservation.Amount))
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return marketing.ErrQuotaAggregateConflict
	}
	nextStatus := marketing.QuotaReservationReleased
	timeColumn := "released_at"
	if status == marketing.GrantSuccess {
		nextStatus = marketing.QuotaReservationConfirmed
		timeColumn = "confirmed_at"
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE marketing_quota_reservation
		SET reservation_status = $2,
		    updated_at = NOW(),
		    `+timeColumn+` = NOW()
		WHERE id = $1 AND reservation_status = 'FROZEN'
	`, reservation.ID, nextStatus)
	if err != nil {
		return err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return marketing.ErrQuotaReservationConflict
	}
	return nil
}

func finishParticipationQuotaReservationsTx(ctx context.Context, tx *sql.Tx, participationID string, status marketing.GrantStatus) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.id, r.tenant_id, r.activity_id, r.grant_order_id, r.command_id, r.quota_type,
		       r.scope, r.scope_key, r.reserved_amount::text, r.reservation_status,
		       r.created_at, r.updated_at, r.confirmed_at, r.released_at
		FROM marketing_quota_reservation r
		JOIN marketing_grant_order g ON g.id = r.grant_order_id
		WHERE g.participation_id = $1
		  AND r.quota_type IN ('USER_LIMIT', 'DAILY_LIMIT')
		  AND r.reservation_status = 'FROZEN'
		ORDER BY r.created_at ASC, r.id ASC
		FOR UPDATE OF r
	`, participationID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var reservations []marketing.QuotaReservation
	for rows.Next() {
		var reservation marketing.QuotaReservation
		var confirmedAt sql.NullTime
		var releasedAt sql.NullTime
		if err := rows.Scan(&reservation.ID, &reservation.TenantID, &reservation.ActivityID,
			&reservation.GrantOrderID, &reservation.CommandID, &reservation.Type, &reservation.Scope,
			&reservation.ScopeKey, &reservation.Amount, &reservation.Status,
			&reservation.CreatedAt, &reservation.UpdatedAt, &confirmedAt, &releasedAt); err != nil {
			return err
		}
		if confirmedAt.Valid {
			reservation.ConfirmedAt = &confirmedAt.Time
		}
		if releasedAt.Valid {
			reservation.ReleasedAt = &releasedAt.Time
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, reservation := range reservations {
		if err := finishQuotaReservationTx(ctx, tx, reservation, status); err != nil {
			return err
		}
	}
	return nil
}

func closeGrantOrderForOutboxTx(ctx context.Context, tx *sql.Tx, tenantID string, commandID string, reason string, closedAt time.Time) error {
	var order marketing.GrantOrder
	err := tx.QueryRowContext(ctx, `
		SELECT id, participation_id
		FROM marketing_grant_order
		WHERE tenant_id = $1 AND bill_no = $2
		FOR UPDATE
	`, tenantID, commandID).Scan(&order.ID, &order.ParticipationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ErrNotFound
		}
		return err
	}
	reservations, err := quotaReservationsForOrderTx(ctx, tx, order.ID)
	if err != nil {
		return err
	}
	for _, reservation := range reservations {
		if reservation.Status != marketing.QuotaReservationFrozen || isParticipationQuotaReservation(reservation) {
			continue
		}
		if err := finishQuotaReservationTx(ctx, tx, reservation, marketing.GrantClosed); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE marketing_grant_order
		SET grant_status = 'CLOSED',
		    error_message = $3,
		    finished_at = $4,
		    updated_at = $4
		WHERE tenant_id = $1 AND bill_no = $2
		  AND grant_status NOT IN ('SUCCESS', 'FAILED', 'CLOSED')
	`, tenantID, commandID, nullString(reason), closedAt)
	if err != nil {
		return err
	}
	status, err := updateParticipationAfterCallbackTx(ctx, tx, order.ParticipationID)
	if err != nil {
		return err
	}
	if status == marketing.ParticipationGranted || status == marketing.ParticipationFailed {
		finalStatus := marketing.GrantSuccess
		if status == marketing.ParticipationFailed {
			finalStatus = marketing.GrantClosed
		}
		if err := finishParticipationQuotaReservationsTx(ctx, tx, order.ParticipationID, finalStatus); err != nil {
			return err
		}
	}
	return nil
}

func participationStatusTx(ctx context.Context, tx *sql.Tx, participationID string) (marketing.ParticipationStatus, error) {
	var status marketing.ParticipationStatus
	err := tx.QueryRowContext(ctx, `SELECT status FROM marketing_participation WHERE id = $1`, participationID).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", marketing.ErrNotFound
		}
		return "", err
	}
	return status, nil
}

func updateParticipationAfterCallbackTx(ctx context.Context, tx *sql.Tx, participationID string) (marketing.ParticipationStatus, error) {
	rows, err := tx.QueryContext(ctx, `SELECT grant_status FROM marketing_grant_order WHERE participation_id = $1`, participationID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	allFinal := true
	anySuccess := false
	anyFailed := false
	for rows.Next() {
		var status marketing.GrantStatus
		if err := rows.Scan(&status); err != nil {
			return "", err
		}
		switch status {
		case marketing.GrantFailed, marketing.GrantClosed:
			anyFailed = true
		case marketing.GrantSuccess:
			anySuccess = true
		default:
			allFinal = false
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if !allFinal {
		return participationStatusTx(ctx, tx, participationID)
	}
	next := marketing.ParticipationGranted
	if anyFailed && !anySuccess {
		next = marketing.ParticipationFailed
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE marketing_participation
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, next, participationID)
	if err != nil {
		return "", err
	}
	return next, nil
}

func activityHadQuotaAtTx(ctx context.Context, tx *sql.Tx, activityID string, at time.Time) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM marketing_quota
		WHERE activity_id = $1
		  AND created_at <= $2
	`, activityID, at).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func outboxRewardCommandID(payload json.RawMessage) string {
	var command marketing.RewardCommand
	if err := json.Unmarshal(payload, &command); err != nil {
		return ""
	}
	return string(bytes.TrimSpace([]byte(command.CommandID)))
}

var _ marketing.Repository = (*Repository)(nil)

func sortActivitiesByPriority(activities []marketing.Activity) {
	slices.SortFunc(activities, func(a, b marketing.Activity) int {
		return b.Priority - a.Priority
	})
}
