package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestSaveServiceProviderMaterializesBuiltInParents(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)

	_, err := service.SaveServiceProvider(ctx, marketing.ServiceProvider{
		TenantID:     "site_001",
		ServiceRef:   "user-profile-service",
		InterfaceRef: "query-user-profile",
		Code:         "custom-profile-provider",
		Name:         "Custom profile provider",
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{"user-profile-provider-v2"},
		Status:       marketing.ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save provider under built-in interface: %v", err)
	}
	materializedService, err := repo.GetServiceDefinition(ctx, "site_001", "user-profile-service")
	if err != nil {
		t.Fatalf("expected built-in service materialized: %v", err)
	}
	if !materializedService.Builtin || materializedService.CatalogVersion == "" || materializedService.SourceHash == "" {
		t.Fatalf("expected built-in service catalog trace fields, got %+v", materializedService)
	}
	materializedInterface, err := repo.GetServiceInterface(ctx, "site_001", "user-profile-service", "query-user-profile")
	if err != nil {
		t.Fatalf("expected built-in interface materialized: %v", err)
	}
	if !materializedInterface.Builtin || materializedInterface.CatalogVersion == "" || materializedInterface.SourceHash == "" {
		t.Fatalf("expected built-in interface catalog trace fields, got %+v", materializedInterface)
	}
}

func TestSaveServiceInterfaceBindingMaterializesBuiltInParents(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)

	saved, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                "site_001",
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeNode,
		WorkspaceID:             "growth",
		ActivityID:              "act_001",
		NodeID:                  "node_fact_query",
		PrincipalType:           marketing.ServicePrincipalNode,
		PrincipalID:             "node_fact_query",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save binding under built-in interface: %v", err)
	}
	if saved.ID == "" || saved.UpdatedAt.IsZero() {
		t.Fatalf("expected saved binding defaults, got %+v", saved)
	}
	if _, err := repo.GetServiceDefinition(ctx, "site_001", "user-profile-service"); err != nil {
		t.Fatalf("expected built-in service materialized: %v", err)
	}
	if _, err := repo.GetServiceInterface(ctx, "site_001", "user-profile-service", "query-user-profile"); err != nil {
		t.Fatalf("expected built-in interface materialized: %v", err)
	}

	items, err := service.ListServiceInterfaceBindings(ctx, marketing.ServiceInterfaceBindingQuery{
		TenantID:   "site_001",
		NodeType:   "fact-query",
		ActivityID: "act_001",
	})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(items) != 1 || items[0].ID != saved.ID {
		t.Fatalf("expected saved binding in list, got %+v", items)
	}
}

func TestSaveServiceSchemaDefaultsAndLists(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	saved, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "user-profile-output",
		SchemaType: marketing.ServiceSchemaOutput,
		SchemaJSON: []byte(`{"type":"object","properties":{"userId":{"type":"string"}}}`),
	})
	if err != nil {
		t.Fatalf("save schema: %v", err)
	}
	if saved.ID == "" || saved.Version != 1 || saved.Scope != marketing.ServiceAssetScopeTenant || saved.Status != marketing.ServiceAssetActive {
		t.Fatalf("expected schema defaults, got %+v", saved)
	}
	items, err := service.ListServiceSchemas(ctx, marketing.ServiceSchemaQuery{
		TenantID:   "site_001",
		SchemaRef:  "user-profile-output",
		SchemaType: marketing.ServiceSchemaOutput,
	})
	if err != nil {
		t.Fatalf("list schemas: %v", err)
	}
	if len(items) != 1 || items[0].ID != saved.ID {
		t.Fatalf("expected saved schema in list, got %+v", items)
	}
}

func TestSaveServiceSchemaRejectsIncompatibleVersion(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-output",
		SchemaType: marketing.ServiceSchemaOutput,
		Version:    1,
		SchemaJSON: []byte(`{"type":"object","required":["balance"],"properties":{"balance":{"type":"number"},"currency":{"type":"string"}}}`),
	}); err != nil {
		t.Fatalf("save v1 schema: %v", err)
	}

	_, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-output",
		SchemaType: marketing.ServiceSchemaOutput,
		Version:    2,
		SchemaJSON: []byte(`{"type":"object","required":["currency"],"properties":{"currency":{"type":"string"}}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_INCOMPATIBLE") {
		t.Fatalf("expected incompatible schema error, got %v", err)
	}
}

func TestSaveServiceSchemaRejectsUnsupportedKeyword(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	_, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-output",
		SchemaType: marketing.ServiceSchemaOutput,
		SchemaJSON: []byte(`{"type":"object","oneOf":[{"required":["balance"]}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_FORMAT_INVALID") {
		t.Fatalf("expected schema format error, got %v", err)
	}
}

func TestSaveServiceInterfaceRequiresActiveSchemas(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	if _, err := service.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		TenantID:    "site_001",
		Code:        "wallet-service",
		Name:        "Wallet Service",
		ServiceType: marketing.ServiceTypeExternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}

	_, err := service.SaveServiceInterface(ctx, marketing.ServiceInterface{
		TenantID:        "site_001",
		ServiceRef:      "wallet-service",
		Code:            "query-wallet",
		Name:            "Query Wallet",
		Operation:       marketing.ServiceOperationCall,
		InputSchemaRef:  "wallet-query-missing",
		OutputSchemaRef: "wallet-output-missing",
		Status:          marketing.ServiceAssetActive,
	})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_NOT_FOUND") {
		t.Fatalf("expected missing schema error, got %v", err)
	}
}

