package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func ptrTime(value time.Time) *time.Time {
	return &value
}

type fakeServiceResourceOperator struct {
	calls           int
	result          workflowNodeResult
	resultsByRef    map[string]workflowNodeResult
	calledRefs      []string
	apiCalls        int
	apiRequests     []workflowResourceAPIRequest
	apiOutputsByRef map[string]map[string]any
	apiErrorsByRef  map[string]error
	apiDelegate     ServiceResourceOperator
	failResourceAPI bool
}

func (op *fakeServiceResourceOperator) ExecuteFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult {
	op.calls++
	ref := workflowConfigString(config, "resourceRef")
	op.calledRefs = append(op.calledRefs, ref)
	result := op.result
	if op.resultsByRef != nil {
		if configured, ok := op.resultsByRef[ref]; ok {
			result = configured
		}
	}
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	return result
}

func (op *fakeServiceResourceOperator) ExecuteResourceAPI(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, req workflowResourceAPIRequest) (map[string]any, error) {
	op.calls++
	op.apiCalls++
	op.apiRequests = append(op.apiRequests, req)
	if op.apiDelegate != nil {
		return op.apiDelegate.ExecuteResourceAPI(ctx, node, runtime, req)
	}
	if op.failResourceAPI {
		return map[string]any{"errorCode": "RESOURCE_API_FAILED", "resourceRef": req.ResourceRef}, errors.New("resource api failed")
	}
	if op.apiErrorsByRef != nil {
		if err, ok := op.apiErrorsByRef[req.ResourceRef]; ok {
			output := map[string]any{"errorCode": "RESOURCE_API_FAILED", "resourceRef": req.ResourceRef}
			if op.apiOutputsByRef != nil {
				if configured, ok := op.apiOutputsByRef[req.ResourceRef]; ok {
					output = configured
				}
			}
			return output, err
		}
	}
	if op.apiOutputsByRef != nil {
		if output, ok := op.apiOutputsByRef[req.ResourceRef]; ok {
			return output, nil
		}
	}
	return map[string]any{"ok": true, "resourceRef": req.ResourceRef, "key": req.Key, "value": req.Value}, nil
}

func TestProviderChainExecutorAuditsSuccessAndUsesResourceOperator(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{
		"facts":       map[string]any{"USER_PROFILE": map[string]any{"level": 7, "displayName": "Visible User"}},
		"accessToken": "plain-secret-token",
	})}
	executor := NewProviderChainExecutor(service, operator)
	node := marketing.WorkflowNode{ID: "fact", Type: "fact-query"}
	result := executor.ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      node,
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderPolicy: "PRIMARY"},
		Remaining: []string{"USER_PROFILE"},
		Facts:     map[string]any{},
	})
	if result.Result.Status != "" {
		t.Fatalf("expected executor step success without terminal node result, got %+v", result.Result)
	}
	if operator.calls != 1 {
		t.Fatalf("expected ResourceOperator called once, got %d", operator.calls)
	}
	if got := result.Facts["USER_PROFILE"].(map[string]any)["level"]; got != 7 {
		t.Fatalf("expected facts from ResourceOperator, got %+v", result.Facts)
	}
	logs, err := service.ListServiceCallAuditLogs(ctx, marketing.ServiceCallAuditQuery{TenantID: runtime.Activity.TenantID})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Status != marketing.ServiceCallAuditSuccess || logs[0].ProviderRef == "" || logs[0].ResourceRef == "" {
		t.Fatalf("expected success service call audit, got %+v", logs)
	}
	if !strings.HasPrefix(logs[0].RequestHash, "hmac-sha256:") || !strings.HasPrefix(logs[0].ResponseHash, "hmac-sha256:") {
		t.Fatalf("expected request/response hashes, got %+v", logs[0])
	}
	if strings.Contains(logs[0].ResponseHash, "plain-secret-token") {
		t.Fatalf("audit hash leaked plaintext secret: %+v", logs[0])
	}
	decisions, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionSelected})
	if err != nil {
		t.Fatalf("list provider decision audit: %v", err)
	}
	if len(decisions) != 1 || decisions[0].CandidateProviderRef == "" || decisions[0].ServiceRef != "user-profile-service" {
		t.Fatalf("expected selected provider decision audit, got %+v", decisions)
	}
	if strings.Contains(string(decisions[0].Details), "plain-secret-token") {
		t.Fatalf("provider decision audit leaked plaintext secret: %s", decisions[0].Details)
	}
	if strings.Contains(string(decisions[0].Details), "Visible User") || strings.Contains(string(decisions[0].Details), `"level":7`) {
		t.Fatalf("provider decision audit stored provider output values: %s", decisions[0].Details)
	}
}

