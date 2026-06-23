package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) listServiceDefinitions(w http.ResponseWriter, r *http.Request) {
	services, err := h.engine.ListServiceDefinitions(r.Context(), marketing.ServiceDefinitionQuery{
		TenantID:    tenantFromRequest(r),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		Status:      marketing.ServiceAssetStatus(strings.ToUpper(firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), string(marketing.ServiceAssetActive)))),
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Limit:       intQuery(r, "limit"),
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
	items, err := h.engine.ListServiceInterfaces(r.Context(), marketing.ServiceInterfaceQuery{
		TenantID:   tenantFromRequest(r),
		ServiceRef: strings.TrimSpace(r.PathValue("serviceCode")),
		Status:     marketing.ServiceAssetStatus(strings.ToUpper(firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), string(marketing.ServiceAssetActive)))),
		Code:       strings.TrimSpace(r.URL.Query().Get("code")),
		Limit:      intQuery(r, "limit"),
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
	items, err := h.engine.ListServiceProviders(r.Context(), marketing.ServiceProviderQuery{
		TenantID:     tenantFromRequest(r),
		ServiceRef:   strings.TrimSpace(r.PathValue("serviceCode")),
		InterfaceRef: strings.TrimSpace(r.PathValue("interfaceCode")),
		Protocol:     marketing.ServiceProviderProtocol(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("protocol")))),
		Status:       marketing.ServiceAssetStatus(strings.ToUpper(firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), string(marketing.ServiceAssetActive)))),
		Code:         strings.TrimSpace(r.URL.Query().Get("code")),
		Limit:        intQuery(r, "limit"),
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