func TestServiceSchemaValidatorRejectsInputAndOutputPayloads(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-query",
		SchemaType: marketing.ServiceSchemaInput,
		SchemaJSON: []byte(`{"type":"object","required":["accountId"],"properties":{"accountId":{"type":"string"}}}`),
	}); err != nil {
		t.Fatalf("save input schema: %v", err)
	}
	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-output",
		SchemaType: marketing.ServiceSchemaOutput,
		SchemaJSON: []byte(`{"type":"object","required":["balance"],"properties":{"balance":{"type":"number"}}}`),
	}); err != nil {
		t.Fatalf("save output schema: %v", err)
	}
	serviceInterface := marketing.ServiceInterface{
		TenantID:        "site_001",
		ServiceRef:      "wallet-service",
		Code:            "query-wallet",
		InputSchemaRef:  "wallet-query",
		OutputSchemaRef: "wallet-output",
	}
	validator := serviceSchemaValidator{service: service}
	if err := validator.validateInterfaceInput(ctx, "site_001", serviceInterface, map[string]any{"accountId": 123}); err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_INPUT_INVALID") {
		t.Fatalf("expected input schema error, got %v", err)
	}
	if err := validator.validateInterfaceOutput(ctx, "site_001", serviceInterface, map[string]any{"balance": "12.00"}); err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_OUTPUT_INVALID") {
		t.Fatalf("expected output schema error, got %v", err)
	}
}

func TestServiceSchemaValidatorSupportsEnumPatternFormatAndBounds(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	validator := serviceSchemaValidator{service: service}
	schema := json.RawMessage(`{
		"type":"object",
		"required":["status","email","score"],
		"properties":{
			"status":{"type":"string","enum":["ACTIVE","LOCKED"]},
			"email":{"type":"string","format":"email"},
			"country":{"type":"string","pattern":"^[A-Z]{2}$"},
			"score":{"type":"number","minimum":0,"maximum":100}
		},
		"additionalProperties":false
	}`)
	if err := validator.validatePayload(schema, map[string]any{"status": "ACTIVE", "email": "a@example.com", "country": "BR", "score": 88}); err != nil {
		t.Fatalf("expected payload to validate: %v", err)
	}
	if err := validator.validatePayload(schema, map[string]any{"status": "PENDING", "email": "bad", "country": "brazil", "score": 101}); err == nil {
		t.Fatalf("expected enum/format/pattern/bounds error")
	}
}

func TestPublishWorkflowBlocksProviderResponseMappingOutsideOutputSchema(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_mapping_schema")
	mustSaveWalletServiceAssets(t, ctx, service, json.RawMessage(`{"responseMapping":{"missing":"$.value"}}`))
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                activity.TenantID,
		ServiceRef:              "wallet-service",
		InterfaceRef:            "query-wallet-balance",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	mustSaveServiceBindingWorkflowDraft(t, ctx, service, activity, `{"targetType":"SERVICE_INTERFACE","serviceRef":"wallet-service","interfaceRef":"query-wallet-balance","onMiss":"BLOCK"}`)

	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_RESPONSE_MAPPING_INVALID") {
		t.Fatalf("expected provider response mapping block, got %v", err)
	}
}

func TestPublishWorkflowBlocksDownstreamPathOutsideServiceOutputSchema(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_downstream_schema")
	mustSaveWalletServiceAssets(t, ctx, service, json.RawMessage(`{"targetResourceRef":"wallet-store"}`))
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                activity.TenantID,
		ServiceRef:              "wallet-service",
		InterfaceRef:            "query-wallet-balance",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	definition := json.RawMessage(`{
		"meta":{"activityId":"` + activity.ID + `","workspaceId":"workspace-1","folderId":"folder-1"},
		"nodes":[
			{"id":"event-in","type":"event-in","kind":"entry","position":{"x":0,"y":0},"outputs":["event"]},
			{"id":"fact","type":"fact-query","kind":"fact","position":{"x":200,"y":0},"inputs":["event"],"outputs":["facts"],"config":{"factKeys":["WALLET"],"providerChain":[{"targetType":"SERVICE_INTERFACE","serviceRef":"wallet-service","interfaceRef":"query-wallet-balance","onMiss":"BLOCK"}]}},
			{"id":"facts-normalize","type":"facts-normalize","kind":"fact","position":{"x":400,"y":0},"inputs":["facts"],"outputs":["standard_context"],"config":{"inputMapping":{"facts":{"sourceNodeId":"fact","outputKey":"facts","path":"$.wallet.missing"}}}}
		],
		"edges":[{"id":"event-fact","source":"event-in","target":"fact"},{"id":"fact-normalize","source":"fact","target":"facts-normalize"}]
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definition}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_DOWNSTREAM_FIELD_UNCOVERED") {
		t.Fatalf("expected downstream schema block, got %v", err)
	}
}

func TestPublishWorkflowBlocksServiceInterfaceMissingSchema(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:            "act_schema_publish",
		TenantID:      "site_001",
		Code:          "SCHEMA_PUBLISH",
		Name:          "Schema Publish",
		Type:          "FIRST_DEPOSIT",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(time.Hour),
		Timezone:      "UTC",
		Priority:      1,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	if _, err := repo.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		ID:          "svc_bad_schema",
		TenantID:    "site_001",
		Code:        "bad-schema-service",
		Name:        "Bad Schema Service",
		ServiceType: marketing.ServiceTypeExternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
		Metadata:    json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if _, err := repo.SaveServiceInterface(ctx, marketing.ServiceInterface{
		ID:              "svcif_bad_schema",
		TenantID:        "site_001",
		ServiceRef:      "bad-schema-service",
		Code:            "query-bad-schema",
		Name:            "Query Bad Schema",
		Operation:       marketing.ServiceOperationCall,
		InputSchemaRef:  "missing-input-schema",
		OutputSchemaRef: "missing-output-schema",
		Status:          marketing.ServiceAssetActive,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("seed interface: %v", err)
	}
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                "site_001",
		ServiceRef:              "bad-schema-service",
		InterfaceRef:            "query-bad-schema",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	definition := json.RawMessage(`{
		"meta":{"activityId":"act_schema_publish","workspaceId":"workspace-1","folderId":"folder-1"},
		"nodes":[
			{"id":"event-in","type":"event-in","kind":"entry","position":{"x":0,"y":0},"outputs":["event"]},
			{"id":"fact","type":"fact-query","kind":"fact","position":{"x":200,"y":0},"inputs":["event"],"outputs":["facts"],"config":{"factKeys":["USER_PROFILE"],"providerChain":[{"targetType":"SERVICE_INTERFACE","serviceRef":"bad-schema-service","interfaceRef":"query-bad-schema","onMiss":"BLOCK"}]}}
		],
		"edges":[{"id":"event-fact","source":"event-in","target":"fact"}]
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definition}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SCHEMA_NOT_FOUND") {
		t.Fatalf("expected missing schema publish block, got %v", err)
	}
}

