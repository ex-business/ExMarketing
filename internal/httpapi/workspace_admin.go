package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	query := marketing.WorkspaceQuery{
		TenantID:     strings.TrimSpace(r.URL.Query().Get("tenantId")),
		Status:       marketing.WorkspaceStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		Visibility:   marketing.WorkspaceVisibility(strings.TrimSpace(r.URL.Query().Get("visibility"))),
		MemberUserID: strings.TrimSpace(r.URL.Query().Get("memberUserId")),
		Limit:        intQuery(r, "limit"),
	}
	items, err := h.engine.ListWorkspaces(r.Context(), query)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var workspace marketing.Workspace
	if err := decodeStrictJSON(r.Body, &workspace); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	actorID := actorFromRequest(r)
	if actorID == "" {
		actorID = strings.TrimSpace(firstNonEmpty(workspace.OwnerID, workspace.CreatedBy, "admin-flow"))
	}
	if strings.TrimSpace(workspace.OwnerID) == "" {
		workspace.OwnerID = actorID
	}
	if strings.TrimSpace(workspace.CreatedBy) == "" {
		workspace.CreatedBy = actorID
	}
	created, err := h.engine.CreateWorkspace(r.Context(), workspace)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	workspace, err := h.engine.GetWorkspace(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	var patch marketing.WorkspacePatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	actorID := actorFromRequest(r)
	if actorID != "" && patch.UpdatedBy == nil {
		patch.UpdatedBy = &actorID
	}
	updated, err := h.engine.PatchWorkspace(r.Context(), r.PathValue("workspaceId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	deleted, err := h.engine.DeleteWorkspace(r.Context(), r.PathValue("workspaceId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	items, err := h.engine.ListWorkspaceMembers(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	var member marketing.WorkspaceMember
	if err := decodeStrictJSON(r.Body, &member); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(member.InvitedBy) == "" {
		member.InvitedBy = actorID
	}
	created, err := h.engine.CreateWorkspaceMember(r.Context(), r.PathValue("workspaceId"), member)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	member, err := h.engine.GetWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

func (h *Handler) updateWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	var member marketing.WorkspaceMember
	if err := decodeStrictJSON(r.Body, &member); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(member.InvitedBy) == "" {
		member.InvitedBy = actorID
	}
	updated, err := h.engine.UpdateWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), member)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	deleted, err := h.engine.DeleteWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceFolders(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	actorID := actorFromRequest(r)
	query := marketing.WorkspaceFolderQuery{
		WorkspaceID: r.PathValue("workspaceId"),
		ParentID:    strings.TrimSpace(r.URL.Query().Get("parentId")),
		Status:      marketing.WorkspaceFolderStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		Visibility:  marketing.WorkspaceFolderVisibility(strings.TrimSpace(r.URL.Query().Get("visibility"))),
		Limit:       intQuery(r, "limit"),
	}
	items, err := h.engine.ListWorkspaceFolders(r.Context(), query)
	if err != nil {
		workspaceError(w, err)
		return
	}
	if actorID != "" {
		items = h.filterVisibleWorkspaceFolders(r, r.PathValue("workspaceId"), items)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	var folder marketing.WorkspaceFolder
	if err := decodeStrictJSON(r.Body, &folder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(folder.CreatedBy) == "" {
		folder.CreatedBy = actorID
	}
	created, err := h.engine.CreateWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), folder)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	folder, err := h.engine.GetWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	if actorFromRequest(r) != "" && !h.canViewFolder(r, r.PathValue("workspaceId"), folder) {
		workspaceError(w, marketing.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, folder)
}

func (h *Handler) updateWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	var patch marketing.WorkspaceFolderPatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	actorID := actorFromRequest(r)
	if actorID != "" && patch.UpdatedBy == nil {
		patch.UpdatedBy = &actorID
	}
	updated, err := h.engine.PatchWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	deleted, err := h.engine.DeleteWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceShareLinks(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	query := marketing.WorkspaceShareLinkQuery{
		WorkspaceID: r.PathValue("workspaceId"),
		Scope:       marketing.WorkspaceShareScope(strings.TrimSpace(r.URL.Query().Get("scope"))),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:  strings.TrimSpace(r.URL.Query().Get("activityId")),
		EnabledOnly: boolQuery(r, "enabledOnly"),
		Limit:       intQuery(r, "limit"),
	}
	items, err := h.engine.ListWorkspaceShareLinks(r.Context(), query)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	var link marketing.WorkspaceShareLink
	if err := decodeStrictJSON(r.Body, &link); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(link.CreatedBy) == "" {
		link.CreatedBy = actorID
	}
	created, err := h.engine.CreateWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), link)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	link, err := h.engine.GetWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *Handler) updateWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	var patch marketing.WorkspaceShareLinkPatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	actorID := actorFromRequest(r)
	if actorID != "" && patch.UpdatedBy == nil {
		patch.UpdatedBy = &actorID
	}
	updated, err := h.engine.PatchWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	deleted, err := h.engine.DeleteWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionView) {
		return
	}
	query := marketing.WorkspaceAuditLogQuery{
		WorkspaceID: r.PathValue("workspaceId"),
		ActorID:     strings.TrimSpace(r.URL.Query().Get("actorId")),
		Action:      strings.TrimSpace(r.URL.Query().Get("action")),
		TargetType:  strings.TrimSpace(r.URL.Query().Get("targetType")),
		TargetID:    strings.TrimSpace(r.URL.Query().Get("targetId")),
		Limit:       intQuery(r, "limit"),
	}
	query.StartTime = timeQuery(r, "startTime")
	query.EndTime = timeQuery(r, "endTime")
	items, err := h.engine.ListWorkspaceAuditLogs(r.Context(), query)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func workspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketing.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, marketing.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, marketing.ErrWorkspaceConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, marketing.ErrInvalidWorkspace),
		errors.Is(err, marketing.ErrInvalidWorkspaceMember),
		errors.Is(err, marketing.ErrInvalidWorkspaceFolder),
		errors.Is(err, marketing.ErrInvalidWorkspaceShare):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func actorFromRequest(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(
		r.Header.Get("X-Actor-Id"),
		r.Header.Get("X-Workspace-User"),
		r.Header.Get("X-User-Id"),
	))
}

