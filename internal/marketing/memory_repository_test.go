package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMemoryWorkerLeaseAcquireConflictExpiryAndRelease(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	first, acquired, err := repo.AcquireWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-a",
		TTL:   time.Minute,
		Now:   now,
	})
	if err != nil || !acquired {
		t.Fatalf("expected first lease acquire, lease=%+v acquired=%v err=%v", first, acquired, err)
	}
	second, acquired, err := repo.AcquireWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-b",
		TTL:   time.Minute,
		Now:   now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("held lease acquire failed: %v", err)
	}
	if acquired || second.Status != WorkerLeaseHeld || second.Owner != "worker-a" {
		t.Fatalf("expected held lease by worker-a, got lease=%+v acquired=%v", second, acquired)
	}
	renewed, ok, err := repo.RenewWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-a",
		TTL:   2 * time.Minute,
		Now:   now.Add(20 * time.Second),
	})
	if err != nil || !ok || !renewed.LeaseUntil.Equal(now.Add(20*time.Second).Add(2*time.Minute)) {
		t.Fatalf("expected owner renew, lease=%+v ok=%v err=%v", renewed, ok, err)
	}
	held, ok, err := repo.RenewWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-b",
		TTL:   time.Minute,
		Now:   now.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("renew from non-owner failed: %v", err)
	}
	if ok || held.Status != WorkerLeaseHeld || held.Owner != "worker-a" {
		t.Fatalf("expected non-owner renew to be rejected, lease=%+v ok=%v", held, ok)
	}
	expired, acquired, err := repo.AcquireWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-b",
		TTL:   time.Minute,
		Now:   now.Add(3 * time.Minute),
	})
	if err != nil || !acquired || expired.Owner != "worker-b" {
		t.Fatalf("expected expired lease takeover, lease=%+v acquired=%v err=%v", expired, acquired, err)
	}
	if err := repo.ReleaseWorkerLease(ctx, "provider-health-worker", "worker-b", now.Add(3*time.Minute+time.Second)); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	reacquired, acquired, err := repo.AcquireWorkerLease(ctx, WorkerLeaseAcquireCommand{
		Name:  "provider-health-worker",
		Owner: "worker-a",
		TTL:   time.Minute,
		Now:   now.Add(3*time.Minute + 2*time.Second),
	})
	if err != nil || !acquired || reacquired.Owner != "worker-a" {
		t.Fatalf("expected release to allow reacquire, lease=%+v acquired=%v err=%v", reacquired, acquired, err)
	}
}

func TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_1"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock_a", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock_b", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_B", TotalValue: "0", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: Participation{
			ID:             "part_1",
			TenantID:       "site_001",
			ActivityID:     activityID,
			EventInboxID:   "inbox_1",
			Subject:        EventSubject{Type: "USER", ID: "user_1"},
			Biz:            EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
			Status:         ParticipationRewardable,
			Snapshot:       []byte(`{}`),
			IdempotencyKey: "idem_1",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		StockReservations: []QuotaReservation{
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_B", Amount: "1"},
		},
		GrantOrders: []GrantOrder{
			{ID: "grant_1", ActivityID: activityID, ParticipationID: "part_1"},
			{ID: "grant_2", ActivityID: activityID, ParticipationID: "part_1"},
		},
		OutboxMessages: []OutboxMessage{
			{ID: "outbox_1"},
			{ID: "outbox_2"},
		},
	})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	if len(repo.participations) != 0 || len(repo.grantOrders) != 0 || len(repo.outboxMessages) != 0 {
		t.Fatalf("expected rollback, got participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	for _, quota := range repo.quotas {
		if quota.UsedValue != "0" {
			t.Fatalf("expected quota %s to remain unused, got %s", quota.ID, quota.UsedValue)
		}
	}
}

