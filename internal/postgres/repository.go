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
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/dbmigration"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
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
	if err := (dbmigration.Manager{DB: db}).Drift(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureActivityUniqueConstraint(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureManualGrantExportTables(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureWorkspaceTables(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureActivityWorkspaceColumns(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureWorkflowVersionColumns(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureWorkflowDraftTemplateTables(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) GetUserProfile(ctx context.Context, tenantID string, subjectType string, subjectID string) (marketing.UserProfile, error) {
	var profile marketing.UserProfile
	var attributes []byte
	var registeredAt sql.NullTime
	var country, channel sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, subject_type, subject_id, user_level, vip_level, country, channel,
		       registered_at, status, attributes_json, created_at, updated_at
		FROM marketing_user_profile
		WHERE tenant_id = $1 AND subject_type = $2 AND subject_id = $3
	`, tenantID, subjectType, subjectID).Scan(
		&profile.TenantID, &profile.SubjectType, &profile.SubjectID, &profile.UserLevel, &profile.VIPLevel,
		&country, &channel, &registeredAt, &profile.Status, &attributes, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.UserProfile{}, marketing.ErrNotFound
		}
		return marketing.UserProfile{}, err
	}
	profile.Country = country.String
	profile.Channel = channel.String
	if registeredAt.Valid {
		profile.RegisteredAt = registeredAt.Time
	}
	if len(attributes) > 0 {
		_ = json.Unmarshal(attributes, &profile.Attributes)
	}
	if profile.Attributes == nil {
		profile.Attributes = map[string]any{}
	}
	return profile, nil
}

func (r *Repository) GetKYCProfile(ctx context.Context, tenantID string, subjectType string, subjectID string) (marketing.KYCProfile, error) {
	var profile marketing.KYCProfile
	var attributes []byte
	var verifiedAt sql.NullTime
	var country, provider sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, subject_type, subject_id, kyc_status, kyc_level, country, provider,
		       verified_at, attributes_json, created_at, updated_at
		FROM marketing_kyc_profile
		WHERE tenant_id = $1 AND subject_type = $2 AND subject_id = $3
	`, tenantID, subjectType, subjectID).Scan(
		&profile.TenantID, &profile.SubjectType, &profile.SubjectID, &profile.Status, &profile.Level,
		&country, &provider, &verifiedAt, &attributes, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.KYCProfile{}, marketing.ErrNotFound
		}
		return marketing.KYCProfile{}, err
	}
	profile.Country = country.String
	profile.Provider = provider.String
	if verifiedAt.Valid {
		profile.VerifiedAt = verifiedAt.Time
	}
	if len(attributes) > 0 {
		_ = json.Unmarshal(attributes, &profile.Attributes)
	}
	if profile.Attributes == nil {
		profile.Attributes = map[string]any{}
	}
	return profile, nil
}

func (r *Repository) GetRiskProfile(ctx context.Context, tenantID string, subjectType string, subjectID string) (marketing.RiskProfile, error) {
	var profile marketing.RiskProfile
	var attributes []byte
	var assessedAt sql.NullTime
	var country, provider sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, subject_type, subject_id, risk_score, risk_level, risk_status, blacklisted,
		       country, provider, assessed_at, attributes_json, created_at, updated_at
		FROM marketing_risk_profile
		WHERE tenant_id = $1 AND subject_type = $2 AND subject_id = $3
	`, tenantID, subjectType, subjectID).Scan(
		&profile.TenantID, &profile.SubjectType, &profile.SubjectID, &profile.Score, &profile.Level,
		&profile.Status, &profile.Blacklisted, &country, &provider, &assessedAt, &attributes,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.RiskProfile{}, marketing.ErrNotFound
		}
		return marketing.RiskProfile{}, err
	}
	profile.Country = country.String
	profile.Provider = provider.String
	if assessedAt.Valid {
		profile.AssessedAt = assessedAt.Time
	}
	if len(attributes) > 0 {
		_ = json.Unmarshal(attributes, &profile.Attributes)
	}
	if profile.Attributes == nil {
		profile.Attributes = map[string]any{}
	}
	return profile, nil
}

func (r *Repository) CreateActivity(ctx context.Context, activity marketing.Activity) (marketing.Activity, error) {
	if activity.Visibility == "" {
		activity.Visibility = marketing.WorkspaceTeam
	}
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
				id, tenant_id, workspace_id, folder_id, activity_code, activity_name, activity_type, status, visibility,
				trigger_events, start_time, end_time, timezone, priority, exclusive_group,
				stackable, plugin_code, version, config_json, created_at, updated_at
			) VALUES (
				$1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, $9,
				$10, $11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20, $21
			)
		`, activity.ID, activity.TenantID, activity.WorkspaceID, activity.FolderID, activity.Code, activity.Name, activity.Type, activity.Status, activity.Visibility,
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
			SELECT id, tenant_id, COALESCE(workspace_id, ''), COALESCE(folder_id, ''), activity_code, activity_name, activity_type, status, COALESCE(visibility, 'TEAM'),
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
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range activities {
		activity := &activities[i]
		activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
	}
	return activities, nil
}

func (r *Repository) ListActivitiesByQuery(ctx context.Context, query marketing.ActivityQuery) ([]marketing.Activity, error) {
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	args := []any{}
	conditions := []string{"1 = 1"}
	if strings.TrimSpace(query.TenantID) != "" {
		args = append(args, query.TenantID)
		conditions = append(conditions, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if strings.TrimSpace(query.WorkspaceID) != "" {
		args = append(args, query.WorkspaceID)
		conditions = append(conditions, "COALESCE(workspace_id, '') = $"+strconv.Itoa(len(args)))
	}
	if strings.TrimSpace(query.FolderID) != "" {
		args = append(args, query.FolderID)
		conditions = append(conditions, "COALESCE(folder_id, '') = $"+strconv.Itoa(len(args)))
	}
	if strings.TrimSpace(query.Status) != "" {
		args = append(args, query.Status)
		conditions = append(conditions, "status = $"+strconv.Itoa(len(args)))
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `
			SELECT id, tenant_id, COALESCE(workspace_id, ''), COALESCE(folder_id, ''), activity_code, activity_name, activity_type, status, COALESCE(visibility, 'TEAM'),
			       trigger_events, start_time, end_time, timezone, priority, exclusive_group,
			       stackable, plugin_code, version, config_json, created_at, updated_at
		FROM marketing_activity
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY created_at DESC
		LIMIT $`+strconv.Itoa(len(args))+`
	`, args...)
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
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range activities {
		activity := &activities[i]
		activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
	}
	return activities, nil
}

func (r *Repository) UpdateActivity(ctx context.Context, activity marketing.Activity) (marketing.Activity, error) {
	if activity.Visibility == "" {
		activity.Visibility = marketing.WorkspaceTeam
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE marketing_activity
		SET activity_name = $2,
		    status = $3,
		    visibility = $4,
		    start_time = $5,
		    end_time = $6,
		    timezone = $7,
		    priority = $8,
		    exclusive_group = $9,
		    stackable = $10,
		    version = $11,
		    config_json = $12,
		    workspace_id = NULLIF($13, ''),
		    folder_id = NULLIF($14, ''),
		    updated_at = $15
		WHERE id = $1
		RETURNING id, tenant_id, COALESCE(workspace_id, ''), COALESCE(folder_id, ''), activity_code, activity_name, activity_type, status, COALESCE(visibility, 'TEAM'),
		          trigger_events, start_time, end_time, timezone, priority, exclusive_group,
		          stackable, plugin_code, version, config_json, created_at, updated_at
	`, activity.ID, activity.Name, activity.Status, activity.Visibility, activity.StartTime, activity.EndTime, activity.Timezone,
		activity.Priority, activity.ExclusiveGroup, activity.Stackable, activity.Version, nullJSON(activity.Config), activity.WorkspaceID, activity.FolderID, activity.UpdatedAt)
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
			SELECT id, tenant_id, COALESCE(workspace_id, ''), COALESCE(folder_id, ''), activity_code, activity_name, activity_type, status, COALESCE(visibility, 'TEAM'),
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

	var activities []marketing.Activity
	for rows.Next() {
		activity, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range activities {
		activity := &activities[i]
		activity.Rules, err = r.rulesForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
		activity.Rewards, err = r.rewardsForActivity(ctx, activity.ID)
		if err != nil {
			return nil, err
		}
	}
	return activities, nil
}

func (r *Repository) SaveWorkflowDraft(ctx context.Context, draft marketing.WorkflowDraft) (marketing.WorkflowDraft, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workflow_draft (
			activity_id, tenant_id, definition_json, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (activity_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			definition_json = EXCLUDED.definition_json,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`, draft.ActivityID, draft.TenantID, draft.Definition, nullString(draft.UpdatedBy), draft.CreatedAt, draft.UpdatedAt)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	return r.GetWorkflowDraft(ctx, draft.ActivityID)
}

func (r *Repository) GetWorkflowDraft(ctx context.Context, activityID string) (marketing.WorkflowDraft, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT activity_id, tenant_id, definition_json, COALESCE(updated_by, ''), created_at, updated_at
		FROM marketing_workflow_draft
		WHERE activity_id = $1
	`, activityID)
	return scanWorkflowDraft(row)
}

func (r *Repository) CreateWorkflowDraftTemplate(ctx context.Context, template marketing.WorkflowDraftTemplate) (marketing.WorkflowDraftTemplate, error) {
	if template.Version <= 0 {
		if err := r.db.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM marketing_workflow_draft_template
			WHERE template_id = $1
		`, template.ID).Scan(&template.Version); err != nil {
			return marketing.WorkflowDraftTemplate{}, err
		}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workflow_draft_template (
			version_id, template_id, version, name, description, category,
			workspace_id, folder_id, visibility, tags_json, source,
			status, definition_json, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, template.VersionID, template.ID, template.Version, template.Name, nullString(template.Description),
		nullString(template.Category), template.WorkspaceID, template.FolderID, template.Visibility, jsonString(template.Tags),
		template.Source, template.Status, template.Definition, nullString(template.CreatedBy), template.CreatedAt, template.UpdatedAt)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	return template, nil
}

func (r *Repository) ListWorkflowDraftTemplates(ctx context.Context, query marketing.WorkflowDraftTemplateQuery) ([]marketing.WorkflowDraftTemplate, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	latestClause := ""
	if query.LatestOnly {
		latestClause = "AND version = (SELECT MAX(t2.version) FROM marketing_workflow_draft_template t2 WHERE t2.template_id = t.template_id)"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT version_id, template_id, version, name, COALESCE(description, ''), COALESCE(category, ''),
		       COALESCE(workspace_id, ''), COALESCE(folder_id, ''), COALESCE(visibility, ''), COALESCE(tags_json, '[]'::jsonb),
		       source, status, definition_json, COALESCE(created_by, ''), created_at, updated_at
		FROM marketing_workflow_draft_template t
		WHERE ($1 = '' OR template_id = $1)
		  AND ($2 = '' OR source = $2)
		  AND ($3 = '' OR status = $3)
		  AND ($4 = '' OR COALESCE(workspace_id, '') = $4)
		  AND ($5 = '' OR COALESCE(folder_id, '') = $5)
		  AND ($6 = '' OR COALESCE(visibility, '') = $6)
		  `+latestClause+`
		ORDER BY template_id ASC, version DESC
		LIMIT $7
	`, query.TemplateID, query.Source, query.Status, query.WorkspaceID, query.FolderID, query.Visibility, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkflowDraftTemplate
	for rows.Next() {
		item, err := scanWorkflowDraftTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) GetWorkflowDraftTemplate(ctx context.Context, templateID string, versionID string, version int) (marketing.WorkflowDraftTemplate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT version_id, template_id, version, name, COALESCE(description, ''), COALESCE(category, ''),
		       COALESCE(workspace_id, ''), COALESCE(folder_id, ''), COALESCE(visibility, ''), COALESCE(tags_json, '[]'::jsonb),
		       source, status, definition_json, COALESCE(created_by, ''), created_at, updated_at
		FROM marketing_workflow_draft_template
		WHERE template_id = $1
		  AND ($2 = '' OR version_id = $2)
		  AND ($3 <= 0 OR version = $3)
		ORDER BY version DESC
		LIMIT 1
	`, templateID, versionID, version)
	return scanWorkflowDraftTemplate(row)
}

func (r *Repository) UpdateWorkflowDraftTemplate(ctx context.Context, template marketing.WorkflowDraftTemplate) (marketing.WorkflowDraftTemplate, error) {
	updated, err := scanWorkflowDraftTemplate(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workflow_draft_template
		SET name = $2, description = NULLIF($3, ''), category = NULLIF($4, ''),
		    workspace_id = $5, folder_id = $6, visibility = $7,
		    tags_json = $8, status = $9, updated_at = $10
		WHERE version_id = $1
		RETURNING version_id, template_id, version, name, COALESCE(description, ''), COALESCE(category, ''),
		          COALESCE(workspace_id, ''), COALESCE(folder_id, ''), COALESCE(visibility, ''), COALESCE(tags_json, '[]'::jsonb),
		          source, status, definition_json, COALESCE(created_by, ''), created_at, updated_at
	`, template.VersionID, template.Name, template.Description, template.Category, template.WorkspaceID, template.FolderID,
		template.Visibility, jsonString(template.Tags), template.Status, template.UpdatedAt))
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, mapPostgresActivityError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkflowVersion(ctx context.Context, version marketing.WorkflowVersion) (marketing.WorkflowVersion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	defer rollback(tx)

	var lockedActivityID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM marketing_activity
		WHERE id = $1
		FOR UPDATE
	`, version.ActivityID).Scan(&lockedActivityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkflowVersion{}, marketing.ErrNotFound
		}
		return marketing.WorkflowVersion{}, err
	}
	if version.Version == 0 {
		err = tx.QueryRowContext(ctx, `
				SELECT COALESCE(MAX(version), 0) + 1
				FROM marketing_workflow_version
				WHERE activity_id = $1 AND version > 0
			`, version.ActivityID).Scan(&version.Version)
		if err != nil {
			return marketing.WorkflowVersion{}, err
		}
	}
	if version.Status == "" {
		version.Status = marketing.WorkflowVersionActive
	}
	if version.Status == marketing.WorkflowVersionActive {
		if _, err := tx.ExecContext(ctx, `
			UPDATE marketing_workflow_version
			SET status = $2
			WHERE activity_id = $1 AND status = $3
		`, version.ActivityID, marketing.WorkflowVersionSuperseded, marketing.WorkflowVersionActive); err != nil {
			return marketing.WorkflowVersion{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO marketing_workflow_version (
			id, tenant_id, activity_id, version, definition_json, status,
			published_by, published_at, release_note, source_version_id, action, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, version.ID, version.TenantID, version.ActivityID, version.Version, version.Definition, version.Status,
		nullString(version.PublishedBy), version.PublishedAt, nullString(version.ReleaseNote), nullString(version.SourceVersionID), nullString(version.Action), version.CreatedAt)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return marketing.WorkflowVersion{}, err
	}
	return version, nil
}

func (r *Repository) GetActiveWorkflowVersion(ctx context.Context, activityID string) (marketing.WorkflowVersion, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, activity_id, version, definition_json, status,
		       COALESCE(published_by, ''), published_at, COALESCE(release_note, ''), COALESCE(source_version_id, ''), COALESCE(action, ''), created_at
		FROM marketing_workflow_version
		WHERE activity_id = $1 AND status = $2
	`, activityID, marketing.WorkflowVersionActive)
	return scanWorkflowVersion(row)
}

func (r *Repository) GetWorkflowVersion(ctx context.Context, activityID string, versionID string) (marketing.WorkflowVersion, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, activity_id, version, definition_json, status,
		       COALESCE(published_by, ''), published_at, COALESCE(release_note, ''), COALESCE(source_version_id, ''), COALESCE(action, ''), created_at
		FROM marketing_workflow_version
		WHERE activity_id = $1 AND id = $2
	`, activityID, versionID)
	return scanWorkflowVersion(row)
}

func (r *Repository) ListWorkflowVersions(ctx context.Context, activityID string) ([]marketing.WorkflowVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_id, version, definition_json, status,
		       COALESCE(published_by, ''), published_at, COALESCE(release_note, ''), COALESCE(source_version_id, ''), COALESCE(action, ''), created_at
		FROM marketing_workflow_version
		WHERE activity_id = $1
		ORDER BY version DESC
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []marketing.WorkflowVersion
	for rows.Next() {
		version, err := scanWorkflowVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (r *Repository) UpdateWorkflowVersionStatus(ctx context.Context, activityID string, versionID string, status marketing.WorkflowVersionStatus) (marketing.WorkflowVersion, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE marketing_workflow_version
		SET status = $3
		WHERE activity_id = $1 AND id = $2
		RETURNING id, tenant_id, activity_id, version, definition_json, status,
		          COALESCE(published_by, ''), published_at, COALESCE(release_note, ''), COALESCE(source_version_id, ''), COALESCE(action, ''), created_at
	`, activityID, versionID, status)
	return scanWorkflowVersion(row)
}

func (r *Repository) CreateWorkflowExecution(ctx context.Context, execution marketing.WorkflowExecution) (marketing.WorkflowExecution, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workflow_execution (
			id, tenant_id, activity_id, workflow_version_id, workflow_version,
			event_inbox_id, event_id, status, input_json, output_json, error_message,
			started_at, finished_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, execution.ID, execution.TenantID, execution.ActivityID, execution.WorkflowVersionID, execution.WorkflowVersion,
		nullString(execution.EventInboxID), nullString(execution.EventID), execution.Status, nullJSON(execution.Input),
		nullJSON(execution.Output), nullString(execution.ErrorMessage), execution.StartedAt, execution.FinishedAt,
		execution.CreatedAt, execution.UpdatedAt)
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	return execution, nil
}

func (r *Repository) UpdateWorkflowExecution(ctx context.Context, execution marketing.WorkflowExecution) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE marketing_workflow_execution
		SET status = $2,
		    output_json = $3,
		    error_message = $4,
		    finished_at = $5,
		    updated_at = $6
		WHERE id = $1
	`, execution.ID, execution.Status, nullJSON(execution.Output), nullString(execution.ErrorMessage),
		execution.FinishedAt, execution.UpdatedAt)
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

func (r *Repository) CreateWorkflowNodeExecution(ctx context.Context, execution marketing.WorkflowNodeExecution) (marketing.WorkflowNodeExecution, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workflow_node_execution (
			id, tenant_id, execution_id, node_id, node_type, node_title, status,
			input_json, output_json, error_message, started_at, finished_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, execution.ID, execution.TenantID, execution.ExecutionID, execution.NodeID, execution.NodeType,
		nullString(execution.NodeTitle), execution.Status, nullJSON(execution.Input), nullJSON(execution.Output),
		nullString(execution.ErrorMessage), execution.StartedAt, execution.FinishedAt, execution.CreatedAt, execution.UpdatedAt)
	if err != nil {
		return marketing.WorkflowNodeExecution{}, err
	}
	return execution, nil
}

func (r *Repository) UpdateWorkflowNodeExecution(ctx context.Context, execution marketing.WorkflowNodeExecution) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE marketing_workflow_node_execution
		SET status = $2,
		    output_json = $3,
		    error_message = $4,
		    finished_at = $5,
		    updated_at = $6
		WHERE id = $1
	`, execution.ID, execution.Status, nullJSON(execution.Output), nullString(execution.ErrorMessage),
		execution.FinishedAt, execution.UpdatedAt)
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

func (r *Repository) ListWorkflowExecutions(ctx context.Context, query marketing.WorkflowExecutionQuery) ([]marketing.WorkflowExecution, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, activity_id, workflow_version_id, workflow_version,
		       COALESCE(event_inbox_id, ''), COALESCE(event_id, ''), status,
		       input_json, output_json, COALESCE(error_message, ''),
		       started_at, finished_at, created_at, updated_at
		FROM marketing_workflow_execution
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR activity_id = $2)
		  AND ($3 = '' OR event_id = $3)
		  AND ($4 = '' OR id = $4)
		  AND ($5 = '' OR workflow_version_id = $5)
		ORDER BY created_at DESC
		LIMIT $6
		OFFSET $7
	`, query.TenantID, query.ActivityID, query.EventID, query.ExecutionID, query.WorkflowVersionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var executions []marketing.WorkflowExecution
	for rows.Next() {
		execution, err := scanWorkflowExecution(rows)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, rows.Err()
}

func (r *Repository) ListWorkflowNodeExecutions(ctx context.Context, query marketing.WorkflowNodeExecutionQuery) ([]marketing.WorkflowNodeExecution, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10000
	}
	if limit > 500 {
		limit = 500
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, execution_id, node_id, node_type, COALESCE(node_title, ''), status,
		       input_json, output_json, COALESCE(error_message, ''),
		       started_at, finished_at, created_at, updated_at
		FROM marketing_workflow_node_execution
		WHERE execution_id = $1
		ORDER BY started_at ASC, created_at ASC
		LIMIT $2 OFFSET $3
	`, query.ExecutionID, limit, query.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var executions []marketing.WorkflowNodeExecution
	for rows.Next() {
		execution, err := scanWorkflowNodeExecution(rows)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, rows.Err()
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

func (r *Repository) SaveServiceDefinition(ctx context.Context, service marketing.ServiceDefinition) (marketing.ServiceDefinition, error) {
	if service.ID == "" {
		service.ID = marketing.NewID("svc")
	}
	if service.ServiceType == "" {
		service.ServiceType = marketing.ServiceTypeExternal
	}
	if service.Scope == "" {
		service.Scope = marketing.ServiceAssetScopeWorkspace
	}
	if service.Status == "" {
		service.Status = marketing.ServiceAssetActive
	}
	if len(service.Metadata) == 0 {
		service.Metadata = json.RawMessage(`{}`)
	}
	if service.CreatedAt.IsZero() {
		service.CreatedAt = time.Now().UTC()
	}
	if service.UpdatedAt.IsZero() {
		service.UpdatedAt = service.CreatedAt
	}
	if err := service.Validate(); err != nil {
		return marketing.ServiceDefinition{}, err
	}
	tags, err := json.Marshal(service.Tags)
	if err != nil {
		return marketing.ServiceDefinition{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service (
			id, tenant_id, workspace_id, folder_id, code, name, description,
			service_type, scope, status, owner, tags_json, metadata_json,
			builtin, catalog_version, source_hash, deleted_at,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (tenant_id, code) WHERE deleted_at IS NULL DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			folder_id = EXCLUDED.folder_id,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			service_type = EXCLUDED.service_type,
			scope = EXCLUDED.scope,
			status = EXCLUDED.status,
			owner = EXCLUDED.owner,
			tags_json = EXCLUDED.tags_json,
			metadata_json = EXCLUDED.metadata_json,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			deleted_at = EXCLUDED.deleted_at,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, workspace_id, folder_id, code, name, description,
		          service_type, scope, status, owner, tags_json, metadata_json,
		          builtin, catalog_version, source_hash, deleted_at,
		          created_by, updated_by, created_at, updated_at
		`, service.ID, service.TenantID, service.WorkspaceID, service.FolderID, service.Code, service.Name, service.Description,
		service.ServiceType, service.Scope, service.Status, service.Owner, json.RawMessage(tags), service.Metadata,
		service.Builtin, service.CatalogVersion, service.SourceHash, service.DeletedAt,
		service.CreatedBy, service.UpdatedBy, service.CreatedAt, service.UpdatedAt)
	saved, err := scanServiceDefinition(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceDefinition{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceDefinition{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) GetServiceDefinition(ctx context.Context, tenantID string, code string) (marketing.ServiceDefinition, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, code, name, description,
		       service_type, scope, status, owner, tags_json, metadata_json,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service
		WHERE tenant_id = $1 AND code = $2
		ORDER BY (status <> 'DELETED' AND deleted_at IS NULL) DESC, updated_at DESC
		LIMIT 1
	`, tenantID, code)
	service, err := scanServiceDefinition(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceDefinition{}, marketing.ErrNotFound
		}
		return marketing.ServiceDefinition{}, err
	}
	return service, nil
}

func (r *Repository) ListServiceDefinitions(ctx context.Context, query marketing.ServiceDefinitionQuery) ([]marketing.ServiceDefinition, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, code, name, description,
		       service_type, scope, status, owner, tags_json, metadata_json,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR code = $5)
		  AND ($6 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY updated_at DESC, code ASC
		LIMIT $7
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.Status, query.Code, query.IncludeDeleted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceDefinition{}
	for rows.Next() {
		service, err := scanServiceDefinition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, service)
	}
	return out, rows.Err()
}

func (r *Repository) SaveServiceInterface(ctx context.Context, serviceInterface marketing.ServiceInterface) (marketing.ServiceInterface, error) {
	if serviceInterface.ID == "" {
		serviceInterface.ID = marketing.NewID("svcif")
	}
	if serviceInterface.Operation == "" {
		serviceInterface.Operation = marketing.ServiceOperationCall
	}
	if serviceInterface.Status == "" {
		serviceInterface.Status = marketing.ServiceAssetActive
	}
	if len(serviceInterface.Metadata) == 0 {
		serviceInterface.Metadata = json.RawMessage(`{}`)
	}
	if serviceInterface.CreatedAt.IsZero() {
		serviceInterface.CreatedAt = time.Now().UTC()
	}
	if serviceInterface.UpdatedAt.IsZero() {
		serviceInterface.UpdatedAt = serviceInterface.CreatedAt
	}
	if err := serviceInterface.Validate(); err != nil {
		return marketing.ServiceInterface{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service_interface (
			id, tenant_id, service_ref, code, name, description, operation,
			input_schema_ref, output_schema_ref, error_schema_ref, idempotency_key_template,
			timeout_ms, retry_policy, lua_callable, status, metadata_json,
			builtin, catalog_version, source_hash, deleted_at,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (tenant_id, service_ref, code) WHERE deleted_at IS NULL DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			operation = EXCLUDED.operation,
			input_schema_ref = EXCLUDED.input_schema_ref,
			output_schema_ref = EXCLUDED.output_schema_ref,
			error_schema_ref = EXCLUDED.error_schema_ref,
			idempotency_key_template = EXCLUDED.idempotency_key_template,
			timeout_ms = EXCLUDED.timeout_ms,
			retry_policy = EXCLUDED.retry_policy,
			lua_callable = EXCLUDED.lua_callable,
			status = EXCLUDED.status,
			metadata_json = EXCLUDED.metadata_json,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			deleted_at = EXCLUDED.deleted_at,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, service_ref, code, name, description, operation,
		          input_schema_ref, output_schema_ref, error_schema_ref, idempotency_key_template,
		          timeout_ms, retry_policy, lua_callable, status, metadata_json,
		          builtin, catalog_version, source_hash, deleted_at,
		          created_by, updated_by, created_at, updated_at
	`, serviceInterface.ID, serviceInterface.TenantID, serviceInterface.ServiceRef, serviceInterface.Code, serviceInterface.Name, serviceInterface.Description,
		serviceInterface.Operation, serviceInterface.InputSchemaRef, serviceInterface.OutputSchemaRef, serviceInterface.ErrorSchemaRef,
		serviceInterface.IdempotencyKeyTemplate, serviceInterface.TimeoutMS, serviceInterface.RetryPolicy, serviceInterface.LuaCallable,
		serviceInterface.Status, serviceInterface.Metadata, serviceInterface.Builtin, serviceInterface.CatalogVersion,
		serviceInterface.SourceHash, serviceInterface.DeletedAt, serviceInterface.CreatedBy, serviceInterface.UpdatedBy,
		serviceInterface.CreatedAt, serviceInterface.UpdatedAt)
	saved, err := scanServiceInterface(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceInterface{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceInterface{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) GetServiceInterface(ctx context.Context, tenantID string, serviceRef string, code string) (marketing.ServiceInterface, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, service_ref, code, name, description, operation,
		       input_schema_ref, output_schema_ref, error_schema_ref, idempotency_key_template,
		       timeout_ms, retry_policy, lua_callable, status, metadata_json,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_interface
		WHERE tenant_id = $1 AND service_ref = $2 AND code = $3
		ORDER BY (status <> 'DELETED' AND deleted_at IS NULL) DESC, updated_at DESC
		LIMIT 1
	`, tenantID, serviceRef, code)
	serviceInterface, err := scanServiceInterface(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceInterface{}, marketing.ErrNotFound
		}
		return marketing.ServiceInterface{}, err
	}
	return serviceInterface, nil
}

func (r *Repository) ListServiceInterfaces(ctx context.Context, query marketing.ServiceInterfaceQuery) ([]marketing.ServiceInterface, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, service_ref, code, name, description, operation,
		       input_schema_ref, output_schema_ref, error_schema_ref, idempotency_key_template,
		       timeout_ms, retry_policy, lua_callable, status, metadata_json,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_interface
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR service_ref = $2)
		  AND ($3 = '' OR status = $3)
		  AND ($4 = '' OR code = $4)
		  AND ($5 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY updated_at DESC, code ASC
		LIMIT $6
	`, query.TenantID, query.ServiceRef, query.Status, query.Code, query.IncludeDeleted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceInterface{}
	for rows.Next() {
		serviceInterface, err := scanServiceInterface(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, serviceInterface)
	}
	return out, rows.Err()
}

func (r *Repository) SaveServiceProvider(ctx context.Context, provider marketing.ServiceProvider) (marketing.ServiceProvider, error) {
	if provider.ID == "" {
		provider.ID = marketing.NewID("svcprov")
	}
	if provider.Protocol == "" {
		provider.Protocol = marketing.ServiceProviderResource
	}
	if provider.Status == "" {
		provider.Status = marketing.ServiceAssetActive
	}
	if len(provider.Config) == 0 {
		provider.Config = json.RawMessage(`{}`)
	}
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = time.Now().UTC()
	}
	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = provider.CreatedAt
	}
	if err := provider.Validate(); err != nil {
		return marketing.ServiceProvider{}, err
	}
	resourceRefs, err := json.Marshal(provider.ResourceRefs)
	if err != nil {
		return marketing.ServiceProvider{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service_provider (
			id, tenant_id, service_ref, interface_ref, code, name, description,
			protocol, endpoint, topic, method, path, auth_ref, resource_refs_json,
			config_json, weight, priority, environment, status,
			builtin, catalog_version, source_hash, deleted_at,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
		ON CONFLICT (tenant_id, service_ref, interface_ref, code) WHERE deleted_at IS NULL DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			protocol = EXCLUDED.protocol,
			endpoint = EXCLUDED.endpoint,
			topic = EXCLUDED.topic,
			method = EXCLUDED.method,
			path = EXCLUDED.path,
			auth_ref = EXCLUDED.auth_ref,
			resource_refs_json = EXCLUDED.resource_refs_json,
			config_json = EXCLUDED.config_json,
			weight = EXCLUDED.weight,
			priority = EXCLUDED.priority,
			environment = EXCLUDED.environment,
			status = EXCLUDED.status,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			deleted_at = EXCLUDED.deleted_at,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, service_ref, interface_ref, code, name, description,
		          protocol, endpoint, topic, method, path, auth_ref, resource_refs_json,
		          config_json, weight, priority, environment, status,
		          builtin, catalog_version, source_hash, deleted_at,
		          created_by, updated_by, created_at, updated_at
	`, provider.ID, provider.TenantID, provider.ServiceRef, provider.InterfaceRef, provider.Code, provider.Name, provider.Description,
		provider.Protocol, provider.Endpoint, provider.Topic, provider.Method, provider.Path, provider.AuthRef, json.RawMessage(resourceRefs),
		provider.Config, provider.Weight, provider.Priority, provider.Environment, provider.Status,
		provider.Builtin, provider.CatalogVersion, provider.SourceHash, provider.DeletedAt,
		provider.CreatedBy, provider.UpdatedBy, provider.CreatedAt, provider.UpdatedAt)
	saved, err := scanServiceProvider(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceProvider{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceProvider{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) GetServiceProvider(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, code string) (marketing.ServiceProvider, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, code, name, description,
		       protocol, endpoint, topic, method, path, auth_ref, resource_refs_json,
		       config_json, weight, priority, environment, status,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_provider
		WHERE tenant_id = $1 AND service_ref = $2 AND interface_ref = $3 AND code = $4
		ORDER BY (status <> 'DELETED' AND deleted_at IS NULL) DESC, updated_at DESC
		LIMIT 1
	`, tenantID, serviceRef, interfaceRef, code)
	provider, err := scanServiceProvider(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceProvider{}, marketing.ErrNotFound
		}
		return marketing.ServiceProvider{}, err
	}
	return provider, nil
}

func (r *Repository) ListServiceProviders(ctx context.Context, query marketing.ServiceProviderQuery) ([]marketing.ServiceProvider, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, code, name, description,
		       protocol, endpoint, topic, method, path, auth_ref, resource_refs_json,
		       config_json, weight, priority, environment, status,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_provider
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR service_ref = $2)
		  AND ($3 = '' OR interface_ref = $3)
		  AND ($4 = '' OR protocol = $4)
		  AND ($5 = '' OR status = $5)
		  AND ($6 = '' OR code = $6)
		  AND ($7 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY priority ASC, updated_at DESC, code ASC
		LIMIT $8 OFFSET $9
	`, query.TenantID, query.ServiceRef, query.InterfaceRef, query.Protocol, query.Status, query.Code, query.IncludeDeleted, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceProvider{}
	for rows.Next() {
		provider, err := scanServiceProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, provider)
	}
	return out, rows.Err()
}

func (r *Repository) SaveServiceSchema(ctx context.Context, schema marketing.ServiceSchema) (marketing.ServiceSchema, error) {
	if schema.ID == "" {
		schema.ID = marketing.NewID("svcschema")
	}
	if schema.Scope == "" {
		schema.Scope = marketing.ServiceAssetScopeTenant
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	if schema.Status == "" {
		schema.Status = marketing.ServiceAssetActive
	}
	if len(schema.ExamplesJSON) == 0 {
		schema.ExamplesJSON = json.RawMessage(`[]`)
	}
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = time.Now().UTC()
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = schema.CreatedAt
	}
	if err := schema.Validate(); err != nil {
		return marketing.ServiceSchema{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service_schema (
			id, tenant_id, workspace_id, folder_id, schema_ref, schema_type, scope,
			version, status, schema_json, examples_json, description,
			builtin, catalog_version, source_hash, deleted_at,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (tenant_id, schema_ref, version) WHERE deleted_at IS NULL DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			folder_id = EXCLUDED.folder_id,
			schema_type = EXCLUDED.schema_type,
			scope = EXCLUDED.scope,
			status = EXCLUDED.status,
			schema_json = EXCLUDED.schema_json,
			examples_json = EXCLUDED.examples_json,
			description = EXCLUDED.description,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			deleted_at = EXCLUDED.deleted_at,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, workspace_id, folder_id, schema_ref, schema_type, scope,
		          version, status, schema_json, examples_json, description,
		          builtin, catalog_version, source_hash, deleted_at,
		          created_by, updated_by, created_at, updated_at
	`, schema.ID, schema.TenantID, schema.WorkspaceID, schema.FolderID, schema.SchemaRef, schema.SchemaType,
		schema.Scope, schema.Version, schema.Status, schema.SchemaJSON, schema.ExamplesJSON, schema.Description,
		schema.Builtin, schema.CatalogVersion, schema.SourceHash, schema.DeletedAt,
		schema.CreatedBy, schema.UpdatedBy, schema.CreatedAt, schema.UpdatedAt)
	saved, err := scanServiceSchema(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceSchema{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceSchema{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) ListServiceSchemas(ctx context.Context, query marketing.ServiceSchemaQuery) ([]marketing.ServiceSchema, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, schema_ref, schema_type, scope,
		       version, status, schema_json, examples_json, description,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_schema
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR schema_ref = $4)
		  AND ($5 = '' OR schema_type = $5)
		  AND ($6 = '' OR status = $6)
		  AND ($7 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY updated_at DESC, schema_ref ASC, version DESC
		LIMIT $8
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.SchemaRef, query.SchemaType, query.Status, query.IncludeDeleted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceSchema{}
	for rows.Next() {
		schema, err := scanServiceSchema(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, schema)
	}
	return out, rows.Err()
}

func (r *Repository) SaveServiceInterfaceBinding(ctx context.Context, binding marketing.ServiceInterfaceBinding) (marketing.ServiceInterfaceBinding, error) {
	if binding.ID == "" {
		binding.ID = marketing.NewID("svcbind")
	}
	if binding.Scope == "" {
		binding.Scope = marketing.ServiceAssetScopeWorkspace
	}
	if binding.Status == "" {
		binding.Status = marketing.ServiceAssetActive
	}
	if len(binding.Metadata) == 0 {
		binding.Metadata = json.RawMessage(`{}`)
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = binding.CreatedAt
	}
	if err := binding.Validate(); err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	allowedNodeTypes, err := json.Marshal(binding.AllowedNodeTypes)
	if err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	allowedOperations, err := json.Marshal(binding.AllowedOperations)
	if err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	allowedProviderPolicies, err := json.Marshal(binding.AllowedProviderPolicies)
	if err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	allowedProviderRefs, err := json.Marshal(binding.AllowedProviderRefs)
	if err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service_interface_binding (
			id, tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id,
			activity_id, node_id, principal_type, principal_id, allowed_node_types_json,
			allowed_operations_json, allowed_provider_policies_json, allow_explicit_provider_ref,
			allowed_provider_refs_json, include_subfolders, min_role, environment, rate_limit_ref,
			lua_callable, require_approval, deny, status, deleted_at, metadata_json,
			builtin, catalog_version, source_hash,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33)
		ON CONFLICT (tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id, activity_id, node_id, principal_type, principal_id) WHERE deleted_at IS NULL DO UPDATE SET
			allowed_node_types_json = EXCLUDED.allowed_node_types_json,
			allowed_operations_json = EXCLUDED.allowed_operations_json,
			allowed_provider_policies_json = EXCLUDED.allowed_provider_policies_json,
			allow_explicit_provider_ref = EXCLUDED.allow_explicit_provider_ref,
			allowed_provider_refs_json = EXCLUDED.allowed_provider_refs_json,
			include_subfolders = EXCLUDED.include_subfolders,
			min_role = EXCLUDED.min_role,
			environment = EXCLUDED.environment,
			rate_limit_ref = EXCLUDED.rate_limit_ref,
			lua_callable = EXCLUDED.lua_callable,
			require_approval = EXCLUDED.require_approval,
			deny = EXCLUDED.deny,
			status = EXCLUDED.status,
			deleted_at = EXCLUDED.deleted_at,
			metadata_json = EXCLUDED.metadata_json,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id,
		          activity_id, node_id, principal_type, principal_id, allowed_node_types_json,
		          allowed_operations_json, allowed_provider_policies_json, allow_explicit_provider_ref,
		          allowed_provider_refs_json, include_subfolders, min_role, environment, rate_limit_ref,
		          lua_callable, require_approval, deny, status, deleted_at, metadata_json,
		          builtin, catalog_version, source_hash,
		          created_by, updated_by, created_at, updated_at
	`, binding.ID, binding.TenantID, binding.ServiceRef, binding.InterfaceRef, binding.Scope, binding.WorkspaceID,
		binding.FolderID, binding.ActivityID, binding.NodeID, binding.PrincipalType, binding.PrincipalID,
		json.RawMessage(allowedNodeTypes), json.RawMessage(allowedOperations), json.RawMessage(allowedProviderPolicies),
		binding.AllowExplicitProviderRef, json.RawMessage(allowedProviderRefs), binding.IncludeSubfolders,
		binding.MinRole, binding.Environment, binding.RateLimitRef, binding.LuaCallable,
		binding.RequireApproval, binding.Deny, binding.Status, binding.DeletedAt, binding.Metadata,
		binding.Builtin, binding.CatalogVersion, binding.SourceHash,
		binding.CreatedBy, binding.UpdatedBy, binding.CreatedAt, binding.UpdatedAt)
	saved, err := scanServiceInterfaceBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceInterfaceBinding{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceInterfaceBinding{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) ListServiceInterfaceBindings(ctx context.Context, query marketing.ServiceInterfaceBindingQuery) ([]marketing.ServiceInterfaceBinding, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id,
		       activity_id, node_id, principal_type, principal_id, allowed_node_types_json,
		       allowed_operations_json, allowed_provider_policies_json, allow_explicit_provider_ref,
		       allowed_provider_refs_json, include_subfolders, min_role, environment, rate_limit_ref,
		       lua_callable, require_approval, deny, status, deleted_at, metadata_json,
		       builtin, catalog_version, source_hash,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_interface_binding
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR service_ref = $2)
		  AND ($3 = '' OR interface_ref = $3)
		  AND ($4 = '' OR scope = $4)
		  AND ($5 = '' OR workspace_id = $5)
		  AND ($6 = '' OR folder_id = $6)
		  AND ($7 = '' OR activity_id = $7)
		  AND ($8 = '' OR node_id = $8)
		  AND ($9 = '' OR status = $9)
		  AND ($10 = '' OR allowed_node_types_json = '[]'::jsonb OR allowed_node_types_json ? $10)
		  AND ($11 = '' OR environment = '' OR lower(environment) = lower($11))
		  AND ($12 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY updated_at DESC, service_ref ASC, interface_ref ASC
		LIMIT $13
	`, query.TenantID, query.ServiceRef, query.InterfaceRef, query.Scope, query.WorkspaceID, query.FolderID,
		query.ActivityID, query.NodeID, query.Status, query.NodeType, query.Environment, query.IncludeDeleted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceInterfaceBinding{}
	for rows.Next() {
		binding, err := scanServiceInterfaceBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, binding)
	}
	return out, rows.Err()
}

func (r *Repository) SaveServiceProviderRuntimeState(ctx context.Context, state marketing.ServiceProviderRuntimeState) (marketing.ServiceProviderRuntimeState, error) {
	if state.ID == "" {
		state.ID = marketing.NewID("svcprovstate")
	}
	if state.HealthStatus == "" {
		state.HealthStatus = marketing.ServiceProviderHealthHealthy
	}
	if state.CircuitStatus == "" {
		state.CircuitStatus = marketing.ServiceProviderCircuitClosed
	}
	if len(state.Metadata) == 0 {
		state.Metadata = json.RawMessage(`{}`)
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.CreatedAt
	}
	if err := state.Validate(); err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	row := r.db.QueryRowContext(ctx, serviceProviderRuntimeStateUpsertSQL(), serviceProviderRuntimeStateArgs(state)...)
	saved, err := scanServiceProviderRuntimeState(row)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func postgresSaveServiceProviderRuntimeStateTx(ctx context.Context, tx *sql.Tx, state marketing.ServiceProviderRuntimeState) (marketing.ServiceProviderRuntimeState, error) {
	row := tx.QueryRowContext(ctx, serviceProviderRuntimeStateUpsertSQL(), serviceProviderRuntimeStateArgs(state)...)
	saved, err := scanServiceProviderRuntimeState(row)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func serviceProviderRuntimeStateUpsertSQL() string {
	return `
		INSERT INTO marketing_service_provider_runtime_state (
			id, tenant_id, service_ref, interface_ref, provider_ref, health_status, health_message,
			circuit_status, failure_count, success_count, consecutive_failures,
			rate_limit_window, rate_limit_count, half_open_probe_count, half_open_probe_limit,
			half_open_success_count, half_open_success_threshold, last_failure_at, last_success_at,
			circuit_opened_at, circuit_reset_at, metadata_json, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (tenant_id, service_ref, interface_ref, provider_ref) DO UPDATE SET
			health_status = EXCLUDED.health_status,
			health_message = EXCLUDED.health_message,
			circuit_status = EXCLUDED.circuit_status,
			failure_count = EXCLUDED.failure_count,
			success_count = EXCLUDED.success_count,
			consecutive_failures = EXCLUDED.consecutive_failures,
			rate_limit_window = EXCLUDED.rate_limit_window,
			rate_limit_count = EXCLUDED.rate_limit_count,
			half_open_probe_count = EXCLUDED.half_open_probe_count,
			half_open_probe_limit = EXCLUDED.half_open_probe_limit,
			half_open_success_count = EXCLUDED.half_open_success_count,
			half_open_success_threshold = EXCLUDED.half_open_success_threshold,
			last_failure_at = EXCLUDED.last_failure_at,
			last_success_at = EXCLUDED.last_success_at,
			circuit_opened_at = EXCLUDED.circuit_opened_at,
			circuit_reset_at = EXCLUDED.circuit_reset_at,
			metadata_json = EXCLUDED.metadata_json,
			updated_at = EXCLUDED.updated_at
		RETURNING id, tenant_id, service_ref, interface_ref, provider_ref, health_status, health_message,
		          circuit_status, failure_count, success_count, consecutive_failures,
		          rate_limit_window, rate_limit_count, half_open_probe_count, half_open_probe_limit,
		          half_open_success_count, half_open_success_threshold, last_failure_at, last_success_at,
		          circuit_opened_at, circuit_reset_at, metadata_json, created_at, updated_at
	`
}

func serviceProviderRuntimeStateArgs(state marketing.ServiceProviderRuntimeState) []any {
	return []any{state.ID, state.TenantID, state.ServiceRef, state.InterfaceRef, state.ProviderRef, state.HealthStatus, state.HealthMessage,
		state.CircuitStatus, state.FailureCount, state.SuccessCount, state.ConsecutiveFailures, nullableTime(state.RateLimitWindow),
		state.RateLimitCount, state.HalfOpenProbeCount, state.HalfOpenProbeLimit, state.HalfOpenSuccessCount,
		state.HalfOpenSuccessThreshold, nullableTimePtr(state.LastFailureAt), nullableTimePtr(state.LastSuccessAt),
		nullableTimePtr(state.CircuitOpenedAt), nullableTimePtr(state.CircuitResetAt), state.Metadata, state.CreatedAt, state.UpdatedAt}
}

func (r *Repository) GetServiceProviderRuntimeState(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string) (marketing.ServiceProviderRuntimeState, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, provider_ref, health_status, health_message,
		       circuit_status, failure_count, success_count, consecutive_failures,
		       rate_limit_window, rate_limit_count, half_open_probe_count, half_open_probe_limit,
		       half_open_success_count, half_open_success_threshold, last_failure_at, last_success_at,
		       circuit_opened_at, circuit_reset_at, metadata_json, created_at, updated_at
		FROM marketing_service_provider_runtime_state
		WHERE tenant_id = $1 AND service_ref = $2 AND interface_ref = $3 AND provider_ref = $4
	`, tenantID, serviceRef, interfaceRef, providerRef)
	state, err := scanServiceProviderRuntimeState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceProviderRuntimeState{}, marketing.ErrNotFound
		}
		return marketing.ServiceProviderRuntimeState{}, err
	}
	return state, nil
}

func (r *Repository) ListServiceProviderRuntimeStates(ctx context.Context, query marketing.ServiceProviderRuntimeStateQuery) ([]marketing.ServiceProviderRuntimeState, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, provider_ref, health_status, health_message,
		       circuit_status, failure_count, success_count, consecutive_failures,
		       rate_limit_window, rate_limit_count, half_open_probe_count, half_open_probe_limit,
		       half_open_success_count, half_open_success_threshold, last_failure_at, last_success_at,
		       circuit_opened_at, circuit_reset_at, metadata_json, created_at, updated_at
		FROM marketing_service_provider_runtime_state
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR service_ref = $2)
		  AND ($3 = '' OR interface_ref = $3)
		  AND ($4 = '' OR provider_ref = $4)
		  AND ($5 = '' OR health_status = $5)
		  AND ($6 = '' OR circuit_status = $6)
		ORDER BY updated_at DESC, service_ref ASC, interface_ref ASC, provider_ref ASC
		LIMIT $7 OFFSET $8
	`, query.TenantID, query.ServiceRef, query.InterfaceRef, query.ProviderRef, query.HealthStatus, query.CircuitStatus, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceProviderRuntimeState{}
	for rows.Next() {
		state, err := scanServiceProviderRuntimeState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func (r *Repository) AcquireWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	name := strings.TrimSpace(cmd.Name)
	owner := strings.TrimSpace(cmd.Owner)
	if name == "" {
		return marketing.WorkerLease{}, false, errors.New("lease name is required")
	}
	if owner == "" {
		return marketing.WorkerLease{}, false, errors.New("lease owner is required")
	}
	if cmd.TTL <= 0 {
		return marketing.WorkerLease{}, false, errors.New("lease ttl must be positive")
	}
	now := cmd.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	leaseUntil := now.Add(cmd.TTL)
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_worker_lease (name, owner, lease_until, acquired_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (name) DO UPDATE SET
			owner = EXCLUDED.owner,
			lease_until = EXCLUDED.lease_until,
			acquired_at = EXCLUDED.acquired_at,
			updated_at = EXCLUDED.updated_at
		WHERE marketing_worker_lease.lease_until <= $4 OR marketing_worker_lease.owner = $2
		RETURNING name, owner, lease_until, acquired_at, updated_at
	`, name, owner, leaseUntil, now)
	lease, err := scanWorkerLease(row)
	if err == nil {
		lease.Status = marketing.WorkerLeaseAcquired
		lease.LeaseTTL = cmd.TTL
		return lease, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return marketing.WorkerLease{}, false, mapPostgresResourceError(err)
	}
	existing, err := r.getWorkerLease(ctx, name)
	if err != nil {
		return marketing.WorkerLease{}, false, err
	}
	existing.Status = marketing.WorkerLeaseHeld
	return existing, false, nil
}

func (r *Repository) RenewWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	name := strings.TrimSpace(cmd.Name)
	owner := strings.TrimSpace(cmd.Owner)
	if name == "" {
		return marketing.WorkerLease{}, false, errors.New("lease name is required")
	}
	if owner == "" {
		return marketing.WorkerLease{}, false, errors.New("lease owner is required")
	}
	if cmd.TTL <= 0 {
		return marketing.WorkerLease{}, false, errors.New("lease ttl must be positive")
	}
	now := cmd.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE marketing_worker_lease
		SET lease_until = $4, updated_at = $3
		WHERE name = $1 AND owner = $2
		RETURNING name, owner, lease_until, acquired_at, updated_at
	`, name, owner, now, now.Add(cmd.TTL))
	lease, err := scanWorkerLease(row)
	if err == nil {
		lease.Status = marketing.WorkerLeaseAcquired
		lease.LeaseTTL = cmd.TTL
		return lease, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return marketing.WorkerLease{}, false, mapPostgresResourceError(err)
	}
	existing, err := r.getWorkerLease(ctx, name)
	if err != nil {
		return marketing.WorkerLease{}, false, err
	}
	existing.Status = marketing.WorkerLeaseHeld
	return existing, false, nil
}

func (r *Repository) ReleaseWorkerLease(ctx context.Context, name string, owner string, now time.Time) error {
	name = strings.TrimSpace(name)
	owner = strings.TrimSpace(owner)
	if name == "" || owner == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE marketing_worker_lease
		SET lease_until = $3, updated_at = $3
		WHERE name = $1 AND owner = $2
	`, name, owner, now)
	return err
}

func (r *Repository) getWorkerLease(ctx context.Context, name string) (marketing.WorkerLease, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT name, owner, lease_until, acquired_at, updated_at
		FROM marketing_worker_lease
		WHERE name = $1
	`, name)
	lease, err := scanWorkerLease(row)
	if err != nil {
		return marketing.WorkerLease{}, err
	}
	return lease, nil
}

func (r *Repository) ConsumeServiceProviderRateLimit(ctx context.Context, cmd marketing.ServiceProviderRateLimitConsume) (marketing.ServiceProviderRuntimeState, bool, error) {
	state := cmd.State
	if state.ID == "" {
		state.ID = marketing.NewID("svcprovstate")
	}
	if state.HealthStatus == "" {
		state.HealthStatus = marketing.ServiceProviderHealthHealthy
	}
	if state.CircuitStatus == "" {
		state.CircuitStatus = marketing.ServiceProviderCircuitClosed
	}
	if len(state.Metadata) == 0 {
		state.Metadata = json.RawMessage(`{}`)
	}
	now := cmd.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cmd.Window <= 0 {
		cmd.Window = time.Minute
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, false, err
	}
	defer rollback(tx)
	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, service_ref, interface_ref, provider_ref, health_status, health_message,
		       circuit_status, failure_count, success_count, consecutive_failures,
		       rate_limit_window, rate_limit_count, half_open_probe_count, half_open_probe_limit,
		       half_open_success_count, half_open_success_threshold, last_failure_at, last_success_at,
		       circuit_opened_at, circuit_reset_at, metadata_json, created_at, updated_at
		FROM marketing_service_provider_runtime_state
		WHERE tenant_id = $1 AND service_ref = $2 AND interface_ref = $3 AND provider_ref = $4
		FOR UPDATE
	`, state.TenantID, state.ServiceRef, state.InterfaceRef, state.ProviderRef)
	existing, err := scanServiceProviderRuntimeState(row)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return marketing.ServiceProviderRuntimeState{}, false, err
	}
	if err == nil {
		state = existing
	}
	if state.RateLimitWindow.IsZero() || now.Sub(state.RateLimitWindow) >= cmd.Window {
		state.RateLimitWindow = now
		state.RateLimitCount = 0
	}
	if cmd.Limit > 0 && state.RateLimitCount >= cmd.Limit {
		state.UpdatedAt = now
		if err := tx.Commit(); err != nil {
			return marketing.ServiceProviderRuntimeState{}, false, err
		}
		return state, false, nil
	}
	state.RateLimitCount++
	state.UpdatedAt = now
	saved, err := postgresSaveServiceProviderRuntimeStateTx(ctx, tx, state)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return marketing.ServiceProviderRuntimeState{}, false, err
	}
	return saved, true, nil
}

func (r *Repository) SaveServiceRateLimitPolicy(ctx context.Context, policy marketing.ServiceRateLimitPolicy) (marketing.ServiceRateLimitPolicy, error) {
	if policy.ID == "" {
		policy.ID = marketing.NewID("svcratelimit")
	}
	if policy.Name == "" {
		policy.Name = policy.Code
	}
	if policy.Backend == "" {
		policy.Backend = marketing.ServiceRateLimitBackendPostgres
	}
	if policy.Status == "" {
		policy.Status = marketing.ServiceAssetActive
	}
	if len(policy.Metadata) == 0 {
		policy.Metadata = json.RawMessage(`{}`)
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = policy.CreatedAt
	}
	if err := policy.Validate(); err != nil {
		return marketing.ServiceRateLimitPolicy{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_service_rate_limit_policy (
			id, tenant_id, code, name, description, service_ref, interface_ref, provider_ref,
			limit_count, window_seconds, backend, status, metadata_json,
			builtin, catalog_version, source_hash, deleted_at,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (tenant_id, code) WHERE deleted_at IS NULL DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			service_ref = EXCLUDED.service_ref,
			interface_ref = EXCLUDED.interface_ref,
			provider_ref = EXCLUDED.provider_ref,
			limit_count = EXCLUDED.limit_count,
			window_seconds = EXCLUDED.window_seconds,
			backend = EXCLUDED.backend,
			status = EXCLUDED.status,
			metadata_json = EXCLUDED.metadata_json,
			builtin = EXCLUDED.builtin,
			catalog_version = EXCLUDED.catalog_version,
			source_hash = EXCLUDED.source_hash,
			deleted_at = EXCLUDED.deleted_at,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
			RETURNING id, tenant_id, code, name, description, service_ref, interface_ref, provider_ref,
		          limit_count, window_seconds, backend, status, metadata_json,
		          builtin, catalog_version, source_hash, deleted_at,
		          created_by, updated_by, created_at, updated_at
	`, policy.ID, policy.TenantID, policy.Code, policy.Name, policy.Description, policy.ServiceRef, policy.InterfaceRef, policy.ProviderRef,
		policy.Limit, policy.WindowSeconds, policy.Backend, policy.Status, policy.Metadata,
		policy.Builtin, policy.CatalogVersion, policy.SourceHash, policy.DeletedAt,
		policy.CreatedBy, policy.UpdatedBy, policy.CreatedAt, policy.UpdatedAt)
	saved, err := scanServiceRateLimitPolicy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ServiceRateLimitPolicy{}, marketing.ErrResourceConflict
		}
		return marketing.ServiceRateLimitPolicy{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) ListServiceRateLimitPolicies(ctx context.Context, query marketing.ServiceRateLimitPolicyQuery) ([]marketing.ServiceRateLimitPolicy, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, code, name, description, service_ref, interface_ref, provider_ref,
		       limit_count, window_seconds, backend, status, metadata_json,
		       builtin, catalog_version, source_hash, deleted_at,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_service_rate_limit_policy
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR code = $2)
		  AND ($3 = '' OR service_ref = $3)
		  AND ($4 = '' OR interface_ref = $4)
		  AND ($5 = '' OR provider_ref = $5)
		  AND ($6 = '' OR status = $6)
		  AND ($7 OR (status <> 'DELETED' AND deleted_at IS NULL))
		ORDER BY updated_at DESC, code ASC
		LIMIT $8
	`, query.TenantID, query.Code, query.ServiceRef, query.InterfaceRef, query.ProviderRef, query.Status, query.IncludeDeleted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceRateLimitPolicy{}
	for rows.Next() {
		policy, err := scanServiceRateLimitPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	return out, rows.Err()
}

func (r *Repository) CreateServiceCallAuditLog(ctx context.Context, log marketing.ServiceCallAuditLog) error {
	if log.ID == "" {
		log.ID = marketing.NewID("svccallaudit")
	}
	if log.Status == "" {
		log.Status = marketing.ServiceCallAuditSuccess
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if err := log.Validate(); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_service_call_audit (
			id, tenant_id, workspace_id, folder_id, activity_id, workflow_id,
			node_id, node_type, service_ref, interface_ref, provider_ref, resource_ref,
			step_id, attempt, status, error_code, error_message, latency_ms, request_hash, response_hash,
			provider_trace_json, schema_trace_json, permission_trace_json, actor, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
	`, log.ID, log.TenantID, log.WorkspaceID, log.FolderID, log.ActivityID, log.WorkflowID,
		log.NodeID, log.NodeType, log.ServiceRef, log.InterfaceRef, log.ProviderRef, log.ResourceRef,
		log.StepID, log.Attempt, log.Status, log.ErrorCode, log.ErrorMessage, log.LatencyMS, log.RequestHash, log.ResponseHash,
		jsonOrObject(log.ProviderTrace), jsonOrObject(log.SchemaTrace), jsonOrObject(log.PermissionTrace), log.Actor, log.CreatedAt)
	return err
}

func (r *Repository) ListServiceCallAuditLogs(ctx context.Context, query marketing.ServiceCallAuditQuery) ([]marketing.ServiceCallAuditLog, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, workflow_id,
		       node_id, node_type, service_ref, interface_ref, provider_ref, resource_ref,
		       step_id, attempt, status, error_code, error_message, latency_ms, request_hash, response_hash,
		       provider_trace_json, schema_trace_json, permission_trace_json, actor, created_at
		FROM marketing_service_call_audit
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR activity_id = $4)
		  AND ($5 = '' OR workflow_id = $5)
		  AND ($6 = '' OR node_id = $6)
		  AND ($7 = '' OR service_ref = $7)
		  AND ($8 = '' OR interface_ref = $8)
		  AND ($9 = '' OR provider_ref = $9)
		  AND ($10 = '' OR resource_ref = $10)
		  AND ($11 = '' OR status = $11)
		  AND ($12 = '' OR error_code = $12)
		  AND ($13 = '' OR (created_at, id) < (split_part($13, '|', 1)::timestamptz, split_part($13, '|', 2)))
		ORDER BY created_at DESC, id DESC
		LIMIT $14
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.ActivityID, query.WorkflowID,
		query.NodeID, query.ServiceRef, query.InterfaceRef, query.ProviderRef, query.ResourceRef,
		query.Status, query.ErrorCode, query.Cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ServiceCallAuditLog{}
	for rows.Next() {
		log, err := scanServiceCallAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (r *Repository) PurgeServiceCallAuditLogs(ctx context.Context, tenantID string, workspaceID string, before time.Time, limit int) (int, error) {
	if before.IsZero() {
		return 0, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	tag, err := r.db.ExecContext(ctx, `
		WITH expired AS (
				SELECT id
				FROM marketing_service_call_audit
				WHERE ($1 = '' OR tenant_id = $1)
				  AND ($2 = '' OR workspace_id = $2)
				  AND created_at < $3
				ORDER BY created_at ASC, id ASC
				LIMIT $4
			)
			DELETE FROM marketing_service_call_audit a
			USING expired
			WHERE a.id = expired.id
		`, strings.TrimSpace(tenantID), strings.TrimSpace(workspaceID), before, limit)
	if err != nil {
		return 0, err
	}
	rows, _ := tag.RowsAffected()
	return int(rows), nil
}

func (r *Repository) CreateProviderDecisionAuditLog(ctx context.Context, log marketing.ProviderDecisionAuditLog) error {
	if log.ID == "" {
		log.ID = marketing.NewID("provdecision")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if err := log.Validate(); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_service_provider_decision_audit (
			id, tenant_id, workspace_id, folder_id, activity_id, node_id, node_type,
			service_ref, interface_ref, provider_policy, candidate_provider_ref, decision,
			reason, latency_ms, weight, environment, mock, trace_id, run_id, details_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`, log.ID, log.TenantID, log.WorkspaceID, log.FolderID, log.ActivityID, log.NodeID, log.NodeType,
		log.ServiceRef, log.InterfaceRef, log.ProviderPolicy, log.CandidateProviderRef, log.Decision,
		log.Reason, log.LatencyMS, log.Weight, log.Environment, log.Mock, log.TraceID, log.RunID,
		jsonOrObject(log.Details), log.CreatedAt)
	return err
}

func (r *Repository) ListProviderDecisionAuditLogs(ctx context.Context, query marketing.ProviderDecisionAuditQuery) ([]marketing.ProviderDecisionAuditLog, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, node_id, node_type,
		       service_ref, interface_ref, provider_policy, candidate_provider_ref, decision,
		       reason, latency_ms, weight, environment, mock, trace_id, run_id, details_json, created_at
		FROM marketing_service_provider_decision_audit
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR activity_id = $4)
		  AND ($5 = '' OR node_id = $5)
		  AND ($6 = '' OR service_ref = $6)
		  AND ($7 = '' OR interface_ref = $7)
		  AND ($8 = '' OR candidate_provider_ref = $8)
		  AND ($9 = '' OR decision = $9)
		  AND ($10 = '' OR (created_at, id) < (split_part($10, '|', 1)::timestamptz, split_part($10, '|', 2)))
		ORDER BY created_at DESC, id DESC
		LIMIT $11
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.ActivityID, query.NodeID,
		query.ServiceRef, query.InterfaceRef, query.ProviderRef, query.Decision, query.Cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.ProviderDecisionAuditLog{}
	for rows.Next() {
		log, err := scanProviderDecisionAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (r *Repository) CreateMigrationAuditLog(ctx context.Context, log marketing.MigrationAuditLog) error {
	if log.ID == "" {
		log.ID = marketing.NewID("migaudit")
	}
	if log.Result == "" {
		log.Result = marketing.MigrationAuditSuccess
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if err := log.Validate(); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_service_migration_audit (
			id, tenant_id, workspace_id, folder_id, activity_id, migration_type, operation,
			dry_run, actor, scope, break_glass_ticket, summary_json, items_count,
			blockers_count, result, error, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, log.ID, log.TenantID, log.WorkspaceID, log.FolderID, log.ActivityID, log.MigrationType, log.Operation,
		log.DryRun, log.Actor, log.Scope, log.BreakGlassTicket, jsonOrObject(log.Summary),
		log.ItemsCount, log.BlockersCount, log.Result, log.Error, log.CreatedAt)
	return err
}

func (r *Repository) ListMigrationAuditLogs(ctx context.Context, query marketing.MigrationAuditQuery) ([]marketing.MigrationAuditLog, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, migration_type, operation,
		       dry_run, actor, scope, break_glass_ticket, summary_json, items_count,
		       blockers_count, result, error, created_at
		FROM marketing_service_migration_audit
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR activity_id = $4)
		  AND ($5 = '' OR migration_type = $5)
		  AND ($6 = '' OR operation = $6)
		  AND ($7 = '' OR actor = $7)
		  AND ($8 = '' OR result = $8)
		  AND ($9 = '' OR (created_at, id) < (split_part($9, '|', 1)::timestamptz, split_part($9, '|', 2)))
		ORDER BY created_at DESC, id DESC
		LIMIT $10
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.ActivityID, query.MigrationType,
		query.Operation, query.Actor, query.Result, query.Cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []marketing.MigrationAuditLog{}
	for rows.Next() {
		log, err := scanMigrationAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (r *Repository) GetActivityState(ctx context.Context, key marketing.ActivityStateKey) (marketing.ActivityState, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, activity_id, namespace, state_key, COALESCE(subject_type, ''), COALESCE(subject_id, ''),
		       data_json, version, COALESCE(last_event_id, ''), expires_at, created_at, updated_at
		FROM marketing_activity_state
		WHERE tenant_id = $1 AND activity_id = $2 AND namespace = $3 AND state_key = $4
	`, key.TenantID, key.ActivityID, key.Namespace, key.StateKey)
	state, err := scanActivityState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.ActivityState{}, marketing.ErrNotFound
		}
		return marketing.ActivityState{}, err
	}
	return state, nil
}

func (r *Repository) UpsertActivityState(ctx context.Context, cmd marketing.UpsertActivityStateCommand) (marketing.ActivityState, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.ActivityState{}, err
	}
	defer rollback(tx)
	state := cmd.State
	if state.ID == "" {
		state.ID = marketing.NewID("actstate")
	}
	if len(state.Data) == 0 {
		state.Data = json.RawMessage(`{}`)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, activity_id, namespace, state_key, COALESCE(subject_type, ''), COALESCE(subject_id, ''),
		       data_json, version, COALESCE(last_event_id, ''), expires_at, created_at, updated_at
		FROM marketing_activity_state
		WHERE tenant_id = $1 AND activity_id = $2 AND namespace = $3 AND state_key = $4
		FOR UPDATE
	`, state.TenantID, state.ActivityID, state.Namespace, state.StateKey)
	existing, err := scanActivityState(row)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return marketing.ActivityState{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if cmd.RequireVersion && cmd.ExpectedVersion > 0 {
			return marketing.ActivityState{}, marketing.ErrNotFound
		}
		inserted, err := postgresInsertActivityState(ctx, tx, state)
		if err != nil {
			return marketing.ActivityState{}, mapPostgresActivityStateError(err)
		}
		if err := tx.Commit(); err != nil {
			return marketing.ActivityState{}, err
		}
		return inserted, nil
	}
	if cmd.RequireVersion && existing.Version != cmd.ExpectedVersion {
		return marketing.ActivityState{}, marketing.ErrActivityStateConflict
	}
	data := state.Data
	if cmd.Merge {
		data = mergeJSONObjects(existing.Data, state.Data)
	}
	updated, err := postgresUpdateActivityState(ctx, tx, existing.ID, state, data, existing.Version+1)
	if err != nil {
		return marketing.ActivityState{}, mapPostgresActivityStateError(err)
	}
	if err := tx.Commit(); err != nil {
		return marketing.ActivityState{}, err
	}
	return updated, nil
}

func (r *Repository) GetStorageState(ctx context.Context, key marketing.StorageStateKey) (marketing.StorageState, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, namespace, storage_key, value_type, data_json, version,
		       COALESCE(last_event_id, ''), expires_at, created_at, updated_at
		FROM marketing_storage_state
		WHERE tenant_id = $1 AND namespace = $2 AND storage_key = $3
	`, key.TenantID, key.Namespace, key.StorageKey)
	state, err := scanStorageState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.StorageState{}, marketing.ErrNotFound
		}
		return marketing.StorageState{}, err
	}
	return state, nil
}

func (r *Repository) UpsertStorageState(ctx context.Context, cmd marketing.UpsertStorageStateCommand) (marketing.StorageState, error) {
	state := cmd.State
	if state.ID == "" {
		state.ID = marketing.NewID("store")
	}
	if len(state.Data) == 0 {
		state.Data = json.RawMessage(`null`)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	if cmd.Append && !cmd.RequireVersion {
		state.Data = marketing.AppendStorageStateData(nil, state.Data)
		state.ValueType = "ARRAY"
		appended, err := postgresAppendStorageState(ctx, r.db, state)
		if err != nil {
			return marketing.StorageState{}, mapPostgresStorageStateError(err)
		}
		return appended, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.StorageState{}, err
	}
	defer rollback(tx)
	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, namespace, storage_key, value_type, data_json, version,
		       COALESCE(last_event_id, ''), expires_at, created_at, updated_at
		FROM marketing_storage_state
		WHERE tenant_id = $1 AND namespace = $2 AND storage_key = $3
		FOR UPDATE
	`, state.TenantID, state.Namespace, state.StorageKey)
	existing, err := scanStorageState(row)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return marketing.StorageState{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if cmd.RequireVersion && cmd.ExpectedVersion > 0 {
			return marketing.StorageState{}, marketing.ErrNotFound
		}
		if cmd.Append {
			state.Data = marketing.AppendStorageStateData(nil, state.Data)
			state.ValueType = "ARRAY"
		}
		inserted, err := postgresInsertStorageState(ctx, tx, state)
		if err != nil {
			return marketing.StorageState{}, mapPostgresStorageStateError(err)
		}
		if err := tx.Commit(); err != nil {
			return marketing.StorageState{}, err
		}
		return inserted, nil
	}
	if cmd.RequireVersion && existing.Version != cmd.ExpectedVersion {
		return marketing.StorageState{}, marketing.ErrStorageStateConflict
	}
	if cmd.Append {
		state.Data = marketing.AppendStorageStateData(existing.Data, state.Data)
		state.ValueType = "ARRAY"
	}
	updated, err := postgresUpdateStorageState(ctx, tx, existing.ID, state, existing.Version+1)
	if err != nil {
		return marketing.StorageState{}, mapPostgresStorageStateError(err)
	}
	if err := tx.Commit(); err != nil {
		return marketing.StorageState{}, err
	}
	return updated, nil
}

func (r *Repository) DeleteStorageState(ctx context.Context, key marketing.StorageStateKey) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM marketing_storage_state
		WHERE tenant_id = $1 AND namespace = $2 AND storage_key = $3
	`, key.TenantID, key.Namespace, key.StorageKey)
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

func (r *Repository) SaveResource(ctx context.Context, resource marketing.Resource) (marketing.Resource, error) {
	if resource.ID == "" {
		resource.ID = marketing.NewID("res")
	}
	if resource.Status == "" {
		resource.Status = marketing.ResourceActive
	}
	if resource.Scope == "" {
		resource.Scope = marketing.ResourceScopeWorkspace
	}
	if resource.Driver == "" {
		resource.Driver = marketing.ResourceDriverInternal
	}
	if len(resource.Config) == 0 {
		resource.Config = json.RawMessage(`{}`)
	}
	if resource.SchemaVersion == 0 {
		resource.SchemaVersion = 1
	}
	if resource.CreatedAt.IsZero() {
		resource.CreatedAt = time.Now().UTC()
	}
	if resource.UpdatedAt.IsZero() {
		resource.UpdatedAt = resource.CreatedAt
	}
	if err := resource.Validate(); err != nil {
		return marketing.Resource{}, err
	}
	capabilities, err := json.Marshal(resource.Capabilities)
	if err != nil {
		return marketing.Resource{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_resource (
			id, tenant_id, workspace_id, folder_id, activity_id, code, name, description,
			type, driver, scope, status, environment, config_json, capabilities_json,
			schema_version, created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (tenant_id, code) DO UPDATE
		SET workspace_id = EXCLUDED.workspace_id,
		    folder_id = EXCLUDED.folder_id,
		    activity_id = EXCLUDED.activity_id,
		    name = EXCLUDED.name,
		    description = EXCLUDED.description,
		    type = EXCLUDED.type,
		    driver = EXCLUDED.driver,
		    scope = EXCLUDED.scope,
		    status = EXCLUDED.status,
		    environment = EXCLUDED.environment,
		    config_json = EXCLUDED.config_json,
		    capabilities_json = EXCLUDED.capabilities_json,
		    schema_version = EXCLUDED.schema_version,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = EXCLUDED.updated_at
		RETURNING id, tenant_id, workspace_id, folder_id, activity_id, code, name, description,
		          type, driver, scope, status, environment, config_json, capabilities_json,
		          schema_version, created_by, updated_by, created_at, updated_at
	`, resource.ID, resource.TenantID, resource.WorkspaceID, resource.FolderID, resource.ActivityID,
		resource.Code, resource.Name, resource.Description, resource.Type, resource.Driver, resource.Scope,
		resource.Status, resource.Environment, resource.Config, json.RawMessage(capabilities), resource.SchemaVersion,
		resource.CreatedBy, resource.UpdatedBy, resource.CreatedAt, resource.UpdatedAt)
	saved, err := scanResource(row)
	if err != nil {
		return marketing.Resource{}, mapPostgresResourceError(err)
	}
	return saved, nil
}

func (r *Repository) GetResource(ctx context.Context, tenantID string, code string) (marketing.Resource, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, code, name, description,
		       type, driver, scope, status, environment, config_json, capabilities_json,
		       schema_version, created_by, updated_by, created_at, updated_at
		FROM marketing_resource
		WHERE tenant_id = $1 AND code = $2
	`, tenantID, code)
	resource, err := scanResource(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.Resource{}, marketing.ErrNotFound
		}
		return marketing.Resource{}, err
	}
	return resource, nil
}

func (r *Repository) GetResourceByID(ctx context.Context, id string) (marketing.Resource, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, code, name, description,
		       type, driver, scope, status, environment, config_json, capabilities_json,
		       schema_version, created_by, updated_by, created_at, updated_at
		FROM marketing_resource
		WHERE id = $1
	`, id)
	resource, err := scanResource(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.Resource{}, marketing.ErrNotFound
		}
		return marketing.Resource{}, err
	}
	return resource, nil
}

func (r *Repository) ListResources(ctx context.Context, query marketing.ResourceQuery) ([]marketing.Resource, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, code, name, description,
		       type, driver, scope, status, environment, config_json, capabilities_json,
		       schema_version, created_by, updated_by, created_at, updated_at
		FROM marketing_resource
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR workspace_id = $2)
		  AND ($3 = '' OR folder_id = $3)
		  AND ($4 = '' OR activity_id = $4)
		  AND ($5 = '' OR type = $5)
		  AND ($6 = '' OR status = $6)
		  AND ($7 = '' OR code = $7)
		ORDER BY updated_at DESC, code ASC
		LIMIT $8
	`, query.TenantID, query.WorkspaceID, query.FolderID, query.ActivityID, query.Type, query.Status, query.Code, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.Resource
	for rows.Next() {
		resource, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, resource)
	}
	return out, rows.Err()
}

func (r *Repository) SaveResourceBinding(ctx context.Context, binding marketing.ResourceBinding) (marketing.ResourceBinding, error) {
	if binding.ID == "" {
		binding.ID = marketing.NewID("resbind")
	}
	if binding.Status == "" {
		binding.Status = marketing.ResourceBindingActive
	}
	if len(binding.KeyPolicy) == 0 {
		binding.KeyPolicy = json.RawMessage(`{}`)
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = binding.CreatedAt
	}
	operations, err := json.Marshal(binding.AllowedOperations)
	if err != nil {
		return marketing.ResourceBinding{}, err
	}
	namespaces, err := json.Marshal(binding.AllowedNamespaces)
	if err != nil {
		return marketing.ResourceBinding{}, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO marketing_resource_binding (
			id, resource_id, tenant_id, workspace_id, folder_id, activity_id, node_type, node_id,
			principal_type, principal_id, min_role, allowed_operations_json, allowed_namespaces_json,
			key_policy_json, include_subfolders, require_approval, status, created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (id) DO UPDATE
		SET workspace_id = EXCLUDED.workspace_id,
		    folder_id = EXCLUDED.folder_id,
		    activity_id = EXCLUDED.activity_id,
		    node_type = EXCLUDED.node_type,
		    node_id = EXCLUDED.node_id,
		    principal_type = EXCLUDED.principal_type,
		    principal_id = EXCLUDED.principal_id,
		    min_role = EXCLUDED.min_role,
		    allowed_operations_json = EXCLUDED.allowed_operations_json,
		    allowed_namespaces_json = EXCLUDED.allowed_namespaces_json,
		    key_policy_json = EXCLUDED.key_policy_json,
		    include_subfolders = EXCLUDED.include_subfolders,
		    require_approval = EXCLUDED.require_approval,
		    status = EXCLUDED.status,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = EXCLUDED.updated_at
		RETURNING id, resource_id, tenant_id, workspace_id, folder_id, activity_id, node_type, node_id,
		          principal_type, principal_id, min_role, allowed_operations_json, allowed_namespaces_json,
		          key_policy_json, include_subfolders, require_approval, status, created_by, updated_by, created_at, updated_at
	`, binding.ID, binding.ResourceID, binding.TenantID, binding.WorkspaceID, binding.FolderID, binding.ActivityID,
		binding.NodeType, binding.NodeID, binding.PrincipalType, binding.PrincipalID, binding.MinRole,
		json.RawMessage(operations), json.RawMessage(namespaces), binding.KeyPolicy, binding.IncludeSubfolders,
		binding.RequireApproval, binding.Status, binding.CreatedBy, binding.UpdatedBy, binding.CreatedAt, binding.UpdatedAt)
	saved, err := scanResourceBinding(row)
	if err != nil {
		return marketing.ResourceBinding{}, err
	}
	return saved, nil
}

func (r *Repository) ListResourceBindings(ctx context.Context, query marketing.ResourceBindingQuery) ([]marketing.ResourceBinding, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, resource_id, tenant_id, workspace_id, folder_id, activity_id, node_type, node_id,
		       principal_type, principal_id, min_role, allowed_operations_json, allowed_namespaces_json,
		       key_policy_json, include_subfolders, require_approval, status, created_by, updated_by, created_at, updated_at
		FROM marketing_resource_binding
		WHERE ($1 = '' OR resource_id = $1)
		  AND ($2 = '' OR tenant_id = $2)
		  AND ($3 = '' OR workspace_id = $3)
		  AND ($4 = '' OR folder_id = $4)
		  AND ($5 = '' OR activity_id = $5)
		  AND ($6 = '' OR node_type = '' OR node_type = $6)
		  AND ($7 = '' OR status = $7)
		ORDER BY updated_at DESC, id ASC
		LIMIT $8
	`, query.ResourceID, query.TenantID, query.WorkspaceID, query.FolderID, query.ActivityID, query.NodeType, query.Status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.ResourceBinding
	for rows.Next() {
		binding, err := scanResourceBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, binding)
	}
	return out, rows.Err()
}

func (r *Repository) CreateResourceAuditLog(ctx context.Context, log marketing.ResourceAuditLog) error {
	if log.ID == "" {
		log.ID = marketing.NewID("resaudit")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_resource_audit (
			id, tenant_id, workspace_id, folder_id, activity_id, workflow_execution_id,
			node_id, node_type, resource_id, resource_code, operation, status, duration_ms,
			error_code, error_message, actor_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, log.ID, log.TenantID, log.WorkspaceID, log.FolderID, log.ActivityID, log.WorkflowExecutionID,
		log.NodeID, log.NodeType, log.ResourceID, log.ResourceCode, log.Operation, log.Status, log.DurationMS,
		log.ErrorCode, log.ErrorMessage, log.ActorID, log.CreatedAt)
	return err
}

func (r *Repository) ListResourceAuditLogs(ctx context.Context, query marketing.ResourceAuditQuery) ([]marketing.ResourceAuditLog, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, workspace_id, folder_id, activity_id, workflow_execution_id,
		       node_id, node_type, resource_id, resource_code, operation, status, duration_ms,
		       error_code, error_message, actor_id, created_at
		FROM marketing_resource_audit
		WHERE ($1 = '' OR resource_id = $1)
		  AND ($2 = '' OR tenant_id = $2)
		  AND ($3 = '' OR workspace_id = $3)
		  AND ($4 = '' OR activity_id = $4)
		  AND ($5 = '' OR status = $5)
		ORDER BY created_at DESC
		LIMIT $6
	`, query.ResourceID, query.TenantID, query.WorkspaceID, query.ActivityID, query.Status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.ResourceAuditLog
	for rows.Next() {
		var log marketing.ResourceAuditLog
		if err := rows.Scan(&log.ID, &log.TenantID, &log.WorkspaceID, &log.FolderID, &log.ActivityID, &log.WorkflowExecutionID,
			&log.NodeID, &log.NodeType, &log.ResourceID, &log.ResourceCode, &log.Operation, &log.Status, &log.DurationMS,
			&log.ErrorCode, &log.ErrorMessage, &log.ActorID, &log.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (r *Repository) CreateInbox(ctx context.Context, inbox marketing.EventInbox) (marketing.EventInbox, bool, error) {
	inbox.Event = inbox.Event.Normalized()
	existing, err := r.inboxByEventIdentity(ctx, inbox.Event.TenantID, inbox.Event.EventID)
	if err == nil {
		existing.ProcessStatus = marketing.InboxDuplicate
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return marketing.EventInbox{}, false, err
	}

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
		ON CONFLICT DO NOTHING
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

	existing, err = r.inboxByEventIdentity(ctx, inbox.Event.TenantID, inbox.Event.EventID)
	if err == nil {
		existing.ProcessStatus = marketing.InboxDuplicate
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return marketing.EventInbox{}, false, err
	}
	existing, err = r.inboxByIdempotency(ctx, inbox.IdempotencyKey)
	if err != nil {
		return marketing.EventInbox{}, false, err
	}
	if !existing.Event.SameStandardPayload(inbox.Event) {
		return marketing.EventInbox{}, false, marketing.ErrIdempotencyConflict
	}
	existing.ProcessStatus = marketing.InboxDuplicate
	return existing, true, nil
}

func (r *Repository) GetInboxByEvent(ctx context.Context, tenantID string, eventID string) (marketing.EventInbox, error) {
	inbox, err := r.inboxByEventIdentity(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(eventID))
	if errors.Is(err, sql.ErrNoRows) {
		return marketing.EventInbox{}, marketing.ErrNotFound
	}
	return inbox, err
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
		ExpiresAt:      now.Add(24 * time.Hour),
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) >= query.Limit || query.CommandID != "" || query.GrantOrderID != "" || query.OutboxID != "" {
		return items, nil
	}
	participationRows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.tenant_id, p.activity_id, p.event_inbox_id, p.subject_type, p.subject_id,
		       p.biz_type, p.biz_id, p.status, COALESCE(p.reject_reason, ''), p.snapshot_json,
		       p.idempotency_key, p.created_at, p.updated_at,
		       ei.id, ei.payload_json, ei.idempotency_key, ei.process_status, COALESCE(ei.error_message, ''), ei.created_at, ei.updated_at
		FROM marketing_participation p
		JOIN marketing_event_inbox ei ON ei.id = p.event_inbox_id
		WHERE p.tenant_id = $1
		  AND ($2 = '' OR p.activity_id = $2)
		  AND ($3 = '' OR p.subject_type = $3)
		  AND ($4 = '' OR p.subject_id = $4)
		  AND ($5 = '' OR ei.event_id = $5)
		  AND ($6 = '' OR ei.idempotency_key = $6)
		  AND ($7 = '' OR p.id = $7)
		  AND NOT EXISTS (
		    SELECT 1 FROM marketing_grant_order go
		    WHERE go.participation_id = p.id
		  )
		ORDER BY p.updated_at DESC
		LIMIT $8
	`, query.TenantID, query.ActivityID, query.SubjectType, query.SubjectID, query.EventID, query.IdempotencyKey, query.ParticipationID, query.Limit-len(items))
	if err != nil {
		return nil, err
	}
	defer participationRows.Close()
	for participationRows.Next() {
		var item marketing.RewardReconciliationItem
		var participation marketing.Participation
		var inbox marketing.EventInbox
		var eventPayload []byte
		if err := participationRows.Scan(&participation.ID, &participation.TenantID, &participation.ActivityID, &participation.EventInboxID,
			&participation.Subject.Type, &participation.Subject.ID, &participation.Biz.BizType, &participation.Biz.BizID,
			&participation.Status, &participation.RejectReason, &participation.Snapshot, &participation.IdempotencyKey,
			&participation.CreatedAt, &participation.UpdatedAt,
			&inbox.ID, &eventPayload, &inbox.IdempotencyKey, &inbox.ProcessStatus, &inbox.ErrorMessage, &inbox.CreatedAt, &inbox.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(eventPayload, &inbox.Event); err != nil {
			return nil, err
		}
		item.TenantID = participation.TenantID
		item.ActivityID = participation.ActivityID
		item.ParticipationID = participation.ID
		item.Subject = participation.Subject
		item.ParticipationStatus = participation.Status
		item.ServerUpdatedAt = participation.UpdatedAt
		item.EventInbox = &inbox
		item.Participation = &participation
		item.Reservations = []marketing.QuotaReservation{}
		items = append(items, item)
	}
	return items, participationRows.Err()
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
		Reservations:   reservations,
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

func (r *Repository) RecoverStaleOutbox(ctx context.Context, staleBefore time.Time, limit int) ([]marketing.OutboxMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)

	rows, err := tx.QueryContext(ctx, `
		WITH stale AS (
			SELECT id
			FROM marketing_outbox
			WHERE send_status = 'SENDING'
			  AND updated_at < $1
			ORDER BY updated_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		),
		before AS (
			SELECT o.id, o.tenant_id, o.message_type, o.target_channel, o.target_ref, o.payload_json,
			       o.send_status, o.retry_count, o.next_retry_at, COALESCE(o.error_message, '') AS error_message,
			       o.created_at, o.updated_at
			FROM marketing_outbox o
			JOIN stale ON stale.id = o.id
		),
		updated AS (
			UPDATE marketing_outbox o
			SET send_status = 'PENDING', next_retry_at = NULL, updated_at = NOW()
			FROM stale
			WHERE o.id = stale.id
			RETURNING o.id
		)
		SELECT id, tenant_id, message_type, target_channel, target_ref, payload_json,
		       send_status, retry_count, next_retry_at, error_message, created_at, updated_at
		FROM before
	`, staleBefore, limit)
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

func (r *Repository) CountOutboxDLQ(ctx context.Context) ([]marketing.OutboxDLQCount, error) {
	startedAt := time.Now()
	var opErr error
	defer func() {
		recordDBOperation("count_outbox_dlq", "marketing_outbox", startedAt, opErr)
	}()
	rows, err := r.db.QueryContext(ctx, `
		SELECT tenant_id, target_channel, COUNT(*)
		FROM marketing_outbox
		WHERE send_status = 'DLQ'
		GROUP BY tenant_id, target_channel
		ORDER BY tenant_id, target_channel
	`)
	if err != nil {
		opErr = err
		return nil, err
	}
	defer rows.Close()

	var counts []marketing.OutboxDLQCount
	for rows.Next() {
		var count marketing.OutboxDLQCount
		if err := rows.Scan(&count.TenantID, &count.TargetChannel, &count.Count); err != nil {
			opErr = err
			return nil, err
		}
		counts = append(counts, count)
	}
	opErr = rows.Err()
	return counts, opErr
}

func (r *Repository) CountOutboxBacklog(ctx context.Context, now time.Time) ([]marketing.OutboxBacklogMetric, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	startedAt := time.Now()
	var opErr error
	defer func() {
		recordDBOperation("count_outbox_backlog", "marketing_outbox", startedAt, opErr)
	}()
	rows, err := r.db.QueryContext(ctx, `
		SELECT tenant_id,
		       target_channel,
		       send_status,
		       COUNT(*),
		       COALESCE(MAX(
		           CASE
		             WHEN send_status IN ('PENDING', 'SENDING', 'DLQ') THEN EXTRACT(EPOCH FROM ($1::timestamptz - created_at))
		             WHEN send_status = 'FAILED' AND (next_retry_at IS NULL OR next_retry_at <= $1::timestamptz) THEN EXTRACT(EPOCH FROM ($1::timestamptz - COALESCE(next_retry_at, created_at)))
		             ELSE 0
		           END
		       ), 0)
		FROM marketing_outbox
		WHERE send_status IN ('PENDING', 'FAILED', 'SENDING', 'DLQ', 'DLQ_CLOSED')
		GROUP BY tenant_id, target_channel, send_status
		ORDER BY tenant_id, target_channel, send_status
	`, now)
	if err != nil {
		opErr = err
		return nil, err
	}
	defer rows.Close()

	var metrics []marketing.OutboxBacklogMetric
	for rows.Next() {
		var metric marketing.OutboxBacklogMetric
		if err := rows.Scan(&metric.TenantID, &metric.TargetChannel, &metric.SendStatus, &metric.Count, &metric.OldestEligibleAgeSeconds); err != nil {
			opErr = err
			return nil, err
		}
		if metric.OldestEligibleAgeSeconds < 0 {
			metric.OldestEligibleAgeSeconds = 0
		}
		metrics = append(metrics, metric)
	}
	opErr = rows.Err()
	return metrics, opErr
}

func recordDBOperation(operation string, table string, startedAt time.Time, err error) {
	result := "success"
	reason := "ok"
	if err != nil {
		result = "failed"
		reason = "db_error"
	}
	metrics.Default.IncCounter("exmarketing_db_operation_total",
		metrics.L("operation", operation),
		metrics.L("table", table),
		metrics.L("result", result),
		metrics.L("reason", reason),
	)
	metrics.Default.Observe("exmarketing_db_operation_latency_seconds", time.Since(startedAt).Seconds(),
		metrics.L("operation", operation),
		metrics.L("table", table),
		metrics.L("result", result),
	)
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
		"targetRef":     redactTargetRef(before.TargetRef),
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
	var startTime any
	if !query.StartTime.IsZero() {
		startTime = query.StartTime.UTC()
	}
	var endTime any
	if !query.EndTime.IsZero() {
		endTime = query.EndTime.UTC()
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
		  AND ($9::timestamptz IS NULL OR created_at >= $9)
		  AND ($10::timestamptz IS NULL OR created_at < $10)
		  AND ($11 = '' OR payload_json -> 'source' ->> 'traceId' = $11)
		  AND ($12 = '' OR payload_json -> 'source' ->> 'requestId' = $12)
		ORDER BY created_at DESC
		LIMIT $13
	`, query.TenantID, query.Actor, query.Action, query.Resource, query.ResourceID,
		query.ActivityID, query.EventID, query.CommandID, startTime, endTime, query.TraceID, query.RequestID, query.Limit)
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

func redactTargetRef(targetRef string) string {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return ""
	}
	if idx := strings.Index(targetRef, "?"); idx >= 0 {
		targetRef = targetRef[:idx]
	}
	if idx := strings.Index(targetRef, "#"); idx >= 0 {
		targetRef = targetRef[:idx]
	}
	for _, marker := range []string{"token=", "secret=", "password=", "authorization="} {
		if strings.Contains(strings.ToLower(targetRef), marker) {
			return "[REDACTED]"
		}
	}
	return targetRef
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
		SELECT id, tenant_id, COALESCE(workspace_id, ''), COALESCE(folder_id, ''), activity_code, activity_name, activity_type, status, COALESCE(visibility, 'TEAM'),
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

func (r *Repository) inboxByEventIdentity(ctx context.Context, tenantID string, eventID string) (marketing.EventInbox, error) {
	var inbox marketing.EventInbox
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, payload_json, idempotency_key, process_status, COALESCE(error_message, ''), created_at, updated_at
		FROM marketing_event_inbox
		WHERE tenant_id = $1 AND event_id = $2
	`, tenantID, eventID).Scan(&inbox.ID, &payload, &inbox.IdempotencyKey, &inbox.ProcessStatus, &inbox.ErrorMessage, &inbox.CreatedAt, &inbox.UpdatedAt)
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

func scanActivityState(scanner outboxScanner) (marketing.ActivityState, error) {
	var state marketing.ActivityState
	var subjectType, subjectID string
	var data []byte
	var lastEventID string
	var expiresAt sql.NullTime
	err := scanner.Scan(
		&state.ID,
		&state.TenantID,
		&state.ActivityID,
		&state.Namespace,
		&state.StateKey,
		&subjectType,
		&subjectID,
		&data,
		&state.Version,
		&lastEventID,
		&expiresAt,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		return marketing.ActivityState{}, err
	}
	state.Subject = marketing.EventSubject{Type: subjectType, ID: subjectID}
	state.Data = append(json.RawMessage(nil), data...)
	state.LastEventID = lastEventID
	if expiresAt.Valid {
		value := expiresAt.Time
		state.ExpiresAt = &value
	}
	return state, nil
}

func scanStorageState(scanner outboxScanner) (marketing.StorageState, error) {
	var state marketing.StorageState
	var data []byte
	var lastEventID string
	var expiresAt sql.NullTime
	err := scanner.Scan(
		&state.ID,
		&state.TenantID,
		&state.Namespace,
		&state.StorageKey,
		&state.ValueType,
		&data,
		&state.Version,
		&lastEventID,
		&expiresAt,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		return marketing.StorageState{}, err
	}
	state.Data = append(json.RawMessage(nil), data...)
	state.LastEventID = lastEventID
	if expiresAt.Valid {
		value := expiresAt.Time
		state.ExpiresAt = &value
	}
	return state, nil
}

func scanServiceDefinition(scanner outboxScanner) (marketing.ServiceDefinition, error) {
	var service marketing.ServiceDefinition
	var tags json.RawMessage
	var deletedAt sql.NullTime
	if err := scanner.Scan(&service.ID, &service.TenantID, &service.WorkspaceID, &service.FolderID,
		&service.Code, &service.Name, &service.Description, &service.ServiceType, &service.Scope,
		&service.Status, &service.Owner, &tags, &service.Metadata,
		&service.Builtin, &service.CatalogVersion, &service.SourceHash, &deletedAt,
		&service.CreatedBy, &service.UpdatedBy,
		&service.CreatedAt, &service.UpdatedAt); err != nil {
		return marketing.ServiceDefinition{}, err
	}
	if deletedAt.Valid {
		service.DeletedAt = &deletedAt.Time
	}
	if len(tags) > 0 {
		_ = json.Unmarshal(tags, &service.Tags)
	}
	if service.Tags == nil {
		service.Tags = []string{}
	}
	if len(service.Metadata) == 0 {
		service.Metadata = json.RawMessage(`{}`)
	}
	return service, nil
}

func scanServiceInterface(scanner outboxScanner) (marketing.ServiceInterface, error) {
	var serviceInterface marketing.ServiceInterface
	var deletedAt sql.NullTime
	if err := scanner.Scan(&serviceInterface.ID, &serviceInterface.TenantID, &serviceInterface.ServiceRef,
		&serviceInterface.Code, &serviceInterface.Name, &serviceInterface.Description, &serviceInterface.Operation,
		&serviceInterface.InputSchemaRef, &serviceInterface.OutputSchemaRef, &serviceInterface.ErrorSchemaRef,
		&serviceInterface.IdempotencyKeyTemplate, &serviceInterface.TimeoutMS, &serviceInterface.RetryPolicy,
		&serviceInterface.LuaCallable, &serviceInterface.Status, &serviceInterface.Metadata,
		&serviceInterface.Builtin, &serviceInterface.CatalogVersion, &serviceInterface.SourceHash, &deletedAt,
		&serviceInterface.CreatedBy, &serviceInterface.UpdatedBy, &serviceInterface.CreatedAt,
		&serviceInterface.UpdatedAt); err != nil {
		return marketing.ServiceInterface{}, err
	}
	if deletedAt.Valid {
		serviceInterface.DeletedAt = &deletedAt.Time
	}
	if len(serviceInterface.Metadata) == 0 {
		serviceInterface.Metadata = json.RawMessage(`{}`)
	}
	return serviceInterface, nil
}

func scanServiceProvider(scanner outboxScanner) (marketing.ServiceProvider, error) {
	var provider marketing.ServiceProvider
	var resourceRefs json.RawMessage
	var deletedAt sql.NullTime
	if err := scanner.Scan(&provider.ID, &provider.TenantID, &provider.ServiceRef, &provider.InterfaceRef,
		&provider.Code, &provider.Name, &provider.Description, &provider.Protocol, &provider.Endpoint,
		&provider.Topic, &provider.Method, &provider.Path, &provider.AuthRef, &resourceRefs,
		&provider.Config, &provider.Weight, &provider.Priority, &provider.Environment, &provider.Status,
		&provider.Builtin, &provider.CatalogVersion, &provider.SourceHash, &deletedAt,
		&provider.CreatedBy, &provider.UpdatedBy, &provider.CreatedAt, &provider.UpdatedAt); err != nil {
		return marketing.ServiceProvider{}, err
	}
	if deletedAt.Valid {
		provider.DeletedAt = &deletedAt.Time
	}
	if len(resourceRefs) > 0 {
		_ = json.Unmarshal(resourceRefs, &provider.ResourceRefs)
	}
	if provider.ResourceRefs == nil {
		provider.ResourceRefs = []string{}
	}
	if len(provider.Config) == 0 {
		provider.Config = json.RawMessage(`{}`)
	}
	return provider, nil
}

func scanServiceSchema(scanner outboxScanner) (marketing.ServiceSchema, error) {
	var schema marketing.ServiceSchema
	var deletedAt sql.NullTime
	if err := scanner.Scan(&schema.ID, &schema.TenantID, &schema.WorkspaceID, &schema.FolderID,
		&schema.SchemaRef, &schema.SchemaType, &schema.Scope, &schema.Version, &schema.Status,
		&schema.SchemaJSON, &schema.ExamplesJSON, &schema.Description,
		&schema.Builtin, &schema.CatalogVersion, &schema.SourceHash, &deletedAt, &schema.CreatedBy,
		&schema.UpdatedBy, &schema.CreatedAt, &schema.UpdatedAt); err != nil {
		return marketing.ServiceSchema{}, err
	}
	if deletedAt.Valid {
		schema.DeletedAt = &deletedAt.Time
	}
	if len(schema.ExamplesJSON) == 0 {
		schema.ExamplesJSON = json.RawMessage(`[]`)
	}
	return schema, nil
}

func scanServiceInterfaceBinding(scanner outboxScanner) (marketing.ServiceInterfaceBinding, error) {
	var binding marketing.ServiceInterfaceBinding
	var allowedNodeTypes json.RawMessage
	var allowedOperations json.RawMessage
	var allowedProviderPolicies json.RawMessage
	var allowedProviderRefs json.RawMessage
	var deletedAt sql.NullTime
	if err := scanner.Scan(&binding.ID, &binding.TenantID, &binding.ServiceRef, &binding.InterfaceRef,
		&binding.Scope, &binding.WorkspaceID, &binding.FolderID, &binding.ActivityID, &binding.NodeID,
		&binding.PrincipalType, &binding.PrincipalID, &allowedNodeTypes, &allowedOperations,
		&allowedProviderPolicies, &binding.AllowExplicitProviderRef, &allowedProviderRefs,
		&binding.IncludeSubfolders, &binding.MinRole, &binding.Environment, &binding.RateLimitRef,
		&binding.LuaCallable, &binding.RequireApproval, &binding.Deny, &binding.Status, &deletedAt, &binding.Metadata,
		&binding.Builtin, &binding.CatalogVersion, &binding.SourceHash,
		&binding.CreatedBy, &binding.UpdatedBy, &binding.CreatedAt, &binding.UpdatedAt); err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	if deletedAt.Valid {
		binding.DeletedAt = &deletedAt.Time
	}
	if len(allowedNodeTypes) > 0 {
		_ = json.Unmarshal(allowedNodeTypes, &binding.AllowedNodeTypes)
	}
	if len(allowedOperations) > 0 {
		_ = json.Unmarshal(allowedOperations, &binding.AllowedOperations)
	}
	if len(allowedProviderPolicies) > 0 {
		_ = json.Unmarshal(allowedProviderPolicies, &binding.AllowedProviderPolicies)
	}
	if len(allowedProviderRefs) > 0 {
		_ = json.Unmarshal(allowedProviderRefs, &binding.AllowedProviderRefs)
	}
	if binding.AllowedNodeTypes == nil {
		binding.AllowedNodeTypes = []string{}
	}
	if binding.AllowedOperations == nil {
		binding.AllowedOperations = []marketing.ServiceInterfaceOperation{}
	}
	if binding.AllowedProviderPolicies == nil {
		binding.AllowedProviderPolicies = []string{}
	}
	if binding.AllowedProviderRefs == nil {
		binding.AllowedProviderRefs = []string{}
	}
	if len(binding.Metadata) == 0 {
		binding.Metadata = json.RawMessage(`{}`)
	}
	return binding, nil
}

func scanServiceProviderRuntimeState(scanner outboxScanner) (marketing.ServiceProviderRuntimeState, error) {
	var state marketing.ServiceProviderRuntimeState
	var rateLimitWindow sql.NullTime
	var lastFailureAt sql.NullTime
	var lastSuccessAt sql.NullTime
	var circuitOpenedAt sql.NullTime
	var circuitResetAt sql.NullTime
	if err := scanner.Scan(&state.ID, &state.TenantID, &state.ServiceRef, &state.InterfaceRef, &state.ProviderRef,
		&state.HealthStatus, &state.HealthMessage, &state.CircuitStatus, &state.FailureCount,
		&state.SuccessCount, &state.ConsecutiveFailures, &rateLimitWindow, &state.RateLimitCount,
		&state.HalfOpenProbeCount, &state.HalfOpenProbeLimit, &state.HalfOpenSuccessCount, &state.HalfOpenSuccessThreshold,
		&lastFailureAt, &lastSuccessAt, &circuitOpenedAt, &circuitResetAt, &state.Metadata,
		&state.CreatedAt, &state.UpdatedAt); err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	if rateLimitWindow.Valid {
		state.RateLimitWindow = rateLimitWindow.Time
	}
	if lastFailureAt.Valid {
		value := lastFailureAt.Time
		state.LastFailureAt = &value
	}
	if lastSuccessAt.Valid {
		value := lastSuccessAt.Time
		state.LastSuccessAt = &value
	}
	if circuitOpenedAt.Valid {
		value := circuitOpenedAt.Time
		state.CircuitOpenedAt = &value
	}
	if circuitResetAt.Valid {
		value := circuitResetAt.Time
		state.CircuitResetAt = &value
	}
	if len(state.Metadata) == 0 {
		state.Metadata = json.RawMessage(`{}`)
	}
	return state, nil
}

func scanWorkerLease(scanner outboxScanner) (marketing.WorkerLease, error) {
	var lease marketing.WorkerLease
	if err := scanner.Scan(&lease.Name, &lease.Owner, &lease.LeaseUntil, &lease.AcquiredAt, &lease.UpdatedAt); err != nil {
		return marketing.WorkerLease{}, err
	}
	return lease, nil
}

func scanServiceRateLimitPolicy(scanner outboxScanner) (marketing.ServiceRateLimitPolicy, error) {
	var policy marketing.ServiceRateLimitPolicy
	var deletedAt sql.NullTime
	if err := scanner.Scan(
		&policy.ID, &policy.TenantID, &policy.Code, &policy.Name, &policy.Description,
		&policy.ServiceRef, &policy.InterfaceRef, &policy.ProviderRef, &policy.Limit,
		&policy.WindowSeconds, &policy.Backend, &policy.Status, &policy.Metadata,
		&policy.Builtin, &policy.CatalogVersion, &policy.SourceHash, &deletedAt,
		&policy.CreatedBy, &policy.UpdatedBy, &policy.CreatedAt, &policy.UpdatedAt,
	); err != nil {
		return marketing.ServiceRateLimitPolicy{}, err
	}
	if deletedAt.Valid {
		policy.DeletedAt = &deletedAt.Time
	}
	return policy, nil
}

func scanServiceCallAuditLog(scanner outboxScanner) (marketing.ServiceCallAuditLog, error) {
	var log marketing.ServiceCallAuditLog
	if err := scanner.Scan(&log.ID, &log.TenantID, &log.WorkspaceID, &log.FolderID,
		&log.ActivityID, &log.WorkflowID, &log.NodeID, &log.NodeType, &log.ServiceRef,
		&log.InterfaceRef, &log.ProviderRef, &log.ResourceRef, &log.StepID, &log.Attempt,
		&log.Status, &log.ErrorCode, &log.ErrorMessage, &log.LatencyMS, &log.RequestHash,
		&log.ResponseHash, &log.ProviderTrace, &log.SchemaTrace, &log.PermissionTrace,
		&log.Actor, &log.CreatedAt); err != nil {
		return marketing.ServiceCallAuditLog{}, err
	}
	return log, nil
}

func scanProviderDecisionAuditLog(scanner outboxScanner) (marketing.ProviderDecisionAuditLog, error) {
	var log marketing.ProviderDecisionAuditLog
	if err := scanner.Scan(&log.ID, &log.TenantID, &log.WorkspaceID, &log.FolderID,
		&log.ActivityID, &log.NodeID, &log.NodeType, &log.ServiceRef, &log.InterfaceRef,
		&log.ProviderPolicy, &log.CandidateProviderRef, &log.Decision, &log.Reason,
		&log.LatencyMS, &log.Weight, &log.Environment, &log.Mock, &log.TraceID, &log.RunID,
		&log.Details, &log.CreatedAt); err != nil {
		return marketing.ProviderDecisionAuditLog{}, err
	}
	return log, nil
}

func scanMigrationAuditLog(scanner outboxScanner) (marketing.MigrationAuditLog, error) {
	var log marketing.MigrationAuditLog
	if err := scanner.Scan(&log.ID, &log.TenantID, &log.WorkspaceID, &log.FolderID,
		&log.ActivityID, &log.MigrationType, &log.Operation, &log.DryRun, &log.Actor,
		&log.Scope, &log.BreakGlassTicket, &log.Summary, &log.ItemsCount, &log.BlockersCount,
		&log.Result, &log.Error, &log.CreatedAt); err != nil {
		return marketing.MigrationAuditLog{}, err
	}
	return log, nil
}

func jsonOrObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableTimePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return *value
}

func scanResource(scanner outboxScanner) (marketing.Resource, error) {
	var resource marketing.Resource
	var capabilities json.RawMessage
	if err := scanner.Scan(&resource.ID, &resource.TenantID, &resource.WorkspaceID, &resource.FolderID, &resource.ActivityID,
		&resource.Code, &resource.Name, &resource.Description, &resource.Type, &resource.Driver, &resource.Scope,
		&resource.Status, &resource.Environment, &resource.Config, &capabilities, &resource.SchemaVersion,
		&resource.CreatedBy, &resource.UpdatedBy, &resource.CreatedAt, &resource.UpdatedAt); err != nil {
		return marketing.Resource{}, err
	}
	if len(capabilities) > 0 {
		_ = json.Unmarshal(capabilities, &resource.Capabilities)
	}
	if resource.Capabilities == nil {
		resource.Capabilities = []marketing.ResourceOperation{}
	}
	return resource, nil
}

func scanResourceBinding(scanner outboxScanner) (marketing.ResourceBinding, error) {
	var binding marketing.ResourceBinding
	var operations json.RawMessage
	var namespaces json.RawMessage
	if err := scanner.Scan(&binding.ID, &binding.ResourceID, &binding.TenantID, &binding.WorkspaceID, &binding.FolderID,
		&binding.ActivityID, &binding.NodeType, &binding.NodeID, &binding.PrincipalType, &binding.PrincipalID,
		&binding.MinRole, &operations, &namespaces, &binding.KeyPolicy, &binding.IncludeSubfolders,
		&binding.RequireApproval, &binding.Status, &binding.CreatedBy, &binding.UpdatedBy, &binding.CreatedAt,
		&binding.UpdatedAt); err != nil {
		return marketing.ResourceBinding{}, err
	}
	if len(operations) > 0 {
		_ = json.Unmarshal(operations, &binding.AllowedOperations)
	}
	if len(namespaces) > 0 {
		_ = json.Unmarshal(namespaces, &binding.AllowedNamespaces)
	}
	return binding, nil
}

func scanActivity(scanner activityScanner) (marketing.Activity, error) {
	var activity marketing.Activity
	var triggerEvents []byte
	var config sql.NullString
	err := scanner.Scan(
		&activity.ID, &activity.TenantID, &activity.WorkspaceID, &activity.FolderID, &activity.Code, &activity.Name, &activity.Type, &activity.Status, &activity.Visibility,
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
	if activity.Visibility == "" {
		activity.Visibility = marketing.WorkspaceTeam
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

func scanWorkflowDraft(scanner outboxScanner) (marketing.WorkflowDraft, error) {
	var draft marketing.WorkflowDraft
	err := scanner.Scan(&draft.ActivityID, &draft.TenantID, &draft.Definition, &draft.UpdatedBy, &draft.CreatedAt, &draft.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkflowDraft{}, marketing.ErrNotFound
		}
		return marketing.WorkflowDraft{}, err
	}
	return draft, nil
}

func scanWorkflowVersion(scanner outboxScanner) (marketing.WorkflowVersion, error) {
	var version marketing.WorkflowVersion
	err := scanner.Scan(&version.ID, &version.TenantID, &version.ActivityID, &version.Version,
		&version.Definition, &version.Status, &version.PublishedBy, &version.PublishedAt,
		&version.ReleaseNote, &version.SourceVersionID, &version.Action, &version.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkflowVersion{}, marketing.ErrNotFound
		}
		return marketing.WorkflowVersion{}, err
	}
	return version, nil
}

func scanWorkflowDraftTemplate(scanner outboxScanner) (marketing.WorkflowDraftTemplate, error) {
	var template marketing.WorkflowDraftTemplate
	var tagsRaw []byte
	err := scanner.Scan(&template.VersionID, &template.ID, &template.Version, &template.Name,
		&template.Description, &template.Category, &template.WorkspaceID, &template.FolderID,
		&template.Visibility, &tagsRaw, &template.Source, &template.Status,
		&template.Definition, &template.CreatedBy, &template.CreatedAt, &template.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkflowDraftTemplate{}, marketing.ErrNotFound
		}
		return marketing.WorkflowDraftTemplate{}, err
	}
	_ = json.Unmarshal(tagsRaw, &template.Tags)
	return template, nil
}

func scanWorkflowExecution(scanner outboxScanner) (marketing.WorkflowExecution, error) {
	var execution marketing.WorkflowExecution
	var input sql.NullString
	var output sql.NullString
	var finishedAt sql.NullTime
	err := scanner.Scan(&execution.ID, &execution.TenantID, &execution.ActivityID, &execution.WorkflowVersionID,
		&execution.WorkflowVersion, &execution.EventInboxID, &execution.EventID, &execution.Status,
		&input, &output, &execution.ErrorMessage, &execution.StartedAt, &finishedAt, &execution.CreatedAt, &execution.UpdatedAt)
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	if input.Valid {
		execution.Input = json.RawMessage(input.String)
	}
	if output.Valid {
		execution.Output = json.RawMessage(output.String)
	}
	if finishedAt.Valid {
		execution.FinishedAt = &finishedAt.Time
	}
	return execution, nil
}

func scanWorkflowNodeExecution(scanner outboxScanner) (marketing.WorkflowNodeExecution, error) {
	var execution marketing.WorkflowNodeExecution
	var input sql.NullString
	var output sql.NullString
	var finishedAt sql.NullTime
	err := scanner.Scan(&execution.ID, &execution.TenantID, &execution.ExecutionID, &execution.NodeID,
		&execution.NodeType, &execution.NodeTitle, &execution.Status, &input, &output,
		&execution.ErrorMessage, &execution.StartedAt, &finishedAt, &execution.CreatedAt, &execution.UpdatedAt)
	if err != nil {
		return marketing.WorkflowNodeExecution{}, err
	}
	if input.Valid {
		execution.Input = json.RawMessage(input.String)
	}
	if output.Valid {
		execution.Output = json.RawMessage(output.String)
	}
	if finishedAt.Valid {
		execution.FinishedAt = &finishedAt.Time
	}
	return execution, nil
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

func ensureActivityWorkspaceColumns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS pg_trgm;

		ALTER TABLE marketing_activity
			ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(64),
			ADD COLUMN IF NOT EXISTS folder_id VARCHAR(64),
			ADD COLUMN IF NOT EXISTS visibility VARCHAR(32) NOT NULL DEFAULT 'TEAM';

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'marketing_activity_workspace_fk'
				  AND conrelid = 'marketing_activity'::regclass
			) THEN
				ALTER TABLE marketing_activity
					ADD CONSTRAINT marketing_activity_workspace_fk
					FOREIGN KEY (workspace_id)
					REFERENCES marketing_workspace(id)
					ON DELETE SET NULL;
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'marketing_activity_folder_requires_workspace'
				  AND conrelid = 'marketing_activity'::regclass
			) THEN
				ALTER TABLE marketing_activity
					ADD CONSTRAINT marketing_activity_folder_requires_workspace
					CHECK (folder_id IS NULL OR workspace_id IS NOT NULL);
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'marketing_activity_folder_workspace_fk'
				  AND conrelid = 'marketing_activity'::regclass
			) THEN
				ALTER TABLE marketing_activity
					ADD CONSTRAINT marketing_activity_folder_workspace_fk
					FOREIGN KEY (workspace_id, folder_id)
					REFERENCES marketing_workspace_folder(workspace_id, id)
					ON DELETE SET NULL;
			END IF;
		END
		$$;

		CREATE INDEX IF NOT EXISTS marketing_activity_search_trgm_idx
			ON marketing_activity USING gin (lower(activity_name || ' ' || activity_code || ' ' || tenant_id || ' ' || status || ' ' || COALESCE(visibility, 'TEAM')) gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS marketing_activity_workspace_visibility_idx
			ON marketing_activity(workspace_id, folder_id, status, visibility, updated_at DESC);
	`)
	return err
}

func ensureWorkflowVersionColumns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		ALTER TABLE marketing_workflow_version
			ADD COLUMN IF NOT EXISTS release_note TEXT;
		ALTER TABLE marketing_workflow_version
			ADD COLUMN IF NOT EXISTS source_version_id VARCHAR(64);
		ALTER TABLE marketing_workflow_version
			ADD COLUMN IF NOT EXISTS action VARCHAR(32);
	`)
	return err
}

func ensureWorkflowDraftTemplateTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS marketing_workflow_draft_template (
			version_id VARCHAR(64) PRIMARY KEY,
			template_id VARCHAR(128) NOT NULL,
			version INTEGER NOT NULL,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			category VARCHAR(128),
			workspace_id VARCHAR(64) NOT NULL DEFAULT '',
			folder_id VARCHAR(64) NOT NULL DEFAULT '',
			visibility VARCHAR(32) NOT NULL DEFAULT '',
			tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			source VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			definition_json JSONB NOT NULL,
			created_by VARCHAR(128),
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			UNIQUE(template_id, version)
		);
		CREATE INDEX IF NOT EXISTS marketing_workflow_draft_template_catalog_idx
			ON marketing_workflow_draft_template(template_id, status, version DESC);
		ALTER TABLE marketing_workflow_draft_template
			ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(64) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS folder_id VARCHAR(64) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS visibility VARCHAR(32) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS tags_json JSONB NOT NULL DEFAULT '[]'::jsonb;
		CREATE INDEX IF NOT EXISTS marketing_workflow_draft_template_scope_idx
			ON marketing_workflow_draft_template(workspace_id, folder_id, visibility);
	`)
	return err
}

func ensureManualGrantExportTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS marketing_manual_grant_export_file (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL,
			file_name VARCHAR(255) NOT NULL,
			file_format VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			operator VARCHAR(128) NOT NULL,
			reason TEXT NOT NULL,
			filter_snapshot_json JSONB,
			row_count INTEGER NOT NULL DEFAULT 0,
			file_checksum VARCHAR(128),
			storage_ref TEXT,
			error_message TEXT,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS marketing_manual_grant_export_detail (
			id VARCHAR(64) PRIMARY KEY,
			export_id VARCHAR(64) NOT NULL REFERENCES marketing_manual_grant_export_file(id) ON DELETE CASCADE,
			tenant_id VARCHAR(64) NOT NULL,
			grant_order_id VARCHAR(64) NOT NULL REFERENCES marketing_grant_order(id) ON DELETE CASCADE,
			bill_no VARCHAR(128) NOT NULL,
			row_number INTEGER NOT NULL,
			export_grant_status VARCHAR(32) NOT NULL,
			detail_status VARCHAR(32) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_manual_grant_export_file_tenant_status
			ON marketing_manual_grant_export_file (tenant_id, status, created_at);
		CREATE INDEX IF NOT EXISTS idx_manual_grant_export_detail_grant_order
			ON marketing_manual_grant_export_detail (grant_order_id);
		CREATE INDEX IF NOT EXISTS idx_manual_grant_export_detail_export
			ON marketing_manual_grant_export_detail (export_id, row_number);
	`)
	return err
}

func mapPostgresActivityError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "marketing_activity_tenant_code_key" {
		return marketing.ErrActivityConflict
	}
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return marketing.ErrNotFound
	}
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return marketing.ErrInvalidActivity
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

func postgresInsertActivityState(ctx context.Context, tx *sql.Tx, state marketing.ActivityState) (marketing.ActivityState, error) {
	row := tx.QueryRowContext(ctx, `
		INSERT INTO marketing_activity_state (
			id, tenant_id, activity_id, namespace, state_key, subject_type, subject_id,
			data_json, version, last_event_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, 1, NULLIF($9, ''), $10, $11, $12)
		RETURNING id, tenant_id, activity_id, namespace, state_key, COALESCE(subject_type, ''), COALESCE(subject_id, ''),
		          data_json, version, COALESCE(last_event_id, ''), expires_at, created_at, updated_at
	`, state.ID, state.TenantID, state.ActivityID, state.Namespace, state.StateKey, state.Subject.Type, state.Subject.ID,
		state.Data, state.LastEventID, state.ExpiresAt, state.CreatedAt, state.UpdatedAt)
	return scanActivityState(row)
}

func postgresUpdateActivityState(ctx context.Context, tx *sql.Tx, id string, state marketing.ActivityState, data json.RawMessage, version int64) (marketing.ActivityState, error) {
	row := tx.QueryRowContext(ctx, `
		UPDATE marketing_activity_state
		SET subject_type = NULLIF($2, ''),
		    subject_id = NULLIF($3, ''),
		    data_json = $4,
		    version = $5,
		    last_event_id = NULLIF($6, ''),
		    expires_at = $7,
		    updated_at = $8
		WHERE id = $1
		RETURNING id, tenant_id, activity_id, namespace, state_key, COALESCE(subject_type, ''), COALESCE(subject_id, ''),
		          data_json, version, COALESCE(last_event_id, ''), expires_at, created_at, updated_at
	`, id, state.Subject.Type, state.Subject.ID, data, version, state.LastEventID, state.ExpiresAt, state.UpdatedAt)
	return scanActivityState(row)
}

func mergeJSONObjects(existing json.RawMessage, patch json.RawMessage) json.RawMessage {
	var existingMap map[string]any
	var patchMap map[string]any
	if json.Unmarshal(existing, &existingMap) != nil || json.Unmarshal(patch, &patchMap) != nil {
		return patch
	}
	if existingMap == nil {
		existingMap = map[string]any{}
	}
	for key, value := range patchMap {
		existingMap[key] = value
	}
	raw, err := json.Marshal(existingMap)
	if err != nil {
		return patch
	}
	return raw
}

func mapPostgresActivityStateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return marketing.ErrNotFound
		case "23505":
			return marketing.ErrActivityStateConflict
		}
	}
	return err
}

func postgresInsertStorageState(ctx context.Context, tx *sql.Tx, state marketing.StorageState) (marketing.StorageState, error) {
	row := tx.QueryRowContext(ctx, `
		INSERT INTO marketing_storage_state (
			id, tenant_id, namespace, storage_key, value_type, data_json,
			version, last_event_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, NULLIF($7, ''), $8, $9, $10)
		RETURNING id, tenant_id, namespace, storage_key, value_type, data_json, version,
		          COALESCE(last_event_id, ''), expires_at, created_at, updated_at
	`, state.ID, state.TenantID, state.Namespace, state.StorageKey, state.ValueType, state.Data,
		state.LastEventID, state.ExpiresAt, state.CreatedAt, state.UpdatedAt)
	return scanStorageState(row)
}

func postgresUpdateStorageState(ctx context.Context, tx *sql.Tx, id string, state marketing.StorageState, version int64) (marketing.StorageState, error) {
	row := tx.QueryRowContext(ctx, `
		UPDATE marketing_storage_state
		SET value_type = $2,
		    data_json = $3,
		    version = $4,
		    last_event_id = NULLIF($5, ''),
		    expires_at = $6,
		    updated_at = $7
		WHERE id = $1
		RETURNING id, tenant_id, namespace, storage_key, value_type, data_json, version,
		          COALESCE(last_event_id, ''), expires_at, created_at, updated_at
	`, id, state.ValueType, state.Data, version, state.LastEventID, state.ExpiresAt, state.UpdatedAt)
	return scanStorageState(row)
}

func postgresAppendStorageState(ctx context.Context, db *sql.DB, state marketing.StorageState) (marketing.StorageState, error) {
	row := db.QueryRowContext(ctx, `
		INSERT INTO marketing_storage_state (
			id, tenant_id, namespace, storage_key, value_type, data_json,
			version, last_event_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'ARRAY', $5, 1, NULLIF($6, ''), $7, $8, $9)
		ON CONFLICT (tenant_id, namespace, storage_key) DO UPDATE
		SET value_type = 'ARRAY',
		    data_json = CASE
		        WHEN jsonb_typeof(marketing_storage_state.data_json) = 'array'
		          THEN marketing_storage_state.data_json || EXCLUDED.data_json
		        ELSE jsonb_build_array(marketing_storage_state.data_json) || EXCLUDED.data_json
		    END,
		    version = marketing_storage_state.version + 1,
		    last_event_id = EXCLUDED.last_event_id,
		    expires_at = EXCLUDED.expires_at,
		    updated_at = EXCLUDED.updated_at
		RETURNING id, tenant_id, namespace, storage_key, value_type, data_json, version,
		          COALESCE(last_event_id, ''), expires_at, created_at, updated_at
	`, state.ID, state.TenantID, state.Namespace, state.StorageKey, state.Data,
		state.LastEventID, state.ExpiresAt, state.CreatedAt, state.UpdatedAt)
	return scanStorageState(row)
}

func mapPostgresStorageStateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return marketing.ErrStorageStateConflict
	}
	return err
}

func mapPostgresResourceError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return marketing.ErrResourceConflict
	}
	return err
}

func jsonString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(raw)
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