type workspacePermission int

const (
	workspacePermissionView workspacePermission = iota
	workspacePermissionEdit
	workspacePermissionAdmin
	workspacePermissionOwner
)

func (h *Handler) requireWorkspacePermission(w http.ResponseWriter, r *http.Request, workspaceID string, permission workspacePermission) bool {
	allowed, err := h.hasWorkspacePermission(r, workspaceID, permission)
	if err != nil {
		workspaceError(w, err)
		return false
	}
	if !allowed {
		workspaceError(w, marketing.ErrForbidden)
		return false
	}
	return true
}

func (h *Handler) requireActivityWorkspacePermission(w http.ResponseWriter, r *http.Request, activityID string, permission workspacePermission) bool {
	allowed, err := h.hasActivityWorkspacePermission(r, activityID, permission)
	if err != nil {
		workspaceError(w, err)
		return false
	}
	if !allowed {
		workspaceError(w, marketing.ErrForbidden)
		return false
	}
	return true
}

func (h *Handler) hasActivityWorkspacePermission(r *http.Request, activityID string, permission workspacePermission) (bool, error) {
	activity, err := h.engine.GetActivity(r.Context(), activityID)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(activity.WorkspaceID) == "" {
		return true, nil
	}
	allowed, err := h.hasWorkspacePermission(r, activity.WorkspaceID, permission)
	if err != nil || !allowed {
		return allowed, err
	}
	if strings.TrimSpace(activity.FolderID) == "" || actorFromRequest(r) == "" {
		return true, nil
	}
	folder, err := h.engine.GetWorkspaceFolder(r.Context(), activity.WorkspaceID, activity.FolderID)
	if err != nil {
		return false, err
	}
	if !h.canViewFolder(r, activity.WorkspaceID, folder) {
		return false, nil
	}
	return true, nil
}