func TestProviderChainExecutorAuditsExplicitProviderRefOverride(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                 runtime.Activity.TenantID,
		ServiceRef:               "user-profile-service",
		InterfaceRef:             "query-user-profile",
		Scope:                    marketing.ServiceAssetScopeActivity,
		ActivityID:               runtime.Activity.ID,
		PrincipalType:            marketing.ServicePrincipalRole,
		PrincipalID:              "*",
		AllowedNodeTypes:         []string{"fact-query"},
		AllowedOperations:        []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies:  []string{"PRIMARY"},
		AllowExplicitProviderRef: true,
		AllowedProviderRefs:      []string{"user-profile-provider-primary"},
		Status:                   marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 3}}})}
	result := NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderRef: "user-profile-provider-primary", ProviderPolicy: "PRIMARY"},
		Remaining: []string{"USER_PROFILE"},
		Request:   map[string]any{"factKeys": []any{"USER_PROFILE"}},
		Facts:     map[string]any{},
	})
	if result.Result.Status == marketing.WorkflowNodeFailed {
		t.Fatalf("expected provider override success, got %+v", result.Result)
	}
	logs, err := service.ListServiceCallAuditLogs(ctx, marketing.ServiceCallAuditQuery{TenantID: runtime.Activity.TenantID})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one audit log, got %+v", logs)
	}
	var trace map[string]any
	if err := json.Unmarshal(logs[0].ProviderTrace, &trace); err != nil {
		t.Fatalf("decode provider trace: %v", err)
	}
	if trace["providerRefOverride"] != true || trace["requestedProviderRef"] != "user-profile-provider-primary" {
		t.Fatalf("expected provider override trace, got %+v", trace)
	}
}

func TestProviderChainExecutorAuditsExplicitProviderRateLimited(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                 runtime.Activity.TenantID,
		ServiceRef:               "user-profile-service",
		InterfaceRef:             "query-user-profile",
		Scope:                    marketing.ServiceAssetScopeActivity,
		ActivityID:               runtime.Activity.ID,
		PrincipalType:            marketing.ServicePrincipalRole,
		PrincipalID:              "*",
		AllowedNodeTypes:         []string{"fact-query"},
		AllowedOperations:        []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies:  []string{"PRIMARY"},
		AllowExplicitProviderRef: true,
		AllowedProviderRefs:      []string{"user-profile-provider-primary"},
		Status:                   marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	if _, err := service.SaveServiceRateLimitPolicy(ctx, marketing.ServiceRateLimitPolicy{
		TenantID:      runtime.Activity.TenantID,
		Code:          "one-call",
		Name:          "One Call",
		ServiceRef:    "user-profile-service",
		InterfaceRef:  "query-user-profile",
		ProviderRef:   "user-profile-provider-primary",
		Limit:         1,
		WindowSeconds: 60,
		Backend:       marketing.ServiceRateLimitBackendPostgres,
		Status:        marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save rate limit policy: %v", err)
	}
	provider, err := service.GetServiceProvider(ctx, runtime.Activity.TenantID, "user-profile-service", "query-user-profile", "user-profile-provider-primary")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if _, err := service.consumeServiceProviderRuntime(ctx, provider); err != nil {
		t.Fatalf("pre-consume provider runtime: %v", err)
	}
	operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 3}}})}
	result := NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderRef: "user-profile-provider-primary", ProviderPolicy: "PRIMARY"},
		Remaining: []string{"USER_PROFILE"},
		Request:   map[string]any{"factKeys": []any{"USER_PROFILE"}},
		Facts:     map[string]any{},
	})
	if result.Result.Status != marketing.WorkflowNodeFailed || !strings.Contains(result.Result.Error, "SERVICE_PROVIDER_RATE_LIMITED") {
		t.Fatalf("expected rate-limited provider failure, got %+v", result.Result)
	}
	if operator.calls != 0 {
		t.Fatalf("rate-limited provider must not call resource operator, got %d", operator.calls)
	}
	decisions, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionRateLimited, ProviderRef: "user-profile-provider-primary"})
	if err != nil {
		t.Fatalf("list provider decision audit: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Reason != "SERVICE_PROVIDER_RATE_LIMITED" {
		t.Fatalf("expected rate_limited decision audit, got %+v", decisions)
	}
}

