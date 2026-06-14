package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	defaultWorkflowStateResource = "default_workflow_state"
	workflowMemoryResource       = "workflow_memory"
)

func (s *Service) SaveResource(ctx context.Context, resource marketing.Resource) (marketing.Resource, error) {
	now := s.now()
	if resource.ID == "" {
		resource.ID = marketing.NewID("res")
	}
	if resource.Status == "" {
		resource.Status = marketing.ResourceActive
	}
	if resource.Scope == "" {
		resource.Scope = marketing.ResourceScopeWorkspace
	}
	if resource.Driver == "" {
		resource.Driver = marketing.ResourceDriverInternal
	}
	if len(resource.Config) == 0 {
		resource.Config = json.RawMessage(`{}`)
	}
	if len(resource.Capabilities) == 0 {
		resource.Capabilities = defaultResourceCapabilities(resource.Type)
	}
	if resource.SchemaVersion == 0 {
		resource.SchemaVersion = 1
	}
	if resource.CreatedAt.IsZero() {
		resource.CreatedAt = now
	}
	resource.UpdatedAt = now
	if err := resource.Validate(); err != nil {
		return marketing.Resource{}, err
	}
	return s.repository.SaveResource(ctx, resource)
}

func (s *Service) GetResource(ctx context.Context, tenantID string, code string) (marketing.Resource, error) {
	if resource, ok := builtInResource(tenantID, code); ok {
		return resource, nil
	}
	return s.repository.GetResource(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(code))
}

