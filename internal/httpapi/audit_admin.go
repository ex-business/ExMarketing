package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) listProviderDecisionAuditLogs(w http.ResponseWriter, r *http.Request) {
	activityID := strings.TrimSpace(r.URL.Query().Get("activityId"))
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if activityID != "" {
		if !h.requireActivityWorkspacePermission(w, r, activityID, workspacePermissionView) {
			return
		}
	} else if workspaceID != "" {
		if !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionView) {
			return
		}
	} else if !isPlatformAdminRequest(r) {
		writeError(w, http.StatusBadRequest, "activityId or workspaceId is required")
		return
	}
	items, err := h.engine.ListProviderDecisionAuditLogs(r.Context(), marketing.ProviderDecisionAuditQuery{
		TenantID:     tenantFromRequest(r),
		WorkspaceID:  workspaceID,
		FolderID:     strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:   activityID,
		NodeID:       strings.TrimSpace(r.URL.Query().Get("nodeId")),
		ServiceRef:   strings.TrimSpace(r.URL.Query().Get("serviceRef")),
		InterfaceRef: strings.TrimSpace(r.URL.Query().Get("interfaceRef")),
		ProviderRef:  strings.TrimSpace(r.URL.Query().Get("providerRef")),
		Decision:     marketing.ProviderDecision(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("decision")))),
		Cursor:       strings.TrimSpace(r.URL.Query().Get("cursor")),
		Limit:        intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": providerDecisionAuditNextCursor(items),
		"limit":      intQuery(r, "limit"),
		"requestId":  r.Header.Get("X-Request-Id"),
	})
}

func (h *Handler) listMigrationAuditLogs(w http.ResponseWriter, r *http.Request) {
	activityID := strings.TrimSpace(r.URL.Query().Get("activityId"))
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if activityID != "" {
		if !h.requireActivityWorkspacePermission(w, r, activityID, workspacePermissionView) {
			return
		}
	} else if workspaceID != "" {
		if !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionView) {
			return
		}
	} else if !h.requireMigrationAdmin(w, r) {
		return
	}
	items, err := h.engine.ListMigrationAuditLogs(r.Context(), marketing.MigrationAuditQuery{
		TenantID:      tenantFromRequest(r),
		WorkspaceID:   workspaceID,
		FolderID:      strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:    activityID,
		MigrationType: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("migrationType"))),
		Operation:     strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("operation"))),
		Actor:         strings.TrimSpace(r.URL.Query().Get("actor")),
		Result:        marketing.MigrationAuditResult(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("result")))),
		Cursor:        strings.TrimSpace(r.URL.Query().Get("cursor")),
		Limit:         intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": migrationAuditNextCursor(items),
		"limit":      intQuery(r, "limit"),
		"requestId":  r.Header.Get("X-Request-Id"),
	})
}

func providerDecisionAuditNextCursor(items []marketing.ProviderDecisionAuditLog) string {
	if len(items) == 0 {
		return ""
	}
	last := items[len(items)-1]
	if last.ID == "" {
		return ""
	}
	return last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
}

func migrationAuditNextCursor(items []marketing.MigrationAuditLog) string {
	if len(items) == 0 {
		return ""
	}
	last := items[len(items)-1]
	if last.ID == "" {
		return ""
	}
	return last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
}
