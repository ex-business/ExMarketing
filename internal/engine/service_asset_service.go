package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (s *Service) SaveServiceDefinition(ctx context.Context, service marketing.ServiceDefinition) (marketing.ServiceDefinition, error) {
	now := s.now()
	if service.ID == "" {
		service.ID = marketing.NewID("svc")
	}
	if service.ServiceType == "" {
		service.ServiceType = marketing.ServiceTypeExternal
	}
	if service.Scope == "" {
		service.Scope = marketing.ServiceAssetScopeWorkspace
	}
	if service.Status == "" {
		service.Status = marketing.ServiceAssetActive
	}
	if len(service.Metadata) == 0 {
		service.Metadata = json.RawMessage(`{}`)
	}
	if service.CreatedAt.IsZero() {
		service.CreatedAt = now
	}
	service.UpdatedAt = now
	if err := service.Validate(); err != nil {
		return marketing.ServiceDefinition{}, err
	}
	return s.repository.SaveServiceDefinition(ctx, service)
}

func (s *Service) GetServiceDefinition(ctx context.Context, tenantID string, code string) (marketing.ServiceDefinition, error) {
	if service, ok := builtInServiceDefinition(tenantID, code); ok {
		return service, nil
	}
	return s.repository.GetServiceDefinition(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(code))
}

