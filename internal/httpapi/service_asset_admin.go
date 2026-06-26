package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) listServiceDefinitions(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	services, err := h.engine.ListServiceDefinitions(r.Context(), marketing.ServiceDefinitionQuery{
		TenantID:       tenantFromRequest(r),
		WorkspaceID:    strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:       strings.TrimSpace(r.URL.Query().Get("folderId")),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		Code:           strings.TrimSpace(r.URL.Query().Get("code")),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": services})
}

func (h *Handler) saveServiceDefinition(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var service marketing.ServiceDefinition
	if err := json.NewDecoder(r.Body).Decode(&service); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service payload")
		return
	}
	if code := strings.TrimSpace(r.PathValue("serviceCode")); code != "" {
		service.Code = code
	}
	if service.TenantID == "" {
		service.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if service.CreatedBy == "" {
			service.CreatedBy = actor
		}
		service.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceDefinition(r.Context(), service)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) listServiceInterfaces(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	items, err := h.engine.ListServiceInterfaces(r.Context(), marketing.ServiceInterfaceQuery{
		TenantID:       tenantFromRequest(r),
		ServiceRef:     strings.TrimSpace(r.PathValue("serviceCode")),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		Code:           strings.TrimSpace(r.URL.Query().Get("code")),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveServiceInterface(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var serviceInterface marketing.ServiceInterface
	if err := json.NewDecoder(r.Body).Decode(&serviceInterface); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service interface payload")
		return
	}
	serviceInterface.ServiceRef = strings.TrimSpace(r.PathValue("serviceCode"))
	if code := strings.TrimSpace(r.PathValue("interfaceCode")); code != "" {
		serviceInterface.Code = code
	}
	if serviceInterface.TenantID == "" {
		serviceInterface.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if serviceInterface.CreatedBy == "" {
			serviceInterface.CreatedBy = actor
		}
		serviceInterface.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceInterface(r.Context(), serviceInterface)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) listServiceProviders(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	items, err := h.engine.ListServiceProviders(r.Context(), marketing.ServiceProviderQuery{
		TenantID:       tenantFromRequest(r),
		ServiceRef:     strings.TrimSpace(r.PathValue("serviceCode")),
		InterfaceRef:   strings.TrimSpace(r.PathValue("interfaceCode")),
		Protocol:       marketing.ServiceProviderProtocol(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("protocol")))),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		Code:           strings.TrimSpace(r.URL.Query().Get("code")),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveServiceProvider(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var provider marketing.ServiceProvider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service provider payload")
		return
	}
	provider.ServiceRef = strings.TrimSpace(r.PathValue("serviceCode"))
	provider.InterfaceRef = strings.TrimSpace(r.PathValue("interfaceCode"))
	if code := strings.TrimSpace(r.PathValue("providerCode")); code != "" {
		provider.Code = code
	}
	if provider.TenantID == "" {
		provider.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if provider.CreatedBy == "" {
			provider.CreatedBy = actor
		}
		provider.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceProvider(r.Context(), provider)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) getServiceProviderRuntimeState(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.ListServiceProviderRuntimeStates(r.Context(), marketing.ServiceProviderRuntimeStateQuery{
		TenantID:     tenantFromRequest(r),
		ServiceRef:   strings.TrimSpace(r.PathValue("serviceCode")),
		InterfaceRef: strings.TrimSpace(r.PathValue("interfaceCode")),
		ProviderRef:  strings.TrimSpace(r.PathValue("providerCode")),
		Limit:        1,
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "service provider runtime state not found")
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}

func (h *Handler) saveServiceProviderRuntimeState(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var state marketing.ServiceProviderRuntimeState
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service provider runtime state payload")
		return
	}
	state.TenantID = firstNonEmpty(state.TenantID, tenantFromRequest(r))
	state.ServiceRef = strings.TrimSpace(r.PathValue("serviceCode"))
	state.InterfaceRef = strings.TrimSpace(r.PathValue("interfaceCode"))
	state.ProviderRef = strings.TrimSpace(r.PathValue("providerCode"))
	saved, err := h.engine.SaveServiceProviderRuntimeState(r.Context(), state)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) resetServiceProviderCircuit(w http.ResponseWriter, r *http.Request) {
	saved, err := h.engine.ResetServiceProviderCircuit(
		r.Context(),
		tenantFromRequest(r),
		strings.TrimSpace(r.PathValue("serviceCode")),
		strings.TrimSpace(r.PathValue("interfaceCode")),
		strings.TrimSpace(r.PathValue("providerCode")),
	)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) checkServiceProviderHealth(w http.ResponseWriter, r *http.Request) {
	saved, err := h.engine.CheckServiceProviderHealth(
		r.Context(),
		tenantFromRequest(r),
		strings.TrimSpace(r.PathValue("serviceCode")),
		strings.TrimSpace(r.PathValue("interfaceCode")),
		strings.TrimSpace(r.PathValue("providerCode")),
		actorFromRequest(r),
	)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) listServiceRateLimitPolicies(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	items, err := h.engine.ListServiceRateLimitPolicies(r.Context(), marketing.ServiceRateLimitPolicyQuery{
		TenantID:       tenantFromRequest(r),
		Code:           strings.TrimSpace(r.URL.Query().Get("code")),
		ServiceRef:     strings.TrimSpace(r.URL.Query().Get("serviceRef")),
		InterfaceRef:   strings.TrimSpace(r.URL.Query().Get("interfaceRef")),
		ProviderRef:    strings.TrimSpace(r.URL.Query().Get("providerRef")),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveServiceRateLimitPolicy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var policy marketing.ServiceRateLimitPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service rate limit policy payload")
		return
	}
	if policy.TenantID == "" {
		policy.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if policy.CreatedBy == "" {
			policy.CreatedBy = actor
		}
		policy.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceRateLimitPolicy(r.Context(), policy)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) testServiceInterface(w http.ResponseWriter, r *http.Request) {
	_, provider, err := h.engine.ResolveServiceProvider(
		r.Context(),
		tenantFromRequest(r),
		strings.TrimSpace(r.PathValue("serviceCode")),
		strings.TrimSpace(r.PathValue("interfaceCode")),
		strings.TrimSpace(r.URL.Query().Get("providerCode")),
		strings.TrimSpace(r.URL.Query().Get("providerPolicy")),
	)
	if err != nil {
		workspaceError(w, err)
		return
	}
	resourceRef := ""
	if len(provider.ResourceRefs) > 0 {
		resourceRef = provider.ResourceRefs[0]
	}
	if resourceRef == "" {
		var config map[string]any
		_ = json.Unmarshal(provider.Config, &config)
		if value, ok := config["targetResourceRef"].(string); ok {
			resourceRef = value
		}
	}
	if resourceRef == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider, "message": "provider has no resource dry-run target"})
		return
	}
	result, err := h.engine.TestResource(r.Context(), tenantFromRequest(r), resourceRef, actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	result["provider"] = provider
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listServiceSchemas(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	items, err := h.engine.ListServiceSchemas(r.Context(), marketing.ServiceSchemaQuery{
		TenantID:       tenantFromRequest(r),
		WorkspaceID:    strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:       strings.TrimSpace(r.URL.Query().Get("folderId")),
		SchemaRef:      strings.TrimSpace(r.URL.Query().Get("schemaRef")),
		SchemaType:     marketing.ServiceSchemaType(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("schemaType")))),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveServiceSchema(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var schema marketing.ServiceSchema
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service schema payload")
		return
	}
	if schema.TenantID == "" {
		schema.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if schema.CreatedBy == "" {
			schema.CreatedBy = actor
		}
		schema.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceSchema(r.Context(), schema)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) validateServiceSchema(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var schema marketing.ServiceSchema
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service schema payload")
		return
	}
	if schema.TenantID == "" {
		schema.TenantID = tenantFromRequest(r)
	}
	writeJSON(w, http.StatusOK, h.engine.ValidateServiceSchemaDocument(schema))
}

func (h *Handler) checkServiceSchemaCompatibility(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var schema marketing.ServiceSchema
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service schema payload")
		return
	}
	if schema.TenantID == "" {
		schema.TenantID = tenantFromRequest(r)
	}
	writeJSON(w, http.StatusOK, h.engine.CheckServiceSchemaCompatibility(r.Context(), schema))
}

func (h *Handler) listServiceInterfaceBindings(w http.ResponseWriter, r *http.Request) {
	includeDeleted := boolQuery(r, "includeDeleted")
	items, err := h.engine.ListServiceInterfaceBindings(r.Context(), marketing.ServiceInterfaceBindingQuery{
		TenantID:       tenantFromRequest(r),
		ServiceRef:     strings.TrimSpace(r.URL.Query().Get("serviceRef")),
		InterfaceRef:   strings.TrimSpace(r.URL.Query().Get("interfaceRef")),
		Scope:          marketing.ServiceAssetScope(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("scope")))),
		WorkspaceID:    strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:       strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:     strings.TrimSpace(r.URL.Query().Get("activityId")),
		NodeID:         strings.TrimSpace(r.URL.Query().Get("nodeId")),
		NodeType:       strings.TrimSpace(r.URL.Query().Get("nodeType")),
		Status:         serviceAssetStatusQuery(r, includeDeleted),
		IncludeDeleted: includeDeleted,
		Limit:          intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveServiceInterfaceBinding(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var binding marketing.ServiceInterfaceBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service interface binding payload")
		return
	}
	if binding.TenantID == "" {
		binding.TenantID = tenantFromRequest(r)
	}
	if actor := actorFromRequest(r); actor != "" {
		if binding.CreatedBy == "" {
			binding.CreatedBy = actor
		}
		binding.UpdatedBy = actor
	}
	saved, err := h.engine.SaveServiceInterfaceBinding(r.Context(), binding)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) checkServiceInterfaceBinding(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var check engine.ServiceInterfaceAccessCheck
	if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service interface binding check payload")
		return
	}
	check.TenantID = firstNonEmpty(check.TenantID, tenantFromRequest(r))
	if serviceCode := strings.TrimSpace(r.PathValue("serviceCode")); serviceCode != "" {
		check.ServiceRef = serviceCode
	}
	if interfaceCode := strings.TrimSpace(r.PathValue("interfaceCode")); interfaceCode != "" {
		check.InterfaceRef = interfaceCode
	}
	checkCtx := marketing.WithAuditSource(r.Context(), map[string]any{
		"actor":            actorFromRequest(r),
		"roles":            csvHeaderValues(r.Header.Get("X-Actor-Roles")),
		"groups":           csvHeaderValues(r.Header.Get("X-Actor-Groups")),
		"serviceAccountId": strings.TrimSpace(r.Header.Get("X-Service-Account-Id")),
	})
	result, err := h.engine.CheckServiceInterfaceAccess(checkCtx, check)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":           result.Allowed,
		"code":              result.Code,
		"message":           result.Message,
		"matchedBindingIds": result.MatchedBindingIDs,
		"requestId":         r.Header.Get("X-Request-Id"),
	})
}

func csvHeaderValues(raw string) []string {
	values := []string{}
	for _, item := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(item); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (h *Handler) listServiceCallAuditLogs(w http.ResponseWriter, r *http.Request) {
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	if !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionView) {
		return
	}
	items, err := h.engine.ListServiceCallAuditLogs(r.Context(), marketing.ServiceCallAuditQuery{
		TenantID:     tenantFromRequest(r),
		WorkspaceID:  workspaceID,
		FolderID:     strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:   strings.TrimSpace(r.URL.Query().Get("activityId")),
		WorkflowID:   strings.TrimSpace(r.URL.Query().Get("workflowId")),
		NodeID:       strings.TrimSpace(r.URL.Query().Get("nodeId")),
		ServiceRef:   strings.TrimSpace(r.URL.Query().Get("serviceRef")),
		InterfaceRef: strings.TrimSpace(r.URL.Query().Get("interfaceRef")),
		ProviderRef:  strings.TrimSpace(r.URL.Query().Get("providerRef")),
		ResourceRef:  strings.TrimSpace(r.URL.Query().Get("resourceRef")),
		Status:       marketing.ServiceCallAuditStatus(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))),
		ErrorCode:    strings.TrimSpace(r.URL.Query().Get("errorCode")),
		Cursor:       strings.TrimSpace(r.URL.Query().Get("cursor")),
		Limit:        intQuery(r, "limit"),
	})
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": serviceCallAuditNextCursor(items),
		"limit":      intQuery(r, "limit"),
		"requestId":  r.Header.Get("X-Request-Id"),
	})
}