func TestPublishWorkflowBlocksServiceInterfaceWithoutBinding(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_binding_missing")
	mustSaveServiceBindingWorkflowDraft(t, ctx, service, activity, `{"targetType":"SERVICE_INTERFACE","serviceRef":"user-profile-service","interfaceRef":"query-user-profile","onMiss":"BLOCK"}`)

	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_INTERFACE_BINDING_REQUIRED") {
		t.Fatalf("expected binding required publish block, got %v", err)
	}
}

func TestPublishWorkflowBlocksDenyServiceInterfaceBinding(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_binding_deny")
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:          activity.TenantID,
		ServiceRef:        "user-profile-service",
		InterfaceRef:      "query-user-profile",
		Scope:             marketing.ServiceAssetScopeActivity,
		ActivityID:        activity.ID,
		PrincipalType:     marketing.ServicePrincipalRole,
		PrincipalID:       "*",
		AllowedNodeTypes:  []string{"fact-query"},
		AllowedOperations: []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		Deny:              true,
		Status:            marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save deny binding: %v", err)
	}
	mustSaveServiceBindingWorkflowDraft(t, ctx, service, activity, `{"targetType":"SERVICE_INTERFACE","serviceRef":"user-profile-service","interfaceRef":"query-user-profile","onMiss":"BLOCK"}`)

	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_INTERFACE_ACCESS_DENIED") {
		t.Fatalf("expected deny binding publish block, got %v", err)
	}
}

func TestPublishWorkflowBlocksUnauthorizedExplicitProviderRef(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_binding_provider_ref")
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                activity.TenantID,
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	mustSaveServiceBindingWorkflowDraft(t, ctx, service, activity, `{"targetType":"SERVICE_INTERFACE","serviceRef":"user-profile-service","interfaceRef":"query-user-profile","providerRef":"user-profile-provider-primary","onMiss":"BLOCK"}`)

	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_REF_DENIED") {
		t.Fatalf("expected providerRef publish block, got %v", err)
	}
}

func TestPublishWorkflowBlocksSoftDeletedServiceInterface(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	activity := mustCreateServiceBindingActivity(t, ctx, repo, "act_soft_deleted_interface")
	mustSaveWalletServiceAssets(t, ctx, service, json.RawMessage(`{"targetResourceRef":"wallet-store"}`))
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                activity.TenantID,
		ServiceRef:              "wallet-service",
		InterfaceRef:            "query-wallet-balance",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	deletedAt := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	serviceInterface, err := service.GetServiceInterface(ctx, activity.TenantID, "wallet-service", "query-wallet-balance")
	if err != nil {
		t.Fatalf("get interface: %v", err)
	}
	serviceInterface.DeletedAt = &deletedAt
	if _, err := service.SaveServiceInterface(ctx, serviceInterface); err != nil {
		t.Fatalf("soft delete interface: %v", err)
	}
	mustSaveServiceBindingWorkflowDraft(t, ctx, service, activity, `{"targetType":"SERVICE_INTERFACE","serviceRef":"wallet-service","interfaceRef":"query-wallet-balance","onMiss":"BLOCK"}`)

	_, err = service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil || !strings.Contains(err.Error(), "SERVICE_INTERFACE_DISABLED") {
		t.Fatalf("expected soft-deleted interface publish block, got %v", err)
	}
}

