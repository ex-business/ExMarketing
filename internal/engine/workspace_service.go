package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	ownerID := strings.TrimSpace(workspaceFirstNonEmpty(created.OwnerID, created.CreatedBy))
	if ownerID != "" {
		_, memberErr := s.repository.CreateWorkspaceMember(ctx, marketing.WorkspaceMember{
			ID:          marketing.NewID("wsm"),
			WorkspaceID: created.ID,
			UserID:      ownerID,
			DisplayName: ownerID,
			Role:        marketing.WorkspaceRoleOwner,
			Status:      marketing.WorkspaceMemberActive,
			InvitedBy:   created.CreatedBy,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if memberErr != nil && !errors.Is(memberErr, marketing.ErrWorkspaceConflict) {
			return marketing.Workspace{}, memberErr
		}
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

func (s *Service) PatchWorkspace(ctx context.Context, id string, patch marketing.WorkspacePatch) (marketing.Workspace, error) {
	before, err := s.repository.GetWorkspace(ctx, strings.TrimSpace(id))
	if err != nil {
		return marketing.Workspace{}, err
	}
	updated := before
	if patch.Name != nil {
		updated.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Slug != nil {
		updated.Slug = workspaceSlug(*patch.Slug)
	}
	if patch.Description != nil {
		updated.Description = *patch.Description
	}
	if patch.Icon != nil {
		updated.Icon = *patch.Icon
	}
	if patch.Color != nil {
		updated.Color = *patch.Color
	}
	if patch.Visibility != nil {
		updated.Visibility = *patch.Visibility
	}
	if patch.DefaultRole != nil {
		updated.DefaultRole = *patch.DefaultRole
	}
	if patch.OwnerID != nil {
		updated.OwnerID = *patch.OwnerID
	}
	if patch.Status != nil {
		updated.Status = *patch.Status
	}
	if patch.UpdatedBy != nil {
		updated.UpdatedBy = *patch.UpdatedBy
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

type workspaceHomeSearcher interface {
	SearchWorkspaceHome(ctx context.Context, query marketing.WorkspaceHomeSearchQuery) (marketing.WorkspaceHomeSearchResponse, error)
}

func (s *Service) SearchWorkspaceHome(ctx context.Context, query marketing.WorkspaceHomeSearchQuery) (marketing.WorkspaceHomeSearchResponse, bool, error) {
	searcher, ok := s.repository.(workspaceHomeSearcher)
	if !ok {
		return marketing.WorkspaceHomeSearchResponse{}, false, nil
	}
	response, err := searcher.SearchWorkspaceHome(ctx, query)
	return response, true, err
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
	if member.Status == marketing.WorkspaceMemberInvited {
		if member.InvitedAt == nil {
			invitedAt := now
			member.InvitedAt = &invitedAt
		}
		if member.InviteExpiresAt == nil {
			expiresAt := now.Add(7 * 24 * time.Hour)
			member.InviteExpiresAt = &expiresAt
		}
		if strings.TrimSpace(member.InviteToken) == "" {
			member.InviteToken = marketing.NewID("invite")
		}
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

func (s *Service) GetWorkspaceMember(ctx context.Context, workspaceID string, memberID string) (marketing.WorkspaceMember, error) {
	members, err := s.repository.ListWorkspaceMembers(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	memberID = strings.TrimSpace(memberID)
	for _, member := range members {
		if member.ID == memberID {
			return member, nil
		}
	}
	return marketing.WorkspaceMember{}, marketing.ErrNotFound
}

func (s *Service) FindWorkspaceMemberByInviteToken(ctx context.Context, token string) (marketing.WorkspaceMember, error) {
	member, err := s.repository.FindWorkspaceMemberByInviteToken(ctx, strings.TrimSpace(token))
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	return member, nil
}

func (s *Service) UpdateWorkspaceMember(ctx context.Context, workspaceID string, memberID string, update marketing.WorkspaceMember) (marketing.WorkspaceMember, error) {
	before, err := s.GetWorkspaceMember(ctx, workspaceID, memberID)
	if err != nil {
		return marketing.WorkspaceMember{}, err
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
	if update.InviteToken != "" || update.Status == marketing.WorkspaceMemberActive {
		updated.InviteToken = update.InviteToken
	}
	if update.InvitedBy != "" {
		updated.InvitedBy = update.InvitedBy
	}
	if update.InvitedAt != nil {
		updated.InvitedAt = update.InvitedAt
	}
	if update.InviteExpiresAt != nil {
		updated.InviteExpiresAt = update.InviteExpiresAt
	}
	if update.InviteAcceptedAt != nil {
		updated.InviteAcceptedAt = update.InviteAcceptedAt
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

func (s *Service) ResendWorkspaceMemberInvite(ctx context.Context, workspaceID string, memberID string, actorID string) (marketing.WorkspaceMember, error) {
	now := s.now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	return s.UpdateWorkspaceMember(ctx, workspaceID, memberID, marketing.WorkspaceMember{
		Status:          marketing.WorkspaceMemberInvited,
		InviteToken:     marketing.NewID("invite"),
		InvitedBy:       actorID,
		InvitedAt:       &now,
		InviteExpiresAt: &expiresAt,
	})
}

func (s *Service) AcceptWorkspaceMemberInvite(ctx context.Context, workspaceID string, memberID string, token string, userID string) (marketing.WorkspaceMember, error) {
	before, err := s.GetWorkspaceMember(ctx, workspaceID, memberID)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	now := s.now()
	if before.Status != marketing.WorkspaceMemberInvited {
		return marketing.WorkspaceMember{}, errors.Join(marketing.ErrInvalidWorkspaceMember, errors.New("member is not invited"))
	}
	if strings.TrimSpace(before.InviteToken) == "" || strings.TrimSpace(before.InviteToken) != strings.TrimSpace(token) {
		return marketing.WorkspaceMember{}, marketing.ErrForbidden
	}
	if before.InviteExpiresAt != nil && now.After(*before.InviteExpiresAt) {
		return marketing.WorkspaceMember{}, errors.Join(marketing.ErrInvalidWorkspaceMember, errors.New("invite is expired"))
	}
	accepted := now
	updated := before
	updated.UserID = workspaceFirstNonEmpty(userID, before.UserID)
	updated.Status = marketing.WorkspaceMemberActive
	updated.InviteToken = ""
	updated.InviteAcceptedAt = &accepted
	updated.JoinedAt = &accepted
	updated.UpdatedAt = accepted
	saved, err := s.repository.UpdateWorkspaceMember(ctx, updated)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, before.InvitedBy, "ACCEPT_MEMBER_INVITE", "MEMBER", memberID, before, saved, s.now()))
	return saved, nil
}

func (s *Service) AcceptWorkspaceMemberInviteByToken(ctx context.Context, token string, userID string, displayName string) (marketing.WorkspaceMember, error) {
	member, err := s.FindWorkspaceMemberByInviteToken(ctx, token)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	updateUserID := workspaceFirstNonEmpty(userID, member.UserID, member.Email)
	accepted, err := s.AcceptWorkspaceMemberInvite(ctx, member.WorkspaceID, member.ID, token, updateUserID)
	if err != nil {
		return marketing.WorkspaceMember{}, err
	}
	if strings.TrimSpace(displayName) != "" && strings.TrimSpace(accepted.DisplayName) == "" {
		accepted, err = s.UpdateWorkspaceMember(ctx, accepted.WorkspaceID, accepted.ID, marketing.WorkspaceMember{
			DisplayName: strings.TrimSpace(displayName),
			InvitedBy:   accepted.InvitedBy,
		})
		if err != nil {
			return marketing.WorkspaceMember{}, err
		}
	}
	return accepted, nil
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

func (s *Service) PatchWorkspaceFolder(ctx context.Context, workspaceID string, folderID string, patch marketing.WorkspaceFolderPatch) (marketing.WorkspaceFolder, error) {
	before, err := s.GetWorkspaceFolder(ctx, workspaceID, folderID)
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	updated := before
	if patch.ParentID != nil {
		updated.ParentID = strings.TrimSpace(*patch.ParentID)
	}
	if patch.Name != nil {
		updated.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Description != nil {
		updated.Description = *patch.Description
	}
	if patch.Icon != nil {
		updated.Icon = *patch.Icon
	}
	if patch.Color != nil {
		updated.Color = *patch.Color
	}
	if patch.Visibility != nil {
		updated.Visibility = *patch.Visibility
	}
	if patch.SortOrder != nil {
		updated.SortOrder = *patch.SortOrder
	}
	if patch.Status != nil {
		updated.Status = *patch.Status
	}
	if patch.UpdatedBy != nil {
		updated.UpdatedBy = *patch.UpdatedBy
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
	deleted, err := s.UpdateWorkspaceFolder(ctx, workspaceID, folderID, marketing.WorkspaceFolder{Status: marketing.WorkspaceFolderDeleted, UpdatedBy: actorID})
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	folders, _ := s.repository.ListWorkspaceFolders(ctx, marketing.WorkspaceFolderQuery{WorkspaceID: workspaceID, Limit: 500})
	for _, folder := range folders {
		if folder.ID == folderID || folder.Status == marketing.WorkspaceFolderDeleted {
			continue
		}
		if workspaceFolderDescendant(folders, folder.ID, folderID) {
			folder.Status = marketing.WorkspaceFolderDeleted
			folder.UpdatedBy = actorID
			folder.UpdatedAt = s.now()
			_, _ = s.repository.UpdateWorkspaceFolder(ctx, folder)
		}
	}
	activities, _ := s.repository.ListActivities(ctx)
	for _, activity := range activities {
		if activity.WorkspaceID != workspaceID || activity.Status == marketing.ActivityArchived {
			continue
		}
		if activity.FolderID == folderID || workspaceFolderDescendant(folders, activity.FolderID, folderID) {
			activity.Status = marketing.ActivityArchived
			activity.UpdatedAt = s.now()
			_, _ = s.repository.UpdateActivity(ctx, activity)
		}
	}
	return deleted, nil
}

func (s *Service) RestoreWorkspaceFolder(ctx context.Context, workspaceID string, folderID string, actorID string) (marketing.WorkspaceFolder, error) {
	restored, err := s.UpdateWorkspaceFolder(ctx, workspaceID, folderID, marketing.WorkspaceFolder{Status: marketing.WorkspaceFolderActive, UpdatedBy: actorID})
	if err != nil {
		return marketing.WorkspaceFolder{}, err
	}
	folders, _ := s.repository.ListWorkspaceFolders(ctx, marketing.WorkspaceFolderQuery{WorkspaceID: workspaceID, Limit: 500})
	for _, folder := range folders {
		if folder.ID == folderID || folder.Status != marketing.WorkspaceFolderDeleted {
			continue
		}
		if workspaceFolderDescendant(folders, folder.ID, folderID) {
			folder.Status = marketing.WorkspaceFolderActive
			folder.UpdatedBy = actorID
			folder.UpdatedAt = s.now()
			_, _ = s.repository.UpdateWorkspaceFolder(ctx, folder)
		}
	}
	activities, _ := s.repository.ListActivities(ctx)
	for _, activity := range activities {
		if activity.WorkspaceID != workspaceID || activity.Status != marketing.ActivityArchived {
			continue
		}
		if activity.FolderID == folderID || workspaceFolderDescendant(folders, activity.FolderID, folderID) {
			active := marketing.ActivityActive
			activity.Status = active
			activity.UpdatedAt = s.now()
			_, _ = s.repository.UpdateActivity(ctx, activity)
		}
	}
	return restored, nil
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
	if strings.TrimSpace(link.PasswordHash) != "" && !strings.HasPrefix(link.PasswordHash, "sha256:") {
		link.PasswordHash = workspacePasswordHash(link.PasswordHash)
	}
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

func (s *Service) GetWorkspaceShareLink(ctx context.Context, workspaceID string, linkID string) (marketing.WorkspaceShareLink, error) {
	links, err := s.repository.ListWorkspaceShareLinks(ctx, marketing.WorkspaceShareLinkQuery{WorkspaceID: strings.TrimSpace(workspaceID)})
	if err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	linkID = strings.TrimSpace(linkID)
	for _, link := range links {
		if link.ID == linkID {
			return link, nil
		}
	}
	return marketing.WorkspaceShareLink{}, marketing.ErrNotFound
}

func (s *Service) PatchWorkspaceShareLink(ctx context.Context, workspaceID string, linkID string, patch marketing.WorkspaceShareLinkPatch) (marketing.WorkspaceShareLink, error) {
	before, err := s.GetWorkspaceShareLink(ctx, workspaceID, linkID)
	if err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	updated := before
	if patch.FolderID != nil {
		updated.FolderID = strings.TrimSpace(*patch.FolderID)
	}
	if patch.ActivityID != nil {
		updated.ActivityID = strings.TrimSpace(*patch.ActivityID)
	}
	if patch.Token != nil {
		updated.Token = strings.TrimSpace(*patch.Token)
	}
	if patch.Scope != nil {
		updated.Scope = *patch.Scope
	}
	if patch.Permission != nil {
		updated.Permission = *patch.Permission
	}
	if patch.ExpiresAt != nil {
		updated.ExpiresAt = patch.ExpiresAt
	}
	if patch.PasswordHash != nil {
		updated.PasswordHash = strings.TrimSpace(*patch.PasswordHash)
		if updated.PasswordHash != "" && !strings.HasPrefix(updated.PasswordHash, "sha256:") {
			updated.PasswordHash = workspacePasswordHash(updated.PasswordHash)
		}
	}
	if patch.MaxAccessCount != nil {
		updated.MaxAccessCount = *patch.MaxAccessCount
	}
	if patch.DisabledReason != nil {
		updated.DisabledReason = *patch.DisabledReason
	}
	if patch.Enabled != nil {
		updated.Enabled = *patch.Enabled
	}
	actorID := ""
	if patch.UpdatedBy != nil {
		actorID = strings.TrimSpace(*patch.UpdatedBy)
	}
	updated.UpdatedAt = s.now()
	if err := updated.Validate(); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	if err := s.ensureShareLinkScope(ctx, updated.WorkspaceID, updated); err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	saved, err := s.repository.UpdateWorkspaceShareLink(ctx, updated)
	if err != nil {
		return marketing.WorkspaceShareLink{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, actorID, "UPDATE_SHARE_LINK", "SHARE_LINK", linkID, before, saved, s.now()))
	return saved, nil
}

func (s *Service) DeleteWorkspaceShareLink(ctx context.Context, workspaceID string, linkID string, actorID string) (marketing.WorkspaceShareLink, error) {
	reason := "disabled"
	return s.PatchWorkspaceShareLink(ctx, workspaceID, linkID, marketing.WorkspaceShareLinkPatch{Enabled: boolPtr(false), DisabledReason: &reason, UpdatedBy: &actorID})
}

func (s *Service) RegenerateWorkspaceShareLink(ctx context.Context, workspaceID string, linkID string, actorID string) (marketing.WorkspaceShareLink, error) {
	token := marketing.NewID("share")
	return s.PatchWorkspaceShareLink(ctx, workspaceID, linkID, marketing.WorkspaceShareLinkPatch{Token: &token, UpdatedBy: &actorID})
}

func (s *Service) ResolveWorkspaceShareLink(ctx context.Context, token string, password string, actorID string) (marketing.WorkspaceShareLink, any, error) {
	links, err := s.repository.ListWorkspaceShareLinks(ctx, marketing.WorkspaceShareLinkQuery{Token: strings.TrimSpace(token), Limit: 1})
	if err != nil {
		return marketing.WorkspaceShareLink{}, nil, err
	}
	if len(links) == 0 {
		return marketing.WorkspaceShareLink{}, nil, marketing.ErrNotFound
	}
	link := links[0]
	now := s.now()
	if !link.Enabled {
		return marketing.WorkspaceShareLink{}, nil, marketing.ErrForbidden
	}
	if link.ExpiresAt != nil && now.After(*link.ExpiresAt) {
		return marketing.WorkspaceShareLink{}, nil, marketing.ErrForbidden
	}
	if link.MaxAccessCount > 0 && link.AccessCount >= link.MaxAccessCount {
		return marketing.WorkspaceShareLink{}, nil, marketing.ErrForbidden
	}
	if strings.TrimSpace(link.PasswordHash) != "" && link.PasswordHash != workspacePasswordHash(password) {
		return marketing.WorkspaceShareLink{}, nil, marketing.ErrForbidden
	}
	var target any
	switch link.Scope {
	case marketing.WorkspaceShareActivity:
		target, err = s.repository.GetActivity(ctx, link.ActivityID)
	case marketing.WorkspaceShareFolder:
		target, err = s.repository.GetWorkspaceFolder(ctx, link.FolderID)
	default:
		target, err = s.repository.GetWorkspace(ctx, link.WorkspaceID)
	}
	if err != nil {
		return marketing.WorkspaceShareLink{}, nil, err
	}
	link.AccessCount++
	link.LastAccessedAt = &now
	link.UpdatedAt = now
	updated, err := s.repository.UpdateWorkspaceShareLink(ctx, link)
	if err != nil {
		return marketing.WorkspaceShareLink{}, nil, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(link.WorkspaceID, actorID, "ACCESS_SHARE_LINK", "SHARE_LINK", link.ID, nil, map[string]any{"scope": link.Scope, "target": target}, now))
	return updated, target, nil
}

func (s *Service) CreateWorkspaceActivityGrant(ctx context.Context, workspaceID string, activityID string, grant marketing.WorkspaceActivityGrant) (marketing.WorkspaceActivityGrant, error) {
	now := s.now()
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(activityID))
	if err != nil {
		return marketing.WorkspaceActivityGrant{}, err
	}
	if activity.WorkspaceID != strings.TrimSpace(workspaceID) {
		return marketing.WorkspaceActivityGrant{}, marketing.ErrNotFound
	}
	grant.WorkspaceID = activity.WorkspaceID
	grant.ActivityID = activity.ID
	if strings.TrimSpace(grant.ID) == "" {
		grant.ID = marketing.NewID("wag")
	}
	if grant.Role == "" {
		grant.Role = marketing.WorkspaceRoleViewer
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = now
	}
	grant.UpdatedAt = now
	if err := grant.Validate(); err != nil {
		return marketing.WorkspaceActivityGrant{}, err
	}
	created, err := s.repository.CreateWorkspaceActivityGrant(ctx, grant)
	if err != nil {
		return marketing.WorkspaceActivityGrant{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, grant.CreatedBy, "CREATE_WORKFLOW_ACL", "ACTIVITY_GRANT", grant.ID, nil, created, now))
	return created, nil
}

func (s *Service) ListWorkspaceActivityGrants(ctx context.Context, workspaceID string, activityID string) ([]marketing.WorkspaceActivityGrant, error) {
	return s.repository.ListWorkspaceActivityGrants(ctx, marketing.WorkspaceActivityGrantQuery{WorkspaceID: strings.TrimSpace(workspaceID), ActivityID: strings.TrimSpace(activityID), Limit: 500})
}

func (s *Service) CreateWorkspaceFileObject(ctx context.Context, file marketing.WorkspaceFileObject) (marketing.WorkspaceFileObject, error) {
	now := s.now()
	if strings.TrimSpace(file.ID) == "" {
		file.ID = marketing.NewID("wsfile")
	}
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	if file.StorageProvider == "" {
		file.StorageProvider = "local"
	}
	if err := file.Validate(); err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	created, err := s.repository.CreateWorkspaceFileObject(ctx, file)
	if err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(file.WorkspaceID, file.CreatedBy, "CREATE_FILE_OBJECT", "FILE_OBJECT", file.ID, nil, created, now))
	return created, nil
}

func (s *Service) ListWorkspaceFileObjects(ctx context.Context, query marketing.WorkspaceFileObjectQuery) ([]marketing.WorkspaceFileObject, error) {
	return s.repository.ListWorkspaceFileObjects(ctx, query)
}

func (s *Service) DeleteWorkspaceFileObject(ctx context.Context, workspaceID string, fileID string, actorID string) (marketing.WorkspaceFileObject, error) {
	deleted, err := s.repository.DeleteWorkspaceFileObject(ctx, workspaceID, fileID)
	if err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, actorID, "DELETE_FILE_OBJECT", "FILE_OBJECT", deleted.ID, deleted, nil, s.now()))
	return deleted, nil
}

func (s *Service) ApplyWorkspaceBatch(ctx context.Context, cmd marketing.WorkspaceBatchCommand) (marketing.WorkspaceBatchResult, error) {
	cmd.WorkspaceID = strings.TrimSpace(cmd.WorkspaceID)
	cmd.FolderID = strings.TrimSpace(cmd.FolderID)
	cmd.ActorID = strings.TrimSpace(cmd.ActorID)
	cmd.Action = marketing.WorkspaceBatchAction(strings.ToUpper(strings.TrimSpace(string(cmd.Action))))
	if cmd.Visibility == "WORKSPACE" {
		cmd.Visibility = marketing.WorkspaceTeam
	}
	cmd.Visibility = marketing.WorkspaceVisibility(strings.ToUpper(strings.TrimSpace(string(cmd.Visibility))))
	for index := range cmd.Items {
		cmd.Items[index].ID = strings.TrimSpace(cmd.Items[index].ID)
		cmd.Items[index].Type = marketing.WorkspaceBatchItemType(strings.ToUpper(strings.TrimSpace(string(cmd.Items[index].Type))))
		if cmd.Items[index].Type == "ACTIVITY" {
			cmd.Items[index].Type = marketing.WorkspaceBatchWorkflow
		}
	}
	result, err := s.repository.ApplyWorkspaceBatch(ctx, cmd)
	if err != nil {
		return result, err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(cmd.WorkspaceID, cmd.ActorID, "APPLY_WORKSPACE_BATCH", "WORKSPACE", cmd.WorkspaceID, nil, result, s.now()))
	return result, nil
}

func (s *Service) DeleteWorkspaceActivityGrant(ctx context.Context, workspaceID string, grantID string, actorID string) error {
	grants, err := s.repository.ListWorkspaceActivityGrants(ctx, marketing.WorkspaceActivityGrantQuery{WorkspaceID: strings.TrimSpace(workspaceID), Limit: 500})
	if err != nil {
		return err
	}
	var before marketing.WorkspaceActivityGrant
	for _, grant := range grants {
		if grant.ID == strings.TrimSpace(grantID) {
			before = grant
			break
		}
	}
	if before.ID == "" {
		return marketing.ErrNotFound
	}
	if err := s.repository.DeleteWorkspaceActivityGrant(ctx, before.ID); err != nil {
		return err
	}
	_ = s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, actorID, "DELETE_WORKFLOW_ACL", "ACTIVITY_GRANT", before.ID, before, nil, s.now()))
	return nil
}

func (s *Service) ListWorkspaceAuditLogs(ctx context.Context, query marketing.WorkspaceAuditLogQuery) ([]marketing.WorkspaceAuditLog, error) {
	return s.repository.ListWorkspaceAuditLogs(ctx, query)
}

func (s *Service) RecordWorkspaceAudit(ctx context.Context, workspaceID string, actorID string, action string, targetType string, targetID string, before any, after any) error {
	return s.repository.CreateWorkspaceAuditLog(ctx, workspaceAudit(workspaceID, actorID, action, targetType, targetID, before, after, s.now()))
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
	folders, err := s.repository.ListWorkspaceFolders(ctx, marketing.WorkspaceFolderQuery{WorkspaceID: strings.TrimSpace(workspaceID), Limit: 500})
	if err != nil {
		return err
	}
	if workspaceFolderDescendant(folders, parentID, strings.TrimSpace(folderID)) {
		return marketing.ErrInvalidWorkspaceFolder
	}
	return nil
}

func (s *Service) ensureShareLinkScope(ctx context.Context, workspaceID string, link marketing.WorkspaceShareLink) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if link.Scope == marketing.WorkspaceShareActivity {
		activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(link.ActivityID))
		if err != nil {
			return err
		}
		if activity.WorkspaceID != workspaceID {
			return marketing.ErrNotFound
		}
		return nil
	}
	if link.Scope != marketing.WorkspaceShareFolder {
		return nil
	}
	folder, err := s.repository.GetWorkspaceFolder(ctx, strings.TrimSpace(link.FolderID))
	if err != nil {
		return err
	}
	if folder.WorkspaceID != workspaceID || folder.Status == marketing.WorkspaceFolderDeleted {
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

func boolPtr(value bool) *bool {
	return &value
}

func workspacePasswordHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func workspaceFolderDescendant(folders []marketing.WorkspaceFolder, folderID string, ancestorID string) bool {
	folderID = strings.TrimSpace(folderID)
	ancestorID = strings.TrimSpace(ancestorID)
	if folderID == "" || ancestorID == "" {
		return false
	}
	cursor := folderID
	seen := map[string]struct{}{}
	for cursor != "" {
		if cursor == ancestorID {
			return true
		}
		if _, ok := seen[cursor]; ok {
			return false
		}
		seen[cursor] = struct{}{}
		next := ""
		for _, folder := range folders {
			if folder.ID == cursor {
				next = folder.ParentID
				break
			}
		}
		cursor = next
	}
	return false
}

func workspaceFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
