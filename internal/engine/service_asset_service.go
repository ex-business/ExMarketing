package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const builtInServiceCatalogVersion = "service-governance-v1"

func serviceAssetStatusActive(status marketing.ServiceAssetStatus, deletedAt *time.Time) bool {
	return status == marketing.ServiceAssetActive && deletedAt == nil
}

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
	if !serviceAssetStatusActive(service.Status, service.DeletedAt) {
		return marketing.ServiceInterface{}, errors.New("SERVICE_PARENT_DELETED")
	}
	if err := s.ensureBuiltInServiceDefinitionPersisted(ctx, service); err != nil {
		return marketing.ServiceInterface{}, err
	}
	if err := s.validateServiceInterfaceSchemas(ctx, serviceInterface); err != nil {
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
	if err := s.ensureServiceDefinitionActive(ctx, provider.TenantID, provider.ServiceRef); err != nil {
		return marketing.ServiceProvider{}, err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return marketing.ServiceProvider{}, errors.New("SERVICE_INTERFACE_PARENT_DELETED")
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

func (s *Service) SaveServiceSchema(ctx context.Context, schema marketing.ServiceSchema) (marketing.ServiceSchema, error) {
	now := s.now()
	if schema.ID == "" {
		schema.ID = marketing.NewID("svcschema")
	}
	if schema.Scope == "" {
		schema.Scope = marketing.ServiceAssetScopeTenant
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	if schema.Status == "" {
		schema.Status = marketing.ServiceAssetActive
	}
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = now
	}
	schema.UpdatedAt = now
	if err := schema.Validate(); err != nil {
		return marketing.ServiceSchema{}, err
	}
	validator := serviceSchemaValidator{service: s}
	if err := validator.validateSchemaDocument(schema.SchemaJSON); err != nil {
		return marketing.ServiceSchema{}, fmt.Errorf("SERVICE_SCHEMA_FORMAT_INVALID:%w", err)
	}
	if err := s.validateServiceSchemaCompatibility(ctx, schema); err != nil {
		return marketing.ServiceSchema{}, err
	}
	return s.repository.SaveServiceSchema(ctx, schema)
}

func (s *Service) ListServiceSchemas(ctx context.Context, query marketing.ServiceSchemaQuery) ([]marketing.ServiceSchema, error) {
	schemas, err := s.repository.ListServiceSchemas(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, schema := range builtInServiceSchemas(firstNonEmpty(query.TenantID, "site_001")) {
		if serviceSchemaMatchesQuery(schema, query) {
			schemas = append([]marketing.ServiceSchema{schema}, schemas...)
		}
	}
	return schemas, nil
}

func (s *Service) SaveServiceInterfaceBinding(ctx context.Context, binding marketing.ServiceInterfaceBinding) (marketing.ServiceInterfaceBinding, error) {
	now := s.now()
	if binding.ID == "" {
		binding.ID = marketing.NewID("svcbind")
	}
	if binding.Scope == "" {
		binding.Scope = marketing.ServiceAssetScopeWorkspace
	}
	if binding.Status == "" {
		binding.Status = marketing.ServiceAssetActive
	}
	if len(binding.Metadata) == 0 {
		binding.Metadata = json.RawMessage(`{}`)
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	if err := binding.Validate(); err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	serviceInterface, err := s.GetServiceInterface(ctx, binding.TenantID, binding.ServiceRef, binding.InterfaceRef)
	if err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	if err := s.ensureServiceDefinitionActive(ctx, binding.TenantID, binding.ServiceRef); err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return marketing.ServiceInterfaceBinding{}, errors.New("SERVICE_INTERFACE_PARENT_DELETED")
	}
	if err := s.ensureBuiltInServiceInterfacePersisted(ctx, serviceInterface); err != nil {
		return marketing.ServiceInterfaceBinding{}, err
	}
	return s.repository.SaveServiceInterfaceBinding(ctx, binding)
}

func (s *Service) ListServiceInterfaceBindings(ctx context.Context, query marketing.ServiceInterfaceBindingQuery) ([]marketing.ServiceInterfaceBinding, error) {
	return s.repository.ListServiceInterfaceBindings(ctx, query)
}

func (s *Service) ensureServiceDefinitionActive(ctx context.Context, tenantID string, serviceRef string) error {
	service, err := s.GetServiceDefinition(ctx, tenantID, serviceRef)
	if err != nil {
		return err
	}
	if !serviceAssetStatusActive(service.Status, service.DeletedAt) {
		return errors.New("SERVICE_PARENT_DELETED")
	}
	return nil
}

func (s *Service) ensureServiceInterfaceActive(ctx context.Context, tenantID string, serviceRef string, interfaceRef string) error {
	if err := s.ensureServiceDefinitionActive(ctx, tenantID, serviceRef); err != nil {
		return err
	}
	serviceInterface, err := s.GetServiceInterface(ctx, tenantID, serviceRef, interfaceRef)
	if err != nil {
		return err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return errors.New("SERVICE_INTERFACE_PARENT_DELETED")
	}
	return nil
}

func (s *Service) ensureServiceDefinitionActiveIfPresent(ctx context.Context, tenantID string, serviceRef string) error {
	service, err := s.GetServiceDefinition(ctx, tenantID, serviceRef)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return nil
		}
		return err
	}
	if !serviceAssetStatusActive(service.Status, service.DeletedAt) {
		return errors.New("SERVICE_PARENT_DELETED")
	}
	return nil
}

func (s *Service) ensureServiceInterfaceActiveIfPresent(ctx context.Context, tenantID string, serviceRef string, interfaceRef string) error {
	if err := s.ensureServiceDefinitionActiveIfPresent(ctx, tenantID, serviceRef); err != nil {
		return err
	}
	serviceInterface, err := s.GetServiceInterface(ctx, tenantID, serviceRef, interfaceRef)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return nil
		}
		return err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return errors.New("SERVICE_INTERFACE_PARENT_DELETED")
	}
	return nil
}

