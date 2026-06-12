package postgres

import (
	"context"
	"database/sql"
	"errors"
	"html"
	"regexp"
	"strings"
	"time"

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

func (r *Repository) SearchWorkspaceHome(ctx context.Context, query marketing.WorkspaceHomeSearchQuery) (marketing.WorkspaceHomeSearchResponse, error) {
	keyword := strings.TrimSpace(query.Keyword)
	typeFilter := strings.ToLower(strings.TrimSpace(query.Type))
	statusFilter := strings.ToUpper(strings.TrimSpace(query.Status))
	visibilityFilter := strings.ToUpper(strings.TrimSpace(query.Visibility))
	sortBy := strings.ToLower(strings.TrimSpace(query.Sort))
	sortOrder := strings.ToLower(strings.TrimSpace(query.Order))
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit <= 0 || query.Limit > 50 {
		query.Limit = 20
	}
	orderSQL := "score DESC, updated_at DESC, title ASC"
	switch sortBy {
	case "title", "name":
		if sortOrder == "desc" {
			orderSQL = "lower(title) DESC, score DESC"
		} else {
			orderSQL = "lower(title) ASC, score DESC"
		}
	case "owner":
		if sortOrder == "desc" {
			orderSQL = "lower(owner) DESC, score DESC"
		} else {
			orderSQL = "lower(owner) ASC, score DESC"
		}
	case "updated", "updatedat":
		if sortOrder == "asc" {
			orderSQL = "updated_at ASC, score DESC"
		} else {
			orderSQL = "updated_at DESC, score DESC"
		}
	}
	sqlText := `
		WITH actor_members AS (
			SELECT workspace_id, id AS member_id, role
			FROM marketing_workspace_member
			WHERE $6 <> '' AND status <> 'REMOVED' AND (user_id = $6 OR email = $6)
		), searchable AS (
			SELECT w.id, 'workspace' AS type, w.name AS title, w.slug AS subtitle,
			       w.workspace_id, '' AS folder_id, w.owner_id AS owner, w.status, w.visibility,
			       w.updated_at, 'workspace' AS matched_by,
			       concat_ws(' ', w.name, w.slug, w.description, w.owner_id) AS search_text,
			       CASE
			         WHEN $6 = '' THEN TRUE
			         WHEN w.owner_id = $6 THEN TRUE
			         WHEN w.visibility = 'PUBLIC' THEN TRUE
			         WHEN EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = w.id) THEN TRUE
			         ELSE FALSE
			       END AS allowed,
			       CASE
			         WHEN $6 = '' THEN 'development actor is empty; unrestricted local search'
			         WHEN w.owner_id = $6 THEN 'workspace owner'
			         WHEN EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = w.id) THEN 'workspace member'
			         WHEN w.visibility = 'PUBLIC' THEN 'workspace visibility PUBLIC'
			         ELSE 'not allowed'
			       END AS permission_reason
			FROM (
				SELECT id, id AS workspace_id, name, slug, description, owner_id, status, visibility, updated_at
				FROM marketing_workspace
				WHERE status = 'ACTIVE'
			) w
			UNION ALL
			SELECT a.id, 'workflow', COALESCE(NULLIF(a.activity_name, ''), a.activity_code),
			       concat_ws(' · ', a.activity_code, a.status, a.visibility),
			       COALESCE(a.workspace_id, ''), COALESCE(a.folder_id, ''), '' AS owner, a.status, COALESCE(a.visibility, 'TEAM'),
			       a.updated_at, 'workflow',
			       concat_ws(' ', a.activity_name, a.activity_code, a.tenant_id, a.status, COALESCE(a.visibility, 'TEAM')),
			       CASE
			         WHEN $6 = '' THEN TRUE
			         WHEN COALESCE(a.visibility, 'TEAM') IN ('TEAM', 'PUBLIC') AND EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = a.workspace_id) THEN TRUE
			         WHEN COALESCE(a.visibility, 'TEAM') = 'PUBLIC' THEN TRUE
			         WHEN EXISTS (
			           SELECT 1 FROM marketing_workspace_activity_grant g
			           LEFT JOIN actor_members am ON am.workspace_id = g.workspace_id
			           WHERE g.activity_id = a.id
			             AND (
			               (g.principal_type = 'USER' AND g.principal_id = $6)
			               OR (g.principal_type = 'MEMBER' AND g.principal_id = am.member_id)
			               OR (g.principal_type = 'ROLE' AND g.principal_id = am.role)
			             )
			         ) THEN TRUE
			         ELSE FALSE
			       END,
			       CASE
			         WHEN $6 = '' THEN 'development actor is empty; unrestricted local search'
			         WHEN COALESCE(a.visibility, 'TEAM') = 'PUBLIC' THEN 'workflow visibility PUBLIC'
			         WHEN COALESCE(a.visibility, 'TEAM') = 'TEAM' AND EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = a.workspace_id) THEN 'workspace member via workflow TEAM visibility'
			         WHEN EXISTS (
			           SELECT 1 FROM marketing_workspace_activity_grant g
			           LEFT JOIN actor_members am ON am.workspace_id = g.workspace_id
			           WHERE g.activity_id = a.id
			             AND (
			               (g.principal_type = 'USER' AND g.principal_id = $6)
			               OR (g.principal_type = 'MEMBER' AND g.principal_id = am.member_id)
			               OR (g.principal_type = 'ROLE' AND g.principal_id = am.role)
			             )
			         ) THEN 'workflow ACL grant'
			         ELSE 'not allowed'
			       END
			FROM marketing_activity a
			WHERE COALESCE(a.workspace_id, '') <> ''
			UNION ALL
			SELECT f.id, 'folder', f.name, concat_ws(' · ', w.name, f.visibility),
			       f.workspace_id, f.id, f.updated_by, f.status, f.visibility,
			       f.updated_at, 'folder',
			       concat_ws(' ', f.name, f.description, w.name, f.visibility),
			       CASE
			         WHEN $6 = '' THEN TRUE
			         WHEN f.visibility = 'PUBLIC' THEN TRUE
			         WHEN f.visibility IN ('TEAM', 'INHERIT') AND EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = f.workspace_id) THEN TRUE
			         ELSE FALSE
			       END,
			       CASE
			         WHEN $6 = '' THEN 'development actor is empty; unrestricted local search'
			         WHEN f.visibility = 'PUBLIC' THEN 'folder visibility PUBLIC'
			         WHEN f.visibility IN ('TEAM', 'INHERIT') AND EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = f.workspace_id) THEN 'workspace member via folder visibility'
			         ELSE 'not allowed'
			       END
			FROM marketing_workspace_folder f
			JOIN marketing_workspace w ON w.id = f.workspace_id
			WHERE f.status = 'ACTIVE'
			UNION ALL
			SELECT m.id, 'member', COALESCE(NULLIF(m.display_name, ''), NULLIF(m.user_id, ''), m.email),
			       concat_ws(' · ', m.role, w.name),
			       m.workspace_id, '' AS folder_id, m.user_id, m.status, w.visibility,
			       m.updated_at, 'member',
			       concat_ws(' ', m.display_name, m.user_id, m.email, m.role, w.name),
			       CASE
			         WHEN $6 = '' THEN TRUE
			         WHEN m.user_id = $6 OR m.email = $6 THEN TRUE
			         WHEN EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = m.workspace_id AND am.role IN ('OWNER', 'ADMIN')) THEN TRUE
			         ELSE FALSE
			       END,
			       CASE
			         WHEN $6 = '' THEN 'development actor is empty; unrestricted local search'
			         WHEN m.user_id = $6 OR m.email = $6 THEN 'self member record'
			         WHEN EXISTS (SELECT 1 FROM actor_members am WHERE am.workspace_id = m.workspace_id AND am.role IN ('OWNER', 'ADMIN')) THEN 'workspace admin can search members'
			         ELSE 'not allowed'
			       END
			FROM marketing_workspace_member m
			JOIN marketing_workspace w ON w.id = m.workspace_id
			WHERE m.status <> 'REMOVED'
		), ranked AS (
			SELECT *,
			       CASE
			         WHEN $1 = '' THEN 1
			         ELSE GREATEST(similarity(lower(search_text), lower($1)), similarity(lower(title), lower($1)))
			       END AS score
			FROM searchable
			WHERE allowed
			  AND ($2 = '' OR type = $2)
			  AND ($3 = '' OR upper(status) = $3)
			  AND ($4 = '' OR upper(visibility) = $4 OR ($4 = 'TEAM' AND upper(visibility) = 'WORKSPACE'))
			  AND ($5 = $5)
			  AND (
			    $1 = ''
			    OR lower(search_text) LIKE '%' || lower($1) || '%'
			    OR lower(title) % lower($1)
			    OR lower(search_text) % lower($1)
			    OR similarity(lower(title), lower($1)) > 0.18
			    OR similarity(lower(search_text), lower($1)) > 0.18
			  )
		), counted AS (
			SELECT *, count(*) OVER() AS total
			FROM ranked
		)
		SELECT id, type, title, subtitle, workspace_id, folder_id, owner, status, visibility,
		       updated_at, matched_by, score, permission_reason, total
		FROM counted
		ORDER BY ` + orderSQL + `
		OFFSET $7
		LIMIT $8
	`
	rows, err := r.db.QueryContext(ctx, sqlText, keyword, typeFilter, statusFilter, visibilityFilter, sortBy, strings.TrimSpace(query.ActorID), query.Offset, query.Limit)
	if err != nil {
		return marketing.WorkspaceHomeSearchResponse{}, err
	}
	defer rows.Close()
	response := marketing.WorkspaceHomeSearchResponse{Offset: query.Offset, Limit: query.Limit}
	for rows.Next() {
		var item marketing.WorkspaceHomeSearchResult
		var total int
		if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Subtitle, &item.WorkspaceID, &item.FolderID, &item.Owner, &item.Status, &item.Visibility, &item.UpdatedAt, &item.MatchedBy, &item.Score, &item.PermissionReason, &total); err != nil {
			return marketing.WorkspaceHomeSearchResponse{}, err
		}
		item.Permission = "VIEW"
		item.PermissionSources = workspaceSearchPermissionSources(item.Type, item.PermissionReason, item.Visibility)
		if len(item.PermissionSources) > 0 {
			item.PermissionReason = strings.Join(item.PermissionSources, " · ")
		}
		item.Highlights = workspaceSearchHighlights(keyword, item.Title, item.Subtitle)
		response.Items = append(response.Items, item)
		response.Total = total
	}
	if err := rows.Err(); err != nil {
		return marketing.WorkspaceHomeSearchResponse{}, err
	}
	return response, nil
}

