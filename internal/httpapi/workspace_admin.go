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
	created, err := h.engine.CreateWorkspace(r.Context(), workspace)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := h.engine.GetWorkspace(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var patch marketing.WorkspacePatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := h.engine.PatchWorkspace(r.Context(), r.PathValue("workspaceId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.engine.DeleteWorkspace(r.Context(), r.PathValue("workspaceId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.ListWorkspaceMembers(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var member marketing.WorkspaceMember
	if err := decodeStrictJSON(r.Body, &member); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	created, err := h.engine.CreateWorkspaceMember(r.Context(), r.PathValue("workspaceId"), member)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	member, err := h.engine.GetWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

func (h *Handler) updateWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var member marketing.WorkspaceMember
	if err := decodeStrictJSON(r.Body, &member); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := h.engine.UpdateWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), member)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.engine.DeleteWorkspaceMember(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceFolders(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	var folder marketing.WorkspaceFolder
	if err := decodeStrictJSON(r.Body, &folder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	created, err := h.engine.CreateWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), folder)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	folder, err := h.engine.GetWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, folder)
}

func (h *Handler) updateWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	var patch marketing.WorkspaceFolderPatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := h.engine.PatchWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.engine.DeleteWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceShareLinks(w http.ResponseWriter, r *http.Request) {
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
	var link marketing.WorkspaceShareLink
	if err := decodeStrictJSON(r.Body, &link); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	created, err := h.engine.CreateWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), link)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	link, err := h.engine.GetWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *Handler) updateWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	var patch marketing.WorkspaceShareLinkPatch
	if err := decodeStrictJSON(r.Body, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := h.engine.PatchWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"), patch)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.engine.DeleteWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *Handler) listWorkspaceAuditLogs(w http.ResponseWriter, r *http.Request) {
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
	return strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("actorId"), r.URL.Query().Get("operator")))
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