func (s *Service) getActiveServiceSchema(ctx context.Context, tenantID string, schemaRef string, schemaType marketing.ServiceSchemaType) (marketing.ServiceSchema, error) {
	schemas, err := s.ListServiceSchemas(ctx, marketing.ServiceSchemaQuery{
		TenantID:   strings.TrimSpace(tenantID),
		SchemaRef:  strings.TrimSpace(schemaRef),
		SchemaType: schemaType,
		Status:     marketing.ServiceAssetActive,
		Limit:      100,
	})
	if err != nil {
		return marketing.ServiceSchema{}, err
	}
	var selected marketing.ServiceSchema
	for _, schema := range schemas {
		if schema.Status != marketing.ServiceAssetActive {
			continue
		}
		if selected.ID == "" || schema.Version > selected.Version {
			selected = schema
		}
	}
	if selected.ID == "" {
		return marketing.ServiceSchema{}, marketing.ErrNotFound
	}
	return selected, nil
}

func (s *Service) validateServiceInterfaceSchemas(ctx context.Context, serviceInterface marketing.ServiceInterface) error {
	return (serviceSchemaValidator{service: s}).validateInterfaceSchemas(ctx, serviceInterface.TenantID, serviceInterface)
}

func (s *Service) validateServiceSchemaCompatibility(ctx context.Context, schema marketing.ServiceSchema) error {
	if schema.Version <= 1 {
		return nil
	}
	schemas, err := s.ListServiceSchemas(ctx, marketing.ServiceSchemaQuery{
		TenantID:   schema.TenantID,
		SchemaRef:  schema.SchemaRef,
		SchemaType: schema.SchemaType,
		Status:     marketing.ServiceAssetActive,
		Limit:      100,
	})
	if err != nil {
		return err
	}
	var previous marketing.ServiceSchema
	for _, candidate := range schemas {
		if candidate.Version >= schema.Version {
			continue
		}
		if previous.ID == "" || candidate.Version > previous.Version {
			previous = candidate
		}
	}
	if previous.ID == "" {
		return nil
	}
	if err := serviceSchemaCompatible(previous, schema); err != nil {
		return fmt.Errorf("SERVICE_SCHEMA_INCOMPATIBLE:%s:%w", schema.SchemaRef, err)
	}
	return nil
}