func TestProviderChainExecutorAuditsUnhealthyAndCircuitOpenDecisions(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name     string
		state    marketing.ServiceProviderRuntimeState
		decision marketing.ProviderDecision
		code     string
	}{
		{
			name: "unhealthy",
			state: marketing.ServiceProviderRuntimeState{
				HealthStatus:  marketing.ServiceProviderHealthUnhealthy,
				HealthMessage: "probe failed",
				CircuitStatus: marketing.ServiceProviderCircuitClosed,
			},
			decision: marketing.ProviderDecisionUnhealthy,
			code:     "SERVICE_PROVIDER_UNHEALTHY",
		},
		{
			name: "circuit-open",
			state: marketing.ServiceProviderRuntimeState{
				HealthStatus:    marketing.ServiceProviderHealthDegraded,
				CircuitStatus:   marketing.ServiceProviderCircuitOpen,
				CircuitOpenedAt: ptrTime(time.Now().UTC()),
			},
			decision: marketing.ProviderDecisionCircuitOpen,
			code:     "SERVICE_PROVIDER_CIRCUIT_OPEN",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(marketing.NewMemoryRepository())
			runtime := testWorkflowHandlerRuntime(true)
			if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
				TenantID:                 runtime.Activity.TenantID,
				ServiceRef:               "user-profile-service",
				InterfaceRef:             "query-user-profile",
				Scope:                    marketing.ServiceAssetScopeActivity,
				ActivityID:               runtime.Activity.ID,
				PrincipalType:            marketing.ServicePrincipalRole,
				PrincipalID:              "*",
				AllowedNodeTypes:         []string{"fact-query"},
				AllowedOperations:        []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
				AllowedProviderPolicies:  []string{"PRIMARY"},
				AllowExplicitProviderRef: true,
				AllowedProviderRefs:      []string{"user-profile-provider-primary"},
				Status:                   marketing.ServiceAssetActive,
			}); err != nil {
				t.Fatalf("save binding: %v", err)
			}
			tt.state.TenantID = runtime.Activity.TenantID
			tt.state.ServiceRef = "user-profile-service"
			tt.state.InterfaceRef = "query-user-profile"
			tt.state.ProviderRef = "user-profile-provider-primary"
			if _, err := service.SaveServiceProviderRuntimeState(ctx, tt.state); err != nil {
				t.Fatalf("save runtime state: %v", err)
			}
			operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 3}}})}
			result := NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
				Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
				Runtime:   runtime,
				Config:    map[string]any{},
				Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderRef: "user-profile-provider-primary", ProviderPolicy: "PRIMARY"},
				Remaining: []string{"USER_PROFILE"},
				Facts:     map[string]any{},
			})
			if result.Result.Status != marketing.WorkflowNodeFailed || !strings.Contains(result.Result.Error, tt.code) {
				t.Fatalf("expected %s failure, got %+v", tt.code, result.Result)
			}
			if operator.calls != 0 {
				t.Fatalf("unavailable provider must not call resource operator, got %d", operator.calls)
			}
			decisions, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: tt.decision, ProviderRef: "user-profile-provider-primary"})
			if err != nil {
				t.Fatalf("list provider decision audit: %v", err)
			}
			if len(decisions) != 1 || decisions[0].Reason != tt.code {
				t.Fatalf("expected %s decision audit, got %+v", tt.decision, decisions)
			}
		})
	}
}