func workspaceSearchPermissionSources(itemType string, reason string, visibility string) []string {
	reason = strings.TrimSpace(reason)
	visibility = strings.ToUpper(strings.TrimSpace(visibility))
	switch reason {
	case "development actor is empty; unrestricted local search":
		return []string{"本地开发模式", "未传 actor，允许搜索"}
	case "workspace owner":
		return []string{"Workspace Owner", "拥有全部可见权限"}
	case "workspace member":
		return []string{"Workspace 成员", "成员可见"}
	case "workspace visibility PUBLIC":
		return []string{"Workspace Public", "公开可见"}
	case "workflow visibility PUBLIC":
		return []string{"Workflow Public", "公开可见"}
	case "workspace member via workflow TEAM visibility":
		return []string{"Workspace 成员", "Workflow Team 可见"}
	case "workflow ACL grant":
		return []string{"Workflow ACL", "单独授权"}
	case "folder visibility PUBLIC":
		return []string{"Folder Public", "公开可见"}
	case "workspace member via folder visibility":
		if visibility == "INHERIT" {
			return []string{"Folder 继承", "Workspace 成员可见"}
		}
		return []string{"Folder Team", "Workspace 成员可见"}
	case "self member record":
		return []string{"成员本人", "只能查看自己的成员记录"}
	case "workspace admin can search members":
		return []string{"Workspace Admin", "可搜索成员"}
	}
	if reason == "" {
		return []string{strings.Title(itemType), "VIEW"}
	}
	return []string{reason}
}