func (s *Service) ListServiceDefinitions(ctx context.Context, query marketing.ServiceDefinitionQuery) ([]marketing.ServiceDefinition, error) {
	services, err := s.repository.ListServiceDefinitions(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, service := range builtInServiceDefinitions(firstNonEmpty(query.TenantID, "site_001")) {
		if serviceDefinitionMatchesQuery(service, query) {
			services = append([]marketing.ServiceDefinition{service}, services...)
		}
	}
	return services, nil
}

func (s *Service) SaveServiceInterface(ctx context.Context, serviceInterface marketing.ServiceInterface) (marketing.ServiceInterface, error) {
	now := s.now()
	if serviceInterface.ID == "" {
		serviceInterface.ID = marketing.NewID("svcif")
	}
	if serviceInterface.Operation == "" {
		serviceInterface.Operation = marketing.ServiceOperationCall
	}
	if serviceInterface.Status == "" {
		serviceInterface.Status = marketing.ServiceAssetActive
	}
	if len(serviceInterface.Metadata) == 0 {
		serviceInterface.Metadata = json.RawMessage(`{}`)
	}
	if serviceInterface.CreatedAt.IsZero() {
		serviceInterface.CreatedAt = now
	}
	serviceInterface.UpdatedAt = now
	if err := serviceInterface.Validate(); err != nil {
		return marketing.ServiceInterface{}, err
	}
	service, err := s.GetServiceDefinition(ctx, serviceInterface.TenantID, serviceInterface.ServiceRef)
	if err != nil {
		return marketing.ServiceInterface{}, err
	}
	if err := s.ensureBuiltInServiceDefinitionPersisted(ctx, service); err != nil {
		return marketing.ServiceInterface{}, err
	}
	return s.repository.SaveServiceInterface(ctx, serviceInterface)
}

func (s *Service) GetServiceInterface(ctx context.Context, tenantID string, serviceRef string, code string) (marketing.ServiceInterface, error) {
	if serviceInterface, ok := builtInServiceInterface(tenantID, serviceRef, code); ok {
		return serviceInterface, nil
	}
	return s.repository.GetServiceInterface(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(serviceRef), strings.TrimSpace(code))
}

func (s *Service) ListServiceInterfaces(ctx context.Context, query marketing.ServiceInterfaceQuery) ([]marketing.ServiceInterface, error) {
	interfaces, err := s.repository.ListServiceInterfaces(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, serviceInterface := range builtInServiceInterfaces(firstNonEmpty(query.TenantID, "site_001")) {
		if serviceInterfaceMatchesQuery(serviceInterface, query) {
			interfaces = append([]marketing.ServiceInterface{serviceInterface}, interfaces...)
		}
	}
	return interfaces, nil
}

func (s *Service) SaveServiceProvider(ctx context.Context, provider marketing.ServiceProvider) (marketing.ServiceProvider, error) {
	now := s.now()
	if provider.ID == "" {
		provider.ID = marketing.NewID("svcprov")
	}
	if provider.Protocol == "" {
		provider.Protocol = marketing.ServiceProviderResource
	}
	if provider.Status == "" {
		provider.Status = marketing.ServiceAssetActive
	}
	if len(provider.Config) == 0 {
		provider.Config = json.RawMessage(`{}`)
	}
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = now
	}
	provider.UpdatedAt = now
	if err := provider.Validate(); err != nil {
		return marketing.ServiceProvider{}, err
	}
	serviceInterface, err := s.GetServiceInterface(ctx, provider.TenantID, provider.ServiceRef, provider.InterfaceRef)
	if err != nil {
		return marketing.ServiceProvider{}, err
	}
	if err := s.ensureBuiltInServiceInterfacePersisted(ctx, serviceInterface); err != nil {
		return marketing.ServiceProvider{}, err
	}
	return s.repository.SaveServiceProvider(ctx, provider)
}

func (s *Service) GetServiceProvider(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, code string) (marketing.ServiceProvider, error) {
	if provider, ok := builtInServiceProvider(tenantID, serviceRef, interfaceRef, code); ok {
		return provider, nil
	}
	return s.repository.GetServiceProvider(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(serviceRef), strings.TrimSpace(interfaceRef), strings.TrimSpace(code))
}

func (s *Service) ListServiceProviders(ctx context.Context, query marketing.ServiceProviderQuery) ([]marketing.ServiceProvider, error) {
	providers, err := s.repository.ListServiceProviders(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, provider := range builtInServiceProviders(firstNonEmpty(query.TenantID, "site_001")) {
		if serviceProviderMatchesQuery(provider, query) {
			providers = append([]marketing.ServiceProvider{provider}, providers...)
		}
	}
	return providers, nil
}

func (s *Service) ResolveServiceProvider(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, providerPolicy string) (marketing.ServiceInterface, marketing.ServiceProvider, error) {
	if policy := strings.ToUpper(strings.TrimSpace(providerPolicy)); policy != "" && policy != "PRIMARY" {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, fmt.Errorf("SERVICE_PROVIDER_POLICY_UNSUPPORTED:%s", policy)
	}
	serviceInterface, err := s.GetServiceInterface(ctx, tenantID, serviceRef, interfaceRef)
	if err != nil {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, err
	}
	if serviceInterface.Status != marketing.ServiceAssetActive {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, errors.New("SERVICE_INTERFACE_DISABLED")
	}
	if providerRef != "" {
		provider, err := s.GetServiceProvider(ctx, tenantID, serviceRef, interfaceRef, providerRef)
		if err != nil {
			return marketing.ServiceInterface{}, marketing.ServiceProvider{}, err
		}
		if provider.Status != marketing.ServiceAssetActive {
			return marketing.ServiceInterface{}, marketing.ServiceProvider{}, errors.New("SERVICE_PROVIDER_DISABLED")
		}
		return serviceInterface, provider, nil
	}
	providers, err := s.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
		TenantID:     tenantID,
		ServiceRef:   serviceRef,
		InterfaceRef: interfaceRef,
		Status:       marketing.ServiceAssetActive,
		Limit:        50,
	})
	if err != nil {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, err
	}
	if len(providers) == 0 {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, marketing.ErrNotFound
	}
	// PRIMARY and omitted policy resolve to the first priority provider.
	return serviceInterface, providers[0], nil
}

func (s *Service) ensureBuiltInServiceDefinitionPersisted(ctx context.Context, service marketing.ServiceDefinition) error {
	if !isBuiltInServiceAsset(service.ID) {
		return nil
	}
	if _, err := s.repository.GetServiceDefinition(ctx, service.TenantID, service.Code); err == nil {
		return nil
	} else if !errors.Is(err, marketing.ErrNotFound) {
		return err
	}
	_, err := s.SaveServiceDefinition(ctx, service)
	return err
}

