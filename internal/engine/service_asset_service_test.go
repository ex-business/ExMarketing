package engine

import (
	"context"
	"strings"
	"testing"

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
	if _, err := repo.GetServiceDefinition(ctx, "site_001", "user-profile-service"); err != nil {
		t.Fatalf("expected built-in service materialized: %v", err)
	}
	if _, err := repo.GetServiceInterface(ctx, "site_001", "user-profile-service", "query-user-profile"); err != nil {
		t.Fatalf("expected built-in interface materialized: %v", err)
	}
}

func TestResolveServiceProviderRejectsUnsupportedPolicy(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	_, _, err := service.ResolveServiceProvider(ctx, "site_001", "user-profile-service", "query-user-profile", "", "PRIMARY_WITH_FALLBACK")
	if err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_POLICY_UNSUPPORTED") {
		t.Fatalf("expected unsupported policy error, got %v", err)
	}
}