func TestRuntimeBlocksServiceInterfaceWithoutBinding(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	activity := marketing.Activity{ID: "act_runtime_binding_missing", TenantID: "site_001", WorkspaceID: "workspace-1", FolderID: "folder-1"}
	node := marketing.WorkflowNode{
		ID:   "fact",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"providerChain":[{"targetType":"SERVICE_INTERFACE","serviceRef":"user-profile-service","interfaceRef":"query-user-profile","onMiss":"BLOCK"}]
		}`),
	}
	config := workflowNodeConfigMap(node)
	result := service.factQueryProviderChainWorkflowNodeHandler(ctx, node, workflowRuntime{Activity: activity}, config, workflowFactQueryProviderChain(config))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "SERVICE_INTERFACE_BINDING_REQUIRED" {
		t.Fatalf("expected runtime binding block, got status=%s output=%+v", result.Status, result.Output)
	}
}

func TestServiceInterfaceAccessAllowsMatchingBinding(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                 "site_001",
		ServiceRef:               "user-profile-service",
		InterfaceRef:             "query-user-profile",
		Scope:                    marketing.ServiceAssetScopeNode,
		WorkspaceID:              "workspace-1",
		FolderID:                 "folder-1",
		ActivityID:               "act_binding_allowed",
		NodeID:                   "fact",
		PrincipalType:            marketing.ServicePrincipalNode,
		PrincipalID:              "fact",
		AllowedNodeTypes:         []string{"fact-query"},
		AllowedOperations:        []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies:  []string{"PRIMARY"},
		AllowExplicitProviderRef: true,
		AllowedProviderRefs:      []string{"user-profile-provider-primary"},
		Status:                   marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	decision, err := service.checkServiceInterfaceAccess(ctx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		FolderID:       "folder-1",
		ActivityID:     "act_binding_allowed",
		NodeID:         "fact",
		NodeType:       "fact-query",
		ServiceRef:     "user-profile-service",
		InterfaceRef:   "query-user-profile",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
		ProviderRef:    "user-profile-provider-primary",
	})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if !decision.Allowed || len(decision.MatchedBindingIDs) != 1 {
		t.Fatalf("expected matching binding allowed, got %+v", decision)
	}
}

func TestServiceInterfaceAccessRejectsSoftDeletedInterface(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                "site_001",
		ServiceRef:              "wallet-service",
		InterfaceRef:            "query-wallet-balance",
		Scope:                   marketing.ServiceAssetScopeWorkspace,
		WorkspaceID:             "workspace-1",
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	deletedAt := time.Date(2026, 6, 26, 10, 30, 0, 0, time.UTC)
	serviceInterface, err := service.GetServiceInterface(ctx, "site_001", "wallet-service", "query-wallet-balance")
	if err != nil {
		t.Fatalf("get interface: %v", err)
	}
	serviceInterface.DeletedAt = &deletedAt
	if _, err := service.SaveServiceInterface(ctx, serviceInterface); err != nil {
		t.Fatalf("soft delete interface: %v", err)
	}

	decision, err := service.checkServiceInterfaceAccess(ctx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		NodeType:       "fact-query",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
	})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if decision.Allowed || decision.Code != "SERVICE_INTERFACE_DISABLED" {
		t.Fatalf("expected soft-deleted interface denied, got %+v", decision)
	}
}

func TestServiceInterfaceBindingLongTermFields(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	deletedAt := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                "site_001",
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeWorkspace,
		WorkspaceID:             "workspace-1",
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "VIEWER",
		MinRole:                 marketing.WorkspaceRoleEditor,
		Environment:             "prod",
		RateLimitRef:            "rl-profile-query",
		IncludeSubfolders:       true,
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	items, err := service.ListServiceInterfaceBindings(ctx, marketing.ServiceInterfaceBindingQuery{
		TenantID:     "site_001",
		ServiceRef:   "user-profile-service",
		InterfaceRef: "query-user-profile",
		Environment:  "prod",
	})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(items) != 1 || items[0].MinRole != marketing.WorkspaceRoleEditor || items[0].RateLimitRef != "rl-profile-query" || !items[0].IncludeSubfolders {
		t.Fatalf("expected long-term fields round-tripped, got %+v", items)
	}

	viewerCtx := marketing.WithAuditSource(ctx, map[string]any{"roles": []string{"VIEWER"}})
	decision, err := service.checkServiceInterfaceAccess(viewerCtx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		NodeType:       "fact-query",
		ServiceRef:     "user-profile-service",
		InterfaceRef:   "query-user-profile",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
		Environment:    "prod",
	})
	if err != nil {
		t.Fatalf("check viewer access: %v", err)
	}
	if decision.Allowed || decision.Code != "SERVICE_INTERFACE_ROLE_DENIED" {
		t.Fatalf("expected viewer below minRole denied, got %+v", decision)
	}

	adminCtx := marketing.WithAuditSource(ctx, map[string]any{"roles": []string{"ADMIN"}})
	decision, err = service.checkServiceInterfaceAccess(adminCtx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		NodeType:       "fact-query",
		ServiceRef:     "user-profile-service",
		InterfaceRef:   "query-user-profile",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
		Environment:    "dev",
	})
	if err != nil {
		t.Fatalf("check environment access: %v", err)
	}
	if decision.Allowed || decision.Code != "SERVICE_INTERFACE_ENVIRONMENT_DENIED" {
		t.Fatalf("expected environment mismatch denied, got %+v", decision)
	}

	decision, err = service.checkServiceInterfaceAccess(adminCtx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		NodeType:       "fact-query",
		ServiceRef:     "user-profile-service",
		InterfaceRef:   "query-user-profile",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
		Environment:    "prod",
	})
	if err != nil {
		t.Fatalf("check admin access: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected admin role to satisfy minRole, got %+v", decision)
	}

	deleted := items[0]
	deleted.DeletedAt = &deletedAt
	if _, err := service.SaveServiceInterfaceBinding(ctx, deleted); err != nil {
		t.Fatalf("save deleted binding: %v", err)
	}
	decision, err = service.checkServiceInterfaceAccess(adminCtx, serviceInterfaceAccessRequest{
		TenantID:       "site_001",
		WorkspaceID:    "workspace-1",
		NodeType:       "fact-query",
		ServiceRef:     "user-profile-service",
		InterfaceRef:   "query-user-profile",
		Operation:      marketing.ServiceOperationCall,
		ProviderPolicy: "PRIMARY",
		Environment:    "prod",
	})
	if err != nil {
		t.Fatalf("check deleted binding access: %v", err)
	}
	if decision.Allowed || decision.Code != "SERVICE_INTERFACE_BINDING_REQUIRED" {
		t.Fatalf("expected soft-deleted binding ignored, got %+v", decision)
	}
}

func mustCreateServiceBindingActivity(t *testing.T, ctx context.Context, repo marketing.Repository, id string) marketing.Activity {
	t.Helper()
	now := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:            id,
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          strings.ToUpper(id),
		Name:          id,
		Type:          "FIRST_DEPOSIT",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(time.Hour),
		Timezone:      "UTC",
		Priority:      1,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	return activity
}

func mustSaveServiceBindingWorkflowDraft(t *testing.T, ctx context.Context, service *Service, activity marketing.Activity, providerStep string) {
	t.Helper()
	definition := json.RawMessage(`{
		"meta":{"activityId":"` + activity.ID + `","workspaceId":"workspace-1","folderId":"folder-1"},
		"nodes":[
			{"id":"event-in","type":"event-in","kind":"entry","position":{"x":0,"y":0},"outputs":["event"]},
			{"id":"fact","type":"fact-query","kind":"fact","position":{"x":200,"y":0},"inputs":["event"],"outputs":["facts"],"config":{"factKeys":["USER_PROFILE"],"providerChain":[` + providerStep + `]}}
		],
		"edges":[{"id":"event-fact","source":"event-in","target":"fact"}]
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definition}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
}