func TestProviderDecisionAuditTenantIsolationRedactionAndSizeLimit(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{ID: "fact", Type: "fact-query"}
	service.recordProviderDecisionAudit(ctx, providerDecisionAuditInput{
		Runtime:              runtime,
		Node:                 node,
		ServiceRef:           "svc",
		InterfaceRef:         "iface",
		ProviderPolicy:       "PRIMARY",
		CandidateProviderRef: "provider-a",
		Decision:             marketing.ProviderDecisionSelected,
		Reason:               "selected",
		Details: map[string]any{
			"password": "super-secret",
			"payload":  strings.Repeat("x", marketing.MaxAuditDetailBytes*2),
		},
	})
	otherRuntime := runtime
	otherRuntime.Activity.TenantID = "site_002"
	service.recordProviderDecisionAudit(ctx, providerDecisionAuditInput{
		Runtime:              otherRuntime,
		Node:                 node,
		ServiceRef:           "svc",
		InterfaceRef:         "iface",
		ProviderPolicy:       "PRIMARY",
		CandidateProviderRef: "provider-b",
		Decision:             marketing.ProviderDecisionSelected,
		Reason:               "selected",
		Details:              map[string]any{"ok": true},
	})
	items, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: "site_001", ServiceRef: "svc", InterfaceRef: "iface", Limit: 10})
	if err != nil {
		t.Fatalf("list provider decision audit: %v", err)
	}
	if len(items) != 1 || items[0].CandidateProviderRef != "provider-a" {
		t.Fatalf("expected tenant-isolated provider-a audit, got %+v", items)
	}
	if len(items[0].Details) > marketing.MaxAuditDetailBytes {
		t.Fatalf("expected capped audit details, got %d bytes", len(items[0].Details))
	}
	if strings.Contains(string(items[0].Details), "super-secret") || strings.Contains(string(items[0].Details), strings.Repeat("x", 256)) {
		t.Fatalf("provider decision audit details were not redacted/capped: %s", items[0].Details)
	}
}

func TestProviderChainExecutorAuditsProviderPolicyDenied(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{}}})}
	result := NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderPolicy: "WEIGHTED"},
		Remaining: []string{"USER_PROFILE"},
		Facts:     map[string]any{},
	})
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected provider policy denial, got %+v", result.Result)
	}
	if operator.calls != 0 {
		t.Fatalf("expected ResourceOperator not called after provider policy denial, got %d", operator.calls)
	}
	logs, err := service.ListServiceCallAuditLogs(ctx, marketing.ServiceCallAuditQuery{TenantID: runtime.Activity.TenantID})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Status != marketing.ServiceCallAuditDenied || logs[0].ErrorCode != "SERVICE_PROVIDER_POLICY_DENIED" {
		t.Fatalf("expected denied service call audit, got %+v", logs)
	}
}

func TestProviderChainExecutorAuditsSchemaFailure(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		TenantID:    runtime.Activity.TenantID,
		Code:        "schema-audit-service",
		Name:        "Schema Audit Service",
		ServiceType: marketing.ServiceTypeExternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   runtime.Activity.TenantID,
		SchemaRef:  "schema-audit-output",
		SchemaType: marketing.ServiceSchemaOutput,
		SchemaJSON: json.RawMessage(`{"type":"object","required":["facts"],"properties":{"facts":{"type":"object"}},"additionalProperties":true}`),
	}); err != nil {
		t.Fatalf("save output schema: %v", err)
	}
	if _, err := service.SaveServiceInterface(ctx, marketing.ServiceInterface{
		TenantID:        runtime.Activity.TenantID,
		ServiceRef:      "schema-audit-service",
		Code:            "query",
		Name:            "Query",
		Operation:       marketing.ServiceOperationCall,
		OutputSchemaRef: "schema-audit-output",
		Status:          marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save interface: %v", err)
	}
	if _, err := service.SaveServiceProvider(ctx, marketing.ServiceProvider{
		TenantID:     runtime.Activity.TenantID,
		ServiceRef:   "schema-audit-service",
		InterfaceRef: "query",
		Code:         "schema-audit-provider",
		Name:         "Schema Audit Provider",
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{"schema-audit-resource"},
		Status:       marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "schema-audit-service",
		InterfaceRef:            "query",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	operator := &fakeServiceResourceOperator{result: nodeSuccess(map[string]any{"token": "secret-token-without-facts"})}
	result := NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "schema-audit-service", InterfaceRef: "query", ProviderPolicy: "PRIMARY"},
		Remaining: []string{"USER_PROFILE"},
		Facts:     map[string]any{},
	})
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected schema failure, got %+v", result.Result)
	}
	logs, err := service.ListServiceCallAuditLogs(ctx, marketing.ServiceCallAuditQuery{TenantID: runtime.Activity.TenantID})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Status != marketing.ServiceCallAuditFailed || logs[0].ErrorCode != "SERVICE_SCHEMA_OUTPUT_INVALID" {
		t.Fatalf("expected schema failure audit, got %+v", logs)
	}
	if strings.Contains(logs[0].ResponseHash, "secret-token-without-facts") {
		t.Fatalf("audit response hash leaked plaintext secret: %+v", logs[0])
	}
	decisions, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionSchemaFailed})
	if err != nil {
		t.Fatalf("list provider decision audit: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Reason != "SERVICE_SCHEMA_OUTPUT_INVALID" {
		t.Fatalf("expected schema_failed provider decision audit, got %+v", decisions)
	}
	if strings.Contains(string(decisions[0].Details), "secret-token-without-facts") {
		t.Fatalf("schema decision audit leaked plaintext secret: %s", decisions[0].Details)
	}
}