func (s *Service) ensureBuiltInServiceInterfacePersisted(ctx context.Context, serviceInterface marketing.ServiceInterface) error {
	if !isBuiltInServiceAsset(serviceInterface.ID) {
		return nil
	}
	if _, err := s.repository.GetServiceInterface(ctx, serviceInterface.TenantID, serviceInterface.ServiceRef, serviceInterface.Code); err == nil {
		return nil
	} else if !errors.Is(err, marketing.ErrNotFound) {
		return err
	}
	if service, ok := builtInServiceDefinition(serviceInterface.TenantID, serviceInterface.ServiceRef); ok {
		if err := s.ensureBuiltInServiceDefinitionPersisted(ctx, service); err != nil {
			return err
		}
	}
	_, err := s.SaveServiceInterface(ctx, serviceInterface)
	return err
}

func isBuiltInServiceAsset(id string) bool {
	return strings.HasPrefix(id, "builtin_")
}

func builtInServiceAssetID(tenantID string, parts ...string) string {
	segments := append([]string{"builtin", strings.TrimSpace(tenantID)}, parts...)
	for i, segment := range segments {
		segments[i] = strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(strings.TrimSpace(segment))
	}
	return strings.Join(segments, "_")
}

func builtInServiceDefinitions(tenantID string) []marketing.ServiceDefinition {
	return []marketing.ServiceDefinition{
		builtInService(tenantID, "user-profile-service", "用户画像服务", "user-platform", []string{"fact", "profile"}),
		builtInService(tenantID, "kyc-service", "KYC 服务", "risk-platform", []string{"fact", "kyc"}),
		builtInService(tenantID, "risk-service", "风控服务", "risk-platform", []string{"fact", "risk"}),
	}
}

func builtInService(tenantID string, code string, name string, owner string, tags []string) marketing.ServiceDefinition {
	return marketing.ServiceDefinition{
		ID:          builtInServiceAssetID(tenantID, code),
		TenantID:    tenantID,
		Code:        code,
		Name:        name,
		ServiceType: marketing.ServiceTypeSystem,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
		Owner:       owner,
		Tags:        tags,
		Metadata:    json.RawMessage(`{"builtin":true}`),
	}
}

func builtInServiceDefinition(tenantID string, code string) (marketing.ServiceDefinition, bool) {
	for _, service := range builtInServiceDefinitions(tenantID) {
		if service.Code == strings.TrimSpace(code) {
			return service, true
		}
	}
	return marketing.ServiceDefinition{}, false
}

func builtInServiceInterfaces(tenantID string) []marketing.ServiceInterface {
	return []marketing.ServiceInterface{
		makeBuiltInServiceInterface(tenantID, "user-profile-service", "query-user-profile", "查询用户画像", "user-profile-query-v1", "user-profile-v1"),
		makeBuiltInServiceInterface(tenantID, "kyc-service", "query-kyc-profile", "查询 KYC 画像", "kyc-profile-query-v1", "kyc-profile-v1"),
		makeBuiltInServiceInterface(tenantID, "risk-service", "query-risk-profile", "查询风控画像", "risk-profile-query-v1", "risk-profile-v1"),
	}
}

func makeBuiltInServiceInterface(tenantID string, serviceRef string, code string, name string, inputSchema string, outputSchema string) marketing.ServiceInterface {
	return marketing.ServiceInterface{
		ID:              builtInServiceAssetID(tenantID, serviceRef, code),
		TenantID:        tenantID,
		ServiceRef:      serviceRef,
		Code:            code,
		Name:            name,
		Operation:       marketing.ServiceOperationCall,
		InputSchemaRef:  inputSchema,
		OutputSchemaRef: outputSchema,
		TimeoutMS:       3000,
		RetryPolicy:     "STANDARD",
		LuaCallable:     true,
		Status:          marketing.ServiceAssetActive,
		Metadata:        json.RawMessage(`{"builtin":true}`),
	}
}