func mustSaveWalletServiceAssets(t *testing.T, ctx context.Context, service *Service, providerConfig json.RawMessage) {
	t.Helper()
	if _, err := service.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		TenantID:    "site_001",
		Code:        "wallet-service",
		Name:        "Wallet Service",
		ServiceType: marketing.ServiceTypeExternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-query",
		SchemaType: marketing.ServiceSchemaInput,
		SchemaJSON: json.RawMessage(`{"type":"object","required":["factKeys"],"properties":{"factKeys":{"type":"array","items":{"type":"string"}}}}`),
	}); err != nil {
		t.Fatalf("save input schema: %v", err)
	}
	if _, err := service.SaveServiceSchema(ctx, marketing.ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "wallet-output",
		SchemaType: marketing.ServiceSchemaOutput,
		SchemaJSON: json.RawMessage(`{"type":"object","required":["facts"],"properties":{"facts":{"type":"object","required":["wallet"],"properties":{"wallet":{"type":"object","required":["balance"],"properties":{"balance":{"type":"number"}}}}}}}`),
	}); err != nil {
		t.Fatalf("save output schema: %v", err)
	}
	if _, err := service.SaveServiceInterface(ctx, marketing.ServiceInterface{
		TenantID:        "site_001",
		ServiceRef:      "wallet-service",
		Code:            "query-wallet-balance",
		Name:            "Query Wallet Balance",
		Operation:       marketing.ServiceOperationCall,
		InputSchemaRef:  "wallet-query",
		OutputSchemaRef: "wallet-output",
		Status:          marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save interface: %v", err)
	}
	if len(providerConfig) == 0 {
		providerConfig = json.RawMessage(`{"targetResourceRef":"wallet-store"}`)
	}
	if _, err := service.SaveServiceProvider(ctx, marketing.ServiceProvider{
		TenantID:     "site_001",
		ServiceRef:   "wallet-service",
		InterfaceRef: "query-wallet-balance",
		Code:         "wallet-provider",
		Name:         "Wallet Provider",
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{"wallet-store"},
		Config:       providerConfig,
		Priority:     10,
		Weight:       100,
		Status:       marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save provider: %v", err)
	}
}

func TestResolveServiceProviderRejectsUnsupportedPolicy(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	_, _, err := service.ResolveServiceProvider(ctx, "site_001", "user-profile-service", "query-user-profile", "", "RANDOM")
	if err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_POLICY_UNSUPPORTED") {
		t.Fatalf("expected unsupported policy error, got %v", err)
	}
}