func (h *Handler) hasWorkspacePermission(r *http.Request, workspaceID string, permission workspacePermission) (bool, error) {
	actorID := actorFromRequest(r)
	if actorID == "" {
		return false, nil
	}
	role, err := h.workspaceActorRole(r, workspaceID, actorID)
	if err != nil {
		return false, err
	}
	return workspaceRoleAllows(role, permission), nil
}

func (h *Handler) workspaceActorRole(r *http.Request, workspaceID string, actorID string) (marketing.WorkspaceRole, error) {
	workspace, err := h.engine.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(workspace.OwnerID) != "" && strings.TrimSpace(workspace.OwnerID) == actorID {
		return marketing.WorkspaceRoleOwner, nil
	}
	if strings.TrimSpace(workspace.OwnerID) == "" && strings.TrimSpace(workspace.CreatedBy) == actorID {
		return marketing.WorkspaceRoleOwner, nil
	}
	members, err := h.engine.ListWorkspaceMembers(r.Context(), workspaceID)
	if err != nil {
		return "", err
	}
	for _, member := range members {
		if member.Status != marketing.WorkspaceMemberActive {
			continue
		}
		if strings.TrimSpace(member.UserID) == actorID || strings.EqualFold(strings.TrimSpace(member.Email), actorID) {
			return member.Role, nil
		}
	}
	if workspace.Visibility == marketing.WorkspacePublic {
		return marketing.WorkspaceRoleViewer, nil
	}
	if workspace.Visibility == marketing.WorkspaceTeam && workspace.DefaultRole == marketing.WorkspaceRoleViewer {
		return marketing.WorkspaceRoleViewer, nil
	}
	return marketing.WorkspaceRoleGuest, nil
}

func workspaceRoleAllows(role marketing.WorkspaceRole, permission workspacePermission) bool {
	rank := map[marketing.WorkspaceRole]int{
		marketing.WorkspaceRoleGuest:  0,
		marketing.WorkspaceRoleViewer: 1,
		marketing.WorkspaceRoleEditor: 2,
		marketing.WorkspaceRoleAdmin:  3,
		marketing.WorkspaceRoleOwner:  4,
	}
	required := map[workspacePermission]int{
		workspacePermissionView:  1,
		workspacePermissionEdit:  2,
		workspacePermissionAdmin: 3,
		workspacePermissionOwner: 4,
	}
	return rank[role] >= required[permission]
}

func (h *Handler) filterVisibleWorkspaceFolders(r *http.Request, workspaceID string, folders []marketing.WorkspaceFolder) []marketing.WorkspaceFolder {
	filtered := folders[:0]
	for _, folder := range folders {
		if h.canViewFolder(r, workspaceID, folder) {
			filtered = append(filtered, folder)
		}
	}
	return filtered
}

func (h *Handler) canViewFolder(r *http.Request, workspaceID string, folder marketing.WorkspaceFolder) bool {
	switch folder.Visibility {
	case marketing.WorkspaceFolderPublic:
		return true
	case marketing.WorkspaceFolderPrivate:
		allowed, _ := h.hasWorkspacePermission(r, workspaceID, workspacePermissionAdmin)
		return allowed
	default:
		allowed, _ := h.hasWorkspacePermission(r, workspaceID, workspacePermissionView)
		return allowed
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func intQuery(r *http.Request, name string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(name)))
	return value
}

func boolQuery(r *http.Request, name string) bool {
	value, _ := strconv.ParseBool(strings.TrimSpace(r.URL.Query().Get(name)))
	return value
}

func timeQuery(r *http.Request, name string) time.Time {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