func TestProviderChainExecutorCacheAsideHitDoesNotCallPrimary(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"]}`))
	operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
		"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": true}, "facts": map[string]any{"USER_PROFILE": map[string]any{"level": 9}}}),
		defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 1}}}),
	}}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status == marketing.WorkflowNodeFailed {
		t.Fatalf("expected cache-aside hit success, got %+v", result.Result)
	}
	if operator.calls != 1 || !reflect.DeepEqual(operator.calledRefs, []string{"user-profile-cache"}) {
		t.Fatalf("expected cache only, calls=%d refs=%v", operator.calls, operator.calledRefs)
	}
	if got := result.Facts["USER_PROFILE"].(map[string]any)["level"]; got != 9 {
		t.Fatalf("expected cache facts, got %+v", result.Facts)
	}
}

func TestProviderChainExecutorCacheAsideHitOutputSchemaInvalidFailsWithoutPrimary(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"]}`))
	operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
		"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": true}, "token": "missing-facts"}),
		defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 1}}}),
	}}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected cache hit output schema failure, got %+v", result.Result)
	}
	if workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_SCHEMA_OUTPUT_INVALID" {
		t.Fatalf("expected output schema failure code, got %+v", result.Result.Output)
	}
	if !reflect.DeepEqual(operator.calledRefs, []string{"user-profile-cache"}) {
		t.Fatalf("expected cache only and no primary call, got %v", operator.calledRefs)
	}
	if operator.apiCalls != 0 {
		t.Fatalf("expected no write-back on cache hit schema failure, got %d", operator.apiCalls)
	}
}

func TestProviderChainExecutorCacheAsideMissCallsPrimaryAndWritesBack(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"]}`))
	operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
		"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
		defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 4}}}),
	}}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status == marketing.WorkflowNodeFailed {
		t.Fatalf("expected cache-aside miss fallback success, got %+v", result.Result)
	}
	if !reflect.DeepEqual(operator.calledRefs, []string{"user-profile-cache", defaultUserProfileProvider}) {
		t.Fatalf("expected cache then primary, got %v", operator.calledRefs)
	}
	if got := result.Facts["USER_PROFILE"].(map[string]any)["level"]; got != 4 {
		t.Fatalf("expected primary facts after miss, got %+v", result.Facts)
	}
	if operator.apiCalls != 1 {
		t.Fatalf("expected one cache write-back call, got %d", operator.apiCalls)
	}
	write := operator.apiRequests[0]
	if write.ResourceRef != "user-profile-cache" || write.Operation != marketing.ResourceUpsert {
		t.Fatalf("expected cache UPSERT write-back, got %+v", write)
	}
	if got := workflowOutputString(write.Value.(map[string]any)["level"]); got != "4" {
		t.Fatalf("expected write-back payload to be fact value, got %+v", write.Value)
	}
	skipped, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionSkipped, ProviderRef: "user-profile-cache"})
	if err != nil {
		t.Fatalf("list skipped decision audit: %v", err)
	}
	if len(skipped) != 1 || skipped[0].Reason != "CACHE_MISS" {
		t.Fatalf("expected cache miss skipped audit, got %+v", skipped)
	}
	selected, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionSelected, ProviderRef: "user-profile-provider-primary"})
	if err != nil {
		t.Fatalf("list selected decision audit: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected primary selected audit, got %+v", selected)
	}
}