func TestMemoryResourceRepositoryStoresBindingsAndAudit(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	resource := Resource{
		ID:           "res_memory_test",
		TenantID:     "site_001",
		WorkspaceID:  "hello",
		Code:         "user_profile_cache",
		Name:         "用户资料缓存",
		Type:         ResourceRedisHash,
		Driver:       ResourceDriverInternal,
		Scope:        ResourceScopeWorkspace,
		Status:       ResourceActive,
		Config:       json.RawMessage(`{"keyPrefix":"marketing:user"}`),
		Capabilities: []ResourceOperation{ResourceRead, ResourceWrite},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	saved, err := repo.SaveResource(ctx, resource)
	if err != nil {
		t.Fatalf("save resource: %v", err)
	}
	if saved.Code != resource.Code || saved.ConfigMap()["keyPrefix"] != "marketing:user" {
		t.Fatalf("unexpected saved resource: %+v", saved)
	}
	updated := saved
	updated.Name = "用户资料缓存 V2"
	updated.Capabilities = append(updated.Capabilities, ResourceUpsert)
	if _, err := repo.SaveResource(ctx, updated); err != nil {
		t.Fatalf("update resource: %v", err)
	}
	listed, err := repo.ListResources(ctx, ResourceQuery{TenantID: "site_001", WorkspaceID: "hello", Type: ResourceRedisHash, Status: ResourceActive})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "用户资料缓存 V2" || !listed[0].Supports(ResourceUpsert) {
		t.Fatalf("expected updated resource in list, got %+v", listed)
	}

	binding, err := repo.SaveResourceBinding(ctx, ResourceBinding{
		ID:                "resbind_memory_test",
		ResourceID:        saved.ID,
		TenantID:          "site_001",
		WorkspaceID:       "hello",
		NodeType:          "fact-query",
		MinRole:           WorkspaceRoleEditor,
		AllowedOperations: []ResourceOperation{ResourceRead},
		AllowedNamespaces: []string{"profile"},
		KeyPolicy:         json.RawMessage(`{"allowedPrefixes":["site_001:profile:"],"deniedPrefixes":["site_001:profile:blocked:"]}`),
		Status:            ResourceBindingActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("save binding: %v", err)
	}
	if !binding.Allows(ResourceAccessRequest{TenantID: "site_001", WorkspaceID: "hello", NodeType: "fact-query", Operation: ResourceRead, Namespace: "profile", ResourceKey: "site_001:profile:u1", ActorRole: WorkspaceRoleEditor}) {
		t.Fatalf("expected binding to allow read profile: %+v", binding)
	}
	if binding.Allows(ResourceAccessRequest{TenantID: "site_001", WorkspaceID: "hello", NodeType: "fact-query", Operation: ResourceRead, Namespace: "profile", ResourceKey: "site_001:profile:u1", ActorRole: WorkspaceRoleViewer}) {
		t.Fatalf("expected binding to deny viewer below minRole")
	}
	if binding.Allows(ResourceAccessRequest{TenantID: "site_001", WorkspaceID: "hello", NodeType: "fact-query", Operation: ResourceRead, Namespace: "profile", ResourceKey: "site_001:profile:blocked:u1", ActorRole: WorkspaceRoleAdmin}) {
		t.Fatalf("expected binding to deny blocked key prefix")
	}
	bindings, err := repo.ListResourceBindings(ctx, ResourceBindingQuery{ResourceID: saved.ID, TenantID: "site_001", NodeType: "fact-query"})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != binding.ID {
		t.Fatalf("expected binding in list, got %+v", bindings)
	}

	if err := repo.CreateResourceAuditLog(ctx, ResourceAuditLog{
		ID:           "resaudit_memory_test",
		TenantID:     "site_001",
		WorkspaceID:  "hello",
		ResourceID:   saved.ID,
		ResourceCode: saved.Code,
		Operation:    ResourceRead,
		Status:       ResourceAuditSuccess,
		NodeID:       "node_fact",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create audit: %v", err)
	}
	logs, err := repo.ListResourceAuditLogs(ctx, ResourceAuditQuery{ResourceID: saved.ID, TenantID: "site_001", WorkspaceID: "hello", Status: ResourceAuditSuccess})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].NodeID != "node_fact" || logs[0].ResourceCode != saved.Code {
		t.Fatalf("expected audit log, got %+v", logs)
	}
}

func TestMemoryRepositoryStoresServiceRateLimitPolicies(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	saved, err := repo.SaveServiceRateLimitPolicy(ctx, ServiceRateLimitPolicy{
		TenantID:      "site_001",
		Code:          "wallet-query-limit",
		Name:          "Wallet query limit",
		ServiceRef:    "wallet-service",
		InterfaceRef:  "query-wallet-balance",
		ProviderRef:   "wallet-primary",
		Limit:         2,
		WindowSeconds: 30,
		Backend:       ServiceRateLimitBackendPostgres,
		Status:        ServiceAssetActive,
		Metadata:      json.RawMessage(`{"source":"test"}`),
	})
	if err != nil {
		t.Fatalf("save rate limit policy: %v", err)
	}
	if saved.ID == "" || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("expected persisted timestamps and id, got %+v", saved)
	}

	items, err := repo.ListServiceRateLimitPolicies(ctx, ServiceRateLimitPolicyQuery{
		TenantID:     "site_001",
		ServiceRef:   "wallet-service",
		InterfaceRef: "query-wallet-balance",
		ProviderRef:  "wallet-primary",
		Status:       ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("list rate limit policies: %v", err)
	}
	if len(items) != 1 || items[0].Code != "wallet-query-limit" || items[0].Limit != 2 {
		t.Fatalf("unexpected policies: %+v", items)
	}
}

func TestMemoryRepositoryAuditListsOrderByCreatedAtAndIDDesc(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateProviderDecisionAuditLog(ctx, ProviderDecisionAuditLog{
		ID:                   "decision-a",
		TenantID:             "site_001",
		ServiceRef:           "svc",
		InterfaceRef:         "iface",
		CandidateProviderRef: "provider-a",
		Decision:             ProviderDecisionSelected,
		CreatedAt:            now,
	}); err != nil {
		t.Fatalf("create provider decision audit a: %v", err)
	}
	if err := repo.CreateProviderDecisionAuditLog(ctx, ProviderDecisionAuditLog{
		ID:                   "decision-b",
		TenantID:             "site_001",
		ServiceRef:           "svc",
		InterfaceRef:         "iface",
		CandidateProviderRef: "provider-b",
		Decision:             ProviderDecisionSelected,
		CreatedAt:            now,
	}); err != nil {
		t.Fatalf("create provider decision audit b: %v", err)
	}
	decisions, err := repo.ListProviderDecisionAuditLogs(ctx, ProviderDecisionAuditQuery{TenantID: "site_001", Limit: 10})
	if err != nil {
		t.Fatalf("list provider decision audits: %v", err)
	}
	if len(decisions) != 2 || decisions[0].ID != "decision-b" || decisions[1].ID != "decision-a" {
		t.Fatalf("expected provider decision audits ordered by createdAt desc, id desc, got %+v", decisions)
	}
	if err := repo.CreateMigrationAuditLog(ctx, MigrationAuditLog{
		ID:            "migration-a",
		TenantID:      "site_001",
		MigrationType: "DB",
		Operation:     "ANALYZE",
		Result:        MigrationAuditSuccess,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create migration audit a: %v", err)
	}
	if err := repo.CreateMigrationAuditLog(ctx, MigrationAuditLog{
		ID:            "migration-b",
		TenantID:      "site_001",
		MigrationType: "DB",
		Operation:     "APPLY",
		Result:        MigrationAuditSuccess,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create migration audit b: %v", err)
	}
	migrations, err := repo.ListMigrationAuditLogs(ctx, MigrationAuditQuery{TenantID: "site_001", Limit: 10})
	if err != nil {
		t.Fatalf("list migration audits: %v", err)
	}
	if len(migrations) != 2 || migrations[0].ID != "migration-b" || migrations[1].ID != "migration-a" {
		t.Fatalf("expected migration audits ordered by createdAt desc, id desc, got %+v", migrations)
	}
}

func TestMemoryRepositoryServiceAssetsExcludeDeletedByDefault(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	deletedAt := time.Now().UTC()

	activeService, err := repo.SaveServiceDefinition(ctx, ServiceDefinition{
		ID:             "svc_active_catalog",
		TenantID:       "site_001",
		Code:           "catalog-active",
		Name:           "Catalog active",
		ServiceType:    ServiceTypeExternal,
		Scope:          ServiceAssetScopeTenant,
		Status:         ServiceAssetActive,
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:active",
	})
	if err != nil {
		t.Fatalf("save active service: %v", err)
	}
	deletedService := activeService
	deletedService.ID = "svc_deleted_catalog"
	deletedService.Code = "catalog-deleted"
	deletedService.Status = ServiceAssetDeleted
	deletedService.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceDefinition(ctx, deletedService); err != nil {
		t.Fatalf("save deleted service: %v", err)
	}

	activeInterface, err := repo.SaveServiceInterface(ctx, ServiceInterface{
		ID:             "svcif_active_catalog",
		TenantID:       "site_001",
		ServiceRef:     activeService.Code,
		Code:           "call-active",
		Name:           "Call active",
		Operation:      ServiceOperationCall,
		Status:         ServiceAssetActive,
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:interface-active",
	})
	if err != nil {
		t.Fatalf("save active interface: %v", err)
	}
	deletedInterface := activeInterface
	deletedInterface.ID = "svcif_deleted_catalog"
	deletedInterface.Code = "call-deleted"
	deletedInterface.Status = ServiceAssetDeleted
	deletedInterface.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceInterface(ctx, deletedInterface); err != nil {
		t.Fatalf("save deleted interface: %v", err)
	}

	if _, err := repo.SaveServiceProvider(ctx, ServiceProvider{
		ID:             "svcprov_deleted_catalog",
		TenantID:       "site_001",
		ServiceRef:     activeService.Code,
		InterfaceRef:   activeInterface.Code,
		Code:           "provider-deleted",
		Name:           "Provider deleted",
		Protocol:       ServiceProviderResource,
		Status:         ServiceAssetDeleted,
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:provider-deleted",
		DeletedAt:      &deletedAt,
	}); err != nil {
		t.Fatalf("save deleted provider: %v", err)
	}
	if _, err := repo.SaveServiceProvider(ctx, ServiceProvider{
		ID:           "svcprov_active_catalog",
		TenantID:     "site_001",
		ServiceRef:   activeService.Code,
		InterfaceRef: activeInterface.Code,
		Code:         "provider-active",
		Name:         "Provider active",
		Protocol:     ServiceProviderResource,
		Status:       ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save active provider: %v", err)
	}

	if _, err := repo.SaveServiceSchema(ctx, ServiceSchema{
		ID:             "schema_deleted_catalog",
		TenantID:       "site_001",
		SchemaRef:      "deleted-schema",
		SchemaType:     ServiceSchemaOutput,
		Scope:          ServiceAssetScopeTenant,
		Version:        1,
		Status:         ServiceAssetDeleted,
		SchemaJSON:     json.RawMessage(`{"type":"object"}`),
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:schema-deleted",
		DeletedAt:      &deletedAt,
	}); err != nil {
		t.Fatalf("save deleted schema: %v", err)
	}

	if _, err := repo.SaveServiceInterfaceBinding(ctx, ServiceInterfaceBinding{
		ID:             "svcbind_deleted_catalog",
		TenantID:       "site_001",
		ServiceRef:     activeService.Code,
		InterfaceRef:   activeInterface.Code,
		Scope:          ServiceAssetScopeWorkspace,
		PrincipalType:  ServicePrincipalRole,
		PrincipalID:    string(WorkspaceRoleViewer),
		Status:         ServiceAssetDeleted,
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:binding-deleted",
		DeletedAt:      &deletedAt,
	}); err != nil {
		t.Fatalf("save deleted binding: %v", err)
	}

	if _, err := repo.SaveServiceRateLimitPolicy(ctx, ServiceRateLimitPolicy{
		ID:             "svcratelimit_deleted_catalog",
		TenantID:       "site_001",
		Code:           "deleted-rate-limit",
		Name:           "Deleted rate limit",
		Limit:          1,
		WindowSeconds:  60,
		Backend:        ServiceRateLimitBackendPostgres,
		Status:         ServiceAssetDeleted,
		Builtin:        true,
		CatalogVersion: "catalog-v1",
		SourceHash:     "sha256:rate-limit-deleted",
		DeletedAt:      &deletedAt,
	}); err != nil {
		t.Fatalf("save deleted rate limit policy: %v", err)
	}

	services, err := repo.ListServiceDefinitions(ctx, ServiceDefinitionQuery{TenantID: "site_001"})
	if err != nil || len(services) != 1 || services[0].Code != activeService.Code || !services[0].Builtin || services[0].SourceHash == "" {
		t.Fatalf("expected only active traced service, items=%+v err=%v", services, err)
	}
	allServices, err := repo.ListServiceDefinitions(ctx, ServiceDefinitionQuery{TenantID: "site_001", IncludeDeleted: true})
	if err != nil || len(allServices) != 2 {
		t.Fatalf("expected include deleted services, items=%+v err=%v", allServices, err)
	}
	providers, err := repo.ListServiceProviders(ctx, ServiceProviderQuery{TenantID: "site_001"})
	if err != nil || len(providers) != 1 || providers[0].Code != "provider-active" {
		t.Fatalf("expected only active provider, items=%+v err=%v", providers, err)
	}
	allProviders, err := repo.ListServiceProviders(ctx, ServiceProviderQuery{TenantID: "site_001", IncludeDeleted: true})
	if err != nil || len(allProviders) != 2 {
		t.Fatalf("expected include deleted providers, items=%+v err=%v", allProviders, err)
	}
	schemas, err := repo.ListServiceSchemas(ctx, ServiceSchemaQuery{TenantID: "site_001", IncludeDeleted: true})
	if err != nil || len(schemas) != 1 || !schemas[0].Builtin || schemas[0].CatalogVersion == "" {
		t.Fatalf("expected deleted schema visible with includeDeleted, items=%+v err=%v", schemas, err)
	}
	defaultSchemas, err := repo.ListServiceSchemas(ctx, ServiceSchemaQuery{TenantID: "site_001"})
	if err != nil || len(defaultSchemas) != 0 {
		t.Fatalf("expected deleted schema hidden by default, items=%+v err=%v", defaultSchemas, err)
	}
	bindings, err := repo.ListServiceInterfaceBindings(ctx, ServiceInterfaceBindingQuery{TenantID: "site_001", IncludeDeleted: true})
	if err != nil || len(bindings) != 1 || bindings[0].DeletedAt == nil {
		t.Fatalf("expected deleted binding visible with includeDeleted, items=%+v err=%v", bindings, err)
	}
	defaultBindings, err := repo.ListServiceInterfaceBindings(ctx, ServiceInterfaceBindingQuery{TenantID: "site_001"})
	if err != nil || len(defaultBindings) != 0 {
		t.Fatalf("expected deleted binding hidden by default, items=%+v err=%v", defaultBindings, err)
	}
	policies, err := repo.ListServiceRateLimitPolicies(ctx, ServiceRateLimitPolicyQuery{TenantID: "site_001", IncludeDeleted: true})
	if err != nil || len(policies) != 1 || policies[0].DeletedAt == nil {
		t.Fatalf("expected deleted rate limit visible with includeDeleted, items=%+v err=%v", policies, err)
	}
	defaultPolicies, err := repo.ListServiceRateLimitPolicies(ctx, ServiceRateLimitPolicyQuery{TenantID: "site_001"})
	if err != nil || len(defaultPolicies) != 0 {
		t.Fatalf("expected deleted rate limit hidden by default, items=%+v err=%v", defaultPolicies, err)
	}
}

func TestMemoryRepositoryServiceAssetSoftDeleteAllowsRecreateSameRefs(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	deletedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)

	service, err := repo.SaveServiceDefinition(ctx, ServiceDefinition{
		ID:          "svc_soft_unique_old",
		TenantID:    "site_001",
		Code:        "soft-unique-service",
		Name:        "Soft unique service",
		ServiceType: ServiceTypeExternal,
		Scope:       ServiceAssetScopeTenant,
		Status:      ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save service: %v", err)
	}
	serviceUpdate, err := repo.SaveServiceDefinition(ctx, ServiceDefinition{
		ID:          "svc_soft_unique_dup",
		TenantID:    service.TenantID,
		Code:        service.Code,
		Name:        "Active service update",
		ServiceType: ServiceTypeExternal,
		Scope:       ServiceAssetScopeTenant,
		Status:      ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("active service natural-key update: %v", err)
	}
	if serviceUpdate.ID != service.ID || serviceUpdate.Name != "Active service update" {
		t.Fatalf("expected active service update to preserve id, got %+v", serviceUpdate)
	}
	service = serviceUpdate
	service.Status = ServiceAssetDeleted
	service.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceDefinition(ctx, service); err != nil {
		t.Fatalf("soft delete service: %v", err)
	}
	recreatedService, err := repo.SaveServiceDefinition(ctx, ServiceDefinition{
		ID:          "svc_soft_unique_new",
		TenantID:    service.TenantID,
		Code:        service.Code,
		Name:        "Soft unique service rebuilt",
		ServiceType: ServiceTypeExternal,
		Scope:       ServiceAssetScopeTenant,
		Status:      ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("recreate service with same code: %v", err)
	}
	if recreatedService.ID == service.ID {
		t.Fatalf("expected new service id after recreate, got %+v", recreatedService)
	}

	serviceInterface, err := repo.SaveServiceInterface(ctx, ServiceInterface{
		ID:         "svcif_soft_unique_old",
		TenantID:   service.TenantID,
		ServiceRef: service.Code,
		Code:       "soft-unique-interface",
		Name:       "Soft unique interface",
		Operation:  ServiceOperationCall,
		Status:     ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save interface: %v", err)
	}
	serviceInterfaceUpdate, err := repo.SaveServiceInterface(ctx, ServiceInterface{
		ID:         "svcif_soft_unique_dup",
		TenantID:   serviceInterface.TenantID,
		ServiceRef: serviceInterface.ServiceRef,
		Code:       serviceInterface.Code,
		Name:       "Active interface update",
		Operation:  ServiceOperationCall,
		Status:     ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("active interface natural-key update: %v", err)
	}
	if serviceInterfaceUpdate.ID != serviceInterface.ID || serviceInterfaceUpdate.Name != "Active interface update" {
		t.Fatalf("expected active interface update to preserve id, got %+v", serviceInterfaceUpdate)
	}
	serviceInterface = serviceInterfaceUpdate
	serviceInterface.Status = ServiceAssetDeleted
	serviceInterface.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceInterface(ctx, serviceInterface); err != nil {
		t.Fatalf("soft delete interface: %v", err)
	}
	if _, err := repo.SaveServiceInterface(ctx, ServiceInterface{
		ID:         "svcif_soft_unique_new",
		TenantID:   serviceInterface.TenantID,
		ServiceRef: serviceInterface.ServiceRef,
		Code:       serviceInterface.Code,
		Name:       "Soft unique interface rebuilt",
		Operation:  ServiceOperationCall,
		Status:     ServiceAssetActive,
	}); err != nil {
		t.Fatalf("recreate interface with same code: %v", err)
	}

	provider, err := repo.SaveServiceProvider(ctx, ServiceProvider{
		ID:           "svcprov_soft_unique_old",
		TenantID:     service.TenantID,
		ServiceRef:   service.Code,
		InterfaceRef: serviceInterface.Code,
		Code:         "soft-unique-provider",
		Name:         "Soft unique provider",
		Protocol:     ServiceProviderResource,
		Status:       ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	providerUpdate, err := repo.SaveServiceProvider(ctx, ServiceProvider{
		ID:           "svcprov_soft_unique_dup",
		TenantID:     provider.TenantID,
		ServiceRef:   provider.ServiceRef,
		InterfaceRef: provider.InterfaceRef,
		Code:         provider.Code,
		Name:         "Active provider update",
		Protocol:     ServiceProviderResource,
		Status:       ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("active provider natural-key update: %v", err)
	}
	if providerUpdate.ID != provider.ID || providerUpdate.Name != "Active provider update" {
		t.Fatalf("expected active provider update to preserve id, got %+v", providerUpdate)
	}
	provider = providerUpdate
	provider.Status = ServiceAssetDeleted
	provider.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceProvider(ctx, provider); err != nil {
		t.Fatalf("soft delete provider: %v", err)
	}
	if _, err := repo.SaveServiceProvider(ctx, ServiceProvider{
		ID:           "svcprov_soft_unique_new",
		TenantID:     provider.TenantID,
		ServiceRef:   provider.ServiceRef,
		InterfaceRef: provider.InterfaceRef,
		Code:         provider.Code,
		Name:         "Soft unique provider rebuilt",
		Protocol:     ServiceProviderResource,
		Status:       ServiceAssetActive,
	}); err != nil {
		t.Fatalf("recreate provider with same code: %v", err)
	}

	schema, err := repo.SaveServiceSchema(ctx, ServiceSchema{
		ID:         "schema_soft_unique_old",
		TenantID:   service.TenantID,
		SchemaRef:  "soft-unique-schema",
		SchemaType: ServiceSchemaOutput,
		Scope:      ServiceAssetScopeTenant,
		Version:    1,
		Status:     ServiceAssetActive,
		SchemaJSON: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("save schema: %v", err)
	}
	schemaUpdate, err := repo.SaveServiceSchema(ctx, ServiceSchema{
		ID:         "schema_soft_unique_dup",
		TenantID:   schema.TenantID,
		SchemaRef:  schema.SchemaRef,
		SchemaType: schema.SchemaType,
		Scope:      ServiceAssetScopeTenant,
		Version:    schema.Version,
		Status:     ServiceAssetActive,
		SchemaJSON: json.RawMessage(`{"type":"object","properties":{"updated":{"type":"boolean"}}}`),
	})
	if err != nil {
		t.Fatalf("active schema natural-key update: %v", err)
	}
	if schemaUpdate.ID != schema.ID {
		t.Fatalf("expected active schema update to preserve id, got %+v", schemaUpdate)
	}
	schema = schemaUpdate
	schema.Status = ServiceAssetDeleted
	schema.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceSchema(ctx, schema); err != nil {
		t.Fatalf("soft delete schema: %v", err)
	}
	if _, err := repo.SaveServiceSchema(ctx, ServiceSchema{
		ID:         "schema_soft_unique_new",
		TenantID:   schema.TenantID,
		SchemaRef:  schema.SchemaRef,
		SchemaType: schema.SchemaType,
		Scope:      ServiceAssetScopeTenant,
		Version:    schema.Version,
		Status:     ServiceAssetActive,
		SchemaJSON: json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("recreate schema with same ref/version: %v", err)
	}

	binding, err := repo.SaveServiceInterfaceBinding(ctx, ServiceInterfaceBinding{
		ID:            "svcbind_soft_unique_old",
		TenantID:      service.TenantID,
		ServiceRef:    service.Code,
		InterfaceRef:  serviceInterface.Code,
		Scope:         ServiceAssetScopeWorkspace,
		WorkspaceID:   "workspace-soft-unique",
		PrincipalType: ServicePrincipalRole,
		PrincipalID:   string(WorkspaceRoleViewer),
		Status:        ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save binding: %v", err)
	}
	bindingUpdate, err := repo.SaveServiceInterfaceBinding(ctx, ServiceInterfaceBinding{
		ID:            "svcbind_soft_unique_dup",
		TenantID:      binding.TenantID,
		ServiceRef:    binding.ServiceRef,
		InterfaceRef:  binding.InterfaceRef,
		Scope:         binding.Scope,
		WorkspaceID:   binding.WorkspaceID,
		PrincipalType: binding.PrincipalType,
		PrincipalID:   binding.PrincipalID,
		MinRole:       WorkspaceRoleAdmin,
		Status:        ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("active binding natural-key update: %v", err)
	}
	if bindingUpdate.ID != binding.ID || bindingUpdate.MinRole != WorkspaceRoleAdmin {
		t.Fatalf("expected active binding update to preserve id, got %+v", bindingUpdate)
	}
	binding = bindingUpdate
	binding.Status = ServiceAssetDeleted
	binding.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceInterfaceBinding(ctx, binding); err != nil {
		t.Fatalf("soft delete binding: %v", err)
	}
	if _, err := repo.SaveServiceInterfaceBinding(ctx, ServiceInterfaceBinding{
		ID:            "svcbind_soft_unique_new",
		TenantID:      binding.TenantID,
		ServiceRef:    binding.ServiceRef,
		InterfaceRef:  binding.InterfaceRef,
		Scope:         binding.Scope,
		WorkspaceID:   binding.WorkspaceID,
		PrincipalType: binding.PrincipalType,
		PrincipalID:   binding.PrincipalID,
		Status:        ServiceAssetActive,
	}); err != nil {
		t.Fatalf("recreate binding with same scope: %v", err)
	}

	policy, err := repo.SaveServiceRateLimitPolicy(ctx, ServiceRateLimitPolicy{
		ID:            "svcratelimit_soft_unique_old",
		TenantID:      service.TenantID,
		Code:          "soft-unique-rate-limit",
		Name:          "Soft unique rate limit",
		Limit:         1,
		WindowSeconds: 60,
		Backend:       ServiceRateLimitBackendPostgres,
		Status:        ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("save rate limit policy: %v", err)
	}
	policyUpdate, err := repo.SaveServiceRateLimitPolicy(ctx, ServiceRateLimitPolicy{
		ID:            "svcratelimit_soft_unique_dup",
		TenantID:      policy.TenantID,
		Code:          policy.Code,
		Name:          "Active rate limit update",
		Limit:         2,
		WindowSeconds: 60,
		Backend:       ServiceRateLimitBackendPostgres,
		Status:        ServiceAssetActive,
	})
	if err != nil {
		t.Fatalf("active rate limit natural-key update: %v", err)
	}
	if policyUpdate.ID != policy.ID || policyUpdate.Limit != 2 {
		t.Fatalf("expected active rate limit update to preserve id, got %+v", policyUpdate)
	}
	policy = policyUpdate
	policy.Status = ServiceAssetDeleted
	policy.DeletedAt = &deletedAt
	if _, err := repo.SaveServiceRateLimitPolicy(ctx, policy); err != nil {
		t.Fatalf("soft delete rate limit policy: %v", err)
	}
	if _, err := repo.SaveServiceRateLimitPolicy(ctx, ServiceRateLimitPolicy{
		ID:            "svcratelimit_soft_unique_new",
		TenantID:      policy.TenantID,
		Code:          policy.Code,
		Name:          "Soft unique rate limit rebuilt",
		Limit:         1,
		WindowSeconds: 60,
		Backend:       ServiceRateLimitBackendPostgres,
		Status:        ServiceAssetActive,
	}); err != nil {
		t.Fatalf("recreate rate limit policy with same code: %v", err)
	}
}

func TestFactProviderValidateRejectsInvalidMappingContract(t *testing.T) {
	base := FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		CacheTTLSeconds: 0,
		Enabled:         true,
		TimeoutMS:       5000,
	}
	for name, mutate := range map[string]func(*FactProvider){
		"facts prefix": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"facts.USER_PROFILE.vipLevel": {Source: "$.data.vipLevel", Type: "string"}}
		},
		"trailing dot": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"risk.": {Source: "$.risk", Type: "object"}}
		},
		"wildcard source": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data.*", Type: "string"}}
		},
		"filter source": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data[?(@.vip)]", Type: "string"}}
		},
		"invalid type": func(p *FactProvider) {
			p.Mapping = map[string]FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "decimal"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := base
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("expected invalid provider")
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid provider rejected: %v", err)
	}
}

func TestExtractFactReferencesValidatedRejectsInvalidFactPaths(t *testing.T) {
	for _, field := range []string{"facts", "facts.USER_PROFILE", "facts..vipLevel", "facts.USER_PROFILE.", "facts.user_profile.vipLevel"} {
		t.Run(field, func(t *testing.T) {
			_, err := ExtractFactReferencesValidated([]Rule{{
				Type:   RuleTypeCondition,
				Config: []byte(`{"logic":"AND","conditions":[{"field":"` + field + `","op":"=","value":"VIP3"}]}`),
			}})
			if err == nil || !strings.Contains(err.Error(), "FACT_FIELD_INVALID") {
				t.Fatalf("expected FACT_FIELD_INVALID, got %v", err)
			}
		})
	}
}

func TestExtractFactReferencesValidatedKeepsNestedFieldExact(t *testing.T) {
	refs, err := ExtractFactReferencesValidated([]Rule{{
		Type:   RuleTypeCondition,
		Config: []byte(`{"logic":"AND","conditions":[{"field":"facts.USER_PROFILE.risk.score","op":">=","value":80}]}`),
	}})
	if err != nil {
		t.Fatalf("extract refs: %v", err)
	}
	if _, ok := refs["USER_PROFILE"]["risk.score"]; !ok {
		t.Fatalf("expected exact nested path, got %#v", refs)
	}
}

func TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_1"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	inbox := EventInbox{ID: "inbox_1", IdempotencyKey: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1"}
	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || duplicate {
		t.Fatalf("create inbox duplicate=%v err=%v", duplicate, err)
	}
	if _, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: Participation{ID: "part_1", ActivityID: activityID, Status: ParticipationRewardable, IdempotencyKey: "part_idem_1", CreatedAt: now, UpdatedAt: now},
		BudgetAmount:  "20",
		StockReservations: []QuotaReservation{
			{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
		GrantOrders:    []GrantOrder{{ID: "grant_1"}},
		OutboxMessages: []OutboxMessage{{ID: "outbox_1"}},
	}); err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || !duplicate {
		t.Fatalf("expected duplicate inbox, duplicate=%v err=%v", duplicate, err)
	}
	if len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected no duplicate grants/outbox, grants=%d outbox=%d", len(repo.grantOrders), len(repo.outboxMessages))
	}
	for _, quota := range repo.quotas {
		if quota.Type == QuotaBudget && quota.FrozenValue != "20.0000" {
			t.Fatalf("expected budget frozen 20, got %s", quota.FrozenValue)
		}
		if quota.Type == QuotaStock && quota.FrozenValue != "1.0000" {
			t.Fatalf("expected stock frozen 1, got %s", quota.FrozenValue)
		}
	}
}

func TestMemoryCallbackFinalizesConfiguredLimitReservations(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_limit_callback"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_daily", ActivityID: activityID, Type: QuotaDailyLimit, Scope: QuotaScopeUser, ScopeKey: "user_1:2026-05-24", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation:      testAtomicParticipation("part_limit_callback", activityID, "part_idem_limit_callback", now),
		BudgetAmount:       "10",
		ReserveUserLimit:   true,
		UserScopeKey:       "user_1",
		ReserveDailyLimit:  true,
		DailyLimitScopeKey: "user_1:2026-05-24",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_limit_callback", activityID, "part_limit_callback", "grant_idem_limit_callback", now)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_limit_callback", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if len(repo.quotaReservations) != 3 {
		t.Fatalf("expected budget, user and daily reservations, got %d", len(repo.quotaReservations))
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_limit_callback", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_limit_callback",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_limit_callback",
		FinishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("expected participation granted, got %s", result.ParticipationStatus)
	}
	for _, reservation := range repo.quotaReservations {
		if reservation.Status != QuotaReservationConfirmed {
			t.Fatalf("expected reservation %s confirmed, got %s", reservation.Type, reservation.Status)
		}
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "10.0000", "0.0000")
	assertMemoryQuotaValues(t, repo, activityID, QuotaUserLimit, QuotaScopeUser, "user_1", "1.0000", "0.0000")
	assertMemoryQuotaValues(t, repo, activityID, QuotaDailyLimit, QuotaScopeUser, "user_1:2026-05-24", "1.0000", "0.0000")
}

func TestMemoryCallbackTreatsMissingReservationsAsUnlimitedWhenNoQuotaWasConfigured(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_unlimited_callback"
	repo.participations = append(repo.participations, testAtomicParticipation("part_unlimited_callback", activityID, "part_idem_unlimited_callback", now))
	repo.grantOrders = append(repo.grantOrders, func() GrantOrder {
		order := testAtomicGrant("grant_unlimited_callback", activityID, "part_unlimited_callback", "grant_idem_unlimited_callback", now)
		order.GrantStatus = GrantSending
		return order
	}())

	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_unlimited_callback",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_unlimited",
		FinishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if result.GrantStatus != GrantSuccess || result.ParticipationStatus != ParticipationGranted || len(result.Reservations) != 0 {
		t.Fatalf("expected unlimited callback to succeed without reservations, got %+v", result)
	}
}

func TestMemoryCloseOutboxDLQOnlyClosesOutbox(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_dlq_close"
	repo.quotas = []Quota{
		{ID: "quota_dlq_close_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: testAtomicParticipation("part_dlq_close", activityID, "part_idem_dlq_close", now),
		BudgetAmount:  "10",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_dlq_close", activityID, "part_dlq_close", "grant_idem_dlq_close", now)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{func() OutboxMessage {
			message := testAtomicOutbox("outbox_dlq_close", now)
			message.Payload = []byte(`{"commandId":"cmd_grant_dlq_close"}`)
			return message
		}()},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_dlq_close", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_dlq_close", GrantRetry, "target returned status 400"); err != nil {
		t.Fatalf("mark retry: %v", err)
	}
	if err := repo.MarkOutboxDLQ(context.Background(), "outbox_dlq_close", 5, "target returned status 400"); err != nil {
		t.Fatalf("mark outbox dlq: %v", err)
	}

	closed, err := repo.CloseOutboxDLQ(context.Background(), "site_001", "outbox_dlq_close", "ops", "business rejected", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("close dlq: %v", err)
	}
	if closed.SendStatus != OutboxDLQClosed {
		t.Fatalf("expected DLQ_CLOSED, got %s", closed.SendStatus)
	}
	order := repo.grantOrderByCommandIDLocked("site_001", "cmd_grant_dlq_close")
	if order == nil || order.GrantStatus != GrantRetry {
		t.Fatalf("expected grant to remain RETRY, got %+v", order)
	}
	if got := repo.participationStatusLocked("part_dlq_close"); got != ParticipationRewardable {
		t.Fatalf("expected participation to remain REWARDABLE, got %s", got)
	}
	for _, reservation := range repo.quotaReservations {
		if reservation.Status != QuotaReservationFrozen {
			t.Fatalf("expected reservation to remain frozen, got %+v", reservation)
		}
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "10.0000")
}

func TestMemoryCallbackConfirmsParticipationLimitOnceWhenAnyGrantSucceeds(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_multi_grant_limit"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation:    testAtomicParticipation("part_multi_grant_limit", activityID, "part_idem_multi_grant_limit", now),
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		GrantOrders: []GrantOrder{
			func() GrantOrder {
				order := testAtomicGrant("grant_multi_success", activityID, "part_multi_grant_limit", "grant_idem_multi_success", now)
				order.RewardAmount = "10"
				return order
			}(),
			func() GrantOrder {
				order := testAtomicGrant("grant_multi_failed", activityID, "part_multi_grant_limit", "grant_idem_multi_failed", now)
				order.RewardAmount = "10"
				return order
			}(),
		},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_multi_success", now), testAtomicOutbox("outbox_multi_failed", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	for _, commandID := range []string{"cmd_grant_multi_success", "cmd_grant_multi_failed"} {
		if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", commandID, GrantSending, "sending"); err != nil {
			t.Fatalf("mark sending %s: %v", commandID, err)
		}
	}
	if _, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:        "site_001",
		CommandID:       "cmd_grant_multi_success",
		Status:          GrantSuccess,
		ExternalGrantID: "wallet_multi_success",
		FinishedAt:      now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("success callback: %v", err)
	}
	result, err := repo.ApplyRewardCallback(context.Background(), RewardCallback{
		TenantID:   "site_001",
		CommandID:  "cmd_grant_multi_failed",
		Status:     GrantFailed,
		Message:    "provider rejected",
		FinishedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed callback: %v", err)
	}
	if result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("expected participation granted because one grant succeeded, got %s", result.ParticipationStatus)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaUserLimit, QuotaScopeUser, "user_1", "1.0000", "0.0000")
}

func TestMemoryTimeoutReleaseDryRunReleaseAndIdempotency(t *testing.T) {
	repo := NewMemoryRepository()
	old := time.Now().UTC().Add(-3 * time.Hour)
	activityID := "act_timeout_release"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: old, UpdatedAt: old},
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
		Participation: testAtomicParticipation("part_timeout_release", activityID, "part_idem_timeout_release", old),
		BudgetAmount:  "10",
		GrantOrders: []GrantOrder{func() GrantOrder {
			order := testAtomicGrant("grant_timeout_release", activityID, "part_timeout_release", "grant_idem_timeout_release", old)
			order.RewardAmount = "10"
			return order
		}()},
		OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_timeout_release", old)},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", "cmd_grant_timeout_release", GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	repo.grantOrders[0].UpdatedAt = old

	dryRun, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		DryRun:    true,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(dryRun.Items) != 1 || dryRun.Items[0].Result != "DRY_RUN" {
		t.Fatalf("expected one dry-run candidate, got %+v", dryRun)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "10.0000")

	released, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(released.Items) != 1 || released.Items[0].GrantStatus != GrantFailed || released.Items[0].Result != "RELEASED" {
		t.Fatalf("expected released item, got %+v", released)
	}
	assertMemoryQuotaValues(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "0", "0.0000")

	repeated, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{
		TenantID:  "site_001",
		OlderThan: time.Now().UTC().Add(-2 * time.Hour),
		Limit:     10,
		Operator:  "ops",
		Reason:    "provider timeout",
	})
	if err != nil {
		t.Fatalf("repeat release: %v", err)
	}
	if len(repeated.Items) != 0 {
		t.Fatalf("expected repeated release to be idempotent with no candidates, got %+v", repeated)
	}
}

func TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	participation := Participation{
		ID:             "part_1",
		TenantID:       "site_001",
		ActivityID:     "act_1",
		EventInboxID:   "inbox_1",
		Status:         ParticipationRejected,
		Snapshot:       []byte(`{}`),
		IdempotencyKey: "site_001:act_1:site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateParticipation(context.Background(), participation); err != nil {
		t.Fatalf("create participation: %v", err)
	}
	participation.ID = "part_2"
	if _, err := repo.CreateParticipation(context.Background(), participation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected duplicate participation idempotency conflict, got %v", err)
	}

	order := GrantOrder{
		ID:             "grant_1",
		IdempotencyKey: "site_001:act_1:site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_1:REWARD_A",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateGrantOrder(context.Background(), order); err != nil {
		t.Fatalf("create grant order: %v", err)
	}
	order.ID = "grant_2"
	if _, err := repo.CreateGrantOrder(context.Background(), order); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected duplicate grant idempotency conflict, got %v", err)
	}
}

func TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_concurrent_duplicate"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	const attempts = 24
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
				Participation:  testAtomicParticipation("part_concurrent_duplicate", activityID, "part_idem_concurrent_duplicate", now),
				BudgetAmount:   "10",
				GrantOrders:    []GrantOrder{testAtomicGrant("grant_concurrent_duplicate", activityID, "part_concurrent_duplicate", "grant_idem_concurrent_duplicate", now)},
				OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_concurrent_duplicate", now)},
				StockReservations: []QuotaReservation{
					{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
				},
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var success, conflicts int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent duplicate error: %v", err)
		}
	}
	if success != 1 || conflicts != attempts-1 {
		t.Fatalf("expected one success and %d conflicts, got success=%d conflicts=%d", attempts-1, success, conflicts)
	}
	if len(repo.participations) != 1 || len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected one persisted grant chain, participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "10.0000")
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaStock, QuotaScopeActivity, "REWARD_A", "1.0000")
}

func TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_concurrent_quota"
	repo.quotas = []Quota{
		{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	}

	const attempts = 24
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			suffix := strings.ReplaceAll(time.Unix(0, int64(i)).Format("150405.000000000"), ".", "_")
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
				Participation:  testAtomicParticipation("part_quota_"+suffix, activityID, "part_idem_quota_"+suffix, now),
				BudgetAmount:   "1",
				GrantOrders:    []GrantOrder{testAtomicGrant("grant_quota_"+suffix, activityID, "part_quota_"+suffix, "grant_idem_quota_"+suffix, now)},
				OutboxMessages: []OutboxMessage{testAtomicOutbox("outbox_quota_"+suffix, now)},
				StockReservations: []QuotaReservation{
					{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
				},
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	var success, quotaExceeded int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrQuotaExceeded):
			quotaExceeded++
		default:
			t.Fatalf("unexpected concurrent quota error: %v", err)
		}
	}
	if success != 1 || quotaExceeded != attempts-1 {
		t.Fatalf("expected one success and %d quota failures, got success=%d quotaExceeded=%d", attempts-1, success, quotaExceeded)
	}
	if len(repo.participations) != 1 || len(repo.grantOrders) != 1 || len(repo.outboxMessages) != 1 {
		t.Fatalf("expected one persisted grant chain, participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
	}
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "1.0000")
	assertMemoryQuotaFrozen(t, repo, activityID, QuotaStock, QuotaScopeActivity, "REWARD_A", "1.0000")
}