func (r *Repository) CreateWorkspaceMember(ctx context.Context, member marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	if err := member.Validate(); err != nil {
		return marketing.WorkspaceMember{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_member (
			id, workspace_id, user_id, email, display_name, role, status,
			invite_token, invited_by, invited_at, invite_expires_at, invite_accepted_at, joined_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, member.ID, member.WorkspaceID, member.UserID, member.Email, member.DisplayName, member.Role,
		member.Status, member.InviteToken, member.InvitedBy, member.InvitedAt, member.InviteExpiresAt, member.InviteAcceptedAt, member.JoinedAt, member.CreatedAt, member.UpdatedAt)
	if err != nil {
		return marketing.WorkspaceMember{}, mapPostgresWorkspaceError(err)
	}
	return member, nil
}

func (r *Repository) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]marketing.WorkspaceMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, user_id, email, display_name, role, status,
		       invite_token, invited_by, invited_at, invite_expires_at, invite_accepted_at, joined_at, created_at, updated_at
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

func (r *Repository) FindWorkspaceMemberByInviteToken(ctx context.Context, token string) (marketing.WorkspaceMember, error) {
	return scanWorkspaceMember(r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, user_id, email, display_name, role, status,
		       invite_token, invited_by, invited_at, invite_expires_at, invite_accepted_at, joined_at, created_at, updated_at
		FROM marketing_workspace_member
		WHERE invite_token = $1 AND invite_token <> ''
	`, strings.TrimSpace(token)))
}

func (r *Repository) UpdateWorkspaceMember(ctx context.Context, member marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	if err := member.Validate(); err != nil {
		return marketing.WorkspaceMember{}, err
	}
	updated, err := scanWorkspaceMember(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace_member
		SET user_id = $2, email = $3, display_name = $4, role = $5, status = $6,
		    invite_token = $7, invited_by = $8, invited_at = $9, invite_expires_at = $10,
		    invite_accepted_at = $11, joined_at = $12, updated_at = $13
		WHERE id = $1
		RETURNING id, workspace_id, user_id, email, display_name, role, status,
		          invite_token, invited_by, invited_at, invite_expires_at, invite_accepted_at, joined_at, created_at, updated_at
	`, member.ID, member.UserID, member.Email, member.DisplayName, member.Role, member.Status,
		member.InviteToken, member.InvitedBy, member.InvitedAt, member.InviteExpiresAt, member.InviteAcceptedAt, member.JoinedAt, member.UpdatedAt))
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
			expires_at, password_hash, max_access_count, access_count, last_accessed_at,
			disabled_reason, enabled, created_by, created_at, updated_at
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, link.ID, link.WorkspaceID, link.FolderID, link.ActivityID, link.Token, link.Scope,
		link.Permission, link.ExpiresAt, link.PasswordHash, link.MaxAccessCount, link.AccessCount, link.LastAccessedAt,
		link.DisabledReason, link.Enabled, link.CreatedBy, link.CreatedAt, link.UpdatedAt)
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
		       scope, permission, expires_at, password_hash, max_access_count, access_count,
		       last_accessed_at, disabled_reason, enabled, created_by, created_at, updated_at
		FROM marketing_workspace_share_link
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR token = $2)
		  AND ($3 = '' OR scope = $3)
		  AND ($4 = '' OR COALESCE(folder_id, '') = $4)
		  AND ($5 = '' OR COALESCE(activity_id, '') = $5)
		  AND ($6 = FALSE OR enabled = TRUE)
		ORDER BY created_at DESC
		LIMIT $7
	`, query.WorkspaceID, query.Token, query.Scope, query.FolderID, query.ActivityID, query.EnabledOnly, query.Limit)
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
		    permission = $6, expires_at = $7, password_hash = $8, max_access_count = $9,
		    access_count = $10, last_accessed_at = $11, disabled_reason = $12, enabled = $13, updated_at = $14
		WHERE id = $1
		RETURNING id, workspace_id, COALESCE(folder_id, ''), COALESCE(activity_id, ''), token,
		          scope, permission, expires_at, password_hash, max_access_count, access_count,
		          last_accessed_at, disabled_reason, enabled, created_by, created_at, updated_at
	`, link.ID, link.FolderID, link.ActivityID, link.Token, link.Scope, link.Permission,
		link.ExpiresAt, link.PasswordHash, link.MaxAccessCount, link.AccessCount, link.LastAccessedAt,
		link.DisabledReason, link.Enabled, link.UpdatedAt))
	if err != nil {
		return marketing.WorkspaceShareLink{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) CreateWorkspaceActivityGrant(ctx context.Context, grant marketing.WorkspaceActivityGrant) (marketing.WorkspaceActivityGrant, error) {
	if err := grant.Validate(); err != nil {
		return marketing.WorkspaceActivityGrant{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_activity_grant (
			id, workspace_id, activity_id, principal_type, principal_id, role,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, grant.ID, grant.WorkspaceID, grant.ActivityID, grant.PrincipalType, grant.PrincipalID,
		grant.Role, grant.CreatedBy, grant.UpdatedBy, grant.CreatedAt, grant.UpdatedAt)
	if err != nil {
		return marketing.WorkspaceActivityGrant{}, mapPostgresWorkspaceError(err)
	}
	return grant, nil
}

func (r *Repository) ListWorkspaceActivityGrants(ctx context.Context, query marketing.WorkspaceActivityGrantQuery) ([]marketing.WorkspaceActivityGrant, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, activity_id, principal_type, principal_id, role,
		       created_by, updated_by, created_at, updated_at
		FROM marketing_workspace_activity_grant
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR activity_id = $2)
		  AND ($3 = '' OR principal_type = $3)
		  AND ($4 = '' OR principal_id = $4)
		ORDER BY created_at DESC
		LIMIT $5
	`, query.WorkspaceID, query.ActivityID, query.PrincipalType, query.PrincipalID, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceActivityGrant
	for rows.Next() {
		item, err := scanWorkspaceActivityGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateWorkspaceActivityGrant(ctx context.Context, grant marketing.WorkspaceActivityGrant) (marketing.WorkspaceActivityGrant, error) {
	if err := grant.Validate(); err != nil {
		return marketing.WorkspaceActivityGrant{}, err
	}
	updated, err := scanWorkspaceActivityGrant(r.db.QueryRowContext(ctx, `
		UPDATE marketing_workspace_activity_grant
		SET principal_type = $2, principal_id = $3, role = $4, updated_by = $5, updated_at = $6
		WHERE id = $1
		RETURNING id, workspace_id, activity_id, principal_type, principal_id, role,
		          created_by, updated_by, created_at, updated_at
	`, grant.ID, grant.PrincipalType, grant.PrincipalID, grant.Role, grant.UpdatedBy, grant.UpdatedAt))
	if err != nil {
		return marketing.WorkspaceActivityGrant{}, mapPostgresWorkspaceError(err)
	}
	return updated, nil
}

func (r *Repository) DeleteWorkspaceActivityGrant(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM marketing_workspace_activity_grant WHERE id = $1`, id)
	if err != nil {
		return mapPostgresWorkspaceError(err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return marketing.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateWorkspaceFileObject(ctx context.Context, file marketing.WorkspaceFileObject) (marketing.WorkspaceFileObject, error) {
	if err := file.Validate(); err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_file_object (
			id, workspace_id, folder_id, activity_id, workflow_version_id, execution_id,
			kind, file_name, content_type, size_bytes, storage_provider, storage_key,
			checksum_sha256, created_by, created_at
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
		          $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, file.ID, file.WorkspaceID, file.FolderID, file.ActivityID, file.WorkflowVersionID, file.ExecutionID,
		file.Kind, file.FileName, file.ContentType, file.SizeBytes, file.StorageProvider, file.StorageKey,
		file.ChecksumSHA256, file.CreatedBy, file.CreatedAt)
	if err != nil {
		return marketing.WorkspaceFileObject{}, mapPostgresWorkspaceError(err)
	}
	return file, nil
}

func (r *Repository) ListWorkspaceFileObjects(ctx context.Context, query marketing.WorkspaceFileObjectQuery) ([]marketing.WorkspaceFileObject, error) {
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, COALESCE(folder_id, ''), COALESCE(activity_id, ''),
		       COALESCE(workflow_version_id, ''), COALESCE(execution_id, ''), kind,
		       file_name, content_type, size_bytes, storage_provider, storage_key,
		       checksum_sha256, created_by, created_at
		FROM marketing_workspace_file_object
		WHERE ($1 = '' OR workspace_id = $1)
		  AND ($2 = '' OR COALESCE(folder_id, '') = $2)
		  AND ($3 = '' OR COALESCE(activity_id, '') = $3)
		  AND ($4 = '' OR COALESCE(workflow_version_id, '') = $4)
		  AND ($5 = '' OR COALESCE(execution_id, '') = $5)
		  AND ($6 = '' OR kind = $6)
		ORDER BY created_at DESC
		LIMIT $7
	`, query.WorkspaceID, query.FolderID, query.ActivityID, query.WorkflowVersionID, query.ExecutionID, query.Kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []marketing.WorkspaceFileObject
	for rows.Next() {
		item, err := scanWorkspaceFileObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteWorkspaceFileObject(ctx context.Context, workspaceID string, fileID string) (marketing.WorkspaceFileObject, error) {
	row := r.db.QueryRowContext(ctx, `
		DELETE FROM marketing_workspace_file_object
		WHERE workspace_id = $1 AND id = $2
		RETURNING id, workspace_id, COALESCE(folder_id, ''), COALESCE(activity_id, ''),
		       COALESCE(workflow_version_id, ''), COALESCE(execution_id, ''), kind,
		       file_name, content_type, size_bytes, storage_provider, storage_key,
		       checksum_sha256, created_by, created_at
	`, workspaceID, fileID)
	item, err := scanWorkspaceFileObject(row)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return marketing.WorkspaceFileObject{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceFileObject{}, mapPostgresWorkspaceError(err)
	}
	return item, nil
}

func (r *Repository) ApplyWorkspaceBatch(ctx context.Context, cmd marketing.WorkspaceBatchCommand) (marketing.WorkspaceBatchResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return marketing.WorkspaceBatchResult{}, err
	}
	defer rollback(tx)
	result, err := validatePostgresWorkspaceBatch(ctx, tx, cmd)
	if err != nil {
		return result, err
	}
	if len(result.Items) > 0 {
		return result, marketing.ErrInvalidWorkspace
	}
	now := time.Now().UTC()
	for _, item := range cmd.Items {
		if err := applyPostgresWorkspaceBatchItem(ctx, tx, cmd, item, now); err != nil {
			return marketing.WorkspaceBatchResult{
				Committed: false,
				Items:     []marketing.WorkspaceBatchItemResult{{ID: item.ID, Type: item.Type, Status: "FAILED", Error: err.Error()}},
			}, err
		}
		result.Items = append(result.Items, marketing.WorkspaceBatchItemResult{ID: item.ID, Type: item.Type, Status: "OK"})
	}
	if err := tx.Commit(); err != nil {
		return marketing.WorkspaceBatchResult{}, err
	}
	result.Committed = true
	result.Count = len(result.Items)
	return result, nil
}

func validatePostgresWorkspaceBatch(ctx context.Context, tx *sql.Tx, cmd marketing.WorkspaceBatchCommand) (marketing.WorkspaceBatchResult, error) {
	var result marketing.WorkspaceBatchResult
	fail := func(item marketing.WorkspaceBatchItem, message string) {
		result.Items = append(result.Items, marketing.WorkspaceBatchItemResult{ID: item.ID, Type: item.Type, Status: "FAILED", Error: message})
	}
	actorID := strings.TrimSpace(cmd.ActorID)
	workspaceAdmin := actorID == "" || postgresWorkspaceAdmin(ctx, tx, cmd.WorkspaceID, actorID)
	switch cmd.Action {
	case marketing.WorkspaceBatchMove:
		if strings.TrimSpace(cmd.FolderID) == "" {
			fail(marketing.WorkspaceBatchItem{}, "folderId is required")
		} else if !postgresFolderExists(ctx, tx, cmd.WorkspaceID, cmd.FolderID, true) {
			fail(marketing.WorkspaceBatchItem{ID: cmd.FolderID, Type: marketing.WorkspaceBatchFolder}, "target folder not found")
		}
	case marketing.WorkspaceBatchPermission:
		switch cmd.Visibility {
		case marketing.WorkspacePrivate, marketing.WorkspaceTeam, marketing.WorkspacePublic:
		default:
			fail(marketing.WorkspaceBatchItem{}, "visibility is invalid")
		}
	case marketing.WorkspaceBatchArchive, marketing.WorkspaceBatchRestore:
	default:
		fail(marketing.WorkspaceBatchItem{}, "unsupported batch action")
	}
	seen := map[string]struct{}{}
	for _, item := range cmd.Items {
		key := string(item.Type) + ":" + item.ID
		if item.ID == "" || item.Type == "" {
			fail(item, "item id and type are required")
			continue
		}
		if _, ok := seen[key]; ok {
			fail(item, "duplicate item")
			continue
		}
		seen[key] = struct{}{}
		switch item.Type {
		case marketing.WorkspaceBatchFolder:
			if !postgresFolderExists(ctx, tx, cmd.WorkspaceID, item.ID, false) {
				fail(item, "folder not found")
				continue
			}
			if !workspaceAdmin {
				fail(item, "actor has no permission for folder")
				continue
			}
			if cmd.Action == marketing.WorkspaceBatchMove && (item.ID == cmd.FolderID || postgresFolderDescendant(ctx, tx, cmd.WorkspaceID, cmd.FolderID, item.ID)) {
				fail(item, "cannot move folder into itself or its descendant")
			}
		case marketing.WorkspaceBatchWorkflow:
			status, exists := postgresActivityBatchStatus(ctx, tx, cmd.WorkspaceID, item.ID)
			if !exists {
				fail(item, "workflow not found")
				continue
			}
			if !workspaceAdmin && !postgresActivityBatchGrant(ctx, tx, cmd.WorkspaceID, item.ID, actorID) {
				fail(item, "actor has no permission for workflow")
				continue
			}
			if cmd.Action != marketing.WorkspaceBatchRestore && status == string(marketing.ActivityArchived) {
				fail(item, "workflow is archived")
				continue
			}
			if cmd.Action == marketing.WorkspaceBatchRestore && status != string(marketing.ActivityArchived) {
				fail(item, "workflow is not archived")
			}
		default:
			fail(item, "item type is invalid")
		}
	}
	return result, nil
}

func applyPostgresWorkspaceBatchItem(ctx context.Context, tx *sql.Tx, cmd marketing.WorkspaceBatchCommand, item marketing.WorkspaceBatchItem, now time.Time) error {
	if item.Type == marketing.WorkspaceBatchFolder {
		switch cmd.Action {
		case marketing.WorkspaceBatchMove:
			_, err := tx.ExecContext(ctx, `
				UPDATE marketing_workspace_folder
				SET parent_id = NULLIF($3, ''), updated_by = $4, updated_at = $5
				WHERE workspace_id = $1 AND id = $2
			`, cmd.WorkspaceID, item.ID, cmd.FolderID, cmd.ActorID, now)
			return err
		case marketing.WorkspaceBatchPermission:
			_, err := tx.ExecContext(ctx, `
				UPDATE marketing_workspace_folder
				SET visibility = $3, updated_by = $4, updated_at = $5
				WHERE workspace_id = $1 AND id = $2
			`, cmd.WorkspaceID, item.ID, marketing.WorkspaceFolderVisibility(cmd.Visibility), cmd.ActorID, now)
			return err
		case marketing.WorkspaceBatchArchive:
			_, err := tx.ExecContext(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM marketing_workspace_folder WHERE workspace_id = $1 AND id = $2
					UNION ALL
					SELECT f.id FROM marketing_workspace_folder f JOIN descendants d ON f.parent_id = d.id
					WHERE f.workspace_id = $1
				)
				UPDATE marketing_workspace_folder
				SET status = 'DELETED', updated_by = $3, updated_at = $4
				WHERE workspace_id = $1 AND id IN (SELECT id FROM descendants)
			`, cmd.WorkspaceID, item.ID, cmd.ActorID, now)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM marketing_workspace_folder WHERE workspace_id = $1 AND id = $2
					UNION ALL
					SELECT f.id FROM marketing_workspace_folder f JOIN descendants d ON f.parent_id = d.id
					WHERE f.workspace_id = $1
				)
				UPDATE marketing_activity
				SET status = 'ARCHIVED', updated_at = $3
				WHERE workspace_id = $1 AND folder_id IN (SELECT id FROM descendants)
			`, cmd.WorkspaceID, item.ID, now)
			return err
		case marketing.WorkspaceBatchRestore:
			_, err := tx.ExecContext(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM marketing_workspace_folder WHERE workspace_id = $1 AND id = $2
					UNION ALL
					SELECT f.id FROM marketing_workspace_folder f JOIN descendants d ON f.parent_id = d.id
					WHERE f.workspace_id = $1
				)
				UPDATE marketing_workspace_folder
				SET status = 'ACTIVE', updated_by = $3, updated_at = $4
				WHERE workspace_id = $1 AND id IN (SELECT id FROM descendants)
			`, cmd.WorkspaceID, item.ID, cmd.ActorID, now)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM marketing_workspace_folder WHERE workspace_id = $1 AND id = $2
					UNION ALL
					SELECT f.id FROM marketing_workspace_folder f JOIN descendants d ON f.parent_id = d.id
					WHERE f.workspace_id = $1
				)
				UPDATE marketing_activity
				SET status = 'ACTIVE', updated_at = $3
				WHERE workspace_id = $1 AND folder_id IN (SELECT id FROM descendants) AND status = 'ARCHIVED'
			`, cmd.WorkspaceID, item.ID, now)
			return err
		}
	}
	switch cmd.Action {
	case marketing.WorkspaceBatchMove:
		_, err := tx.ExecContext(ctx, `
			UPDATE marketing_activity
			SET workspace_id = $2, folder_id = $3, updated_at = $4
			WHERE id = $1
		`, item.ID, cmd.WorkspaceID, cmd.FolderID, now)
		return err
	case marketing.WorkspaceBatchPermission:
		_, err := tx.ExecContext(ctx, `
			UPDATE marketing_activity
			SET visibility = $2, updated_at = $3
			WHERE id = $1
		`, item.ID, cmd.Visibility, now)
		return err
	case marketing.WorkspaceBatchArchive:
		_, err := tx.ExecContext(ctx, `
			UPDATE marketing_activity
			SET status = 'ARCHIVED', updated_at = $2
			WHERE id = $1
		`, item.ID, now)
		return err
	case marketing.WorkspaceBatchRestore:
		_, err := tx.ExecContext(ctx, `
			UPDATE marketing_activity
			SET status = 'ACTIVE', updated_at = $2
			WHERE id = $1
		`, item.ID, now)
		return err
	}
	return marketing.ErrInvalidWorkspace
}

func postgresFolderExists(ctx context.Context, tx *sql.Tx, workspaceID string, folderID string, activeOnly bool) bool {
	var id string
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM marketing_workspace_folder
		WHERE workspace_id = $1 AND id = $2 AND ($3 = FALSE OR status <> 'DELETED')
		FOR UPDATE
	`, workspaceID, folderID, activeOnly).Scan(&id)
	return err == nil
}