func TestProviderChainExecutorCacheAsideWriteBackValuePathExtractsPayload(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name        string
		configField string
	}{
		{name: "writeBackValuePath", configField: "writeBackValuePath"},
		{name: "cacheWriteBackValuePath", configField: "cacheWriteBackValuePath"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cacheConfig := json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"` + tt.configField + `":"$.meta.cachePayload"}`)
			service, runtime := newCacheAsideProviderChainFixture(t, ctx, cacheConfig)
			operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
				"user-profile-cache": nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
				defaultUserProfileProvider: nodeSuccess(map[string]any{
					"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 4}},
					"meta":  map[string]any{"cachePayload": map[string]any{"compact": true, "version": 2}},
				}),
			}}
			result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
			if result.Result.Status == marketing.WorkflowNodeFailed {
				t.Fatalf("expected write-back value path success, got %+v", result.Result)
			}
			if operator.apiCalls != 1 {
				t.Fatalf("expected one write-back call, got %d", operator.apiCalls)
			}
			payload, ok := operator.apiRequests[0].Value.(map[string]any)
			if !ok || payload["compact"] != true || workflowOutputString(payload["version"]) != "2" {
				t.Fatalf("expected extracted cache payload, got %+v", operator.apiRequests[0].Value)
			}
		})
	}
}

func TestProviderChainExecutorCacheAsideErrorFallbackWarnsWhenWriteBackUnavailable(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"cache":true}`))
	operator := &fakeServiceResourceOperator{
		resultsByRef: map[string]workflowNodeResult{
			"user-profile-cache":       nodeFailed("cache unavailable", map[string]any{"errorCode": "CACHE_DOWN"}),
			defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 5}}}),
		},
		apiErrorsByRef: map[string]error{"user-profile-cache": errors.New("cache still unavailable")},
	}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status == marketing.WorkflowNodeFailed {
		t.Fatalf("expected cache error fallback success, got %+v", result.Result)
	}
	if !reflect.DeepEqual(operator.calledRefs, []string{"user-profile-cache", defaultUserProfileProvider}) {
		t.Fatalf("expected cache then primary, got %v", operator.calledRefs)
	}
	if operator.apiCalls != 1 {
		t.Fatalf("expected write-back attempt after primary fallback, got %d", operator.apiCalls)
	}
	if !cacheAsideTraceHasWriteBackStatus(result.Traces, "FAILED") {
		t.Fatalf("expected write-back warning when cache remains unavailable, got %+v", result.Traces)
	}
	fallbacks, err := service.ListProviderDecisionAuditLogs(ctx, marketing.ProviderDecisionAuditQuery{TenantID: runtime.Activity.TenantID, Decision: marketing.ProviderDecisionFallback, ProviderRef: "user-profile-cache"})
	if err != nil {
		t.Fatalf("list fallback decision audit: %v", err)
	}
	if len(fallbacks) != 1 || fallbacks[0].Reason != "CACHE_DOWN" {
		t.Fatalf("expected cache error fallback audit, got %+v", fallbacks)
	}
}

func TestProviderChainExecutorCacheAsideErrorFail(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"cache":true,"cacheErrorPolicy":"FAIL"}`))
	operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
		"user-profile-cache":       nodeFailed("cache unavailable", map[string]any{"errorCode": "CACHE_DOWN"}),
		defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 5}}}),
	}}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected cache error fail, got %+v", result.Result)
	}
	if !reflect.DeepEqual(operator.calledRefs, []string{"user-profile-cache"}) {
		t.Fatalf("expected cache only after FAIL policy, got %v", operator.calledRefs)
	}
}

func TestProviderChainExecutorCacheAsideWriteBackEmptyPayloadWarnsByDefaultAndFailsWhenStrict(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name      string
		config    json.RawMessage
		wantError bool
	}{
		{
			name:   "default warning",
			config: json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackValuePath":"$.meta.empty"}`),
		},
		{
			name:      "strict failure",
			config:    json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackValuePath":"$.meta.empty","writeBackRequired":true}`),
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, runtime := newCacheAsideProviderChainFixture(t, ctx, tt.config)
			operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
				"user-profile-cache": nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
				defaultUserProfileProvider: nodeSuccess(map[string]any{
					"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 6}},
					"meta":  map[string]any{"empty": map[string]any{}},
				}),
			}}
			result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
			if tt.wantError {
				if result.Result.Status != marketing.WorkflowNodeFailed || workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_CACHE_WRITE_BACK_FAILED" {
					t.Fatalf("expected strict empty payload failure, got %+v", result.Result)
				}
			} else if result.Result.Status == marketing.WorkflowNodeFailed {
				t.Fatalf("expected empty payload warning only, got %+v", result.Result)
			}
			if operator.apiCalls != 0 {
				t.Fatalf("expected empty payload to fail before ExecuteResourceAPI, got %d calls", operator.apiCalls)
			}
			if !cacheAsideTraceHasWriteBackStatus(result.Traces, "FAILED") && !tt.wantError {
				t.Fatalf("expected empty payload warning trace, got %+v", result.Traces)
			}
		})
	}
}

