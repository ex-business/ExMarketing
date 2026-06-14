package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) listResources(w http.ResponseWriter, r *http.Request) {
	resources, err := h.engine.ListResources(r.Context(), marketing.ResourceQuery{
		TenantID:    tenantFromRequest(r),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:  strings.TrimSpace(r.URL.Query().Get("activityId")),
		Type:        marketing.ResourceType(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))),
		Status:      marketing.ResourceStatus(strings.ToUpper(firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), string(marketing.ResourceActive)))),
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Limit:       intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resources})
}

func (h *Handler) getResource(w http.ResponseWriter, r *http.Request) {
	resource, err := h.engine.GetResource(r.Context(), tenantFromRequest(r), r.PathValue("code"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (h *Handler) saveResource(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var resource marketing.Resource
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		writeError(w, http.StatusBadRequest, "invalid resource payload")
		return
	}
	if code := strings.TrimSpace(r.PathValue("code")); code != "" {
		resource.Code = code
	}
	if resource.TenantID == "" {
		resource.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if resource.CreatedBy == "" {
			resource.CreatedBy = actor
		}
		resource.UpdatedBy = actor
	}
	saved, err := h.engine.SaveResource(r.Context(), resource)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) listAvailableResources(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.ListAvailableResources(r.Context(), marketing.ResourceAccessRequest{
		TenantID:    tenantFromRequest(r),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:  strings.TrimSpace(r.URL.Query().Get("activityId")),
		NodeID:      strings.TrimSpace(r.URL.Query().Get("nodeId")),
		NodeType:    strings.TrimSpace(r.URL.Query().Get("nodeType")),
		Operation:   marketing.ResourceOperation(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("operation")))),
		Namespace:   strings.TrimSpace(r.URL.Query().Get("namespace")),
		ActorID:     actorFromRequest(r),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) testResource(w http.ResponseWriter, r *http.Request) {
	result, err := h.engine.TestResource(r.Context(), tenantFromRequest(r), r.PathValue("code"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listResourceBindings(w http.ResponseWriter, r *http.Request) {
	resource, err := h.engine.GetResource(r.Context(), tenantFromRequest(r), r.PathValue("code"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	bindings, err := h.engine.ListResourceBindings(r.Context(), marketing.ResourceBindingQuery{
		ResourceID: resource.ID,
		TenantID:   tenantFromRequest(r),
		Status:     marketing.ResourceBindingStatus(strings.ToUpper(firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), string(marketing.ResourceBindingActive)))),
		Limit:      intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": bindings})
}

func (h *Handler) saveResourceBinding(w http.ResponseWriter, r *http.Request) {
	resource, err := h.engine.GetResource(r.Context(), tenantFromRequest(r), r.PathValue("code"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	defer r.Body.Close()
	var binding marketing.ResourceBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, http.StatusBadRequest, "invalid resource binding payload")
		return
	}
	binding.ResourceID = resource.ID
	if binding.TenantID == "" {
		binding.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if binding.CreatedBy == "" {
			binding.CreatedBy = actor
		}
		binding.UpdatedBy = actor
	}
	saved, err := h.engine.SaveResourceBinding(r.Context(), binding)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) listResourceAuditLogs(w http.ResponseWriter, r *http.Request) {
	resource, err := h.engine.GetResource(r.Context(), tenantFromRequest(r), r.PathValue("code"))
	if err != nil {
		workspaceError(w, err)
		return
	}
	logs, err := h.engine.ListResourceAuditLogs(r.Context(), marketing.ResourceAuditQuery{
		ResourceID: resource.ID,
		TenantID:   tenantFromRequest(r),
		Status:     marketing.ResourceAuditStatus(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))),
		Limit:      intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs})
}

func tenantFromRequest(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("tenantId"), r.Header.Get("X-Tenant-Id"), "site_001"))
}