func TestMemoryConcurrentQuotaCompetitionCoversAllQuotaTypes(t *testing.T) {
	tests := []struct {
		name       string
		bottleneck QuotaType
	}{
		{name: "budget", bottleneck: QuotaBudget},
		{name: "stock", bottleneck: QuotaStock},
		{name: "user_limit", bottleneck: QuotaUserLimit},
		{name: "daily_limit", bottleneck: QuotaDailyLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMemoryRepository()
			now := time.Now().UTC()
			activityID := "act_concurrent_quota_" + tt.name
			capacity := "5"
			ample := "100"
			total := func(quotaType QuotaType) string {
				if quotaType == tt.bottleneck {
					return capacity
				}
				return ample
			}
			repo.quotas = []Quota{
				{ID: "quota_budget_" + tt.name, ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: total(QuotaBudget), UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
				{ID: "quota_stock_" + tt.name, ActivityID: activityID, Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: total(QuotaStock), UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
				{ID: "quota_user_" + tt.name, ActivityID: activityID, Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: total(QuotaUserLimit), UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
				{ID: "quota_daily_" + tt.name, ActivityID: activityID, Type: QuotaDailyLimit, Scope: QuotaScopeUser, ScopeKey: "2026-05-27:user_1", TotalValue: total(QuotaDailyLimit), UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
			}

			const attempts = 24
			var wg sync.WaitGroup
			results := make(chan error, attempts)
			for i := 0; i < attempts; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					suffix := strings.ReplaceAll(time.Unix(0, int64(i)).Format("150405.000000000"), ".", "_")
					participationID := "part_" + tt.name + "_" + suffix
					grant := testAtomicGrant("grant_"+tt.name+"_"+suffix, activityID, participationID, "grant_idem_"+tt.name+"_"+suffix, now)
					grant.RewardPayload = []byte(`{"rewardCode":"REWARD_A","rewardType":"BONUS","calculateType":"FIXED","amount":"1"}`)
					_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), AtomicGrantCommand{
						Participation:      testAtomicParticipation(participationID, activityID, "part_idem_"+tt.name+"_"+suffix, now),
						BudgetAmount:       "1",
						ReserveUserLimit:   true,
						UserScopeKey:       "user_1",
						ReserveDailyLimit:  true,
						DailyLimitScopeKey: "2026-05-27:user_1",
						GrantOrders:        []GrantOrder{grant},
						OutboxMessages:     []OutboxMessage{testAtomicOutbox("outbox_"+tt.name+"_"+suffix, now)},
						StockReservations: []QuotaReservation{
							{Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
						},
					})
					results <- err
				}(i)
			}
			wg.Wait()
			close(results)

			var success, quotaExceeded int
			for err := range results {
				switch {
				case err == nil:
					success++
				case errors.Is(err, ErrQuotaExceeded):
					quotaExceeded++
				default:
					t.Fatalf("unexpected concurrent quota error: %v", err)
				}
			}
			if success != 5 || quotaExceeded != attempts-5 {
				t.Fatalf("expected 5 successes and %d quota failures, got success=%d quotaExceeded=%d", attempts-5, success, quotaExceeded)
			}
			if len(repo.participations) != 5 || len(repo.grantOrders) != 5 || len(repo.outboxMessages) != 5 {
				t.Fatalf("expected only successful chains to persist, participations=%d grants=%d outbox=%d", len(repo.participations), len(repo.grantOrders), len(repo.outboxMessages))
			}
			if len(repo.quotaReservations) != 20 {
				t.Fatalf("expected four reservations per successful chain, got %d", len(repo.quotaReservations))
			}
			assertMemoryQuotaFrozen(t, repo, activityID, QuotaBudget, QuotaScopeActivity, "TOTAL", "5.0000")
			assertMemoryQuotaFrozen(t, repo, activityID, QuotaStock, QuotaScopeActivity, "REWARD_A", "5.0000")
			assertMemoryQuotaFrozen(t, repo, activityID, QuotaUserLimit, QuotaScopeUser, "user_1", "5.0000")
			assertMemoryQuotaFrozen(t, repo, activityID, QuotaDailyLimit, QuotaScopeUser, "2026-05-27:user_1", "5.0000")
		})
	}
}

