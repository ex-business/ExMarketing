package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{
			name: "redis resource requires key prefix",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_redis",
				Name:     "Bad Redis",
				Type:     marketing.ResourceRedisKV,
				Driver:   marketing.ResourceDriverInternal,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{}`),
			},
			errorCode: "RESOURCE_REDIS_KEY_PREFIX_REQUIRED",
		},
		{
			name: "mq topic requires topic",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_mq",
				Name:     "Bad MQ",
				Type:     marketing.ResourceMQTopic,
				Driver:   marketing.ResourceDriverInternal,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"provider":"KAFKA"}`),
			},
			errorCode: "RESOURCE_MQ_TOPIC_REQUIRED",
		},
		{
			name: "database table rejects invalid schema column type",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_table_schema",
				Name:     "Bad Table Schema",
				Type:     marketing.ResourceDatabaseTable,
				Driver:   marketing.ResourceDriverPostgres,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"physicalTable":"marketing_user_state","primaryKey":["tenantId"],"schema":{"columns":{"tenantId":{"type":"STRING"},"bad":{"type":"BINARY"}}}}`),
			},
			errorCode: "RESOURCE_TABLE_SCHEMA_INVALID",
		},
		{
			name: "database table rejects invalid key mapping",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_table_mapping",
				Name:     "Bad Table Mapping",
				Type:     marketing.ResourceDatabaseTable,
				Driver:   marketing.ResourceDriverPostgres,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"physicalTable":"marketing_user_state","primaryKey":["tenantId"],"keyMapping":{"tenantId":{"unknown":"$.tenantId"}}}`),
			},
			errorCode: "RESOURCE_TABLE_KEY_MAPPING_INVALID",
		},
		{
			name: "database table rejects uncovered primary key",
			resource: marketing.Resource{
				TenantID: "site_001",
				Code:     "bad_table_pk_mapping",
				Name:     "Bad Table PK Mapping",
				Type:     marketing.ResourceDatabaseTable,
				Driver:   marketing.ResourceDriverPostgres,
				Scope:    marketing.ResourceScopeWorkspace,
				Status:   marketing.ResourceActive,
				Config:   json.RawMessage(`{"physicalTable":"marketing_user_state","primaryKey":["tenantId","externalId"],"schema":{"columns":{"tenantId":{"type":"STRING"}}}}`),
			},
			errorCode: "RESOURCE_TABLE_PRIMARY_KEY_MAPPING_INVALID",
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

	redisCounter, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "redis_counter",
		Name:     "Redis Counter",
		Type:     marketing.ResourceRedisCounter,
		Driver:   marketing.ResourceDriverInternal,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"keyPrefix":"counter"}`),
	})
	if err != nil {
		t.Fatalf("save redis counter resource: %v", err)
	}
	if !redisCounter.Supports(marketing.ResourceCheckAndIncrement) || !redisCounter.Supports(marketing.ResourceRead) {
		t.Fatalf("expected redis counter check-and-increment and read capabilities, got %+v", redisCounter.Capabilities)
	}

	mqTopic, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "reward_topic",
		Name:     "Reward Topic",
		Type:     marketing.ResourceMQTopic,
		Driver:   marketing.ResourceDriverInternal,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"provider":"KAFKA","topic":"reward.commands"}`),
	})
	if err != nil {
		t.Fatalf("save mq topic resource: %v", err)
	}
	if !mqTopic.Supports(marketing.ResourcePublish) || !mqTopic.Supports(marketing.ResourceCall) {
		t.Fatalf("expected mq topic publish and call capabilities, got %+v", mqTopic.Capabilities)
	}
}

func TestResourceTestHTTPProviderDryRunAndResponseSchema(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET dry-run, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"ok":true},"version":1}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "dry_run_http",
		Name:     "Dry Run HTTP",
		Type:     marketing.ResourceHTTPProvider,
		Driver:   marketing.ResourceDriverREST,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","dryRun":true,"dryRunMethod":"GET","responseSchema":{"required":["$.value"],"fields":{"$.version":"NUMBER","$.value":"OBJECT"}}}`),
	}); err != nil {
		t.Fatalf("save dry-run resource: %v", err)
	}
	result, err := service.TestResource(ctx, "site_001", "dry_run_http", "tester")
	if err != nil {
		t.Fatalf("test dry-run resource: %v", err)
	}
	if result["ok"] != true || result["errorCode"] != "" {
		t.Fatalf("expected dry-run success, got %+v", result)
	}
}

func TestResourceTestHTTPProviderDryRunRejectsResponseSchemaMismatch(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"bad","version":"v1"}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "dry_run_bad_schema",
		Name:     "Dry Run Bad Schema",
		Type:     marketing.ResourceHTTPProvider,
		Driver:   marketing.ResourceDriverREST,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","dryRun":true,"responseSchema":{"fields":{"$.version":"NUMBER","$.value":"OBJECT"}}}`),
	}); err != nil {
		t.Fatalf("save dry-run bad schema resource: %v", err)
	}
	result, err := service.TestResource(ctx, "site_001", "dry_run_bad_schema", "tester")
	if err != nil {
		t.Fatalf("test dry-run resource: %v", err)
	}
	if result["ok"] != false || result["errorCode"] != "RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID" {
		t.Fatalf("expected dry-run schema failure, got %+v", result)
	}
}