func TestProviderChainExecutorCacheAsideWriteBackPayloadTooLargeWarnsByDefaultAndFailsWhenStrict(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name      string
		config    json.RawMessage
		wantError bool
	}{
		{
			name:   "default warning",
			config: json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackMaxBytes":8}`),
		},
		{
			name:      "strict failure",
			config:    json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackMaxBytes":8,"strict":true}`),
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, runtime := newCacheAsideProviderChainFixture(t, ctx, tt.config)
			operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
				"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
				defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"large": strings.Repeat("x", 64)}}}),
			}}
			result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
			if tt.wantError {
				if result.Result.Status != marketing.WorkflowNodeFailed || workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_CACHE_WRITE_BACK_FAILED" {
					t.Fatalf("expected strict payload-too-large failure, got %+v", result.Result)
				}
			} else if result.Result.Status == marketing.WorkflowNodeFailed {
				t.Fatalf("expected payload-too-large warning only, got %+v", result.Result)
			}
			if operator.apiCalls != 0 {
				t.Fatalf("expected oversized payload to fail before ExecuteResourceAPI, got %d calls", operator.apiCalls)
			}
			if !cacheAsideTraceHasWriteBackStatus(result.Traces, "FAILED") && !tt.wantError {
				t.Fatalf("expected payload-too-large warning trace, got %+v", result.Traces)
			}
		})
	}
}

func TestProviderChainExecutorCacheAsideWriteBackUsesResourceGovernance(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackResourceRef":"governed-cache","writeBackRequired":true}`))
	resource, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "governed-cache",
		Name:        "Governed Cache",
		Type:        marketing.ResourceMemory,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
	})
	if err != nil {
		t.Fatalf("save governed cache resource: %v", err)
	}
	if _, err := service.SaveResourceBinding(ctx, marketing.ResourceBinding{
		ResourceID:        resource.ID,
		TenantID:          runtime.Activity.TenantID,
		WorkspaceID:       runtime.Activity.WorkspaceID,
		NodeType:          "fact-query",
		AllowedOperations: []marketing.ResourceOperation{marketing.ResourceRead},
		AllowedNamespaces: []string{"facts"},
		Status:            marketing.ResourceBindingActive,
	}); err != nil {
		t.Fatalf("save denying resource binding: %v", err)
	}
	operator := &fakeServiceResourceOperator{
		resultsByRef: map[string]workflowNodeResult{
			"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
			defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 10}}}),
		},
		apiDelegate: newWorkflowResourceOperator(service),
	}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status != marketing.WorkflowNodeFailed || workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_CACHE_WRITE_BACK_FAILED" {
		t.Fatalf("expected strict governed write-back failure, got %+v", result.Result)
	}
	if operator.apiCalls != 1 || len(operator.apiRequests) != 1 {
		t.Fatalf("expected one real ExecuteResourceAPI attempt, got calls=%d requests=%+v", operator.apiCalls, operator.apiRequests)
	}
	summary := cacheAsideOutputWriteBackSummary(result.Result.Output)
	resourceOutput, _ := summary["resourceOutput"].(map[string]any)
	if workflowConfigString(summary, "resourceRef") != "governed-cache" || workflowConfigString(resourceOutput, "errorCode") != "RESOURCE_PERMISSION_DENIED" {
		t.Fatalf("expected Resource governance permission denial in write-back trace, summary=%+v", summary)
	}
}

func TestProviderChainExecutorCacheAsideWriteBackFailureWarnsByDefault(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"]}`))
	operator := &fakeServiceResourceOperator{
		resultsByRef: map[string]workflowNodeResult{
			"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
			defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 6}}}),
		},
		apiErrorsByRef: map[string]error{"user-profile-cache": errors.New("cache write unavailable")},
	}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status == marketing.WorkflowNodeFailed {
		t.Fatalf("expected default write-back failure to warn only, got %+v", result.Result)
	}
	if got := result.Facts["USER_PROFILE"].(map[string]any)["level"]; got != 6 {
		t.Fatalf("expected primary facts after write-back warning, got %+v", result.Facts)
	}
	if operator.apiCalls != 1 {
		t.Fatalf("expected one write-back attempt, got %d", operator.apiCalls)
	}
	if !cacheAsideTraceHasWriteBackStatus(result.Traces, "FAILED") {
		t.Fatalf("expected write-back warning trace, got %+v", result.Traces)
	}
}

