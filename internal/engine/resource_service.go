package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	} else if code := s.testResourceDryRun(ctx, resource); code != "" {
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

func (s *Service) testResourceDryRun(ctx context.Context, resource marketing.Resource) string {
	config := resource.ConfigMap()
	if !workflowConfigBool(config, "dryRun", false) {
		return ""
	}
	switch resource.Type {
	case marketing.ResourceHTTPProvider:
		endpoint := firstNonEmpty(resourceConfigString(config, "testEndpoint"), resourceConfigString(config, "endpoint"), resourceConfigString(config, "providerEndpoint"))
		if err := workflowValidateHTTPProviderEndpoint(endpoint, config); err != nil {
			return "RESOURCE_HTTP_ENDPOINT_DENIED"
		}
		timeoutSeconds := workflowConfigNumber(config, "timeoutSeconds")
		if timeoutSeconds <= 0 {
			timeoutSeconds = 3
		}
		requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
		defer cancel()
		method := strings.ToUpper(firstNonEmpty(resourceConfigString(config, "dryRunMethod"), http.MethodGet))
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodHead {
			return "RESOURCE_HTTP_DRY_RUN_METHOD_UNSUPPORTED"
		}
		var body io.Reader
		if method == http.MethodPost {
			body = bytes.NewReader([]byte(firstNonEmpty(resourceConfigString(config, "dryRunBody"), `{}`)))
		}
		req, err := http.NewRequestWithContext(requestCtx, method, endpoint, body)
		if err != nil {
			return "RESOURCE_HTTP_DRY_RUN_REQUEST_INVALID"
		}
		req.Header.Set("Content-Type", "application/json")
		if err := workflowApplyHTTPProviderHeaders(req, config); err != nil {
			return "RESOURCE_HTTP_SECRET_REF_UNRESOLVED"
		}
		resp, err := workflowHTTPProviderClient(config).Do(req)
		if err != nil {
			return "RESOURCE_HTTP_DRY_RUN_FAILED"
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "RESOURCE_HTTP_DRY_RUN_FAILED"
		}
		if len(strings.TrimSpace(string(respBody))) > 0 {
			var decoded any
			if err := json.Unmarshal(respBody, &decoded); err == nil {
				if err := workflowValidateHTTPResponseSchema(config, decoded); err != nil {
					return "RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID"
				}
			}
		}
	}
	return ""
}

func resourceAdapterConfigError(resource marketing.Resource) string {
	config := resource.ConfigMap()
	switch resource.Type {
	case marketing.ResourceDatabaseTable:
		if firstNonEmpty(resourceConfigString(config, "physicalTable"), resourceConfigString(config, "table"), resourceConfigString(config, "tableNamespace")) == "" {
			return "RESOURCE_TABLE_CONFIG_INVALID"
		}
		primaryKey := resourceConfigStringList(config, "primaryKey")
		if len(primaryKey) == 0 {
			return "RESOURCE_TABLE_PRIMARY_KEY_REQUIRED"
		}
		if !resourceTableSchemaValid(config) {
			return "RESOURCE_TABLE_SCHEMA_INVALID"
		}
		if !resourceTableMappingValid(config, "keyMapping") {
			return "RESOURCE_TABLE_KEY_MAPPING_INVALID"
		}
		if !resourceTableMappingValid(config, "valueMapping") {
			return "RESOURCE_TABLE_VALUE_MAPPING_INVALID"
		}
		if !resourceTablePrimaryKeyCovered(config, primaryKey) {
			return "RESOURCE_TABLE_PRIMARY_KEY_MAPPING_INVALID"
		}
	case marketing.ResourceHTTPProvider:
		if firstNonEmpty(resourceConfigString(config, "endpoint"), resourceConfigString(config, "providerEndpoint")) == "" {
			return "RESOURCE_HTTP_ENDPOINT_REQUIRED"
		}
		method := strings.ToUpper(firstNonEmpty(resourceConfigString(config, "method"), resourceConfigString(config, "httpMethod"), "POST"))
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut {
			return "RESOURCE_HTTP_METHOD_UNSUPPORTED"
		}
		if !resourceHTTPSecretConfigValid(config) {
			return "RESOURCE_HTTP_SECRET_REF_REQUIRED"
		}
		if !resourceHTTPResponseSchemaConfigValid(config) {
			return "RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID"
		}
	case marketing.ResourceRedisKV, marketing.ResourceRedisHash, marketing.ResourceRedisCounter, marketing.ResourceRedisLock:
		if firstNonEmpty(resourceConfigString(config, "keyPrefix"), resourceConfigString(config, "prefix")) == "" {
			return "RESOURCE_REDIS_KEY_PREFIX_REQUIRED"
		}
	case marketing.ResourceMQTopic:
		if firstNonEmpty(resourceConfigString(config, "topic"), resourceConfigString(config, "destination"), resourceConfigString(config, "targetRef")) == "" {
			return "RESOURCE_MQ_TOPIC_REQUIRED"
		}
		provider := strings.ToUpper(firstNonEmpty(resourceConfigString(config, "provider"), resourceConfigString(config, "broker"), "KAFKA"))
		if provider != "KAFKA" && provider != "RABBITMQ" && provider != "PULSAR" {
			return "RESOURCE_MQ_PROVIDER_UNSUPPORTED"
		}
	}
	return ""
}

func resourceHTTPSecretConfigValid(config map[string]any) bool {
	if resourceConfigString(config, "secretRef") != "" || resourceConfigString(config, "headersRef") != "" {
		return true
	}
	headers, _ := workflowJSONValue(firstNonEmptyAny(config["headers"], config["requestHeaders"])).(map[string]any)
	for key := range headers {
		if resourceHTTPSensitiveHeader(key) {
			return false
		}
	}
	return true
}

func resourceHTTPSensitiveHeader(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", ""), "_", ""))
	for _, marker := range []string{"authorization", "cookie", "setcookie", "token", "secret", "apikey", "xapikey"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func resourceHTTPResponseSchemaConfigValid(config map[string]any) bool {
	raw, ok := config["responseSchema"]
	if !ok || raw == nil {
		return true
	}
	schema, ok := normalizedResourceMap(raw)
	if !ok {
		return false
	}
	for _, key := range []string{"required", "requiredPaths"} {
		for _, path := range resourceConfigStringList(schema, key) {
			if path != "$" && !strings.HasPrefix(path, "$.") {
				return false
			}
		}
	}
	if fields, ok := workflowJSONValue(schema["fields"]).(map[string]any); ok {
		for path, rawType := range fields {
			if path != "$" && !strings.HasPrefix(path, "$.") {
				return false
			}
			if !resourceTableColumnTypeValid(fmt.Sprint(rawType)) {
				return false
			}
		}
	}
	return true
}

func workflowApplyHTTPProviderHeaders(req *http.Request, config map[string]any) error {
	if rawHeaders, ok := config["requestHeaders"].(map[string]any); ok {
		for key, value := range rawHeaders {
			if strings.TrimSpace(key) != "" && value != nil {
				req.Header.Set(key, fmt.Sprint(value))
			}
		}
	}
	if rawHeaders, ok := config["headers"].(map[string]any); ok {
		for key, value := range rawHeaders {
			if strings.TrimSpace(key) != "" && value != nil && req.Header.Get(key) == "" {
				req.Header.Set(key, fmt.Sprint(value))
			}
		}
	}
	if headersRef := resourceConfigString(config, "headersRef"); headersRef != "" {
		secret, ok := workflowResolveResourceSecret(headersRef)
		if !ok {
			return fmt.Errorf("headersRef %s could not be resolved", headersRef)
		}
		var headers map[string]any
		if err := json.Unmarshal([]byte(secret), &headers); err != nil {
			return fmt.Errorf("headersRef %s must resolve to json object", headersRef)
		}
		for key, value := range headers {
			if strings.TrimSpace(key) != "" && value != nil {
				req.Header.Set(key, fmt.Sprint(value))
			}
		}
	}
	if secretRef := resourceConfigString(config, "secretRef"); secretRef != "" {
		secret, ok := workflowResolveResourceSecret(secretRef)
		if !ok {
			return fmt.Errorf("secretRef %s could not be resolved", secretRef)
		}
		header := firstNonEmpty(resourceConfigString(config, "authHeader"), "Authorization")
		scheme := resourceConfigString(config, "authScheme")
		if scheme == "" {
			scheme = "Bearer"
		}
		if strings.EqualFold(scheme, "RAW") || strings.EqualFold(scheme, "NONE") {
			req.Header.Set(header, secret)
		} else {
			req.Header.Set(header, scheme+" "+secret)
		}
	}
	return nil
}

func workflowResolveResourceSecret(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	if strings.HasPrefix(ref, "env:") {
		value, ok := os.LookupEnv(strings.TrimSpace(strings.TrimPrefix(ref, "env:")))
		return value, ok
	}
	envName := "EXM_RESOURCE_SECRET_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", "/", "_", ":", "_").Replace(ref))
	value, ok := os.LookupEnv(envName)
	return value, ok
}

func resourceTableSchemaValid(config map[string]any) bool {
	raw, ok := config["schema"]
	if !ok || raw == nil {
		raw, ok = config["tableSchema"]
		if !ok || raw == nil {
			return true
		}
	}
	schema, ok := normalizedResourceMap(raw)
	if !ok {
		return false
	}
	rawColumns, ok := schema["columns"]
	if !ok || rawColumns == nil {
		rawColumns = schema["fields"]
	}
	if rawColumns == nil {
		return true
	}
	switch typed := workflowJSONValue(rawColumns).(type) {
	case []any:
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok || firstNonEmpty(resourceConfigString(mapped, "name"), resourceConfigString(mapped, "key")) == "" {
				return false
			}
			if !resourceTableColumnTypeValid(resourceConfigString(mapped, "type")) {
				return false
			}
		}
		return true
	case map[string]any:
		for name, item := range typed {
			if strings.TrimSpace(name) == "" {
				return false
			}
			if mapped, ok := item.(map[string]any); ok {
				if !resourceTableColumnTypeValid(resourceConfigString(mapped, "type")) {
					return false
				}
			} else if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && !resourceTableColumnTypeValid(text) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func resourceTableMappingValid(config map[string]any, key string) bool {
	raw, ok := config[key]
	if !ok || raw == nil {
		return true
	}
	mapping, ok := normalizedResourceMap(raw)
	if !ok {
		return false
	}
	for column, spec := range mapping {
		if strings.TrimSpace(column) == "" {
			return false
		}
		if mapped, ok := workflowJSONValue(spec).(map[string]any); ok {
			if resourceConfigString(mapped, "path") == "" && resourceConfigString(mapped, "template") == "" {
				if _, hasValue := mapped["value"]; !hasValue {
					if _, hasDefault := mapped["default"]; !hasDefault {
						return false
					}
				}
			}
		}
	}
	return true
}

func resourceTablePrimaryKeyCovered(config map[string]any, primaryKey []string) bool {
	available := map[string]struct{}{
		"tenantId":   {},
		"activityId": {},
		"subjectId":  {},
		"eventId":    {},
	}
	for _, column := range resourceTableSchemaColumnNames(config) {
		available[column] = struct{}{}
	}
	for _, mappingKey := range []string{"keyMapping", "valueMapping"} {
		if mapping, ok := normalizedResourceMap(config[mappingKey]); ok {
			for column := range mapping {
				available[strings.TrimSpace(column)] = struct{}{}
			}
		}
	}
	for _, key := range primaryKey {
		if _, ok := available[strings.TrimSpace(key)]; !ok {
			return false
		}
	}
	return true
}

func resourceTableSchemaColumnNames(config map[string]any) []string {
	raw, ok := config["schema"]
	if !ok || raw == nil {
		raw = config["tableSchema"]
	}
	schema, ok := normalizedResourceMap(raw)
	if !ok {
		return nil
	}
	rawColumns := firstNonEmptyAny(schema["columns"], schema["fields"])
	switch typed := workflowJSONValue(rawColumns).(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				if name := firstNonEmpty(resourceConfigString(mapped, "name"), resourceConfigString(mapped, "key")); name != "" {
					out = append(out, name)
				}
			}
		}
		return out
	case map[string]any:
		out := make([]string, 0, len(typed))
		for name := range typed {
			if strings.TrimSpace(name) != "" {
				out = append(out, strings.TrimSpace(name))
			}
		}
		return out
	default:
		return nil
	}
}

func resourceTableColumnTypeValid(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "ANY", "STRING", "NUMBER", "DECIMAL", "BOOLEAN", "BOOL", "OBJECT", "JSON", "ARRAY", "TABLE":
		return true
	default:
		return false
	}
}

func normalizedResourceMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	mapped, ok := workflowJSONValue(value).(map[string]any)
	return mapped, ok
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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
	case marketing.ResourceRedisKV, marketing.ResourceRedisHash:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCAS, marketing.ResourceTestConnection}
	case marketing.ResourceRedisCounter:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCheckAndIncrement, marketing.ResourceTestConnection}
	case marketing.ResourceRedisLock:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceTestConnection}
	case marketing.ResourceMQTopic:
		return []marketing.ResourceOperation{marketing.ResourcePublish, marketing.ResourceCall, marketing.ResourceTestConnection}
	default:
		return []marketing.ResourceOperation{marketing.ResourceRead}
	}
}
