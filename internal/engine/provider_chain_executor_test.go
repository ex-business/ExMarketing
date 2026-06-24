package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type fakeServiceResourceOperator struct {
	calls  int
	result workflowNodeResult
}

func (op *fakeServiceResourceOperator) ExecuteFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult {
	op.calls++
	if op.result.Output == nil {
		op.result.Output = map[string]any{}
	}
	return op.result
}

func (op *fakeServiceResourceOperator) ExecuteResourceAPI(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, req workflowResourceAPIRequest) (map[string]any, error) {
	op.calls++
	if op.result.Output == nil {
		op.result.Output = map[string]any{}
	}
	return op.result.Output, nil
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
		"facts":       map[string]any{"USER_PROFILE": map[string]any{"level": 7}},
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
}
