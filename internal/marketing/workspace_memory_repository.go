package marketing

import (
	"context"
	"slices"
	"strings"
)

func (r *MemoryRepository) CreateWorkspace(ctx context.Context, workspace Workspace) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if err := workspace.Validate(); err != nil {
		return Workspace{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaces {
		if sameWorkspaceSlug(existing, workspace) {
			return Workspace{}, ErrWorkspaceConflict
		}
	}
	r.workspaces = append(r.workspaces, workspace)
	return workspace, nil
}

func (r *MemoryRepository) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, workspace := range r.workspaces {
		if workspace.ID == id {
			return workspace, nil
		}
	}
	return Workspace{}, ErrNotFound
}

func (r *MemoryRepository) ListWorkspaces(ctx context.Context, query WorkspaceQuery) ([]Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := workspaceLimit(query.Limit)
	var out []Workspace
	for _, workspace := range r.workspaces {
		if len(out) >= limit {
			break
		}
		if query.TenantID != "" && workspace.TenantID != query.TenantID {
			continue
		}
		if query.Status != "" && workspace.Status != query.Status {
			continue
		}
		if query.Visibility != "" && workspace.Visibility != query.Visibility {
			continue
		}
		if query.MemberUserID != "" && !r.workspaceHasMemberLocked(workspace.ID, query.MemberUserID) {
			continue
		}
		out = append(out, workspace)
	}
	return out, nil
}

