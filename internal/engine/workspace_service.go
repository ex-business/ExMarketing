package engine

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

var workspaceSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Service) CreateWorkspace(ctx context.Context, workspace marketing.Workspace) (marketing.Workspace, error) {
	now := s.now()
	if strings.TrimSpace(workspace.ID) == "" {
		workspace.ID = marketing.NewID("ws")
	}
	workspace.Name = strings.TrimSpace(workspace.Name)
	if strings.TrimSpace(workspace.Slug) == "" {
		workspace.Slug = workspaceSlug(workspace.Name)
	}
	if workspace.Visibility == "" {
		workspace.Visibility = marketing.WorkspaceTeam
	}
	if workspace.DefaultRole == "" {
		workspace.DefaultRole = marketing.WorkspaceRoleViewer
	}
	if workspace.Status == "" {
		workspace.Status = marketing.WorkspaceActive
	}
	if workspace.CreatedAt.IsZero() {
		workspace.CreatedAt = now
	}
	workspace.UpdatedAt = now
	if err := workspace.Validate(); err != nil {
		return marketing.Workspace{}, err
	}
	created, err := s.repository.CreateWorkspace(ctx, workspace)
	if err != nil {
		return marketing.Workspace{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(created.ID, created.CreatedBy, "CREATE_WORKSPACE", "WORKSPACE", created.ID, nil, created, s.now()))
	return created, nil
}

func (s *Service) GetWorkspace(ctx context.Context, id string) (marketing.Workspace, error) {
	return s.repository.GetWorkspace(ctx, strings.TrimSpace(id))
}

func (s *Service) ListWorkspaces(ctx context.Context, query marketing.WorkspaceQuery) ([]marketing.Workspace, error) {
	return s.repository.ListWorkspaces(ctx, query)
}

func (s *Service) UpdateWorkspace(ctx context.Context, id string, update marketing.Workspace) (marketing.Workspace, error) {
	before, err := s.repository.GetWorkspace(ctx, strings.TrimSpace(id))
	if err != nil {
		return marketing.Workspace{}, err
	}
	updated := before
	if strings.TrimSpace(update.Name) != "" {
		updated.Name = strings.TrimSpace(update.Name)
	}
	if strings.TrimSpace(update.Slug) != "" {
		updated.Slug = workspaceSlug(update.Slug)
	}
	if update.Description != "" {
		updated.Description = update.Description
	}
	if update.Icon != "" {
		updated.Icon = update.Icon
	}
	if update.Color != "" {
		updated.Color = update.Color
	}
	if update.Visibility != "" {
		updated.Visibility = update.Visibility
	}
	if update.DefaultRole != "" {
		updated.DefaultRole = update.DefaultRole
	}
	if update.OwnerID != "" {
		updated.OwnerID = update.OwnerID
	}
	if update.Status != "" {
		updated.Status = update.Status
	}
	if update.UpdatedBy != "" {
		updated.UpdatedBy = update.UpdatedBy
	}
	updated.UpdatedAt = s.now()
	saved, err := s.repository.UpdateWorkspace(ctx, updated)
	if err != nil {
		return marketing.Workspace{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(saved.ID, saved.UpdatedBy, "UPDATE_WORKSPACE", "WORKSPACE", saved.ID, before, saved, s.now()))
	return saved, nil
}

func (s *Service) DeleteWorkspace(ctx context.Context, id string, actorID string) (marketing.Workspace, error) {
	return s.UpdateWorkspace(ctx, id, marketing.Workspace{Status: marketing.WorkspaceDeleted, UpdatedBy: actorID})
}

func (s *Service) CreateWorkspaceMember(ctx context.Context, workspaceID string, member marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	now := s.now()
	member.WorkspaceID = strings.TrimSpace(workspaceID)
	if strings.TrimSpace(member.ID) == "" {
		member.ID = marketing.NewID("wsm")
	}
	if member.Role == "" {
		member.Role = marketing.WorkspaceRoleViewer
	}
	if member.Status == "" {
		member.Status = marketing.WorkspaceMemberInvited
	}
	if member.CreatedAt.IsZero() {
		member.CreatedAt = now
	}
	member.UpdatedAt = now
	if err := member.Validate(); err != nil {
		return marketing.WorkspaceMember{}, err
	}
	created, err := s.repository.CreateWorkspaceMember(ctx, member)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(member.WorkspaceID, member.InvitedBy, "CREATE_MEMBER", "MEMBER", member.ID, nil, created, s.now()))
	return created, nil
}

func (s *Service) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]marketing.WorkspaceMember, error) {
	return s.repository.ListWorkspaceMembers(ctx, strings.TrimSpace(workspaceID))
}