func (h *Handler) purgeServiceCallAuditLogs(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var payload struct {
		TenantID      string `json:"tenantId"`
		WorkspaceID   string `json:"workspaceId"`
		Before        string `json:"before"`
		OlderThanDays int    `json:"olderThanDays"`
		Limit         int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service call audit purge payload")
		return
	}
	tenantID := firstNonEmpty(payload.TenantID, tenantFromRequest(r))
	workspaceID := firstNonEmpty(strings.TrimSpace(payload.WorkspaceID), strings.TrimSpace(r.URL.Query().Get("workspaceId")))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	if !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionAdmin) {
		return
	}
	var before time.Time
	if strings.TrimSpace(payload.Before) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.Before))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid before timestamp")
			return
		}
		before = parsed
	} else if payload.OlderThanDays > 0 {
		before = time.Now().UTC().Add(-time.Duration(payload.OlderThanDays) * 24 * time.Hour)
	} else {
		writeError(w, http.StatusBadRequest, "before or olderThanDays is required")
		return
	}
	deleted, err := h.engine.PurgeServiceCallAuditLogs(r.Context(), tenantID, workspaceID, before, payload.Limit)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   deleted,
		"before":    before.UTC().Format(time.RFC3339Nano),
		"requestId": r.Header.Get("X-Request-Id"),
	})
}

func serviceCallAuditNextCursor(items []marketing.ServiceCallAuditLog) string {
	if len(items) == 0 {
		return ""
	}
	last := items[len(items)-1]
	if last.ID == "" {
		return ""
	}
	return last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
}

func serviceAssetStatusQuery(r *http.Request, includeDeleted bool) marketing.ServiceAssetStatus {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" {
		return marketing.ServiceAssetStatus(strings.ToUpper(status))
	}
	if includeDeleted {
		return ""
	}
	return marketing.ServiceAssetActive
}