type ServiceSchemaValidationResult struct {
	Valid bool   `json:"valid"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type ServiceSchemaCompatibilityResult struct {
	Compatible bool   `json:"compatible"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Service) ValidateServiceSchemaDocument(schema marketing.ServiceSchema) ServiceSchemaValidationResult {
	if err := schema.Validate(); err != nil {
		return ServiceSchemaValidationResult{Valid: false, Code: "SERVICE_SCHEMA_INVALID", Error: err.Error()}
	}
	if err := (serviceSchemaValidator{service: s}).validateSchemaDocument(schema.SchemaJSON); err != nil {
		return ServiceSchemaValidationResult{Valid: false, Code: "SERVICE_SCHEMA_FORMAT_INVALID", Error: err.Error()}
	}
	if len(schema.ExamplesJSON) > 0 {
		var examples []any
		if err := json.Unmarshal(schema.ExamplesJSON, &examples); err == nil {
			validator := serviceSchemaValidator{service: s}
			for index, example := range examples {
				if err := validator.validatePayload(schema.SchemaJSON, example); err != nil {
					return ServiceSchemaValidationResult{Valid: false, Code: "SERVICE_SCHEMA_EXAMPLE_INVALID", Error: fmt.Sprintf("examplesJson[%d]: %v", index, err)}
				}
			}
		}
	}
	return ServiceSchemaValidationResult{Valid: true}
}

func (s *Service) CheckServiceSchemaCompatibility(ctx context.Context, schema marketing.ServiceSchema) ServiceSchemaCompatibilityResult {
	if result := s.ValidateServiceSchemaDocument(schema); !result.Valid {
		return ServiceSchemaCompatibilityResult{Compatible: false, Code: result.Code, Error: result.Error}
	}
	if err := s.validateServiceSchemaCompatibility(ctx, schema); err != nil {
		return ServiceSchemaCompatibilityResult{Compatible: false, Code: "SERVICE_SCHEMA_INCOMPATIBLE", Error: err.Error()}
	}
	return ServiceSchemaCompatibilityResult{Compatible: true}
}

func (s *Service) ResolveServiceProvider(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, providerPolicy string) (marketing.ServiceInterface, marketing.ServiceProvider, error) {
	serviceInterface, providers, err := s.ResolveServiceProviders(ctx, tenantID, serviceRef, interfaceRef, providerRef, providerPolicy)
	if err != nil {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, err
	}
	if len(providers) == 0 {
		return marketing.ServiceInterface{}, marketing.ServiceProvider{}, marketing.ErrNotFound
	}
	return serviceInterface, providers[0], nil
}

func (s *Service) ResolveServiceProviders(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, providerPolicy string) (marketing.ServiceInterface, []marketing.ServiceProvider, error) {
	return s.resolveServiceProviders(ctx, serviceProviderResolveRequest{
		TenantID:       tenantID,
		ServiceRef:     serviceRef,
		InterfaceRef:   interfaceRef,
		ProviderRef:    providerRef,
		ProviderPolicy: providerPolicy,
	})
}

type serviceProviderResolveRequest struct {
	TenantID       string
	ServiceRef     string
	InterfaceRef   string
	ProviderRef    string
	ProviderPolicy string
	Environment    string
	Binding        marketing.ServiceInterfaceBinding
	SeedParts      []string
}

