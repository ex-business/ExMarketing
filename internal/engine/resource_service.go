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
	defaultMarketingDatabase     = "default_marketing_database"
	defaultUserProfileStore      = "default_user_profile_store"
	defaultKYCProfileStore       = "default_kyc_profile_store"
	defaultRiskProfileStore      = "default_risk_profile_store"
	defaultUserProfileProvider   = "user-profile-provider"
	defaultKYCProvider           = "kyc-provider"
	defaultRiskProvider          = "risk-provider"
	defaultCheckinQuotaCounter   = "default_checkin_quota_counter"
	defaultActivityRewardConfig  = "default_activity_reward_config"
	defaultCheckinRewardConfig   = "checkin_reward_config_ref"
	defaultCheckinRewardBudget   = "checkin_reward_budget_ref"
	defaultCheckinPrizeStock     = "checkin_prize_stock_ref"
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
	case marketing.ResourceHTTP, marketing.ResourceHTTPProvider:
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
	case marketing.ResourceTable, marketing.ResourceDatabaseTable:
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
	case marketing.ResourceHTTP, marketing.ResourceHTTPProvider:
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
	case marketing.ResourceLua:
		if workflowLuaResourceSource(config) == "" {
			return "RESOURCE_LUA_SOURCE_REQUIRED"
		}
	case marketing.ResourceProvider:
		if firstNonEmpty(resourceConfigString(config, "targetResourceRef"), resourceConfigString(config, "resourceRef"), resourceConfigString(config, "storeRef")) == "" {
			return "RESOURCE_PROVIDER_TARGET_REQUIRED"
		}
	case marketing.ResourceRedis, marketing.ResourceRedisKV, marketing.ResourceRedisHash, marketing.ResourceRedisCounter, marketing.ResourceRedisLock:
		if firstNonEmpty(resourceConfigString(config, "keyPrefix"), resourceConfigString(config, "prefix")) == "" {
			return "RESOURCE_REDIS_KEY_PREFIX_REQUIRED"
		}
	case marketing.ResourceMQ, marketing.ResourceMQTopic:
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
			Type:          marketing.ResourceDatabase,
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
		{
			ID:          "res_builtin_default_checkin_quota_counter",
			TenantID:    tenantID,
			Code:        defaultCheckinQuotaCounter,
			Name:        "默认签到频控计数器",
			Description: "每日签到频控默认计数资源，按 namespace、subject、activity 和窗口分段读写受控计数器。",
			Type:        marketing.ResourceRedis,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"keyPrefix":"quota:checkin",
					"mode":"COUNTER"
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCheckAndIncrement, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_default_activity_reward_config",
			TenantID:    tenantID,
			Code:        defaultActivityRewardConfig,
			Name:        "默认活动奖励配置",
			Description: "当前活动版本上的奖励配置资源，供 candidate-reward 生成待发奖励候选。",
			Type:        marketing.ResourceWorkflowContext,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"contextPath":"$.activity.rewards"
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_checkin_reward_config_ref",
			TenantID:    tenantID,
			Code:        defaultCheckinRewardConfig,
			Name:        "签到奖励配置 Redis Ref",
			Description: "每日签到奖励配置资源，Redis KV 语义，key 通常为 activityId，供 candidate-reward 读取 reward_items。",
			Type:        marketing.ResourceRedis,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"keyPrefix":"reward:checkin",
					"mode":"KV",
					"valueType":"OBJECT",
					"contract":"PRIZE_CATALOG_V1",
					"supportedRewardTypes":["BONUS","CASH","POINT","COUPON","GIFT","DRAW_CHANCE","MEMBERSHIP"],
					"supportedCalculateTypes":["FIXED","RATIO","LADDER","RANDOM","PROBABILITY","POOL_SPLIT"],
					"defaultValue":{
						"reward_items":[
							{"rewardCode":"DAILY_CHECKIN_1_USDT","rewardType":"CASH","calculateType":"FIXED","amount":"1","currency":"USDT","grantMode":"AUTO","grantChannel":"WALLET"}
						],
						"examples":{
							"fixedPoint":{"rewardCode":"CHECKIN_100_POINTS","rewardType":"POINT","calculateType":"FIXED","points":"100","unit":"LOYALTY_POINT","grantMode":"AUTO","grantChannel":"INTERNAL"},
							"coupon":{"rewardCode":"CHECKIN_COUPON_5","rewardType":"COUPON","calculateType":"FIXED","quantity":"1","couponTemplateId":"tpl_checkin_5","faceValue":"5","currency":"USDT","grantMode":"AUTO","grantChannel":"INTERNAL"},
							"gift":{"rewardCode":"CHECKIN_TSHIRT","rewardType":"GIFT","calculateType":"FIXED","quantity":"1","giftSkuId":"sku_tshirt","deliveryMode":"ADDRESS_REQUIRED","valuationAmount":"20","currency":"USDT","grantMode":"MANUAL","grantChannel":"MANUAL_FILE"},
							"drawChance":{"rewardCode":"CHECKIN_DRAW_CHANCE","rewardType":"DRAW_CHANCE","calculateType":"FIXED","chanceCount":"1","drawPoolCode":"daily_checkin_lottery","grantMode":"AUTO","grantChannel":"INTERNAL"},
							"randomCash":{"rewardCode":"CHECKIN_RANDOM_USDT","rewardType":"CASH","calculateType":"RANDOM","minAmount":"0.1","maxAmount":"5","step":"0.1","currency":"USDT","grantMode":"AUTO","grantChannel":"WALLET"},
							"mixedProbabilityPool":{"rewardCode":"CHECKIN_MIXED_POOL","rewardType":"DRAW_CHANCE","calculateType":"PROBABILITY","grantMode":"AUTO","grantChannel":"INTERNAL","items":[
								{"itemCode":"POINT_100","weight":"70","rewardType":"POINT","points":"100","unit":"LOYALTY_POINT","label":"100 points"},
								{"itemCode":"COUPON_5","weight":"25","rewardType":"COUPON","quantity":"1","couponTemplateId":"tpl_checkin_5","faceValue":"5","currency":"USDT","label":"5 USDT coupon"},
								{"itemCode":"GIFT_TSHIRT","weight":"5","rewardType":"GIFT","quantity":"1","giftSkuId":"sku_tshirt","deliveryMode":"ADDRESS_REQUIRED","valuationAmount":"20","currency":"USDT","label":"T-shirt"}
							]},
							"membership":{"rewardCode":"CHECKIN_VIP_7D","rewardType":"MEMBERSHIP","calculateType":"FIXED","durationDays":"7","membershipTier":"VIP","grantMode":"AUTO","grantChannel":"INTERNAL"}
						}
					}
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_checkin_reward_budget_ref",
			TenantID:    tenantID,
			Code:        defaultCheckinRewardBudget,
			Name:        "签到奖励预算 Ref",
			Description: "每日签到发现金类奖励前使用的预算锁定资源，budget-reserve 会读取该 Ref 并走原子 quota reservation。",
			Type:        marketing.ResourceProvider,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"contract":"QUOTA_RESERVE_V1",
					"quotaCode":"DAILY_CHECKIN_REWARD",
					"quotaType":"BUDGET",
					"scope":"ACTIVITY",
					"scopeKeyTemplate":"${activityId}",
					"amountField":"candidate_rewards[0].amount",
					"reserveMode":"ATOMIC",
					"releaseOnFailure":true
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceCAS, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_checkin_prize_stock_ref",
			TenantID:    tenantID,
			Code:        defaultCheckinPrizeStock,
			Name:        "签到奖品库存 Ref",
			Description: "每日签到发券、实物或权益类奖励前使用的库存锁定资源，scopeKey 默认按 activityId + rewardCode 分桶。",
			Type:        marketing.ResourceProvider,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"contract":"QUOTA_RESERVE_V1",
					"quotaCode":"CHECKIN_PRIZE_STOCK",
					"quotaType":"STOCK",
					"scope":"ACTIVITY",
					"scopeKeyTemplate":"${activityId}:${candidate_rewards[0].rewardCode}",
					"amountField":"candidate_rewards[0].quantity",
					"reserveMode":"ATOMIC",
					"releaseOnFailure":true
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceCAS, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_default_marketing_database",
			TenantID:    tenantID,
			Code:        defaultMarketingDatabase,
			Name:        "默认营销数据库",
			Description: "营销域默认数据库资源，承载用户资料、活动状态、频控和订单摘要等数据集。",
			Type:        marketing.ResourceDatabase,
			Driver:      marketing.ResourceDriverPostgres,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"connection":"default",
					"managedStores":["default_user_profile_store","default_kyc_profile_store","default_risk_profile_store"]
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_default_user_profile_store",
			TenantID:    tenantID,
			Code:        defaultUserProfileStore,
			Name:        "默认用户资料 Store",
			Description: "基于 default_marketing_database 创建的用户资料 store，读取 marketing_user_profile 并输出 USER_PROFILE 事实。",
			Type:        marketing.ResourceTable,
			Driver:      marketing.ResourceDriverPostgres,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"databaseRef":"default_marketing_database",
					"storeKind":"FACT_STORE",
					"factCode":"USER_PROFILE",
					"adapter":"USER_PROFILE",
					"physicalTable":"marketing_user_profile",
					"primaryKey":["tenantId","subjectType","subjectId"],
					"keyTemplate":"${tenantId}:${subject.type}:${subject.id}",
					"schema":{"columns":{
						"tenantId":"STRING",
						"subjectType":"STRING",
						"subjectId":"STRING",
						"userLevel":"NUMBER",
						"vipLevel":"NUMBER",
						"status":"STRING"
					}},
					"keyMapping":{
						"tenantId":{"path":"$.tenantId"},
						"subjectType":{"path":"$.subject.type"},
						"subjectId":{"path":"$.subject.id"}
					}
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_user_profile_provider",
			TenantID:    tenantID,
			Code:        defaultUserProfileProvider,
			Name:        "用户资料 Provider",
			Description: "公共事实 Provider，输出 USER_PROFILE；默认由 default_user_profile_store 实现。",
			Type:        marketing.ResourceProvider,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"providerType":"TABLE",
					"contract":"FACTS_V1",
					"capabilities":["FACT_QUERY"],
					"factKeys":["USER_PROFILE"],
					"targetResourceRef":"default_user_profile_store"
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_default_kyc_profile_store",
			TenantID:    tenantID,
			Code:        defaultKYCProfileStore,
			Name:        "默认 KYC 资料 Store",
			Description: "基于 default_marketing_database 创建的 KYC 用户资源，读取 marketing_kyc_profile 并输出 KYC_PROFILE 事实。",
			Type:        marketing.ResourceTable,
			Driver:      marketing.ResourceDriverPostgres,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"databaseRef":"default_marketing_database",
					"storeKind":"FACT_STORE",
					"factCode":"KYC_PROFILE",
					"adapter":"KYC_PROFILE",
					"physicalTable":"marketing_kyc_profile",
					"primaryKey":["tenantId","subjectType","subjectId"],
					"keyTemplate":"${tenantId}:${subject.type}:${subject.id}",
					"schema":{"columns":{
						"tenantId":"STRING",
						"subjectType":"STRING",
						"subjectId":"STRING",
						"status":"STRING",
						"level":"STRING",
						"verified":"BOOLEAN",
						"provider":"STRING"
					}},
					"keyMapping":{
						"tenantId":{"path":"$.tenantId"},
						"subjectType":{"path":"$.subject.type"},
						"subjectId":{"path":"$.subject.id"}
					}
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_kyc_provider",
			TenantID:    tenantID,
			Code:        defaultKYCProvider,
			Name:        "KYC Provider",
			Description: "公共事实 Provider，输出 KYC_PROFILE；默认由 default_kyc_profile_store 实现。",
			Type:        marketing.ResourceProvider,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"providerType":"TABLE",
					"contract":"FACTS_V1",
					"capabilities":["FACT_QUERY"],
					"factKeys":["KYC_PROFILE"],
					"targetResourceRef":"default_kyc_profile_store"
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_default_risk_profile_store",
			TenantID:    tenantID,
			Code:        defaultRiskProfileStore,
			Name:        "默认风控资料 Store",
			Description: "基于 default_marketing_database 创建的风控用户资源，读取 marketing_risk_profile 并输出 RISK_PROFILE 事实。",
			Type:        marketing.ResourceTable,
			Driver:      marketing.ResourceDriverPostgres,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"databaseRef":"default_marketing_database",
					"storeKind":"FACT_STORE",
					"factCode":"RISK_PROFILE",
					"adapter":"RISK_PROFILE",
					"physicalTable":"marketing_risk_profile",
					"primaryKey":["tenantId","subjectType","subjectId"],
					"keyTemplate":"${tenantId}:${subject.type}:${subject.id}",
					"schema":{"columns":{
						"tenantId":"STRING",
						"subjectType":"STRING",
						"subjectId":"STRING",
						"score":"NUMBER",
						"level":"STRING",
						"status":"STRING",
						"blacklisted":"BOOLEAN",
						"clear":"BOOLEAN",
						"provider":"STRING"
					}},
					"keyMapping":{
						"tenantId":{"path":"$.tenantId"},
						"subjectType":{"path":"$.subject.type"},
						"subjectId":{"path":"$.subject.id"}
					}
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
			SchemaVersion: 1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:          "res_builtin_risk_provider",
			TenantID:    tenantID,
			Code:        defaultRiskProvider,
			Name:        "风控 Provider",
			Description: "公共事实 Provider，输出 RISK_PROFILE；默认由 default_risk_profile_store 实现。",
			Type:        marketing.ResourceProvider,
			Driver:      marketing.ResourceDriverInternal,
			Scope:       marketing.ResourceScopeSystem,
			Status:      marketing.ResourceActive,
			Config: json.RawMessage(`{
					"providerType":"TABLE",
					"contract":"FACTS_V1",
					"capabilities":["FACT_QUERY"],
					"factKeys":["RISK_PROFILE"],
					"targetResourceRef":"default_risk_profile_store"
				}`),
			Capabilities:  []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceTestConnection},
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
	case marketing.ResourceMemory, marketing.ResourceWorkflowContext, marketing.ResourceDatabase, marketing.ResourceDatabaseKV, marketing.ResourceTable, marketing.ResourceArray, marketing.ResourceDatabaseTable:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceAppend, marketing.ResourceDelete, marketing.ResourceCAS}
	case marketing.ResourceHTTP, marketing.ResourceHTTPProvider, marketing.ResourceProvider:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCall, marketing.ResourceTestConnection}
	case marketing.ResourceLua:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceCall, marketing.ResourceTestConnection}
	case marketing.ResourceRedis:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCAS, marketing.ResourceCheckAndIncrement, marketing.ResourceTestConnection}
	case marketing.ResourceRedisKV, marketing.ResourceRedisHash:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCAS, marketing.ResourceTestConnection}
	case marketing.ResourceRedisCounter:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceCheckAndIncrement, marketing.ResourceTestConnection}
	case marketing.ResourceRedisLock:
		return []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceWrite, marketing.ResourceUpsert, marketing.ResourceDelete, marketing.ResourceTestConnection}
	case marketing.ResourceMQ, marketing.ResourceMQTopic:
		return []marketing.ResourceOperation{marketing.ResourcePublish, marketing.ResourceCall, marketing.ResourceTestConnection}
	default:
		return []marketing.ResourceOperation{marketing.ResourceRead}
	}
}