func TestMemoryCreateInboxDetectsDuplicateAndConflict(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	event := testInboxEvent(now)
	inbox := EventInbox{
		ID:             "inbox_1",
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if _, duplicate, err := repo.CreateInbox(context.Background(), inbox); err != nil || duplicate {
		t.Fatalf("create inbox duplicate=%v err=%v", duplicate, err)
	}

	same := inbox
	same.ID = "inbox_2"
	same.Event.Attributes = map[string]any{"count": 2, "amount": "12.34", "nested": map[string]any{"ok": true}}
	existing, duplicate, err := repo.CreateInbox(context.Background(), same)
	if err != nil || !duplicate {
		t.Fatalf("expected duplicate inbox, duplicate=%v err=%v", duplicate, err)
	}
	if existing.ProcessStatus != InboxDuplicate {
		t.Fatalf("expected duplicate status, got %s", existing.ProcessStatus)
	}

	conflict := inbox
	conflict.ID = "inbox_3"
	conflict.Event.EventID = "evt_conflict"
	if _, duplicate, err := repo.CreateInbox(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) || duplicate {
		t.Fatalf("expected idempotency conflict, duplicate=%v err=%v", duplicate, err)
	}

	eventIDConflict := inbox
	eventIDConflict.ID = "inbox_4"
	eventIDConflict.Event.Biz.BizID = "pay_other"
	eventIDConflict.IdempotencyKey = eventIDConflict.Event.IdempotencyKey()
	existing, duplicate, err = repo.CreateInbox(context.Background(), eventIDConflict)
	if err != nil || !duplicate {
		t.Fatalf("expected same event id duplicate, duplicate=%v err=%v", duplicate, err)
	}
	if existing.ID != inbox.ID || existing.ProcessStatus != InboxDuplicate {
		t.Fatalf("expected original inbox duplicate, got %+v", existing)
	}
}

func TestActivityEventSameStandardPayloadCanonicalizesNumbers(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	left := testInboxEvent(now)
	left.Attributes = map[string]any{"amount": jsonNumber("12.3400"), "count": jsonNumber("2")}
	right := testInboxEvent(now)
	right.Attributes = map[string]any{"count": jsonNumber("2.0"), "amount": 12.34}

	if !left.SameStandardPayload(right) {
		t.Fatal("expected canonical payload comparison to treat equivalent numeric attributes as equal")
	}
}

func testInboxEvent(now time.Time) ActivityEvent {
	return ActivityEvent{
		EventID:      "evt_1",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      EventSubject{Type: "USER", ID: "10001"},
		Biz:          EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "12.34", "count": 2, "nested": map[string]any{"ok": true}},
	}
}