func (s *Service) resolveServiceProviders(ctx context.Context, req serviceProviderResolveRequest) (marketing.ServiceInterface, []marketing.ServiceProvider, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	serviceRef := strings.TrimSpace(req.ServiceRef)
	interfaceRef := strings.TrimSpace(req.InterfaceRef)
	providerRef := strings.TrimSpace(req.ProviderRef)
	policy := strings.ToUpper(strings.TrimSpace(req.ProviderPolicy))
	if policy == "" {
		policy = "PRIMARY"
	}
	if !serviceProviderPolicySupported(policy) {
		return marketing.ServiceInterface{}, nil, fmt.Errorf("SERVICE_PROVIDER_POLICY_UNSUPPORTED:%s", policy)
	}
	serviceInterface, err := s.GetServiceInterface(ctx, tenantID, serviceRef, interfaceRef)
	if err != nil {
		return marketing.ServiceInterface{}, nil, err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return marketing.ServiceInterface{}, nil, errors.New("SERVICE_INTERFACE_DISABLED")
	}
	if providerRef != "" {
		provider, err := s.GetServiceProvider(ctx, tenantID, serviceRef, interfaceRef, providerRef)
		if err != nil {
			return marketing.ServiceInterface{}, nil, err
		}
		if !serviceAssetStatusActive(provider.Status, provider.DeletedAt) {
			return marketing.ServiceInterface{}, nil, errors.New("SERVICE_PROVIDER_DISABLED")
		}
		if !serviceProviderEnvironmentAllowed(provider, req.Environment) {
			return marketing.ServiceInterface{}, nil, fmt.Errorf("SERVICE_PROVIDER_ENVIRONMENT_DENIED:%s", provider.Code)
		}
		if err := serviceProviderMockAllowed(provider, policy); err != nil {
			return marketing.ServiceInterface{}, nil, err
		}
		available, code, message, err := s.serviceProviderRuntimeAvailable(ctx, provider, serviceProviderGovernanceConfigOption{Binding: req.Binding})
		if err != nil {
			return marketing.ServiceInterface{}, nil, err
		}
		if !available {
			if message != "" {
				return marketing.ServiceInterface{}, nil, fmt.Errorf("%s:%s", code, message)
			}
			return marketing.ServiceInterface{}, nil, errors.New(code)
		}
		return serviceInterface, []marketing.ServiceProvider{provider}, nil
	}
	providers, err := s.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
		TenantID:     tenantID,
		ServiceRef:   serviceRef,
		InterfaceRef: interfaceRef,
		Status:       marketing.ServiceAssetActive,
		Limit:        50,
	})
	if err != nil {
		return marketing.ServiceInterface{}, nil, err
	}
	providers = filterServiceProviderCandidates(providers, policy)
	if policy == "CACHE_ASIDE" {
		providers = filterServiceProviderCacheAsideEnvironmentCandidates(providers, req.Environment)
	} else {
		providers = filterServiceProviderEnvironmentCandidates(providers, req.Environment)
	}
	providers, err = s.filterServiceProviderRuntimeCandidates(ctx, providers, req.Binding)
	if err != nil {
		return marketing.ServiceInterface{}, nil, err
	}
	if len(providers) == 0 {
		return marketing.ServiceInterface{}, nil, marketing.ErrNotFound
	}
	sortServiceProviderCandidates(providers)
	if policy == "CACHE_ASIDE" {
		cacheProviders, primaryProviders := splitCacheAsideServiceProviders(providers)
		out := make([]marketing.ServiceProvider, 0, len(cacheProviders)+1)
		out = append(out, cacheProviders...)
		if len(primaryProviders) > 0 {
			out = append(out, primaryProviders[0])
		}
		if len(out) == 0 {
			return marketing.ServiceInterface{}, nil, marketing.ErrNotFound
		}
		return serviceInterface, out, nil
	}
	if policy == "WEIGHTED" || policy == "WEIGHTED_WITH_FALLBACK" {
		selected, err := selectWeightedServiceProvider(providers, serviceProviderSelectionSeed(req, providers))
		if err != nil {
			return marketing.ServiceInterface{}, nil, err
		}
		if policy == "WEIGHTED_WITH_FALLBACK" {
			out := []marketing.ServiceProvider{selected}
			for _, provider := range providers {
				if provider.Code != selected.Code {
					out = append(out, provider)
				}
			}
			return serviceInterface, out, nil
		}
		return serviceInterface, []marketing.ServiceProvider{selected}, nil
	}
	if policy == "PRIMARY" || policy == "ENVIRONMENT_MATCH" || policy == "MOCK_ONLY" {
		return serviceInterface, providers[:1], nil
	}
	return serviceInterface, providers, nil
}