func TestResolveServiceProviderRejectsSoftDeletedInterfaceAndExplicitProvider(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-soft-delete", 0, 100, nil)

	deletedAt := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	provider.DeletedAt = &deletedAt
	if _, err := service.SaveServiceProvider(ctx, provider); err != nil {
		t.Fatalf("soft delete provider: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-soft-delete", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_DISABLED") {
		t.Fatalf("expected soft-deleted explicit provider rejected, got %v", err)
	}

	serviceInterface, err := service.GetServiceInterface(ctx, "site_001", "wallet-service", "query-wallet-balance")
	if err != nil {
		t.Fatalf("get interface: %v", err)
	}
	serviceInterface.DeletedAt = &deletedAt
	if _, err := service.SaveServiceInterface(ctx, serviceInterface); err != nil {
		t.Fatalf("soft delete interface: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_INTERFACE_DISABLED") {
		t.Fatalf("expected soft-deleted interface rejected, got %v", err)
	}
}

func TestResolveServiceProviderSupportsGovernedPolicies(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	mustSaveServiceAssetFixture(t, ctx, service)

	for _, provider := range []marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet-balance",
			Code:         "wallet-fallback",
			Name:         "Wallet fallback",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"wallet-store-fallback"},
			Priority:     20,
			Weight:       10,
			Status:       marketing.ServiceAssetActive,
		},
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet-balance",
			Code:         "wallet-primary",
			Name:         "Wallet primary",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"wallet-store-primary"},
			Priority:     10,
			Weight:       100,
			Status:       marketing.ServiceAssetActive,
		},
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet-balance",
			Code:         "wallet-zero-weight",
			Name:         "Wallet zero weight",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"wallet-store-zero"},
			Priority:     0,
			Weight:       0,
			Status:       marketing.ServiceAssetActive,
		},
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet-balance",
			Code:         "wallet-mock",
			Name:         "Wallet mock",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"wallet-store-mock"},
			Config:       []byte(`{"mockEnabled":true}`),
			Priority:     100,
			Weight:       1,
			Status:       marketing.ServiceAssetActive,
		},
	} {
		if _, err := service.SaveServiceProvider(ctx, provider); err != nil {
			t.Fatalf("save provider %s: %v", provider.Code, err)
		}
	}

	_, primary, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "", "PRIMARY_WITH_FALLBACK")
	if err != nil {
		t.Fatalf("resolve PRIMARY_WITH_FALLBACK: %v", err)
	}
	if primary.Code != "wallet-zero-weight" {
		t.Fatalf("expected lowest priority provider for PRIMARY_WITH_FALLBACK, got %s", primary.Code)
	}

	_, weighted, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "", "WEIGHTED")
	if err != nil {
		t.Fatalf("resolve WEIGHTED: %v", err)
	}
	if weighted.Code != "wallet-primary" {
		t.Fatalf("expected positive weight provider ordered by priority/weight, got %s", weighted.Code)
	}

	_, mock, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "", "MOCK_ONLY")
	if err != nil {
		t.Fatalf("resolve MOCK_ONLY: %v", err)
	}
	if mock.Code != "wallet-mock" {
		t.Fatalf("expected mock provider, got %s", mock.Code)
	}
}

func TestResolveServiceProviderWeightedUsesStableHash(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-weight-a", 0, 3, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-weight-b", 0, 1, nil)

	req := serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "WEIGHTED",
		SeedParts:      []string{"act_weight", "node_weight", "event_42"},
	}
	_, first, err := service.resolveServiceProviders(ctx, req)
	if err != nil {
		t.Fatalf("first weighted resolve: %v", err)
	}
	_, second, err := service.resolveServiceProviders(ctx, req)
	if err != nil {
		t.Fatalf("second weighted resolve: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Code != second[0].Code {
		t.Fatalf("expected stable weighted selection, got first=%+v second=%+v", first, second)
	}

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		req.SeedParts = []string{"act_weight", "node_weight", fmt.Sprintf("event_%03d", i)}
		_, selected, err := service.resolveServiceProviders(ctx, req)
		if err != nil {
			t.Fatalf("weighted resolve %d: %v", i, err)
		}
		counts[selected[0].Code]++
	}
	if counts["wallet-weight-a"] <= counts["wallet-weight-b"]*2 {
		t.Fatalf("expected weighted distribution to favor wallet-weight-a, got %+v", counts)
	}
	if counts["wallet-weight-a"] == 0 || counts["wallet-weight-b"] == 0 {
		t.Fatalf("expected both weighted providers to be selected across seeds, got %+v", counts)
	}
}

func TestResolveServiceProviderEnvironmentMatchAndFallback(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-default", 20, 10, nil)
	prod := mustSaveWalletProvider(t, ctx, service, "wallet-prod", 10, 10, nil)
	prod.Environment = "prod"
	if _, err := service.SaveServiceProvider(ctx, prod); err != nil {
		t.Fatalf("save prod provider: %v", err)
	}
	dev := mustSaveWalletProvider(t, ctx, service, "wallet-dev", 0, 10, nil)
	dev.Environment = "dev"
	if _, err := service.SaveServiceProvider(ctx, dev); err != nil {
		t.Fatalf("save dev provider: %v", err)
	}

	_, providers, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "ENVIRONMENT_MATCH",
		Environment:    "prod",
	})
	if err != nil {
		t.Fatalf("resolve prod: %v", err)
	}
	if len(providers) != 1 || providers[0].Code != "wallet-prod" {
		t.Fatalf("expected prod provider only, got %+v", providers)
	}

	_, fallback, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "ENVIRONMENT_MATCH",
		Environment:    "staging",
	})
	if err != nil {
		t.Fatalf("resolve staging fallback: %v", err)
	}
	if len(fallback) != 1 || fallback[0].Code != "wallet-default" {
		t.Fatalf("expected default fallback provider, got %+v", fallback)
	}

	if _, _, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderRef:    "wallet-dev",
		ProviderPolicy: "PRIMARY",
		Environment:    "prod",
	}); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_ENVIRONMENT_DENIED") {
		t.Fatalf("expected environment denied for explicit dev provider, got %v", err)
	}
}

func TestResolveServiceProviderMockPolicy(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-real", 10, 10, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-mock-config", 0, 10, json.RawMessage(`{"mockEnabled":true}`))
	mockEnv := mustSaveWalletProvider(t, ctx, service, "wallet-mock-env", 5, 10, nil)
	mockEnv.Environment = "mock"
	if _, err := service.SaveServiceProvider(ctx, mockEnv); err != nil {
		t.Fatalf("save mock env provider: %v", err)
	}

	_, primary, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "PRIMARY_WITH_FALLBACK",
	})
	if err != nil {
		t.Fatalf("resolve primary fallback: %v", err)
	}
	for _, provider := range primary {
		if serviceProviderMockEnabled(provider) {
			t.Fatalf("non-mock policy selected mock provider: %+v", primary)
		}
	}

	_, mock, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "MOCK_ONLY",
	})
	if err != nil {
		t.Fatalf("resolve mock only: %v", err)
	}
	if len(mock) != 1 || !serviceProviderMockEnabled(mock[0]) {
		t.Fatalf("expected mock provider only, got %+v", mock)
	}
	if _, _, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderRef:    "wallet-real",
		ProviderPolicy: "MOCK_ONLY",
	}); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_MOCK_REQUIRED") {
		t.Fatalf("expected mock required for explicit real provider, got %v", err)
	}
}