func (s *Service) ListResources(ctx context.Context, query marketing.ResourceQuery) ([]marketing.Resource, error) {
	resources, err := s.repository.ListResources(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, resource := range builtInResources(firstNonEmpty(query.TenantID, "site_001")) {
		if resourceMatchesQuery(resource, query) {
			resources = append([]marketing.Resource{resource}, resources...)
		}
	}
	return resources, nil
}

func (s *Service) SaveResourceBinding(ctx context.Context, binding marketing.ResourceBinding) (marketing.ResourceBinding, error) {
	now := s.now()
	if binding.ID == "" {
		binding.ID = marketing.NewID("resbind")
	}
	if binding.Status == "" {
		binding.Status = marketing.ResourceBindingActive
	}
	if len(binding.KeyPolicy) == 0 {
		binding.KeyPolicy = json.RawMessage(`{}`)
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	if err := binding.Validate(); err != nil {
		return marketing.ResourceBinding{}, err
	}
	return s.repository.SaveResourceBinding(ctx, binding)
}

func (s *Service) ListResourceBindings(ctx context.Context, query marketing.ResourceBindingQuery) ([]marketing.ResourceBinding, error) {
	return s.repository.ListResourceBindings(ctx, query)
}

func (s *Service) ListResourceAuditLogs(ctx context.Context, query marketing.ResourceAuditQuery) ([]marketing.ResourceAuditLog, error) {
	return s.repository.ListResourceAuditLogs(ctx, query)
}

func (s *Service) ListAvailableResources(ctx context.Context, req marketing.ResourceAccessRequest) ([]marketing.AvailableResource, error) {
	resources, err := s.ListResources(ctx, marketing.ResourceQuery{
		TenantID:    req.TenantID,
		WorkspaceID: "",
		Status:      marketing.ResourceActive,
		Limit:       500,
	})
	if err != nil {
		return nil, err
	}
	out := make([]marketing.AvailableResource, 0, len(resources))
	for _, resource := range resources {
		err := s.ValidateResourceAccess(ctx, resource, req)
		item := marketing.AvailableResource{Resource: resource, Available: err == nil}
		if err != nil {
			item.UnavailableReason = err.Error()
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) ValidateResourceAccess(ctx context.Context, resource marketing.Resource, req marketing.ResourceAccessRequest) error {
	if resource.Status != marketing.ResourceActive {
		return errors.New("RESOURCE_DISABLED")
	}
	if !resource.Supports(req.Operation) {
		return errors.New("RESOURCE_CAPABILITY_DENIED")
	}
	if req.TenantID != "" && resource.TenantID != "" && req.TenantID != resource.TenantID {
		return errors.New("RESOURCE_SCOPE_DENIED")
	}
	if !resourceScopeAllows(resource, req) {
		return errors.New("RESOURCE_SCOPE_DENIED")
	}
	bindings, err := s.repository.ListResourceBindings(ctx, marketing.ResourceBindingQuery{ResourceID: resource.ID, Status: marketing.ResourceBindingActive, Limit: 500})
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	for _, binding := range bindings {
		if binding.Allows(req) {
			return nil
		}
	}
	return errors.New("RESOURCE_PERMISSION_DENIED")
}

func (s *Service) TestResource(ctx context.Context, tenantID string, code string, actorID string) (map[string]any, error) {
	start := s.now()
	resource, err := s.GetResource(ctx, tenantID, code)
	status := marketing.ResourceAuditSuccess
	errorCode := ""
	if err != nil {
		status = marketing.ResourceAuditFailed
		errorCode = "RESOURCE_NOT_FOUND"
	} else if resource.Status != marketing.ResourceActive {
		status = marketing.ResourceAuditFailed
		errorCode = "RESOURCE_DISABLED"
	} else if code := resourceAdapterConfigError(resource); code != "" {
		status = marketing.ResourceAuditFailed
		errorCode = code
	}
	if err == nil {
		err = s.repository.CreateResourceAuditLog(ctx, marketing.ResourceAuditLog{
			ID:           marketing.NewID("resaudit"),
			TenantID:     resource.TenantID,
			WorkspaceID:  resource.WorkspaceID,
			FolderID:     resource.FolderID,
			ActivityID:   resource.ActivityID,
			ResourceID:   resource.ID,
			ResourceCode: resource.Code,
			Operation:    marketing.ResourceTestConnection,
			Status:       status,
			DurationMS:   int(time.Since(start) / time.Millisecond),
			ErrorCode:    errorCode,
			ActorID:      actorID,
			CreatedAt:    s.now(),
		})
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": status == marketing.ResourceAuditSuccess, "resource": resource, "errorCode": errorCode}, nil
}

func resourceAdapterConfigError(resource marketing.Resource) string {
	config := resource.ConfigMap()
	switch resource.Type {
	case marketing.ResourceDatabaseTable:
		if firstNonEmpty(resourceConfigString(config, "physicalTable"), resourceConfigString(config, "table"), resourceConfigString(config, "tableNamespace")) == "" {
			return "RESOURCE_TABLE_CONFIG_INVALID"
		}
		if len(resourceConfigStringList(config, "primaryKey")) == 0 {
			return "RESOURCE_TABLE_PRIMARY_KEY_REQUIRED"
		}
	case marketing.ResourceHTTPProvider:
		if firstNonEmpty(resourceConfigString(config, "endpoint"), resourceConfigString(config, "providerEndpoint")) == "" {
			return "RESOURCE_HTTP_ENDPOINT_REQUIRED"
		}
		method := strings.ToUpper(firstNonEmpty(resourceConfigString(config, "method"), resourceConfigString(config, "httpMethod"), "POST"))
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut {
			return "RESOURCE_HTTP_METHOD_UNSUPPORTED"
		}
	}
	return ""
}

func resourceConfigString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func resourceConfigStringList(config map[string]any, key string) []string {
	value, ok := config[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		parts := strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' })
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func builtInResources(tenantID string) []marketing.Resource {
	now := time.Unix(0, 0).UTC()
	return []marketing.Resource{
		{
			ID:            "res_builtin_default_workflow_state",
			TenantID:      tenantID,
			Code:          defaultWorkflowStateResource,
			Name:          "默认工作流状态存储",
			Type:          marketing.ResourceDatabaseKV,
			Driver:        marketing.ResourceDriverPostgres,
			Scope:         marketing.ResourceScopeSystem,
			Status:        marketing.ResourceActive,
			Config:        json.RawMessage(`{"table":"marketing_storage_state"}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceAppend, marketing.ResourceDelete, marketing.ResourceCAS},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "res_builtin_workflow_memory",
			TenantID:      tenantID,
			Code:          workflowMemoryResource,
			Name:          "工作流内存资源",
			Type:          marketing.ResourceMemory,
			Driver:        marketing.ResourceDriverInternal,
			Scope:         marketing.ResourceScopeSystem,
			Status:        marketing.ResourceActive,
			Config:        json.RawMessage(`{}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceAppend, marketing.ResourceDelete},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
}

func builtInResource(tenantID string, code string) (marketing.Resource, bool) {
	for _, resource := range builtInResources(tenantID) {
		if resource.Code == strings.TrimSpace(code) {
			return resource, true
		}
	}
	return marketing.Resource{}, false
}

func resourceMatchesQuery(resource marketing.Resource, query marketing.ResourceQuery) bool {
	if query.TenantID != "" && resource.TenantID != query.TenantID {
		return false
	}
	if query.Type != "" && resource.Type != query.Type {
		return false
	}
	if query.Status != "" && resource.Status != query.Status {
		return false
	}
	if query.Code != "" && resource.Code != query.Code {
		return false
	}
	return true
}

func resourceScopeAllows(resource marketing.Resource, req marketing.ResourceAccessRequest) bool {
	switch resource.Scope {
	case marketing.ResourceScopeSystem, marketing.ResourceScopeTenant:
		return true
	case marketing.ResourceScopeWorkspace:
		return resource.WorkspaceID == "" || resource.WorkspaceID == req.WorkspaceID
	case marketing.ResourceScopeFolder:
		return resource.WorkspaceID == req.WorkspaceID && (resource.FolderID == "" || resource.FolderID == req.FolderID)
	case marketing.ResourceScopeActivity:
		return resource.ActivityID == req.ActivityID
	case marketing.ResourceScopeNode:
		return resource.ActivityID == req.ActivityID && resource.FolderID == req.FolderID && req.NodeID != ""
	default:
		return false
	}
}

func defaultResourceCapabilities(resourceType marketing.ResourceType) []marketing.ResourceOperation {
	switch resourceType {
	case marketing.ResourceMemory, marketing.ResourceWorkflowContext, marketing.ResourceDatabaseKV, marketing.ResourceDatabaseTable:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceAppend, marketing.ResourceDelete, marketing.ResourceCAS}
	case marketing.ResourceHTTPProvider:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCall, marketing.ResourceTestConnection}
	default:
		return []marketing.ResourceOperation{marketing.ResourceRead}
	}
}