func (s *Service) UpdateWorkspaceMember(ctx context.Context, workspaceID string, memberID string, update marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	members, err := s.repository.ListWorkspaceMembers(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	var before marketing.WorkspaceMember
	for _, member := range members {
		if member.ID == memberID {
			before = member
			break
		}
	}
	if before.ID == "" {
		return marketing.WorkspaceMember{}, marketing.ErrNotFound
	}
	updated := before
	if update.UserID != "" {
		updated.UserID = update.UserID
	}
	if update.Email != "" {
		updated.Email = update.Email
	}
	if update.DisplayName != "" {
		updated.DisplayName = update.DisplayName
	}
	if update.Role != "" {
		updated.Role = update.Role
	}
	if update.Status != "" {
		updated.Status = update.Status
	}
	if update.InvitedBy != "" {
		updated.InvitedBy = update.InvitedBy
	}
	if update.InvitedAt != nil {
		updated.InvitedAt = update.InvitedAt
	}
	if update.JoinedAt != nil {
		updated.JoinedAt = update.JoinedAt
	}
	updated.UpdatedAt = s.now()
	saved, err := s.repository.UpdateWorkspaceMember(ctx, updated)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, update.InvitedBy, "UPDATE_MEMBER", "MEMBER", memberID, before, saved, s.now()))
	return saved, nil
}

func (s *Service) DeleteWorkspaceMember(ctx context.Context, workspaceID string, memberID string, actorID string) (marketing.WorkspaceMember, error) {
	return s.UpdateWorkspaceMember(ctx, workspaceID, memberID, marketing.WorkspaceMember{Status: marketing.WorkspaceMemberRemoved, InvitedBy: actorID})
}

func (s *Service) CreateWorkspaceFolder(ctx context.Context, workspaceID string, folder marketing.WorkspaceFolder) (marketing.WorkspaceFolder, error) {
	now := s.now()
	folder.WorkspaceID = strings.TrimSpace(workspaceID)
	if strings.TrimSpace(folder.ID) == "" {
		folder.ID = marketing.NewID("wsf")
	}
	folder.Name = strings.TrimSpace(folder.Name)
	if folder.Visibility == "" {
		folder.Visibility = marketing.WorkspaceFolderInherit
	}
	if folder.Status == "" {
		folder.Status = marketing.WorkspaceFolderActive
	}
	if folder.CreatedAt.IsZero() {
		folder.CreatedAt = now
	}
	folder.UpdatedAt = now
	if err := folder.Validate(); err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	if err := s.ensureWorkspaceFolderParent(ctx, folder.WorkspaceID, folder.ParentID, folder.ID); err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	created, err := s.repository.CreateWorkspaceFolder(ctx, folder)
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, folder.CreatedBy, "CREATE_FOLDER", "FOLDER", folder.ID, nil, created, s.now()))
	return created, nil
}

func (s *Service) GetWorkspaceFolder(ctx context.Context, workspaceID string, folderID string) (marketing.WorkspaceFolder, error) {
	folder, err := s.repository.GetWorkspaceFolder(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	if folder.WorkspaceID != strings.TrimSpace(workspaceID) {
		return marketing.WorkspaceFolder{}, marketing.ErrNotFound
	}
	return folder, nil
}

func (s *Service) ListWorkspaceFolders(ctx context.Context, query marketing.WorkspaceFolderQuery) ([]marketing.WorkspaceFolder, error) {
	return s.repository.ListWorkspaceFolders(ctx, query)
}

func (s *Service) UpdateWorkspaceFolder(ctx context.Context, workspaceID string, folderID string, update marketing.WorkspaceFolder) (marketing.WorkspaceFolder, error) {
	before, err := s.GetWorkspaceFolder(ctx, workspaceID, folderID)
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	updated := before
	if update.ParentID != "" {
		updated.ParentID = update.ParentID
	}
	if update.Name != "" {
		updated.Name = strings.TrimSpace(update.Name)
	}
	if update.Description != "" {
		updated.Description = update.Description
	}
	if update.Icon != "" {
		updated.Icon = update.Icon
	}
	if update.Color != "" {
		updated.Color = update.Color
	}
	if update.Visibility != "" {
		updated.Visibility = update.Visibility
	}
	if update.SortOrder != 0 {
		updated.SortOrder = update.SortOrder
	}
	if update.Status != "" {
		updated.Status = update.Status
	}
	if update.UpdatedBy != "" {
		updated.UpdatedBy = update.UpdatedBy
	}
	updated.UpdatedAt = s.now()
	if err := s.ensureWorkspaceFolderParent(ctx, updated.WorkspaceID, updated.ParentID, updated.ID); err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	saved, err := s.repository.UpdateWorkspaceFolder(ctx, updated)
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, updated.UpdatedBy, "UPDATE_FOLDER", "FOLDER", folderID, before, saved, s.now()))
	return saved, nil
}

