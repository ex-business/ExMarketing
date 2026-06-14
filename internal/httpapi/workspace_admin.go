package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) searchWorkspaceHome(w http.ResponseWriter, r *http.Request) {
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	statusFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	visibilityFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("visibility")))
	if visibilityFilter == "WORKSPACE" {
		visibilityFilter = "TEAM"
	}
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	sortOrder := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("order")))
	offset := intQuery(r, "offset")
	if offset < 0 {
		offset = 0
	}
	limit := intQuery(r, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if response, ok, err := h.engine.SearchWorkspaceHome(r.Context(), marketing.WorkspaceHomeSearchQuery{
		Keyword:    keyword,
		Type:       typeFilter,
		Status:     statusFilter,
		Visibility: visibilityFilter,
		Sort:       sortBy,
		Order:      sortOrder,
		ActorID:    actorFromRequest(r),
		Offset:     offset,
		Limit:      limit,
	}); err != nil {
		workspaceError(w, err)
		return
	} else if ok {
		writeJSON(w, http.StatusOK, response)
		return
	}
	workspaces, err := h.engine.ListWorkspaces(r.Context(), marketing.WorkspaceQuery{Status: marketing.WorkspaceActive, Limit: 200})
	if err != nil {
		workspaceError(w, err)
		return
	}
	activities, err := h.engine.ListActivities(r.Context())
	if err != nil {
		workspaceError(w, err)
		return
	}
	type result map[string]any
	allResults := make([]result, 0, limit)
	add := func(item result) {
		if typeFilter != "" && item["type"] != typeFilter {
			return
		}
		allResults = append(allResults, item)
	}
	match := func(value string) bool {
		if keyword == "" {
			return true
		}
		return strings.Contains(strings.ToLower(value), keyword)
	}
	workspaceNames := map[string]string{}
	for _, workspace := range workspaces {
		workspaceNames[workspace.ID] = workspace.Name
		if visibilityFilter != "" && string(workspace.Visibility) != visibilityFilter {
			continue
		}
		if match(workspace.Name + " " + workspace.Slug) {
			add(result{"id": workspace.ID, "type": "workspace", "title": workspace.Name, "subtitle": workspace.Slug, "workspaceId": workspace.ID, "owner": workspace.OwnerID, "updatedAt": workspace.UpdatedAt, "matchedBy": "workspace"})
		}
	}
	for _, activity := range activities {
		if actorFromRequest(r) != "" && !h.canViewActivity(r, activity) {
			continue
		}
		if statusFilter != "" && string(activity.Status) != statusFilter {
			continue
		}
		if visibilityFilter != "" && string(activity.Visibility) != visibilityFilter {
			continue
		}
		if !match(activity.Name + " " + activity.Code + " " + activity.TenantID) {
			continue
		}
		add(result{
			"id": activity.ID, "type": "workflow", "title": firstNonEmpty(activity.Name, activity.Code),
			"subtitle":    activity.Code + " · " + string(activity.Status) + " · " + string(activity.Visibility),
			"workspaceId": activity.WorkspaceID, "folderId": activity.FolderID, "updatedAt": activity.UpdatedAt, "matchedBy": "workflow",
		})
	}
	for _, workspace := range workspaces {
		folders, folderErr := h.engine.ListWorkspaceFolders(r.Context(), marketing.WorkspaceFolderQuery{WorkspaceID: workspace.ID, Status: marketing.WorkspaceFolderActive, Limit: 200})
		if folderErr != nil {
			continue
		}
		for _, folder := range folders {
			if visibilityFilter != "" && string(folder.Visibility) != visibilityFilter {
				continue
			}
			if match(folder.Name) {
				add(result{"id": folder.ID, "type": "folder", "title": folder.Name, "subtitle": workspaceNames[folder.WorkspaceID] + " · " + string(folder.Visibility), "workspaceId": folder.WorkspaceID, "folderId": folder.ID, "owner": folder.UpdatedBy, "updatedAt": folder.UpdatedAt, "matchedBy": "folder"})
			}
		}
		members, memberErr := h.engine.ListWorkspaceMembers(r.Context(), workspace.ID)
		if memberErr != nil {
			continue
		}
		for _, member := range members {
			haystack := strings.ToLower(member.DisplayName + " " + member.UserID + " " + member.Email)
			if keyword == "" || strings.Contains(haystack, keyword) {
				add(result{"id": member.ID, "type": "member", "title": firstNonEmpty(member.DisplayName, member.UserID, member.Email), "subtitle": string(member.Role) + " · " + workspace.Name, "workspaceId": workspace.ID, "owner": member.UserID, "updatedAt": member.UpdatedAt, "matchedBy": "member"})
			}
		}
	}
	sort.SliceStable(allResults, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(anyString(allResults[i][sortBy])))
		right := strings.ToLower(strings.TrimSpace(anyString(allResults[j][sortBy])))
		if sortBy == "updated" || sortBy == "updatedat" {
			left = strings.TrimSpace(anyString(allResults[i]["updatedAt"]))
			right = strings.TrimSpace(anyString(allResults[j]["updatedAt"]))
		}
		if left == "" && right == "" {
			left = strings.ToLower(strings.TrimSpace(anyString(allResults[i]["title"])))
			right = strings.ToLower(strings.TrimSpace(anyString(allResults[j]["title"])))
		}
		if sortOrder == "asc" {
			return left < right
		}
		return left > right
	})
	total := len(allResults)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": allResults[offset:end], "total": total, "offset": offset, "limit": limit})
}