func (r *MemoryRepository) UpdateWorkspace(ctx context.Context, workspace Workspace) (Workspace, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if err := workspace.Validate(); err != nil {
		return Workspace{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaces {
		if existing.ID != workspace.ID && sameWorkspaceSlug(existing, workspace) {
			return Workspace{}, ErrWorkspaceConflict
		}
	}
	for i := range r.workspaces {
		if r.workspaces[i].ID == workspace.ID {
			r.workspaces[i] = workspace
			return workspace, nil
		}
	}
	return Workspace{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkspaceMember(ctx context.Context, member WorkspaceMember) (WorkspaceMember, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceMember{}, err
	}
	if err := member.Validate(); err != nil {
		return WorkspaceMember{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaceMembers {
		if sameWorkspaceMember(existing, member) {
			return WorkspaceMember{}, ErrWorkspaceConflict
		}
	}
	r.workspaceMembers = append(r.workspaceMembers, member)
	return member, nil
}

func (r *MemoryRepository) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]WorkspaceMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []WorkspaceMember
	for _, member := range r.workspaceMembers {
		if member.WorkspaceID == workspaceID {
			out = append(out, member)
		}
	}
	return out, nil
}

func (r *MemoryRepository) UpdateWorkspaceMember(ctx context.Context, member WorkspaceMember) (WorkspaceMember, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceMember{}, err
	}
	if err := member.Validate(); err != nil {
		return WorkspaceMember{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaceMembers {
		if existing.ID != member.ID && sameWorkspaceMember(existing, member) {
			return WorkspaceMember{}, ErrWorkspaceConflict
		}
	}
	for i := range r.workspaceMembers {
		if r.workspaceMembers[i].ID == member.ID {
			r.workspaceMembers[i] = member
			return member, nil
		}
	}
	return WorkspaceMember{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkspaceFolder(ctx context.Context, folder WorkspaceFolder) (WorkspaceFolder, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceFolder{}, err
	}
	if err := folder.Validate(); err != nil {
		return WorkspaceFolder{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceFolders = append(r.workspaceFolders, folder)
	return folder, nil
}

func (r *MemoryRepository) GetWorkspaceFolder(ctx context.Context, id string) (WorkspaceFolder, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceFolder{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, folder := range r.workspaceFolders {
		if folder.ID == id {
			return folder, nil
		}
	}
	return WorkspaceFolder{}, ErrNotFound
}

func (r *MemoryRepository) ListWorkspaceFolders(ctx context.Context, query WorkspaceFolderQuery) ([]WorkspaceFolder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := workspaceLimit(query.Limit)
	var out []WorkspaceFolder
	for _, folder := range r.workspaceFolders {
		if len(out) >= limit {
			break
		}
		if query.WorkspaceID != "" && folder.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.ParentID != "" && folder.ParentID != query.ParentID {
			continue
		}
		if query.Status != "" && folder.Status != query.Status {
			continue
		}
		if query.Visibility != "" && folder.Visibility != query.Visibility {
			continue
		}
		out = append(out, folder)
	}
	slices.SortFunc(out, func(a, b WorkspaceFolder) int {
		return a.SortOrder - b.SortOrder
	})
	return out, nil
}

func (r *MemoryRepository) UpdateWorkspaceFolder(ctx context.Context, folder WorkspaceFolder) (WorkspaceFolder, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceFolder{}, err
	}
	if err := folder.Validate(); err != nil {
		return WorkspaceFolder{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workspaceFolders {
		if r.workspaceFolders[i].ID == folder.ID {
			r.workspaceFolders[i] = folder
			return folder, nil
		}
	}
	return WorkspaceFolder{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkspaceShareLink(ctx context.Context, link WorkspaceShareLink) (WorkspaceShareLink, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceShareLink{}, err
	}
	if err := link.Validate(); err != nil {
		return WorkspaceShareLink{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaceShareLinks {
		if strings.TrimSpace(existing.Token) == strings.TrimSpace(link.Token) {
			return WorkspaceShareLink{}, ErrWorkspaceConflict
		}
	}
	r.workspaceShareLinks = append(r.workspaceShareLinks, link)
	return link, nil
}

func (r *MemoryRepository) ListWorkspaceShareLinks(ctx context.Context, query WorkspaceShareLinkQuery) ([]WorkspaceShareLink, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := workspaceLimit(query.Limit)
	var out []WorkspaceShareLink
	for _, link := range r.workspaceShareLinks {
		if len(out) >= limit {
			break
		}
		if query.WorkspaceID != "" && link.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.Scope != "" && link.Scope != query.Scope {
			continue
		}
		if query.FolderID != "" && link.FolderID != query.FolderID {
			continue
		}
		if query.ActivityID != "" && link.ActivityID != query.ActivityID {
			continue
		}
		if query.EnabledOnly && !link.Enabled {
			continue
		}
		out = append(out, link)
	}
	return out, nil
}

func (r *MemoryRepository) UpdateWorkspaceShareLink(ctx context.Context, link WorkspaceShareLink) (WorkspaceShareLink, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceShareLink{}, err
	}
	if err := link.Validate(); err != nil {
		return WorkspaceShareLink{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workspaceShareLinks {
		if r.workspaceShareLinks[i].ID == link.ID {
			r.workspaceShareLinks[i] = link
			return link, nil
		}
	}
	return WorkspaceShareLink{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkspaceAuditLog(ctx context.Context, log WorkspaceAuditLog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceAuditLogs = append(r.workspaceAuditLogs, log)
	return nil
}

func (r *MemoryRepository) ListWorkspaceAuditLogs(ctx context.Context, query WorkspaceAuditLogQuery) ([]WorkspaceAuditLog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := workspaceLimit(query.Limit)
	var out []WorkspaceAuditLog
	for _, log := range r.workspaceAuditLogs {
		if len(out) >= limit {
			break
		}
		if query.WorkspaceID != "" && log.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.ActorID != "" && log.ActorID != query.ActorID {
			continue
		}
		if query.Action != "" && log.Action != query.Action {
			continue
		}
		if query.TargetType != "" && log.TargetType != query.TargetType {
			continue
		}
		if query.TargetID != "" && log.TargetID != query.TargetID {
			continue
		}
		if !query.StartTime.IsZero() && log.CreatedAt.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && !log.CreatedAt.Before(query.EndTime) {
			continue
		}
		out = append(out, log)
	}
	return out, nil
}

func (r *MemoryRepository) workspaceHasMemberLocked(workspaceID string, userID string) bool {
	for _, member := range r.workspaceMembers {
		if member.WorkspaceID == workspaceID && member.UserID == userID && member.Status != WorkspaceMemberRemoved {
			return true
		}
	}
	return false
}

func sameWorkspaceSlug(a Workspace, b Workspace) bool {
	return strings.TrimSpace(a.TenantID) == strings.TrimSpace(b.TenantID) &&
		strings.TrimSpace(a.Slug) == strings.TrimSpace(b.Slug)
}

func sameWorkspaceMember(a WorkspaceMember, b WorkspaceMember) bool {
	if a.WorkspaceID != b.WorkspaceID {
		return false
	}
	if strings.TrimSpace(a.UserID) != "" && strings.TrimSpace(a.UserID) == strings.TrimSpace(b.UserID) {
		return true
	}
	return strings.TrimSpace(a.Email) != "" && strings.EqualFold(strings.TrimSpace(a.Email), strings.TrimSpace(b.Email))
}

func workspaceLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}
