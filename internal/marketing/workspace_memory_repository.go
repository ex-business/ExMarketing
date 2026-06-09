package marketing

import (
	"context"
	"slices"
	"strings"
	"time"
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

func (r *MemoryRepository) FindWorkspaceMemberByInviteToken(ctx context.Context, token string) (WorkspaceMember, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceMember{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return WorkspaceMember{}, ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, member := range r.workspaceMembers {
		if strings.TrimSpace(member.InviteToken) == token {
			return member, nil
		}
	}
	return WorkspaceMember{}, ErrNotFound
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
		if query.Token != "" && link.Token != query.Token {
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

func (r *MemoryRepository) CreateWorkspaceActivityGrant(ctx context.Context, grant WorkspaceActivityGrant) (WorkspaceActivityGrant, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceActivityGrant{}, err
	}
	if err := grant.Validate(); err != nil {
		return WorkspaceActivityGrant{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaceActivityGrants {
		if sameWorkspaceActivityGrant(existing, grant) {
			return WorkspaceActivityGrant{}, ErrWorkspaceConflict
		}
	}
	r.workspaceActivityGrants = append(r.workspaceActivityGrants, grant)
	return grant, nil
}

func (r *MemoryRepository) ListWorkspaceActivityGrants(ctx context.Context, query WorkspaceActivityGrantQuery) ([]WorkspaceActivityGrant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := workspaceLimit(query.Limit)
	var out []WorkspaceActivityGrant
	for _, grant := range r.workspaceActivityGrants {
		if len(out) >= limit {
			break
		}
		if query.WorkspaceID != "" && grant.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.ActivityID != "" && grant.ActivityID != query.ActivityID {
			continue
		}
		if query.PrincipalType != "" && grant.PrincipalType != query.PrincipalType {
			continue
		}
		if query.PrincipalID != "" && grant.PrincipalID != query.PrincipalID {
			continue
		}
		out = append(out, grant)
	}
	return out, nil
}

func (r *MemoryRepository) UpdateWorkspaceActivityGrant(ctx context.Context, grant WorkspaceActivityGrant) (WorkspaceActivityGrant, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceActivityGrant{}, err
	}
	if err := grant.Validate(); err != nil {
		return WorkspaceActivityGrant{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.workspaceActivityGrants {
		if existing.ID != grant.ID && sameWorkspaceActivityGrant(existing, grant) {
			return WorkspaceActivityGrant{}, ErrWorkspaceConflict
		}
	}
	for i := range r.workspaceActivityGrants {
		if r.workspaceActivityGrants[i].ID == grant.ID {
			r.workspaceActivityGrants[i] = grant
			return grant, nil
		}
	}
	return WorkspaceActivityGrant{}, ErrNotFound
}

func (r *MemoryRepository) DeleteWorkspaceActivityGrant(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workspaceActivityGrants {
		if r.workspaceActivityGrants[i].ID == id {
			r.workspaceActivityGrants = append(r.workspaceActivityGrants[:i], r.workspaceActivityGrants[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) CreateWorkspaceFileObject(ctx context.Context, file WorkspaceFileObject) (WorkspaceFileObject, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceFileObject{}, err
	}
	if err := file.Validate(); err != nil {
		return WorkspaceFileObject{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceFileObjects = append(r.workspaceFileObjects, file)
	return file, nil
}

func (r *MemoryRepository) ListWorkspaceFileObjects(ctx context.Context, query WorkspaceFileObjectQuery) ([]WorkspaceFileObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	out := make([]WorkspaceFileObject, 0, len(r.workspaceFileObjects))
	for _, file := range r.workspaceFileObjects {
		if query.WorkspaceID != "" && file.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.FolderID != "" && file.FolderID != query.FolderID {
			continue
		}
		if query.ActivityID != "" && file.ActivityID != query.ActivityID {
			continue
		}
		if query.WorkflowVersionID != "" && file.WorkflowVersionID != query.WorkflowVersionID {
			continue
		}
		if query.ExecutionID != "" && file.ExecutionID != query.ExecutionID {
			continue
		}
		if query.Kind != "" && file.Kind != query.Kind {
			continue
		}
		out = append(out, file)
	}
	slices.SortFunc(out, func(a, b WorkspaceFileObject) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) DeleteWorkspaceFileObject(ctx context.Context, workspaceID string, fileID string) (WorkspaceFileObject, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceFileObject{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, file := range r.workspaceFileObjects {
		if file.ID != fileID || file.WorkspaceID != workspaceID {
			continue
		}
		r.workspaceFileObjects = append(r.workspaceFileObjects[:index], r.workspaceFileObjects[index+1:]...)
		return file, nil
	}
	return WorkspaceFileObject{}, ErrNotFound
}

func (r *MemoryRepository) ApplyWorkspaceBatch(ctx context.Context, cmd WorkspaceBatchCommand) (WorkspaceBatchResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceBatchResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := validateMemoryWorkspaceBatch(r, cmd)
	if len(result.Items) > 0 {
		return result, ErrInvalidWorkspace
	}
	snapshotFolders := slices.Clone(r.workspaceFolders)
	snapshotActivities := slices.Clone(r.activities)
	for _, item := range cmd.Items {
		if err := applyMemoryWorkspaceBatchItem(r, cmd, item); err != nil {
			r.workspaceFolders = snapshotFolders
			r.activities = snapshotActivities
			return WorkspaceBatchResult{Committed: false, Items: []WorkspaceBatchItemResult{{ID: item.ID, Type: item.Type, Status: "FAILED", Error: err.Error()}}}, err
		}
		result.Items = append(result.Items, WorkspaceBatchItemResult{ID: item.ID, Type: item.Type, Status: "OK"})
	}
	result.Committed = true
	result.Count = len(result.Items)
	return result, nil
}

func validateMemoryWorkspaceBatch(r *MemoryRepository, cmd WorkspaceBatchCommand) WorkspaceBatchResult {
	var result WorkspaceBatchResult
	seen := map[string]struct{}{}
	fail := func(item WorkspaceBatchItem, message string) {
		result.Items = append(result.Items, WorkspaceBatchItemResult{ID: item.ID, Type: item.Type, Status: "FAILED", Error: message})
	}
	switch cmd.Action {
	case WorkspaceBatchMove:
		if strings.TrimSpace(cmd.FolderID) == "" {
			fail(WorkspaceBatchItem{}, "folderId is required")
		}
	case WorkspaceBatchPermission:
		if !validWorkspaceVisibility(cmd.Visibility) {
			fail(WorkspaceBatchItem{}, "visibility is invalid")
		}
	case WorkspaceBatchArchive, WorkspaceBatchRestore:
	default:
		fail(WorkspaceBatchItem{}, "unsupported batch action")
	}
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
		case WorkspaceBatchFolder:
			folder := memoryFolderByID(r.workspaceFolders, item.ID)
			if folder == nil || folder.WorkspaceID != cmd.WorkspaceID {
				fail(item, "folder not found")
				continue
			}
			if cmd.Action == WorkspaceBatchMove {
				if item.ID == cmd.FolderID || memoryFolderDescendant(r.workspaceFolders, cmd.FolderID, item.ID) {
					fail(item, "cannot move folder into itself or its descendant")
				}
			}
		case WorkspaceBatchWorkflow:
			activity := memoryActivityByID(r.activities, item.ID)
			if activity == nil || activity.WorkspaceID != cmd.WorkspaceID {
				fail(item, "workflow not found")
			}
		default:
			fail(item, "item type is invalid")
		}
	}
	return result
}

func applyMemoryWorkspaceBatchItem(r *MemoryRepository, cmd WorkspaceBatchCommand, item WorkspaceBatchItem) error {
	now := time.Now().UTC()
	if item.Type == WorkspaceBatchFolder {
		for index := range r.workspaceFolders {
			if r.workspaceFolders[index].ID != item.ID {
				continue
			}
			switch cmd.Action {
			case WorkspaceBatchMove:
				r.workspaceFolders[index].ParentID = cmd.FolderID
			case WorkspaceBatchPermission:
				r.workspaceFolders[index].Visibility = WorkspaceFolderVisibility(cmd.Visibility)
			case WorkspaceBatchArchive:
				r.workspaceFolders[index].Status = WorkspaceFolderDeleted
			case WorkspaceBatchRestore:
				r.workspaceFolders[index].Status = WorkspaceFolderActive
			}
			r.workspaceFolders[index].UpdatedBy = cmd.ActorID
			r.workspaceFolders[index].UpdatedAt = now
			return nil
		}
		return ErrNotFound
	}
	for index := range r.activities {
		if r.activities[index].ID != item.ID {
			continue
		}
		switch cmd.Action {
		case WorkspaceBatchMove:
			r.activities[index].FolderID = cmd.FolderID
		case WorkspaceBatchPermission:
			r.activities[index].Visibility = cmd.Visibility
		case WorkspaceBatchArchive:
			r.activities[index].Status = ActivityArchived
		case WorkspaceBatchRestore:
			r.activities[index].Status = ActivityActive
		}
		r.activities[index].UpdatedAt = now
		return nil
	}
	return ErrNotFound
}

func memoryFolderByID(folders []WorkspaceFolder, id string) *WorkspaceFolder {
	for index := range folders {
		if folders[index].ID == id {
			return &folders[index]
		}
	}
	return nil
}

func memoryActivityByID(activities []Activity, id string) *Activity {
	for index := range activities {
		if activities[index].ID == id {
			return &activities[index]
		}
	}
	return nil
}

func memoryFolderDescendant(folders []WorkspaceFolder, folderID string, ancestorID string) bool {
	cursor := strings.TrimSpace(folderID)
	for cursor != "" {
		if cursor == ancestorID {
			return true
		}
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

func (r *MemoryRepository) CreateWorkspaceAuditLog(ctx context.Context, log WorkspaceAuditLog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source := AuditSourceFromContext(ctx)
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

func sameWorkspaceActivityGrant(a WorkspaceActivityGrant, b WorkspaceActivityGrant) bool {
	return a.WorkspaceID == b.WorkspaceID &&
		a.ActivityID == b.ActivityID &&
		a.PrincipalType == b.PrincipalType &&
		strings.TrimSpace(a.PrincipalID) == strings.TrimSpace(b.PrincipalID)
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