func (s *Service) filterServiceProviderRuntimeCandidates(ctx context.Context, providers []marketing.ServiceProvider, binding marketing.ServiceInterfaceBinding) ([]marketing.ServiceProvider, error) {
	out := make([]marketing.ServiceProvider, 0, len(providers))
	for _, provider := range providers {
		available, _, _, err := s.serviceProviderRuntimeAvailable(ctx, provider, serviceProviderGovernanceConfigOption{Binding: binding})
		if err != nil {
			return nil, err
		}
		if !available {
			continue
		}
		out = append(out, provider)
	}
	return out, nil
}

func serviceProviderPolicySupported(policy string) bool {
	switch strings.ToUpper(strings.TrimSpace(policy)) {
	case "", "PRIMARY", "PRIMARY_WITH_FALLBACK", "CACHE_ASIDE", "WEIGHTED", "WEIGHTED_WITH_FALLBACK", "ENVIRONMENT_MATCH", "MOCK_ONLY":
		return true
	default:
		return false
	}
}

func filterServiceProviderCandidates(providers []marketing.ServiceProvider, policy string) []marketing.ServiceProvider {
	out := make([]marketing.ServiceProvider, 0, len(providers))
	for _, provider := range providers {
		if !serviceAssetStatusActive(provider.Status, provider.DeletedAt) {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(policy)) {
		case "MOCK_ONLY":
			if !serviceProviderMockEnabled(provider) {
				continue
			}
		case "CACHE_ASIDE":
			if serviceProviderMockEnabled(provider) {
				continue
			}
		case "WEIGHTED", "WEIGHTED_WITH_FALLBACK":
			if serviceProviderMockEnabled(provider) || provider.Weight <= 0 {
				continue
			}
		default:
			if serviceProviderMockEnabled(provider) {
				continue
			}
		}
		out = append(out, provider)
	}
	return out
}

func splitCacheAsideServiceProviders(providers []marketing.ServiceProvider) ([]marketing.ServiceProvider, []marketing.ServiceProvider) {
	cacheProviders := make([]marketing.ServiceProvider, 0, len(providers))
	primaryProviders := make([]marketing.ServiceProvider, 0, len(providers))
	for _, provider := range providers {
		if serviceProviderCacheEnabled(provider) {
			cacheProviders = append(cacheProviders, provider)
			continue
		}
		primaryProviders = append(primaryProviders, provider)
	}
	return cacheProviders, primaryProviders
}

func filterServiceProviderEnvironmentCandidates(providers []marketing.ServiceProvider, environment string) []marketing.ServiceProvider {
	target := normalizeServiceProviderEnvironment(environment)
	exact := make([]marketing.ServiceProvider, 0, len(providers))
	fallback := make([]marketing.ServiceProvider, 0, len(providers))
	for _, provider := range providers {
		providerEnv := normalizeServiceProviderEnvironment(provider.Environment)
		if serviceProviderDefaultEnvironment(providerEnv) {
			fallback = append(fallback, provider)
			continue
		}
		if target != "" && providerEnv == target {
			exact = append(exact, provider)
		}
	}
	if target == "" {
		return fallback
	}
	if len(exact) > 0 {
		return exact
	}
	return fallback
}

func filterServiceProviderCacheAsideEnvironmentCandidates(providers []marketing.ServiceProvider, environment string) []marketing.ServiceProvider {
	cacheProviders := make([]marketing.ServiceProvider, 0, len(providers))
	primaryProviders := make([]marketing.ServiceProvider, 0, len(providers))
	for _, provider := range providers {
		if serviceProviderCacheEnabled(provider) {
			cacheProviders = append(cacheProviders, provider)
			continue
		}
		primaryProviders = append(primaryProviders, provider)
	}
	primaryProviders = filterServiceProviderEnvironmentCandidates(primaryProviders, environment)
	return append(cacheProviders, primaryProviders...)
}

func sortServiceProviderCandidates(providers []marketing.ServiceProvider) {
	sort.SliceStable(providers, func(i, j int) bool {
		left, right := providers[i], providers[j]
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.Weight != right.Weight {
			return left.Weight > right.Weight
		}
		return left.Code < right.Code
	})
}