func (s *Service) DeleteWorkspaceFolder(ctx context.Context, workspaceID string, folderID string, actorID string) (marketing.WorkspaceFolder, error) {
	return s.UpdateWorkspaceFolder(ctx, workspaceID, folderID, marketing.WorkspaceFolder{Status: marketing.WorkspaceFolderDeleted, UpdatedBy: actorID})
}

func (s *Service) CreateWorkspaceShareLink(ctx context.Context, workspaceID string, link marketing.WorkspaceShareLink) (marketing.WorkspaceShareLink, error) {
	now := s.now()
	link.WorkspaceID = strings.TrimSpace(workspaceID)
	if link.ID == "" {
		link.ID = marketing.NewID("wss")
	}
	if link.Token == "" {
		link.Token = marketing.NewID("share")
	}
	if link.Permission == "" {
		link.Permission = marketing.WorkspaceShareView
	}
	if link.Scope == "" {
		link.Scope = marketing.WorkspaceShareWorkspace
	}
	link.Enabled = true
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now
	if err := link.Validate(); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	if err := s.ensureShareLinkScope(ctx, link.WorkspaceID, link); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	created, err := s.repository.CreateWorkspaceShareLink(ctx, link)
	if err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, link.CreatedBy, "CREATE_SHARE_LINK", "SHARE_LINK", link.ID, nil, created, s.now()))
	return created, nil
}

func (s *Service) ListWorkspaceShareLinks(ctx context.Context, query marketing.WorkspaceShareLinkQuery) ([]marketing.WorkspaceShareLink, error) {
	return s.repository.ListWorkspaceShareLinks(ctx, query)
}

func (s *Service) DeleteWorkspaceShareLink(ctx context.Context, workspaceID string, linkID string, actorID string) (marketing.WorkspaceShareLink, error) {
	links, err := s.repository.ListWorkspaceShareLinks(ctx, marketing.WorkspaceShareLinkQuery{WorkspaceID: strings.TrimSpace(workspaceID)})
	if err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	for _, link := range links {
		if link.ID == linkID {
			before := link
			link.Enabled = false
			link.UpdatedAt = s.now()
			saved, err := s.repository.UpdateWorkspaceShareLink(ctx, link)
			if err != nil {
				return marketing.WorkspaceShareLink{}, err
			}
			_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, actorID, "DELETE_SHARE_LINK", "SHARE_LINK", linkID, before, saved, s.now()))
			return saved, nil
		}
	}
	return marketing.WorkspaceShareLink{}, marketing.ErrNotFound
}

func (s *Service) ListWorkspaceAuditLogs(ctx context.Context, query marketing.WorkspaceAuditLogQuery) ([]marketing.WorkspaceAuditLog, error) {
	return s.repository.ListWorkspaceAuditLogs(ctx, query)
}

func (s *Service) ensureWorkspaceFolderParent(ctx context.Context, workspaceID string, parentID string, folderID string) error {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	if parentID == strings.TrimSpace(folderID) {
		return marketing.ErrInvalidWorkspaceFolder
	}
	parent, err := s.repository.GetWorkspaceFolder(ctx, parentID)
	if err != nil {
		return err
	}
	if parent.WorkspaceID != strings.TrimSpace(workspaceID) || parent.Status == marketing.WorkspaceFolderDeleted {
		return marketing.ErrNotFound
	}
	return nil
}

func (s *Service) ensureShareLinkScope(ctx context.Context, workspaceID string, link marketing.WorkspaceShareLink) error {
	if link.Scope != marketing.WorkspaceShareFolder {
		return nil
	}
	folder, err := s.repository.GetWorkspaceFolder(ctx, strings.TrimSpace(link.FolderID))
	if err != nil {
		return err
	}
	if folder.WorkspaceID != strings.TrimSpace(workspaceID) || folder.Status == marketing.WorkspaceFolderDeleted {
		return marketing.ErrNotFound
	}
	return nil
}

func workspaceSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = workspaceSlugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "workspace"
	}
	return value
}

func workspaceAudit(workspaceID string, actorID string, action string, targetType string, targetID string, before any, after any, createdAt time.Time) marketing.WorkspaceAuditLog {
	var beforeRaw json.RawMessage
	var afterRaw json.RawMessage
	if before != nil {
		beforeRaw, _ = json.Marshal(before)
	}
	if after != nil {
		afterRaw, _ = json.Marshal(after)
	}
	return marketing.WorkspaceAuditLog{
		ID:          marketing.NewID("wsa"),
		WorkspaceID: workspaceID,
		ActorID:     actorID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Before:      beforeRaw,
		After:       afterRaw,
		CreatedAt:   createdAt,
	}
}