func postgresActivityExists(ctx context.Context, tx *sql.Tx, workspaceID string, activityID string) bool {
	var id string
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM marketing_activity
		WHERE workspace_id = $1 AND id = $2
		FOR UPDATE
	`, workspaceID, activityID).Scan(&id)
	return err == nil
}

func postgresActivityBatchStatus(ctx context.Context, tx *sql.Tx, workspaceID string, activityID string) (string, bool) {
	var status string
	err := tx.QueryRowContext(ctx, `
		SELECT status FROM marketing_activity
		WHERE workspace_id = $1 AND id = $2
		FOR UPDATE
	`, workspaceID, activityID).Scan(&status)
	return status, err == nil
}

func postgresWorkspaceAdmin(ctx context.Context, tx *sql.Tx, workspaceID string, actorID string) bool {
	if strings.TrimSpace(actorID) == "" {
		return true
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM marketing_workspace w
			WHERE w.id = $1 AND w.owner_id = $2
			UNION ALL
			SELECT 1 FROM marketing_workspace_member m
			WHERE m.workspace_id = $1
			  AND m.status = 'ACTIVE'
			  AND m.role IN ('OWNER', 'ADMIN')
			  AND (m.user_id = $2 OR lower(m.email) = lower($2))
		)
	`, workspaceID, actorID).Scan(&allowed)
	return err == nil && allowed
}

