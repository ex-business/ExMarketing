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

	_, _, err := service.ResolveServiceProvider(ctx, "site_001", "user-profile-service", "query-user-profile", "", "RANDOM")
	if err == nil || !strings.Contains(err.Error(), "SERVICE_PROVIDER_POLICY_UNSUPPORTED") {
		t.Fatalf("expected unsupported policy error, got %v", err)
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