func TestProviderChainExecutorCacheAsideStrictWriteBackFailureFails(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"],"writeBackRequired":true}`))
	operator := &fakeServiceResourceOperator{
		resultsByRef: map[string]workflowNodeResult{
			"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
			defaultUserProfileProvider: nodeSuccess(map[string]any{"facts": map[string]any{"USER_PROFILE": map[string]any{"level": 8}}}),
		},
		apiErrorsByRef: map[string]error{"user-profile-cache": errors.New("cache write unavailable")},
	}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected strict write-back failure to fail, got %+v", result.Result)
	}
	if workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_CACHE_WRITE_BACK_FAILED" {
		t.Fatalf("expected write-back failure code, got %+v", result.Result.Output)
	}
	if operator.apiCalls != 1 {
		t.Fatalf("expected one write-back attempt, got %d", operator.apiCalls)
	}
}

func TestProviderChainExecutorCacheAsideMissPrimaryOutputSchemaInvalidFailsBeforeWriteBack(t *testing.T) {
	ctx := context.Background()
	service, runtime := newCacheAsideProviderChainFixture(t, ctx, json.RawMessage(`{"role":"CACHE","missPath":"$.cache.hit","missValues":["false"]}`))
	operator := &fakeServiceResourceOperator{resultsByRef: map[string]workflowNodeResult{
		"user-profile-cache":       nodeSuccess(map[string]any{"cache": map[string]any{"hit": false}}),
		defaultUserProfileProvider: nodeSuccess(map[string]any{"token": "missing-facts"}),
	}}
	result := executeCacheAsideProviderChain(ctx, service, runtime, operator)
	if result.Result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected miss primary output schema failure, got %+v", result.Result)
	}
	if workflowConfigString(result.Result.Output, "errorCode") != "SERVICE_SCHEMA_OUTPUT_INVALID" {
		t.Fatalf("expected output schema failure code, got %+v", result.Result.Output)
	}
	if operator.apiCalls != 0 {
		t.Fatalf("expected no write-back before schema-valid primary output, got %d", operator.apiCalls)
	}
}

func newCacheAsideProviderChainFixture(t *testing.T, ctx context.Context, cacheConfig json.RawMessage) (*Service, workflowRuntime) {
	t.Helper()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveServiceProvider(ctx, marketing.ServiceProvider{
		TenantID:     runtime.Activity.TenantID,
		ServiceRef:   "user-profile-service",
		InterfaceRef: "query-user-profile",
		Code:         "user-profile-cache",
		Name:         "User profile cache",
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{"user-profile-cache"},
		Config:       cacheConfig,
		Priority:     0,
		Status:       marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save cache provider: %v", err)
	}
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"CACHE_ASIDE"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	return service, runtime
}

func executeCacheAsideProviderChain(ctx context.Context, service *Service, runtime workflowRuntime, operator *fakeServiceResourceOperator) providerChainServiceFactResult {
	return NewProviderChainExecutor(service, operator).ExecuteServiceInterfaceFactQuery(ctx, providerChainServiceFactRequest{
		Node:      marketing.WorkflowNode{ID: "fact", Type: "fact-query"},
		Runtime:   runtime,
		Config:    map[string]any{},
		Step:      workflowFactQueryProviderStep{Source: "SERVICE_INTERFACE", ServiceRef: "user-profile-service", InterfaceRef: "query-user-profile", ProviderPolicy: "CACHE_ASIDE"},
		Remaining: []string{"USER_PROFILE"},
		Facts:     map[string]any{},
	})
}

func cacheAsideTraceHasWriteBackStatus(traces []any, status string) bool {
	for _, raw := range traces {
		trace, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		output, _ := trace["output"].(map[string]any)
		writeBack, _ := output["cacheAsideWriteBack"].(map[string]any)
		if strings.EqualFold(workflowConfigString(writeBack, "status"), status) {
			return true
		}
	}
	return false
}

func cacheAsideOutputWriteBackSummary(output map[string]any) map[string]any {
	if summary, ok := output["cacheAsideWriteBack"].(map[string]any); ok {
		return summary
	}
	responses, _ := output["responses"].([]any)
	for _, raw := range responses {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if summary, ok := item["cacheAsideWriteBack"].(map[string]any); ok {
			return summary
		}
	}
	return nil
}