func TestResolveServiceProviderCacheAsidePolicy(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 10, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-cache-role", 20, 10, json.RawMessage(`{"role":"CACHE"}`))
	cacheEnv := mustSaveWalletProvider(t, ctx, service, "wallet-cache-env", 10, 10, nil)
	cacheEnv.Environment = "cache"
	if _, err := service.SaveServiceProvider(ctx, cacheEnv); err != nil {
		t.Fatalf("save cache env provider: %v", err)
	}

	_, providers, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "CACHE_ASIDE",
	})
	if err != nil {
		t.Fatalf("resolve cache aside: %v", err)
	}
	got := []string{}
	for _, provider := range providers {
		got = append(got, provider.Code)
	}
	want := []string{"wallet-cache-env", "wallet-cache-role", "wallet-primary"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cache-aside order mismatch: got %v want %v", got, want)
	}

	cacheEnv.Status = marketing.ServiceAssetDisabled
	if _, err := service.SaveServiceProvider(ctx, cacheEnv); err != nil {
		t.Fatalf("disable cache env provider: %v", err)
	}
	cacheRole, err := service.GetServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-cache-role")
	if err != nil {
		t.Fatalf("get cache role provider: %v", err)
	}
	cacheRole.Status = marketing.ServiceAssetDisabled
	if _, err := service.SaveServiceProvider(ctx, cacheRole); err != nil {
		t.Fatalf("disable cache role provider: %v", err)
	}
	_, primaryOnly, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "CACHE_ASIDE",
	})
	if err != nil {
		t.Fatalf("resolve cache aside primary only: %v", err)
	}
	if len(primaryOnly) != 1 || primaryOnly[0].Code != "wallet-primary" {
		t.Fatalf("expected primary fallback without cache provider, got %+v", primaryOnly)
	}
}

func TestResolveServiceProviderFallbackOrderStable(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-b", 10, 20, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-a", 10, 20, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-c", 20, 100, nil)

	_, providers, err := service.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       "site_001",
		ServiceRef:     "wallet-service",
		InterfaceRef:   "query-wallet-balance",
		ProviderPolicy: "PRIMARY_WITH_FALLBACK",
	})
	if err != nil {
		t.Fatalf("resolve fallback: %v", err)
	}
	got := []string{}
	for _, provider := range providers {
		got = append(got, provider.Code)
	}
	want := []string{"wallet-a", "wallet-b", "wallet-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback order mismatch: got %v want %v", got, want)
	}
}

func TestResolveServiceProviderSkipsUnhealthyFallbackCandidate(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 100, nil)
	mustSaveWalletProvider(t, ctx, service, "wallet-fallback", 10, 10, nil)

	if _, err := service.SaveServiceProviderRuntimeState(ctx, marketing.ServiceProviderRuntimeState{
		TenantID:      "site_001",
		ServiceRef:    "wallet-service",
		InterfaceRef:  "query-wallet-balance",
		ProviderRef:   "wallet-primary",
		HealthStatus:  marketing.ServiceProviderHealthUnhealthy,
		HealthMessage: "health check failed",
		CircuitStatus: marketing.ServiceProviderCircuitClosed,
	}); err != nil {
		t.Fatalf("save provider state: %v", err)
	}

	_, provider, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "", "PRIMARY_WITH_FALLBACK")
	if err != nil {
		t.Fatalf("resolve with fallback: %v", err)
	}
	if provider.Code != "wallet-fallback" {
		t.Fatalf("expected fallback provider, got %s", provider.Code)
	}
}

func TestResolveServiceProviderBlocksOpenCircuitAndResetAllowsCall(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 100, []byte(`{"circuitFailureThreshold":1}`))

	if err := service.recordServiceProviderFailure(ctx, provider, "boom"); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_CIRCUIT_OPEN") {
		t.Fatalf("expected circuit open error, got %v", err)
	}

	state, err := service.ResetServiceProviderCircuit(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary")
	if err != nil {
		t.Fatalf("reset circuit: %v", err)
	}
	if state.CircuitStatus != marketing.ServiceProviderCircuitClosed || state.ConsecutiveFailures != 0 {
		t.Fatalf("expected closed circuit after reset, got %+v", state)
	}
	_, resolved, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary", "PRIMARY")
	if err != nil {
		t.Fatalf("resolve after reset: %v", err)
	}
	if resolved.Code != "wallet-primary" {
		t.Fatalf("expected wallet-primary, got %s", resolved.Code)
	}
}

func TestProviderRuntimeOperationsRejectSoftDeletedProvider(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-runtime-deleted", 0, 100, nil)
	deletedAt := time.Date(2026, 6, 26, 11, 30, 0, 0, time.UTC)
	provider.DeletedAt = &deletedAt
	if _, err := service.SaveServiceProvider(ctx, provider); err != nil {
		t.Fatalf("soft delete provider: %v", err)
	}

	if _, err := service.ResetServiceProviderCircuit(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-runtime-deleted"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_DISABLED") {
		t.Fatalf("expected reset circuit to reject soft-deleted provider, got %v", err)
	}
	if _, err := service.CheckServiceProviderHealth(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-runtime-deleted", "tester"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_DISABLED") {
		t.Fatalf("expected health check to reject soft-deleted provider, got %v", err)
	}
}