func builtInServiceInterface(tenantID string, serviceRef string, code string) (marketing.ServiceInterface, bool) {
	for _, serviceInterface := range builtInServiceInterfaces(tenantID) {
		if serviceInterface.ServiceRef == strings.TrimSpace(serviceRef) && serviceInterface.Code == strings.TrimSpace(code) {
			return serviceInterface, true
		}
	}
	return marketing.ServiceInterface{}, false
}

func builtInServiceProviders(tenantID string) []marketing.ServiceProvider {
	return []marketing.ServiceProvider{
		builtInResourceBackedProvider(tenantID, "user-profile-service", "query-user-profile", "user-profile-provider-primary", defaultUserProfileProvider),
		builtInResourceBackedProvider(tenantID, "kyc-service", "query-kyc-profile", "kyc-provider-primary", defaultKYCProvider),
		builtInResourceBackedProvider(tenantID, "risk-service", "query-risk-profile", "risk-provider-primary", defaultRiskProvider),
	}
}

func builtInResourceBackedProvider(tenantID string, serviceRef string, interfaceRef string, code string, resourceRef string) marketing.ServiceProvider {
	config, _ := json.Marshal(map[string]any{"targetResourceRef": resourceRef, "builtin": true})
	return marketing.ServiceProvider{
		ID:           builtInServiceAssetID(tenantID, serviceRef, interfaceRef, code),
		TenantID:     tenantID,
		ServiceRef:   serviceRef,
		InterfaceRef: interfaceRef,
		Code:         code,
		Name:         code,
		Protocol:     marketing.ServiceProviderResource,
		ResourceRefs: []string{resourceRef},
		Config:       config,
		Priority:     0,
		Status:       marketing.ServiceAssetActive,
	}
}

func builtInServiceProvider(tenantID string, serviceRef string, interfaceRef string, code string) (marketing.ServiceProvider, bool) {
	for _, provider := range builtInServiceProviders(tenantID) {
		if provider.ServiceRef == strings.TrimSpace(serviceRef) && provider.InterfaceRef == strings.TrimSpace(interfaceRef) && provider.Code == strings.TrimSpace(code) {
			return provider, true
		}
	}
	return marketing.ServiceProvider{}, false
}

func serviceDefinitionMatchesQuery(service marketing.ServiceDefinition, query marketing.ServiceDefinitionQuery) bool {
	return (query.TenantID == "" || service.TenantID == query.TenantID) &&
		(query.WorkspaceID == "" || service.WorkspaceID == query.WorkspaceID) &&
		(query.FolderID == "" || service.FolderID == query.FolderID) &&
		(query.Status == "" || service.Status == query.Status) &&
		(query.Code == "" || service.Code == query.Code)
}

func serviceInterfaceMatchesQuery(serviceInterface marketing.ServiceInterface, query marketing.ServiceInterfaceQuery) bool {
	return (query.TenantID == "" || serviceInterface.TenantID == query.TenantID) &&
		(query.ServiceRef == "" || serviceInterface.ServiceRef == query.ServiceRef) &&
		(query.Status == "" || serviceInterface.Status == query.Status) &&
		(query.Code == "" || serviceInterface.Code == query.Code)
}

func serviceProviderMatchesQuery(provider marketing.ServiceProvider, query marketing.ServiceProviderQuery) bool {
	return (query.TenantID == "" || provider.TenantID == query.TenantID) &&
		(query.ServiceRef == "" || provider.ServiceRef == query.ServiceRef) &&
		(query.InterfaceRef == "" || provider.InterfaceRef == query.InterfaceRef) &&
		(query.Protocol == "" || provider.Protocol == query.Protocol) &&
		(query.Status == "" || provider.Status == query.Status) &&
		(query.Code == "" || provider.Code == query.Code)
}
