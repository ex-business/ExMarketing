package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/jackc/pgx/v5/pgconn"
)

func (r *Repository) CreateWorkspace(ctx context.Context, workspace marketing.Workspace) (marketing.Workspace, error) {
	if err := workspace.Validate(); err != nil {
		return marketing.Workspace{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace (
			id, tenant_id, name, slug, description, icon, color, visibility,
			default_role, owner_id, status, created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, workspace.ID, workspace.TenantID, workspace.Name, workspace.Slug, workspace.Description,
		workspace.Icon, workspace.Color, workspace.Visibility, workspace.DefaultRole, workspace.OwnerID,
		workspace.Status, workspace.CreatedBy, workspace.UpdatedBy, workspace.CreatedAt, workspace.UpdatedAt)
	if err != nil {
		return marketing.Workspace{}, mapPostgresWorkspaceError(err)
	}
	return workspace, nil
}

func (r *Repository) GetWorkspace(ctx context.Context, id string) (marketing.Workspace, error) {
	return scanWorkspace(r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, slug, description, icon, color, visibility,
		       default_role, owner_id, status, created_by, updated_by, created_at, updated_at
		FROM marketing_workspace
		WHERE id = $1
	`, id))
}

func (r *Repository) ListWorkspaces(ctx context.Context, query marketing.WorkspaceQuery) ([]marketing.Workspace, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.id, w.tenant_id, w.name, w.slug, w.description, w.icon, w.color, w.visibility,
		       w.default_role, w.owner_id, w.status, w.created_by, w.updated_by, w.created_at, w.updated_at
		FROM marketing_workspace w
		WHERE ($1 = '' OR w.tenant_id = $1)
		  AND ($2 = '' OR w.status = $2)
		  AND ($3 = '' OR w.visibility = $3)
		  AND ($4 = '' OR EXISTS (
		    SELECT 1 FROM marketing_workspace_member m
		    WHERE m.workspace_id = w.id AND m.user_id = $4 AND m.status <> 'REMOVED'
		  ))
		ORDER BY w.updated_at DESC
		LIMIT $5
	`, query.TenantID, query.Status, query.Visibility, query.MemberUserID, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.Workspace
	for rows.Next() {
		item, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateWorkspace(ctx context.Context, workspace marketing.Workspace) (marketing.Workspace, error) {
	if err := workspace.Validate(); err != nil {
		return marketing.Workspace{}, err
	}
	updated, err := scanWorkspace(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace
		SET name = $2, slug = $3, description = $4, icon = $5, color = $6,
		    visibility = $7, default_role = $8, owner_id = $9, status = $10,
		    updated_by = $11, updated_at = $12
		WHERE id = $1
		RETURNING id, tenant_id, name, slug, description, icon, color, visibility,
		          default_role, owner_id, status, created_by, updated_by, created_at, updated_at
	`, workspace.ID, workspace.Name, workspace.Slug, workspace.Description, workspace.Icon, workspace.Color,
		workspace.Visibility, workspace.DefaultRole, workspace.OwnerID, workspace.Status, workspace.UpdatedBy, workspace.UpdatedAt))
	if err != nil {
		return marketing.Workspace{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkspaceMember(ctx context.Context, member marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	if err := member.Validate(); err != nil {
		return marketing.WorkspaceMember{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_member (
			id, workspace_id, user_id, email, display_name, role, status,
			invited_by, invited_at, joined_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, member.ID, member.WorkspaceID, member.UserID, member.Email, member.DisplayName, member.Role,
		member.Status, member.InvitedBy, member.InvitedAt, member.JoinedAt, member.CreatedAt, member.UpdatedAt)
	if err != nil {
		return marketing.WorkspaceMember{}, mapPostgresWorkspaceError(err)
	}
	return member, nil
}

func (r *Repository) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]marketing.WorkspaceMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, user_id, email, display_name, role, status,
		       invited_by, invited_at, joined_at, created_at, updated_at
		FROM marketing_workspace_member
		WHERE workspace_id = $1
		ORDER BY created_at ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceMember
	for rows.Next() {
		item, err := scanWorkspaceMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateWorkspaceMember(ctx context.Context, member marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	if err := member.Validate(); err != nil {
		return marketing.WorkspaceMember{}, err
	}
	updated, err := scanWorkspaceMember(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace_member
		SET user_id = $2, email = $3, display_name = $4, role = $5, status = $6,
		    invited_by = $7, invited_at = $8, joined_at = $9, updated_at = $10
		WHERE id = $1
		RETURNING id, workspace_id, user_id, email, display_name, role, status,
		          invited_by, invited_at, joined_at, created_at, updated_at
	`, member.ID, member.UserID, member.Email, member.DisplayName, member.Role, member.Status,
		member.InvitedBy, member.InvitedAt, member.JoinedAt, member.UpdatedAt))
	if err != nil {
		return marketing.WorkspaceMember{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkspaceFolder(ctx context.Context, folder marketing.WorkspaceFolder) (marketing.WorkspaceFolder, error) {
	if err := folder.Validate(); err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_folder (
			id, workspace_id, parent_id, name, description, icon, color, visibility,
			sort_order, status, created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, folder.ID, folder.WorkspaceID, folder.ParentID, folder.Name, folder.Description, folder.Icon,
		folder.Color, folder.Visibility, folder.SortOrder, folder.Status, folder.CreatedBy, folder.UpdatedBy,
		folder.CreatedAt, folder.UpdatedAt)
	if err != nil {
		return marketing.WorkspaceFolder{}, mapPostgresWorkspaceError(err)
	}
	return folder, nil
}

func (r *Repository) GetWorkspaceFolder(ctx context.Context, id string) (marketing.WorkspaceFolder, error) {
	return scanWorkspaceFolder(r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, COALESCE(parent_id, ''), name, description, icon, color,
		       visibility, sort_order, status, created_by, updated_by, created_at, updated_at
		FROM marketing_workspace_folder
		WHERE id = $1
	`, id))
}

func (r *Repository) ListWorkspaceFolders(ctx context.Context, query marketing.WorkspaceFolderQuery) ([]marketing.WorkspaceFolder, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, COALESCE(parent_id, ''), name, description, icon, color,
		       visibility, sort_order, status, created_by, updated_by, created_at, updated_at
		FROM marketing_workspace_folder
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR COALESCE(parent_id, '') = $2)
		  AND ($3 = '' OR status = $3)
		  AND ($4 = '' OR visibility = $4)
		ORDER BY sort_order ASC, created_at ASC
		LIMIT $5
	`, query.WorkspaceID, query.ParentID, query.Status, query.Visibility, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceFolder
	for rows.Next() {
		item, err := scanWorkspaceFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateWorkspaceFolder(ctx context.Context, folder marketing.WorkspaceFolder) (marketing.WorkspaceFolder, error) {
	if err := folder.Validate(); err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	updated, err := scanWorkspaceFolder(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace_folder
		SET parent_id = NULLIF($2, ''), name = $3, description = $4, icon = $5, color = $6,
		    visibility = $7, sort_order = $8, status = $9, updated_by = $10, updated_at = $11
		WHERE id = $1
		RETURNING id, workspace_id, COALESCE(parent_id, ''), name, description, icon, color,
		          visibility, sort_order, status, created_by, updated_by, created_at, updated_at
	`, folder.ID, folder.ParentID, folder.Name, folder.Description, folder.Icon, folder.Color,
		folder.Visibility, folder.SortOrder, folder.Status, folder.UpdatedBy, folder.UpdatedAt))
	if err != nil {
		return marketing.WorkspaceFolder{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkspaceShareLink(ctx context.Context, link marketing.WorkspaceShareLink) (marketing.WorkspaceShareLink, error) {
	if err := link.Validate(); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_share_link (
			id, workspace_id, folder_id, activity_id, token, scope, permission,
			expires_at, password_hash, enabled, created_by, created_at, updated_at
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, link.ID, link.WorkspaceID, link.FolderID, link.ActivityID, link.Token, link.Scope,
		link.Permission, link.ExpiresAt, link.PasswordHash, link.Enabled, link.CreatedBy, link.CreatedAt, link.UpdatedAt)
	if err != nil {
		return marketing.WorkspaceShareLink{}, mapPostgresWorkspaceError(err)
	}
	return link, nil
}

func (r *Repository) ListWorkspaceShareLinks(ctx context.Context, query marketing.WorkspaceShareLinkQuery) ([]marketing.WorkspaceShareLink, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, COALESCE(folder_id, ''), COALESCE(activity_id, ''), token,
		       scope, permission, expires_at, password_hash, enabled, created_by, created_at, updated_at
		FROM marketing_workspace_share_link
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR scope = $2)
		  AND ($3 = '' OR COALESCE(folder_id, '') = $3)
		  AND ($4 = '' OR COALESCE(activity_id, '') = $4)
		  AND ($5 = FALSE OR enabled = TRUE)
		ORDER BY created_at DESC
		LIMIT $6
	`, query.WorkspaceID, query.Scope, query.FolderID, query.ActivityID, query.EnabledOnly, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceShareLink
	for rows.Next() {
		item, err := scanWorkspaceShareLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateWorkspaceShareLink(ctx context.Context, link marketing.WorkspaceShareLink) (marketing.WorkspaceShareLink, error) {
	if err := link.Validate(); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	updated, err := scanWorkspaceShareLink(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace_share_link
		SET folder_id = NULLIF($2, ''), activity_id = NULLIF($3, ''), token = $4, scope = $5,
		    permission = $6, expires_at = $7, password_hash = $8, enabled = $9, updated_at = $10
		WHERE id = $1
		RETURNING id, workspace_id, COALESCE(folder_id, ''), COALESCE(activity_id, ''), token,
		          scope, permission, expires_at, password_hash, enabled, created_by, created_at, updated_at
	`, link.ID, link.FolderID, link.ActivityID, link.Token, link.Scope, link.Permission,
		link.ExpiresAt, link.PasswordHash, link.Enabled, link.UpdatedAt))
	if err != nil {
		return marketing.WorkspaceShareLink{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkspaceAuditLog(ctx context.Context, log marketing.WorkspaceAuditLog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_audit_log (
			id, workspace_id, actor_id, action, target_type, target_id,
			before_json, after_json, ip, user_agent, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, log.ID, log.WorkspaceID, log.ActorID, log.Action, log.TargetType, log.TargetID,
		nullJSON(log.Before), nullJSON(log.After), log.IP, log.UserAgent, log.CreatedAt)
	return err
}

func (r *Repository) ListWorkspaceAuditLogs(ctx context.Context, query marketing.WorkspaceAuditLogQuery) ([]marketing.WorkspaceAuditLog, error) {
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
		SELECT id, workspace_id, actor_id, action, target_type, target_id,
		       before_json, after_json, ip, user_agent, created_at
		FROM marketing_workspace_audit_log
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR actor_id = $2)
		  AND ($3 = '' OR action = $3)
		  AND ($4 = '' OR target_type = $4)
		  AND ($5 = '' OR target_id = $5)
		  AND ($6::timestamptz IS NULL OR created_at >= $6)
		  AND ($7::timestamptz IS NULL OR created_at < $7)
		ORDER BY created_at DESC
		LIMIT $8
	`, query.WorkspaceID, query.ActorID, query.Action, query.TargetType, query.TargetID,
		startTime, endTime, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceAuditLog
	for rows.Next() {
		item, err := scanWorkspaceAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type workspaceScanner interface {
	Scan(dest ...any) error
}

func scanWorkspace(scanner workspaceScanner) (marketing.Workspace, error) {
	var item marketing.Workspace
	err := scanner.Scan(&item.ID, &item.TenantID, &item.Name, &item.Slug, &item.Description,
		&item.Icon, &item.Color, &item.Visibility, &item.DefaultRole, &item.OwnerID,
		&item.Status, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.Workspace{}, marketing.ErrNotFound
		}
		return marketing.Workspace{}, err
	}
	return item, nil
}

func scanWorkspaceMember(scanner workspaceScanner) (marketing.WorkspaceMember, error) {
	var item marketing.WorkspaceMember
	var invitedAt sql.NullTime
	var joinedAt sql.NullTime
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.UserID, &item.Email, &item.DisplayName,
		&item.Role, &item.Status, &item.InvitedBy, &invitedAt, &joinedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceMember{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceMember{}, err
	}
	if invitedAt.Valid {
		item.InvitedAt = &invitedAt.Time
	}
	if joinedAt.Valid {
		item.JoinedAt = &joinedAt.Time
	}
	return item, nil
}

func scanWorkspaceFolder(scanner workspaceScanner) (marketing.WorkspaceFolder, error) {
	var item marketing.WorkspaceFolder
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.ParentID, &item.Name, &item.Description,
		&item.Icon, &item.Color, &item.Visibility, &item.SortOrder, &item.Status, &item.CreatedBy,
		&item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceFolder{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceFolder{}, err
	}
	return item, nil
}

func scanWorkspaceShareLink(scanner workspaceScanner) (marketing.WorkspaceShareLink, error) {
	var item marketing.WorkspaceShareLink
	var expiresAt sql.NullTime
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.FolderID, &item.ActivityID, &item.Token,
		&item.Scope, &item.Permission, &expiresAt, &item.PasswordHash, &item.Enabled,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceShareLink{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceShareLink{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	return item, nil
}

func scanWorkspaceAuditLog(scanner workspaceScanner) (marketing.WorkspaceAuditLog, error) {
	var item marketing.WorkspaceAuditLog
	var before sql.NullString
	var after sql.NullString
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.ActorID, &item.Action,
		&item.TargetType, &item.TargetID, &before, &after, &item.IP, &item.UserAgent, &item.CreatedAt)
	if err != nil {
		return marketing.WorkspaceAuditLog{}, err
	}
	if before.Valid {
		item.Before = []byte(before.String)
	}
	if after.Valid {
		item.After = []byte(after.String)
	}
	return item, nil
}

func ensureWorkspaceTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS marketing_workspace (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL,
			name VARCHAR(255) NOT NULL,
			slug VARCHAR(128) NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			icon VARCHAR(128) NOT NULL DEFAULT '',
			color VARCHAR(64) NOT NULL DEFAULT '',
			visibility VARCHAR(32) NOT NULL,
			default_role VARCHAR(32) NOT NULL,
			owner_id VARCHAR(128) NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL,
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			updated_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			UNIQUE (tenant_id, slug)
		);

		CREATE TABLE IF NOT EXISTS marketing_workspace_member (
			id VARCHAR(64) PRIMARY KEY,
			workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
			user_id VARCHAR(128) NOT NULL DEFAULT '',
			email VARCHAR(255) NOT NULL DEFAULT '',
			display_name VARCHAR(255) NOT NULL DEFAULT '',
			role VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			invited_by VARCHAR(128) NOT NULL DEFAULT '',
			invited_at TIMESTAMPTZ,
			joined_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS marketing_workspace_member_user_key
			ON marketing_workspace_member(workspace_id, user_id) WHERE user_id <> '';
		CREATE UNIQUE INDEX IF NOT EXISTS marketing_workspace_member_email_key
			ON marketing_workspace_member(workspace_id, lower(email)) WHERE email <> '';

			CREATE TABLE IF NOT EXISTS marketing_workspace_folder (
				id VARCHAR(64) PRIMARY KEY,
				workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
				parent_id VARCHAR(64) REFERENCES marketing_workspace_folder(id) ON DELETE SET NULL,
			name VARCHAR(255) NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			icon VARCHAR(128) NOT NULL DEFAULT '',
			color VARCHAR(64) NOT NULL DEFAULT '',
			visibility VARCHAR(32) NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL,
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			updated_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);

			CREATE TABLE IF NOT EXISTS marketing_workspace_share_link (
				id VARCHAR(64) PRIMARY KEY,
				workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
			folder_id VARCHAR(64) REFERENCES marketing_workspace_folder(id) ON DELETE CASCADE,
			activity_id VARCHAR(64),
			token VARCHAR(128) NOT NULL UNIQUE,
			scope VARCHAR(32) NOT NULL,
			permission VARCHAR(32) NOT NULL,
			expires_at TIMESTAMPTZ,
			password_hash VARCHAR(255) NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS marketing_workspace_audit_log (
			id VARCHAR(64) PRIMARY KEY,
			workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
			actor_id VARCHAR(128) NOT NULL DEFAULT '',
			action VARCHAR(128) NOT NULL,
			target_type VARCHAR(64) NOT NULL,
			target_id VARCHAR(128) NOT NULL DEFAULT '',
			before_json JSONB,
			after_json JSONB,
			ip VARCHAR(64) NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL
			);

			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conname = 'marketing_workspace_folder_workspace_id_id_key'
					  AND conrelid = 'marketing_workspace_folder'::regclass
				) THEN
					ALTER TABLE marketing_workspace_folder
						ADD CONSTRAINT marketing_workspace_folder_workspace_id_id_key
						UNIQUE (workspace_id, id);
				END IF;
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conname = 'marketing_workspace_folder_parent_workspace_fk'
					  AND conrelid = 'marketing_workspace_folder'::regclass
				) THEN
					ALTER TABLE marketing_workspace_folder
						ADD CONSTRAINT marketing_workspace_folder_parent_workspace_fk
						FOREIGN KEY (workspace_id, parent_id)
						REFERENCES marketing_workspace_folder(workspace_id, id);
				END IF;
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conname = 'marketing_workspace_share_link_folder_workspace_fk'
					  AND conrelid = 'marketing_workspace_share_link'::regclass
				) THEN
					ALTER TABLE marketing_workspace_share_link
						ADD CONSTRAINT marketing_workspace_share_link_folder_workspace_fk
						FOREIGN KEY (workspace_id, folder_id)
						REFERENCES marketing_workspace_folder(workspace_id, id)
						ON DELETE CASCADE;
				END IF;
			END $$;
		`)
	return err
}

func mapPostgresWorkspaceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return marketing.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return marketing.ErrWorkspaceConflict
	}
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return marketing.ErrNotFound
	}
	return err
}