func postgresActivityBatchGrant(ctx context.Context, tx *sql.Tx, workspaceID string, activityID string, actorID string) bool {
	if strings.TrimSpace(actorID) == "" {
		return true
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `
		WITH actor_members AS (
			SELECT id AS member_id, role
			FROM marketing_workspace_member
			WHERE workspace_id = $1
			  AND status = 'ACTIVE'
			  AND (user_id = $3 OR lower(email) = lower($3))
		)
		SELECT EXISTS (
			SELECT 1
			FROM marketing_workspace_activity_grant g
			LEFT JOIN actor_members am ON am.member_id = g.principal_id OR am.role = g.principal_id
			WHERE g.workspace_id = $1
			  AND g.activity_id = $2
			  AND g.role IN ('OWNER', 'ADMIN', 'EDITOR')
			  AND (
			    (g.principal_type = 'USER' AND g.principal_id = $3)
			    OR (g.principal_type = 'MEMBER' AND g.principal_id = am.member_id)
			    OR (g.principal_type = 'ROLE' AND g.principal_id = am.role)
			  )
		)
	`, workspaceID, activityID, actorID).Scan(&allowed)
	return err == nil && allowed
}

func postgresFolderDescendant(ctx context.Context, tx *sql.Tx, workspaceID string, folderID string, ancestorID string) bool {
	var found bool
	err := tx.QueryRowContext(ctx, `
		WITH RECURSIVE parents AS (
			SELECT id, parent_id FROM marketing_workspace_folder WHERE workspace_id = $1 AND id = $2
			UNION ALL
			SELECT f.id, f.parent_id FROM marketing_workspace_folder f JOIN parents p ON f.id = p.parent_id
			WHERE f.workspace_id = $1
		)
		SELECT EXISTS(SELECT 1 FROM parents WHERE id = $3)
	`, workspaceID, folderID, ancestorID).Scan(&found)
	return err == nil && found
}