func jsonNumber(value string) any {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		panic(err)
	}
	return out
}

func TestMemoryManualExportRequiresReadyFileBeforeSuccess(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	activityID := "act_manual_export"
	repo.participations = append(repo.participations, testAtomicParticipation("part_manual_export", activityID, "part_idem_manual_export", now))
	repo.grantOrders = append(repo.grantOrders, testManualGrant("grant_manual_export", activityID, "part_manual_export", "grant_idem_manual_export", "REWARD_EXPORT", "5", GrantManualExport, now))

	_, err := repo.ApplyManualGrantDecision(context.Background(), ManualGrantDecisionCommand{TenantID: "site_001", CommandID: "cmd_grant_manual_export", Status: GrantSuccess, ExternalGrantID: "offline_export", Operator: "ops", Reason: "paid", FinishedAt: now.Add(time.Minute)})
	if !errors.Is(err, ErrManualGrantExportRequired) {
		t.Fatalf("expected export required, got %v", err)
	}
	exportResult, err := repo.CreateManualGrantExport(context.Background(), ManualGrantExportCreateCommand{TenantID: "site_001", Operator: "ops", Reason: "daily export", ActivityID: activityID})
	if err != nil {
		t.Fatalf("create export: %v", err)
	}
	if exportResult.ExportFile.Status != ManualGrantExportReady || len(exportResult.Details) != 1 {
		t.Fatalf("unexpected export result: %+v", exportResult)
	}
	if exportResult.ExportFile.FileChecksum == "" || exportResult.ExportFile.Operator != "ops" || exportResult.ExportFile.ExpiresAt.IsZero() || !exportResult.ExportFile.ExpiresAt.After(exportResult.ExportFile.CreatedAt) {
		t.Fatalf("export security metadata missing: %+v", exportResult.ExportFile)
	}
	if repo.grantOrders[0].GrantStatus != GrantSending {
		t.Fatalf("expected exported grant sending, got %s", repo.grantOrders[0].GrantStatus)
	}
	result, err := repo.ApplyManualGrantDecision(context.Background(), ManualGrantDecisionCommand{TenantID: "site_001", CommandID: "cmd_grant_manual_export", Status: GrantSuccess, ExternalGrantID: "offline_export", Operator: "ops", Reason: "paid", FinishedAt: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("manual export success: %v", err)
	}
	if result.GrantStatus != GrantSuccess || result.ParticipationStatus != ParticipationGranted {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMemoryTimeoutReleaseSkipsManualGrantOrders(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC().Add(-3 * time.Hour)
	activityID := "act_manual_timeout"
	repo.quotas = []Quota{{ID: "quota_budget", ActivityID: activityID, Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "10", Version: 1, CreatedAt: now, UpdatedAt: now}}
	repo.participations = append(repo.participations, testAtomicParticipation("part_manual_timeout", activityID, "part_idem_manual_timeout", now))
	repo.grantOrders = append(repo.grantOrders, testManualGrant("grant_manual_timeout", activityID, "part_manual_timeout", "grant_idem_manual_timeout", "REWARD_TIMEOUT", "10", GrantManual, now))
	repo.quotaReservations = append(repo.quotaReservations, QuotaReservation{ID: "reservation_manual_timeout", TenantID: "site_001", ActivityID: activityID, GrantOrderID: "grant_manual_timeout", CommandID: "cmd_grant_manual_timeout", Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", Amount: "10", Status: QuotaReservationFrozen, CreatedAt: now, UpdatedAt: now})
	result, err := repo.ReleaseExpiredQuotaReservations(context.Background(), QuotaTimeoutReleaseCommand{TenantID: "site_001", OlderThan: time.Now().UTC().Add(-2 * time.Hour), Operator: "ops", Reason: "timeout"})
	if err != nil {
		t.Fatalf("timeout release: %v", err)
	}
	if len(result.Items) != 0 || repo.grantOrders[0].GrantStatus != GrantPending {
		t.Fatalf("manual order should be skipped, result=%+v status=%s", result, repo.grantOrders[0].GrantStatus)
	}
}

func testManualGrant(id string, activityID string, participationID string, idempotencyKey string, rewardCode string, amount string, mode GrantMode, now time.Time) GrantOrder {
	payload, _ := json.Marshal(map[string]any{"rewardCode": rewardCode, "rewardType": RewardBonus, "calculateType": CalculateFixed, "grantMode": mode, "grantChannel": GrantManualFile})
	order := testAtomicGrant(id, activityID, participationID, idempotencyKey, now)
	order.RewardAmount = amount
	order.RewardPayload = payload
	order.GrantChannel = GrantManualFile
	return order
}

func testAtomicParticipation(id string, activityID string, idempotencyKey string, now time.Time) Participation {
	return Participation{
		ID:             id,
		TenantID:       "site_001",
		ActivityID:     activityID,
		EventInboxID:   "inbox_" + id,
		Subject:        EventSubject{Type: "USER", ID: "user_1"},
		Biz:            EventBiz{BizType: "DEPOSIT", BizID: "pay_" + id},
		Status:         ParticipationRewardable,
		Snapshot:       []byte(`{}`),
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testAtomicGrant(id string, activityID string, participationID string, idempotencyKey string, now time.Time) GrantOrder {
	return GrantOrder{
		ID:              id,
		TenantID:        "site_001",
		ActivityID:      activityID,
		ParticipationID: participationID,
		Subject:         EventSubject{Type: "USER", ID: "user_1"},
		RewardType:      RewardBonus,
		RewardAmount:    "1",
		RewardPayload:   []byte(`{}`),
		BillNo:          "cmd_" + id,
		GrantStatus:     GrantPending,
		GrantChannel:    GrantHTTP,
		IdempotencyKey:  idempotencyKey,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func testAtomicOutbox(id string, now time.Time) OutboxMessage {
	return OutboxMessage{
		ID:            id,
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: GrantHTTP,
		TargetRef:     "https://reward.example.com",
		Payload:       []byte(`{}`),
		SendStatus:    OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func assertMemoryQuotaFrozen(t *testing.T, repo *MemoryRepository, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, want string) {
	t.Helper()
	for _, quota := range repo.quotas {
		if quota.ActivityID == activityID && quota.Type == quotaType && quota.Scope == scope && quota.ScopeKey == scopeKey {
			if quota.FrozenValue != want {
				t.Fatalf("expected quota %s/%s/%s frozen %s, got %s", quotaType, scope, scopeKey, want, quota.FrozenValue)
			}
			return
		}
	}
	t.Fatalf("quota not found: %s/%s/%s", quotaType, scope, scopeKey)
}

func assertMemoryQuotaValues(t *testing.T, repo *MemoryRepository, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, wantUsed string, wantFrozen string) {
	t.Helper()
	for _, quota := range repo.quotas {
		if quota.ActivityID == activityID && quota.Type == quotaType && quota.Scope == scope && quota.ScopeKey == scopeKey {
			if quota.UsedValue != wantUsed || quota.FrozenValue != wantFrozen {
				t.Fatalf("expected quota %s/%s/%s used=%s frozen=%s, got used=%s frozen=%s", quotaType, scope, scopeKey, wantUsed, wantFrozen, quota.UsedValue, quota.FrozenValue)
			}
			return
		}
	}
	t.Fatalf("quota not found: %s/%s/%s", quotaType, scope, scopeKey)
}