func TestServiceProviderRateLimitBlocksProvider(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 100, []byte(`{"rateLimitPerWindow":1,"rateLimitWindowSeconds":60}`))

	if _, err := service.consumeServiceProviderRuntime(ctx, provider); err != nil {
		t.Fatalf("first consume should pass: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_RATE_LIMITED") {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}

func TestServiceProviderRateLimitPolicyOverridesProviderConfig(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 100, []byte(`{"rateLimitPerWindow":100,"rateLimitWindowSeconds":60}`))
	if _, err := service.SaveServiceRateLimitPolicy(ctx, marketing.ServiceRateLimitPolicy{
		TenantID:      "site_001",
		Code:          "wallet-primary-limit",
		Name:          "Wallet primary limit",
		ServiceRef:    "wallet-service",
		InterfaceRef:  "query-wallet-balance",
		ProviderRef:   "wallet-primary",
		Limit:         1,
		WindowSeconds: 60,
		Backend:       marketing.ServiceRateLimitBackendPostgres,
		Status:        marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save rate limit policy: %v", err)
	}

	if _, err := service.consumeServiceProviderRuntime(ctx, provider); err != nil {
		t.Fatalf("first consume should pass: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_RATE_LIMITED") {
		t.Fatalf("expected policy rate limit error, got %v", err)
	}
}

func TestServiceProviderRateLimitFallsBackToProviderConfigWithoutPolicy(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-primary", 0, 100, []byte(`{"rateLimitPerWindow":1,"rateLimitWindowSeconds":60}`))

	if _, err := service.consumeServiceProviderRuntime(ctx, provider); err != nil {
		t.Fatalf("first consume should pass: %v", err)
	}
	if _, _, err := service.ResolveServiceProvider(ctx, "site_001", "wallet-service", "query-wallet-balance", "wallet-primary", "PRIMARY"); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_RATE_LIMITED") {
		t.Fatalf("expected provider config rate limit error, got %v", err)
	}
}

func TestServiceProviderHalfOpenProbeLimitAndSuccessThreshold(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	mustSaveServiceAssetFixture(t, ctx, service)
	provider := mustSaveWalletProvider(t, ctx, service, "wallet-half-open", 0, 100, []byte(`{
		"circuitFailureThreshold":1,
		"circuitCooldownSeconds":1,
		"halfOpenProbeLimit":1,
		"halfOpenSuccessThreshold":2
	}`))

	if err := service.recordServiceProviderFailure(ctx, provider, "boom"); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	now = now.Add(2 * time.Second)
	state, err := service.consumeServiceProviderRuntime(ctx, provider)
	if err != nil {
		t.Fatalf("first half-open probe should pass: %v", err)
	}
	if state.CircuitStatus != marketing.ServiceProviderCircuitHalfOpen || state.HalfOpenProbeCount != 1 {
		t.Fatalf("expected half-open first probe, got %+v", state)
	}
	if _, err := service.consumeServiceProviderRuntime(ctx, provider); err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_HALF_OPEN_PROBE_LIMITED") {
		t.Fatalf("expected half-open probe limit, got %v", err)
	}
	if err := service.recordServiceProviderSuccess(ctx, provider); err != nil {
		t.Fatalf("record first success: %v", err)
	}
	state, err = service.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		t.Fatalf("get state after first success: %v", err)
	}
	if state.CircuitStatus != marketing.ServiceProviderCircuitHalfOpen || state.HalfOpenSuccessCount != 1 {
		t.Fatalf("expected half-open until success threshold, got %+v", state)
	}
	if err := service.recordServiceProviderSuccess(ctx, provider); err != nil {
		t.Fatalf("record second success: %v", err)
	}
	state, err = service.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		t.Fatalf("get state after threshold: %v", err)
	}
	if state.CircuitStatus != marketing.ServiceProviderCircuitClosed || state.HalfOpenProbeCount != 0 || state.HalfOpenSuccessCount != 0 {
		t.Fatalf("expected closed circuit after success threshold, got %+v", state)
	}
}

func mustSaveServiceAssetFixture(t *testing.T, ctx context.Context, service *Service) {
	t.Helper()
	if _, err := service.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		TenantID:    "site_001",
		Code:        "wallet-service",
		Name:        "Wallet Service",
		ServiceType: marketing.ServiceTypeInternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if _, err := service.SaveServiceInterface(ctx, marketing.ServiceInterface{
		TenantID:    "site_001",
		ServiceRef:  "wallet-service",
		Code:        "query-wallet-balance",
		Name:        "Query wallet balance",
		Operation:   marketing.ServiceOperationCall,
		TimeoutMS:   1000,
		LuaCallable: false,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save interface: %v", err)
	}
}

func mustSaveWalletProvider(t *testing.T, ctx context.Context, service *Service, code string, priority int, weight int, config json.RawMessage) marketing.ServiceProvider {
	t.Helper()
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	provider, err := service.SaveServiceProvider(ctx, marketing.ServiceProvider{
		TenantID:     "site_001",
		ServiceRef:   "wallet-service",
		InterfaceRef: "query-wallet-balance",
		Code:         code,
		Name:         code,
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{code + "-resource"},
		Config:       config,
		Priority:     priority,
		Weight:       weight,
		Status:       marketing.ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save provider %s: %v", code, err)
	}
	return provider
}