func (h *Handler) getWorkspaceHomeStorage(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.engine.ListWorkspaces(r.Context(), marketing.WorkspaceQuery{Status: marketing.WorkspaceActive, Limit: 500})
	if err != nil {
		workspaceError(w, err)
		return
	}
	activities, err := h.engine.ListActivities(r.Context())
	if err != nil {
		workspaceError(w, err)
		return
	}
	folderCount := 0
	memberCount := 0
	shareCount := 0
	auditCount := 0
	draftCount := 0
	versionCount := 0
	executionCount := 0
	nodeExecutionCount := 0
	grantCount := 0
	fileObjectCount := 0
	fileBytes := int64(0)
	usedBytes := int64(0)
	addSize := func(value any) {
		payload, _ := json.Marshal(value)
		usedBytes += int64(len(payload))
	}
	for _, workspace := range workspaces {
		addSize(workspace)
	}
	for _, activity := range activities {
		addSize(activity)
		if draft, draftErr := h.engine.GetWorkflowDraft(r.Context(), activity.ID); draftErr == nil {
			draftCount++
			addSize(draft)
		}
		if versions, versionsErr := h.engine.ListWorkflowVersions(r.Context(), activity.ID); versionsErr == nil {
			for _, version := range versions {
				versionCount++
				addSize(version)
			}
		}
		if executions, executionsErr := h.engine.ListWorkflowExecutions(r.Context(), marketing.WorkflowExecutionQuery{ActivityID: activity.ID, Limit: 1000}); executionsErr == nil {
			for _, execution := range executions {
				executionCount++
				addSize(execution)
				if nodeExecutions, nodeErr := h.engine.ListWorkflowNodeExecutionsPage(r.Context(), marketing.WorkflowNodeExecutionQuery{ExecutionID: execution.ID, Limit: 1000}); nodeErr == nil {
					for _, nodeExecution := range nodeExecutions {
						nodeExecutionCount++
						addSize(nodeExecution)
					}
				}
			}
		}
	}
	for _, workspace := range workspaces {
		folders, _ := h.engine.ListWorkspaceFolders(r.Context(), marketing.WorkspaceFolderQuery{WorkspaceID: workspace.ID, Status: marketing.WorkspaceFolderActive, Limit: 1000})
		members, _ := h.engine.ListWorkspaceMembers(r.Context(), workspace.ID)
		shares, _ := h.engine.ListWorkspaceShareLinks(r.Context(), marketing.WorkspaceShareLinkQuery{WorkspaceID: workspace.ID, Limit: 1000})
		audits, _ := h.engine.ListWorkspaceAuditLogs(r.Context(), marketing.WorkspaceAuditLogQuery{WorkspaceID: workspace.ID, Limit: 1000})
		grants, _ := h.engine.ListWorkspaceActivityGrants(r.Context(), workspace.ID, "")
		files, _ := h.engine.ListWorkspaceFileObjects(r.Context(), marketing.WorkspaceFileObjectQuery{WorkspaceID: workspace.ID, Limit: 1000})
		folderCount += len(folders)
		memberCount += len(members)
		shareCount += len(shares)
		auditCount += len(audits)
		grantCount += len(grants)
		fileObjectCount += len(files)
		for _, folder := range folders {
			addSize(folder)
		}
		for _, member := range members {
			addSize(member)
		}
		for _, share := range shares {
			addSize(share)
		}
		for _, audit := range audits {
			addSize(audit)
		}
		for _, grant := range grants {
			addSize(grant)
		}
		for _, file := range files {
			fileBytes += file.SizeBytes
			addSize(file)
		}
	}
	workflowCount := len(activities)
	usedBytes += int64(len(workspaces)+folderCount+workflowCount+memberCount+shareCount+auditCount+draftCount+versionCount+executionCount+nodeExecutionCount+grantCount) * 4 * 1024
	usedBytes += fileBytes
	totalBytes := int64(100 * 1024 * 1024 * 1024)
	writeJSON(w, http.StatusOK, map[string]any{
		"usedBytes":      usedBytes,
		"totalBytes":     totalBytes,
		"workspaces":     len(workspaces),
		"folders":        folderCount,
		"workflows":      workflowCount,
		"members":        memberCount,
		"shareLinks":     shareCount,
		"auditLogs":      auditCount,
		"drafts":         draftCount,
		"versions":       versionCount,
		"executions":     executionCount,
		"nodeExecutions": nodeExecutionCount,
		"activityGrants": grantCount,
		"fileObjects":    fileObjectCount,
		"fileBytes":      fileBytes,
	})
}

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
	if created.Status == marketing.WorkspaceMemberInvited {
		writeJSON(w, http.StatusCreated, h.deliverWorkspaceInvite(r, created))
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

func (h *Handler) resendWorkspaceMemberInvite(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	member, err := h.engine.ResendWorkspaceMemberInvite(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.deliverWorkspaceInvite(r, member))
}

func (h *Handler) acceptWorkspaceMemberInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token  string `json:"token"`
		UserID string `json:"userId"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeStrictJSON(r.Body, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	member, err := h.engine.AcceptWorkspaceMemberInvite(r.Context(), r.PathValue("workspaceId"), r.PathValue("memberId"), body.Token, body.UserID)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

type workspaceMemberInviteResponse struct {
	marketing.WorkspaceMember
	InviteURL     string              `json:"inviteUrl"`
	EmailDelivery inviteEmailDelivery `json:"emailDelivery"`
}

func (h *Handler) deliverWorkspaceInvite(r *http.Request, member marketing.WorkspaceMember) workspaceMemberInviteResponse {
	inviteURL := h.workspaceInviteURL(r, member.InviteToken)
	delivery := inviteEmailDelivery{Status: "skipped", Provider: "none", Recipient: member.Email, Message: "member is not invited"}
	workspaceName := ""
	if member.Status == marketing.WorkspaceMemberInvited {
		workspace, err := h.engine.GetWorkspace(r.Context(), member.WorkspaceID)
		if err != nil {
			delivery = inviteEmailDelivery{Status: "failed", Provider: "workspace", Recipient: member.Email, Message: err.Error()}
		} else {
			workspaceName = workspace.Name
			delivery = h.inviteEmailSender.SendWorkspaceInvite(r.Context(), inviteEmailMessage{
				Workspace: workspace,
				Member:    member,
				InviteURL: inviteURL,
			})
		}
	}
	delivery.Message = marketing.RedactSensitiveString(delivery.Message)
	_ = h.engine.RecordWorkspaceAudit(r.Context(), member.WorkspaceID, actorFromRequest(r), "SEND_MEMBER_INVITE_EMAIL", "MEMBER", member.ID, nil, map[string]any{
		"workspaceName": workspaceName,
		"recipient":     member.Email,
		"provider":      delivery.Provider,
		"status":        delivery.Status,
		"attempts":      delivery.Attempts,
		"message":       delivery.Message,
	})
	return workspaceMemberInviteResponse{WorkspaceMember: member, InviteURL: inviteURL, EmailDelivery: delivery}
}

func (h *Handler) workspaceInviteURL(r *http.Request, token string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(h.inviteEmailCfg.BaseURL), "/")
	if baseURL == "" && r != nil {
		scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		if scheme == "" {
			scheme = "http"
			if r.TLS != nil {
				scheme = "https"
			}
		}
		host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
		if host == "" {
			host = r.Host
		}
		if host != "" {
			baseURL = scheme + "://" + host
		}
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	return baseURL + "/invite/" + token
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

func (h *Handler) restoreWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	restored, err := h.engine.RestoreWorkspaceFolder(r.Context(), r.PathValue("workspaceId"), r.PathValue("folderId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, restored)
}

func (h *Handler) batchWorkspaceHome(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionAdmin) {
		return
	}
	var request marketing.WorkspaceBatchCommand
	if err := decodeStrictJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	request.WorkspaceID = r.PathValue("workspaceId")
	request.ActorID = actorFromRequest(r)
	if len(request.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}
	result, err := h.engine.ApplyWorkspaceBatch(r.Context(), request)
	if err != nil {
		if result.Items != nil {
			writeJSON(w, http.StatusBadRequest, result)
			return
		}
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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

func (h *Handler) regenerateWorkspaceShareLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorkspacePermission(w, r, r.PathValue("workspaceId"), workspacePermissionOwner) {
		return
	}
	updated, err := h.engine.RegenerateWorkspaceShareLink(r.Context(), r.PathValue("workspaceId"), r.PathValue("linkId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) resolveShareLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if r.Method == http.MethodPost && r.Body != nil && r.ContentLength != 0 {
		if err := decodeStrictJSON(r.Body, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	link, target, err := h.engine.ResolveWorkspaceShareLink(r.Context(), r.PathValue("token"), body.Password, actorFromRequest(r))
	if err != nil {
		h.shareResolveError(w, r, r.PathValue("token"), body.Password, err)
		return
	}
	payload := h.shareLinkPayload(r, link, target)
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) shareResolveError(w http.ResponseWriter, r *http.Request, token string, password string, err error) {
	if errors.Is(err, marketing.ErrNotFound) {
		writeErrorCode(w, http.StatusNotFound, "SHARE_NOT_FOUND", "分享链接不存在")
		return
	}
	if errors.Is(err, marketing.ErrForbidden) {
		links, listErr := h.engine.ListWorkspaceShareLinks(r.Context(), marketing.WorkspaceShareLinkQuery{Token: strings.TrimSpace(token), Limit: 1})
		if listErr == nil && len(links) > 0 {
			link := links[0]
			now := time.Now()
			switch {
			case !link.Enabled:
				message := "分享链接已停用"
				if strings.TrimSpace(link.DisabledReason) != "" {
					message += "：" + strings.TrimSpace(link.DisabledReason)
				}
				writeErrorCode(w, http.StatusForbidden, "SHARE_DISABLED", message)
				return
			case link.ExpiresAt != nil && now.After(*link.ExpiresAt):
				writeErrorCode(w, http.StatusGone, "SHARE_EXPIRED", "分享链接已过期")
				return
			case link.MaxAccessCount > 0 && link.AccessCount >= link.MaxAccessCount:
				writeErrorCode(w, http.StatusGone, "SHARE_ACCESS_LIMIT", "分享链接访问次数已用完")
				return
			case strings.TrimSpace(link.PasswordHash) != "":
				if strings.TrimSpace(password) == "" {
					writeErrorCode(w, http.StatusUnauthorized, "SHARE_PASSWORD_REQUIRED", "该分享链接需要访问密码")
					return
				}
				writeErrorCode(w, http.StatusUnauthorized, "SHARE_PASSWORD_INVALID", "密码错误，请重新输入")
				return
			}
		}
		writeErrorCode(w, http.StatusForbidden, "SHARE_FORBIDDEN", "当前访问无权限")
		return
	}
	workspaceError(w, err)
}

func (h *Handler) shareLinkPayload(r *http.Request, link marketing.WorkspaceShareLink, target any) map[string]any {
	items := make([]map[string]any, 0)
	stats := map[string]any{"workflows": 0, "folders": 0, "members": 0}
	activities, _ := h.engine.ListActivities(r.Context())
	switch link.Scope {
	case marketing.WorkspaceShareWorkspace:
		folders, _ := h.engine.ListWorkspaceFolders(r.Context(), marketing.WorkspaceFolderQuery{WorkspaceID: link.WorkspaceID, Status: marketing.WorkspaceFolderActive, Limit: 500})
		members, _ := h.engine.ListWorkspaceMembers(r.Context(), link.WorkspaceID)
		workflowCount := 0
		for _, activity := range activities {
			if activity.WorkspaceID != link.WorkspaceID || activity.Status == marketing.ActivityArchived {
				continue
			}
			workflowCount++
			items = append(items, map[string]any{"id": activity.ID, "name": firstNonEmpty(activity.Name, activity.Code), "type": "Workflow", "status": activity.Status, "visibility": activity.Visibility})
		}
		for _, folder := range folders {
			items = append(items, map[string]any{"id": folder.ID, "name": folder.Name, "type": "Folder", "status": folder.Status, "visibility": folder.Visibility})
		}
		stats["workflows"] = workflowCount
		stats["folders"] = len(folders)
		stats["members"] = len(members)
	case marketing.WorkspaceShareFolder:
		folders, _ := h.engine.ListWorkspaceFolders(r.Context(), marketing.WorkspaceFolderQuery{WorkspaceID: link.WorkspaceID, ParentID: link.FolderID, Status: marketing.WorkspaceFolderActive, Limit: 500})
		workflowCount := 0
		for _, activity := range activities {
			if activity.WorkspaceID != link.WorkspaceID || activity.FolderID != link.FolderID || activity.Status == marketing.ActivityArchived {
				continue
			}
			workflowCount++
			items = append(items, map[string]any{"id": activity.ID, "name": firstNonEmpty(activity.Name, activity.Code), "type": "Workflow", "status": activity.Status, "visibility": activity.Visibility})
		}
		for _, folder := range folders {
			items = append(items, map[string]any{"id": folder.ID, "name": folder.Name, "type": "Folder", "status": folder.Status, "visibility": folder.Visibility})
		}
		members, _ := h.engine.ListWorkspaceMembers(r.Context(), link.WorkspaceID)
		stats["workflows"] = workflowCount
		stats["folders"] = len(folders)
		stats["members"] = len(members)
	case marketing.WorkspaceShareActivity:
		stats["workflows"] = 1
	}
	audits, _ := h.engine.ListWorkspaceAuditLogs(r.Context(), marketing.WorkspaceAuditLogQuery{
		WorkspaceID: link.WorkspaceID,
		Action:      "ACCESS_SHARE_LINK",
		TargetType:  "SHARE_LINK",
		TargetID:    link.ID,
		Limit:       8,
	})
	return map[string]any{"link": shareLinkPublicPayload(link), "target": target, "items": items, "stats": stats, "audits": shareAuditPublicPayload(audits)}
}

func shareLinkPublicPayload(link marketing.WorkspaceShareLink) map[string]any {
	return map[string]any{
		"id":             link.ID,
		"workspaceId":    link.WorkspaceID,
		"folderId":       link.FolderID,
		"activityId":     link.ActivityID,
		"scope":          link.Scope,
		"permission":     link.Permission,
		"expiresAt":      link.ExpiresAt,
		"maxAccessCount": link.MaxAccessCount,
		"accessCount":    link.AccessCount,
		"lastAccessedAt": link.LastAccessedAt,
		"enabled":        link.Enabled,
		"createdAt":      link.CreatedAt,
		"updatedAt":      link.UpdatedAt,
	}
}

func shareAuditPublicPayload(audits []marketing.WorkspaceAuditLog) []map[string]any {
	items := make([]map[string]any, 0, len(audits))
	for _, audit := range audits {
		actorLabel := "Anonymous visitor"
		if strings.TrimSpace(audit.ActorID) != "" {
			actorLabel = "Authenticated visitor"
		}
		items = append(items, map[string]any{
			"createdAt":  audit.CreatedAt,
			"actorLabel": actorLabel,
			"source":     "Share page",
		})
	}
	return items
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

func (h *Handler) listActivityACL(w http.ResponseWriter, r *http.Request) {
	activity, err := h.engine.GetActivity(r.Context(), r.PathValue("id"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	if !h.requireActivityWorkspacePermission(w, r, activity.ID, workspacePermissionAdmin) {
		return
	}
	items, err := h.engine.ListWorkspaceActivityGrants(r.Context(), activity.WorkspaceID, activity.ID)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createActivityACL(w http.ResponseWriter, r *http.Request) {
	activity, err := h.engine.GetActivity(r.Context(), r.PathValue("id"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	if !h.requireActivityWorkspacePermission(w, r, activity.ID, workspacePermissionAdmin) {
		return
	}
	var grant marketing.WorkspaceActivityGrant
	if err := decodeStrictJSON(r.Body, &grant); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(grant.CreatedBy) == "" {
		grant.CreatedBy = actorID
		grant.UpdatedBy = actorID
	}
	created, err := h.engine.CreateWorkspaceActivityGrant(r.Context(), activity.WorkspaceID, activity.ID, grant)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) deleteActivityACL(w http.ResponseWriter, r *http.Request) {
	activity, err := h.engine.GetActivity(r.Context(), r.PathValue("id"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	if !h.requireActivityWorkspacePermission(w, r, activity.ID, workspacePermissionAdmin) {
		return
	}
	if err := h.engine.DeleteWorkspaceActivityGrant(r.Context(), activity.WorkspaceID, r.PathValue("grantId"), actorFromRequest(r)); err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("grantId")})
}

func workspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketing.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, marketing.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, marketing.ErrWorkspaceConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, marketing.ErrResourceConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, marketing.ErrInvalidWorkspace),
		errors.Is(err, marketing.ErrInvalidWorkspaceMember),
		errors.Is(err, marketing.ErrInvalidWorkspaceFolder),
		errors.Is(err, marketing.ErrInvalidWorkspaceShare),
		errors.Is(err, marketing.ErrInvalidResource):
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
	if permission == workspacePermissionView && !h.canViewActivity(r, activity) {
		return false, nil
	}
	if strings.TrimSpace(activity.WorkspaceID) == "" {
		return true, nil
	}
	allowed, err := h.hasWorkspacePermission(r, activity.WorkspaceID, permission)
	if err != nil || !allowed {
		if err != nil {
			return false, err
		}
		if h.hasActivityGrantPermission(r, activity, permission) {
			allowed = true
		}
		if !allowed {
			return false, nil
		}
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

func (h *Handler) hasActivityGrantPermission(r *http.Request, activity marketing.Activity, permission workspacePermission) bool {
	actorID := actorFromRequest(r)
	if actorID == "" || strings.TrimSpace(activity.WorkspaceID) == "" {
		return false
	}
	grants, err := h.engine.ListWorkspaceActivityGrants(r.Context(), activity.WorkspaceID, activity.ID)
	if err != nil {
		return false
	}
	workspaceRole, _ := h.workspaceActorRole(r, activity.WorkspaceID, actorID)
	members, _ := h.engine.ListWorkspaceMembers(r.Context(), activity.WorkspaceID)
	memberIDs := map[string]struct{}{}
	for _, member := range members {
		if member.Status != marketing.WorkspaceMemberActive {
			continue
		}
		if strings.TrimSpace(member.UserID) == actorID || strings.EqualFold(strings.TrimSpace(member.Email), actorID) {
			memberIDs[member.ID] = struct{}{}
		}
	}
	for _, grant := range grants {
		if !workspaceRoleAllows(grant.Role, permission) {
			continue
		}
		switch grant.PrincipalType {
		case marketing.WorkspacePrincipalUser:
			if strings.TrimSpace(grant.PrincipalID) == actorID {
				return true
			}
		case marketing.WorkspacePrincipalMember:
			if _, ok := memberIDs[grant.PrincipalID]; ok {
				return true
			}
		case marketing.WorkspacePrincipalRole:
			if strings.EqualFold(string(grant.PrincipalID), string(workspaceRole)) {
				return true
			}
		}
	}
	return false
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

func (h *Handler) filterVisibleActivities(r *http.Request, activities []marketing.Activity) []marketing.Activity {
	filtered := activities[:0]
	for _, activity := range activities {
		if h.canViewActivity(r, activity) {
			filtered = append(filtered, activity)
		}
	}
	return filtered
}

func (h *Handler) canViewActivity(r *http.Request, activity marketing.Activity) bool {
	if strings.TrimSpace(activity.WorkspaceID) == "" {
		return true
	}
	switch activity.Visibility {
	case marketing.WorkspacePublic:
		return true
	case marketing.WorkspacePrivate:
		allowed, _ := h.hasWorkspacePermission(r, activity.WorkspaceID, workspacePermissionAdmin)
		return allowed || h.hasActivityGrantPermission(r, activity, workspacePermissionView)
	default:
		allowed, _ := h.hasWorkspacePermission(r, activity.WorkspaceID, workspacePermissionView)
		if !allowed && !h.hasActivityGrantPermission(r, activity, workspacePermissionView) {
			return false
		}
		if strings.TrimSpace(activity.FolderID) == "" {
			return true
		}
		folder, err := h.engine.GetWorkspaceFolder(r.Context(), activity.WorkspaceID, activity.FolderID)
		if err != nil {
			return false
		}
		return h.canViewFolder(r, activity.WorkspaceID, folder)
	}
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

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