func serviceProviderMockEnabled(provider marketing.ServiceProvider) bool {
	if normalizeServiceProviderEnvironment(provider.Environment) == "mock" {
		return true
	}
	var config map[string]any
	if len(provider.Config) == 0 || json.Unmarshal(provider.Config, &config) != nil {
		return false
	}
	if enabled, ok := config["mockEnabled"].(bool); ok {
		return enabled
	}
	if enabled, ok := config["mock"].(bool); ok {
		return enabled
	}
	return false
}

func serviceProviderCacheEnabled(provider marketing.ServiceProvider) bool {
	if normalizeServiceProviderEnvironment(provider.Environment) == "cache" {
		return true
	}
	config := serviceProviderConfigMap(provider)
	if role, ok := config["role"].(string); ok && strings.EqualFold(strings.TrimSpace(role), "CACHE") {
		return true
	}
	if enabled, ok := config["cache"].(bool); ok {
		return enabled
	}
	return false
}

func serviceProviderMockAllowed(provider marketing.ServiceProvider, policy string) error {
	mock := serviceProviderMockEnabled(provider)
	if strings.EqualFold(strings.TrimSpace(policy), "MOCK_ONLY") {
		if !mock {
			return fmt.Errorf("SERVICE_PROVIDER_MOCK_REQUIRED:%s", provider.Code)
		}
		return nil
	}
	if mock {
		return fmt.Errorf("SERVICE_PROVIDER_MOCK_DENIED:%s", provider.Code)
	}
	return nil
}

func serviceProviderEnvironmentAllowed(provider marketing.ServiceProvider, environment string) bool {
	providerEnv := normalizeServiceProviderEnvironment(provider.Environment)
	target := normalizeServiceProviderEnvironment(environment)
	if serviceProviderDefaultEnvironment(providerEnv) {
		return true
	}
	return target != "" && providerEnv == target
}

func normalizeServiceProviderEnvironment(environment string) string {
	return strings.ToLower(strings.TrimSpace(environment))
}

func serviceProviderDefaultEnvironment(environment string) bool {
	switch normalizeServiceProviderEnvironment(environment) {
	case "", "default":
		return true
	default:
		return false
	}
}

func selectWeightedServiceProvider(providers []marketing.ServiceProvider, seed string) (marketing.ServiceProvider, error) {
	total := 0
	for _, provider := range providers {
		if provider.Weight > 0 {
			total += provider.Weight
		}
	}
	if total <= 0 {
		return marketing.ServiceProvider{}, errors.New("SERVICE_PROVIDER_WEIGHT_INVALID")
	}
	bucket := serviceProviderSelectionBucket(seed, total)
	cursor := 0
	for _, provider := range providers {
		if provider.Weight <= 0 {
			continue
		}
		cursor += provider.Weight
		if bucket < cursor {
			return provider, nil
		}
	}
	return providers[len(providers)-1], nil
}

func serviceProviderSelectionBucket(seed string, total int) int {
	if total <= 0 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(seed))
	return int(hash.Sum64() % uint64(total))
}

