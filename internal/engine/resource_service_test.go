package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestResourceTestValidatesAdapterConfig(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)

	cases := []struct {
		name      string
		resource  marketing.Resource
		errorCode string
	}{
		{
			name: "database table requires table config",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_table",
				Name:     "Bad Table",
				Type:     marketing.ResourceDatabaseTable,
				Driver:   marketing.ResourceDriverPostgres,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"primaryKey":["tenantId"]}`),
			},
			errorCode: "RESOURCE_TABLE_CONFIG_INVALID",
		},
		{
			name: "database table requires primary key",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_table_pk",
				Name:     "Bad Table PK",
				Type:     marketing.ResourceDatabaseTable,
				Driver:   marketing.ResourceDriverPostgres,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"physicalTable":"marketing_user_state"}`),
			},
			errorCode: "RESOURCE_TABLE_PRIMARY_KEY_REQUIRED",
		},
		{
			name: "http provider requires endpoint",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_http",
				Name:     "Bad HTTP",
				Type:     marketing.ResourceHTTPProvider,
				Driver:   marketing.ResourceDriverREST,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"method":"POST"}`),
			},
			errorCode: "RESOURCE_HTTP_ENDPOINT_REQUIRED",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.SaveResource(ctx, tt.resource); err != nil {
				t.Fatalf("save resource: %v", err)
			}
			result, err := service.TestResource(ctx, tt.resource.TenantID, tt.resource.Code, "tester")
			if err != nil {
				t.Fatalf("test resource: %v", err)
			}
			if result["ok"] != false || result["errorCode"] != tt.errorCode {
				t.Fatalf("expected %s, got %+v", tt.errorCode, result)
			}
		})
	}
}

func TestResourceDefaultCapabilitiesIncludeAdapterOperations(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	table, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "user_state_table",
		Name:     "User State Table",
		Type:     marketing.ResourceDatabaseTable,
		Driver:   marketing.ResourceDriverPostgres,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"physicalTable":"marketing_user_state","primaryKey":["tenantId","subjectId"]}`),
	})
	if err != nil {
		t.Fatalf("save table resource: %v", err)
	}
	if !table.Supports(marketing.ResourceUpsert) || !table.Supports(marketing.ResourceCAS) {
		t.Fatalf("expected table upsert and cas capabilities, got %+v", table.Capabilities)
	}

	httpProvider, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "external_provider",
		Name:     "External Provider",
		Type:     marketing.ResourceHTTPProvider,
		Driver:   marketing.ResourceDriverREST,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"endpoint":"https://provider.example/state","method":"POST"}`),
	})
	if err != nil {
		t.Fatalf("save http provider resource: %v", err)
	}
	if !httpProvider.Supports(marketing.ResourceUpsert) || !httpProvider.Supports(marketing.ResourceCall) {
		t.Fatalf("expected http provider upsert and call capabilities, got %+v", httpProvider.Capabilities)
	}
}