func (r *Repository) CreateWorkspaceAuditLog(ctx context.Context, log marketing.WorkspaceAuditLog) error {
	source := marketing.AuditSourceFromContext(ctx)
	if log.IP == "" {
		if ip, ok := source["ip"].(string); ok {
			log.IP = ip
		}
	}
	if log.UserAgent == "" {
		if ua, ok := source["userAgent"].(string); ok {
			log.UserAgent = ua
		}
	}
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

func workspaceSearchHighlights(keyword string, title string, subtitle string) map[string]any {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil
	}
	out := map[string]any{}
	if highlighted := workspaceHighlightText(title, keyword); highlighted != html.EscapeString(title) {
		out["title"] = highlighted
	}
	if highlighted := workspaceHighlightText(subtitle, keyword); highlighted != html.EscapeString(subtitle) {
		out["subtitle"] = highlighted
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func workspaceHighlightText(value string, keyword string) string {
	escaped := html.EscapeString(value)
	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(keyword))
	if err != nil {
		return escaped
	}
	return pattern.ReplaceAllStringFunc(escaped, func(match string) string {
		return "<mark>" + match + "</mark>"
	})
}

func scanWorkspaceMember(scanner workspaceScanner) (marketing.WorkspaceMember, error) {
	var item marketing.WorkspaceMember
	var invitedAt sql.NullTime
	var inviteExpiresAt sql.NullTime
	var inviteAcceptedAt sql.NullTime
	var joinedAt sql.NullTime
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.UserID, &item.Email, &item.DisplayName,
		&item.Role, &item.Status, &item.InviteToken, &item.InvitedBy, &invitedAt, &inviteExpiresAt,
		&inviteAcceptedAt, &joinedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceMember{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceMember{}, err
	}
	if invitedAt.Valid {
		item.InvitedAt = &invitedAt.Time
	}
	if inviteExpiresAt.Valid {
		item.InviteExpiresAt = &inviteExpiresAt.Time
	}
	if inviteAcceptedAt.Valid {
		item.InviteAcceptedAt = &inviteAcceptedAt.Time
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
	var lastAccessedAt sql.NullTime
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.FolderID, &item.ActivityID, &item.Token,
		&item.Scope, &item.Permission, &expiresAt, &item.PasswordHash, &item.MaxAccessCount,
		&item.AccessCount, &lastAccessedAt, &item.DisabledReason, &item.Enabled, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceShareLink{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceShareLink{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	if lastAccessedAt.Valid {
		item.LastAccessedAt = &lastAccessedAt.Time
	}
	return item, nil
}

func scanWorkspaceActivityGrant(scanner workspaceScanner) (marketing.WorkspaceActivityGrant, error) {
	var item marketing.WorkspaceActivityGrant
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.ActivityID, &item.PrincipalType,
		&item.PrincipalID, &item.Role, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceActivityGrant{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceActivityGrant{}, err
	}
	return item, nil
}

func scanWorkspaceFileObject(scanner workspaceScanner) (marketing.WorkspaceFileObject, error) {
	var item marketing.WorkspaceFileObject
	err := scanner.Scan(&item.ID, &item.WorkspaceID, &item.FolderID, &item.ActivityID,
		&item.WorkflowVersionID, &item.ExecutionID, &item.Kind, &item.FileName,
		&item.ContentType, &item.SizeBytes, &item.StorageProvider, &item.StorageKey,
		&item.ChecksumSHA256, &item.CreatedBy, &item.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return marketing.WorkspaceFileObject{}, marketing.ErrNotFound
		}
		return marketing.WorkspaceFileObject{}, err
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
		CREATE EXTENSION IF NOT EXISTS pg_trgm;

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
			invite_token VARCHAR(128) NOT NULL DEFAULT '',
			invited_by VARCHAR(128) NOT NULL DEFAULT '',
			invited_at TIMESTAMPTZ,
			invite_expires_at TIMESTAMPTZ,
			invite_accepted_at TIMESTAMPTZ,
			joined_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS marketing_workspace_member_user_key
			ON marketing_workspace_member(workspace_id, user_id) WHERE user_id <> '';
		CREATE UNIQUE INDEX IF NOT EXISTS marketing_workspace_member_email_key
			ON marketing_workspace_member(workspace_id, lower(email)) WHERE email <> '';
		CREATE INDEX IF NOT EXISTS marketing_workspace_search_trgm_idx
			ON marketing_workspace USING gin (lower(name || ' ' || slug || ' ' || description || ' ' || owner_id) gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS marketing_workspace_member_search_trgm_idx
			ON marketing_workspace_member USING gin (lower(display_name || ' ' || user_id || ' ' || email || ' ' || role) gin_trgm_ops);

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
		CREATE INDEX IF NOT EXISTS marketing_workspace_folder_search_trgm_idx
			ON marketing_workspace_folder USING gin (lower(name || ' ' || description || ' ' || visibility) gin_trgm_ops);

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
			max_access_count INTEGER NOT NULL DEFAULT 0,
			access_count INTEGER NOT NULL DEFAULT 0,
			last_accessed_at TIMESTAMPTZ,
			disabled_reason TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS marketing_workspace_activity_grant (
			id VARCHAR(64) PRIMARY KEY,
			workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
			activity_id VARCHAR(64) NOT NULL,
			principal_type VARCHAR(32) NOT NULL,
			principal_id VARCHAR(128) NOT NULL,
			role VARCHAR(32) NOT NULL,
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			updated_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			UNIQUE (workspace_id, activity_id, principal_type, principal_id)
		);
		CREATE INDEX IF NOT EXISTS marketing_workspace_activity_grant_activity_idx
			ON marketing_workspace_activity_grant(activity_id, principal_type, principal_id);

		CREATE TABLE IF NOT EXISTS marketing_workspace_file_object (
			id VARCHAR(64) PRIMARY KEY,
			workspace_id VARCHAR(64) NOT NULL REFERENCES marketing_workspace(id) ON DELETE CASCADE,
			folder_id VARCHAR(64) REFERENCES marketing_workspace_folder(id) ON DELETE SET NULL,
			activity_id VARCHAR(64),
			workflow_version_id VARCHAR(64),
			execution_id VARCHAR(64),
			kind VARCHAR(64) NOT NULL,
			file_name VARCHAR(512) NOT NULL,
			content_type VARCHAR(255) NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL,
			storage_provider VARCHAR(64) NOT NULL,
			storage_key TEXT NOT NULL,
			checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
			created_by VARCHAR(128) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL
		);
		CREATE INDEX IF NOT EXISTS marketing_workspace_file_object_scope_idx
			ON marketing_workspace_file_object(workspace_id, kind, created_at DESC);
		CREATE INDEX IF NOT EXISTS marketing_workspace_file_object_activity_idx
			ON marketing_workspace_file_object(activity_id, workflow_version_id, execution_id);

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
				ALTER TABLE marketing_workspace_member
					ADD COLUMN IF NOT EXISTS invite_token VARCHAR(128) NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS invite_expires_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS invite_accepted_at TIMESTAMPTZ;
				ALTER TABLE marketing_workspace_share_link
					ADD COLUMN IF NOT EXISTS max_access_count INTEGER NOT NULL DEFAULT 0,
					ADD COLUMN IF NOT EXISTS access_count INTEGER NOT NULL DEFAULT 0,
					ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS disabled_reason TEXT NOT NULL DEFAULT '';
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