func serviceProviderSelectionSeed(req serviceProviderResolveRequest, providers []marketing.ServiceProvider) string {
	parts := []string{
		strings.TrimSpace(req.TenantID),
		strings.TrimSpace(req.ServiceRef),
		strings.TrimSpace(req.InterfaceRef),
		strings.ToUpper(strings.TrimSpace(req.ProviderPolicy)),
		normalizeServiceProviderEnvironment(req.Environment),
	}
	parts = append(parts, req.SeedParts...)
	for _, provider := range providers {
		parts = append(parts, provider.Code, fmt.Sprint(provider.Weight))
	}
	return strings.Join(parts, "|")
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
		ID:             builtInServiceAssetID(tenantID, code),
		TenantID:       tenantID,
		Code:           code,
		Name:           name,
		ServiceType:    marketing.ServiceTypeSystem,
		Scope:          marketing.ServiceAssetScopeTenant,
		Status:         marketing.ServiceAssetActive,
		Owner:          owner,
		Tags:           tags,
		Builtin:        true,
		CatalogVersion: builtInServiceCatalogVersion,
		SourceHash:     builtInServiceSourceHash("service", tenantID, code, name, owner, strings.Join(tags, ",")),
		Metadata:       json.RawMessage(`{"builtin":true}`),
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
		Builtin:         true,
		CatalogVersion:  builtInServiceCatalogVersion,
		SourceHash:      builtInServiceSourceHash("interface", tenantID, serviceRef, code, inputSchema, outputSchema),
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

func builtInServiceSchemas(tenantID string) []marketing.ServiceSchema {
	return []marketing.ServiceSchema{
		builtInServiceSchema(tenantID, "user-profile-query-v1", marketing.ServiceSchemaInput, `{"type":"object","required":["factKeys"],"properties":{"factKeys":{"type":"array","items":{"type":"string"}}}}`),
		builtInServiceSchema(tenantID, "user-profile-v1", marketing.ServiceSchemaOutput, `{"type":"object","required":["facts"],"properties":{"facts":{"type":"object","additionalProperties":true}}}`),
		builtInServiceSchema(tenantID, "kyc-profile-query-v1", marketing.ServiceSchemaInput, `{"type":"object","required":["factKeys"],"properties":{"factKeys":{"type":"array","items":{"type":"string"}}}}`),
		builtInServiceSchema(tenantID, "kyc-profile-v1", marketing.ServiceSchemaOutput, `{"type":"object","required":["facts"],"properties":{"facts":{"type":"object","additionalProperties":true}}}`),
		builtInServiceSchema(tenantID, "risk-profile-query-v1", marketing.ServiceSchemaInput, `{"type":"object","required":["factKeys"],"properties":{"factKeys":{"type":"array","items":{"type":"string"}}}}`),
		builtInServiceSchema(tenantID, "risk-profile-v1", marketing.ServiceSchemaOutput, `{"type":"object","required":["facts"],"properties":{"facts":{"type":"object","additionalProperties":true}}}`),
	}
}

func builtInServiceSchema(tenantID string, schemaRef string, schemaType marketing.ServiceSchemaType, schemaJSON string) marketing.ServiceSchema {
	return marketing.ServiceSchema{
		ID:             builtInServiceAssetID(tenantID, schemaRef),
		TenantID:       tenantID,
		SchemaRef:      schemaRef,
		SchemaType:     schemaType,
		Scope:          marketing.ServiceAssetScopeTenant,
		Version:        1,
		Status:         marketing.ServiceAssetActive,
		SchemaJSON:     json.RawMessage(schemaJSON),
		ExamplesJSON:   json.RawMessage(`[]`),
		Description:    "built-in service schema",
		Builtin:        true,
		CatalogVersion: builtInServiceCatalogVersion,
		SourceHash:     builtInServiceSourceHash("schema", tenantID, schemaRef, string(schemaType), schemaJSON),
	}
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
		ID:             builtInServiceAssetID(tenantID, serviceRef, interfaceRef, code),
		TenantID:       tenantID,
		ServiceRef:     serviceRef,
		InterfaceRef:   interfaceRef,
		Code:           code,
		Name:           code,
		Protocol:       marketing.ServiceProviderResource,
		ResourceRefs:   []string{resourceRef},
		Config:         config,
		Priority:       0,
		Status:         marketing.ServiceAssetActive,
		Builtin:        true,
		CatalogVersion: builtInServiceCatalogVersion,
		SourceHash:     builtInServiceSourceHash("provider", tenantID, serviceRef, interfaceRef, code, resourceRef),
	}
}

func builtInServiceSourceHash(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "sha256:" + hex.EncodeToString(hash[:])
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

func serviceSchemaMatchesQuery(schema marketing.ServiceSchema, query marketing.ServiceSchemaQuery) bool {
	return (query.TenantID == "" || schema.TenantID == query.TenantID) &&
		(query.WorkspaceID == "" || schema.WorkspaceID == query.WorkspaceID) &&
		(query.FolderID == "" || schema.FolderID == query.FolderID) &&
		(query.SchemaRef == "" || schema.SchemaRef == query.SchemaRef) &&
		(query.SchemaType == "" || schema.SchemaType == query.SchemaType) &&
		(query.Status == "" || schema.Status == query.Status)
}
