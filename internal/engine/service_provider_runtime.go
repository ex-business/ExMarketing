package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	defaultProviderRateLimitPerWindow       = 60
	defaultProviderRateLimitWindow          = time.Minute
	defaultProviderCircuitThreshold         = 3
	defaultProviderCircuitCooldown          = 30 * time.Second
	defaultProviderHalfOpenProbeLimit       = 1
	defaultProviderHalfOpenSuccessThreshold = 1
)

type serviceProviderRuntimeConfig struct {
	RateLimitPerWindow       int
	RateLimitWindow          time.Duration
	RateLimitPolicyRef       string
	RateLimitBackend         marketing.ServiceRateLimitBackend
	RateLimitScope           string
	CircuitThreshold         int
	CircuitCooldown          time.Duration
	HalfOpenProbeLimit       int
	HalfOpenSuccessThreshold int
}

type workerLeaseRepository interface {
	AcquireWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error)
	RenewWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error)
	ReleaseWorkerLease(ctx context.Context, name string, owner string, now time.Time) error
}

func (s *Service) SaveServiceProviderRuntimeState(ctx context.Context, state marketing.ServiceProviderRuntimeState) (marketing.ServiceProviderRuntimeState, error) {
	state = s.normalizeServiceProviderRuntimeState(state)
	if err := state.Validate(); err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	return s.repository.SaveServiceProviderRuntimeState(ctx, state)
}

func (s *Service) ListServiceProviderRuntimeStates(ctx context.Context, query marketing.ServiceProviderRuntimeStateQuery) ([]marketing.ServiceProviderRuntimeState, error) {
	return s.repository.ListServiceProviderRuntimeStates(ctx, query)
}

func (s *Service) AcquireWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	repository, ok := s.repository.(workerLeaseRepository)
	if !ok {
		return marketing.WorkerLease{}, false, errors.New("WORKER_LEASE_REPOSITORY_NOT_CONFIGURED")
	}
	if cmd.Now.IsZero() {
		cmd.Now = s.now()
	}
	return repository.AcquireWorkerLease(ctx, cmd)
}

func (s *Service) ReleaseWorkerLease(ctx context.Context, name string, owner string) error {
	repository, ok := s.repository.(workerLeaseRepository)
	if !ok {
		return errors.New("WORKER_LEASE_REPOSITORY_NOT_CONFIGURED")
	}
	return repository.ReleaseWorkerLease(ctx, name, owner, s.now())
}

func (s *Service) RenewWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	repository, ok := s.repository.(workerLeaseRepository)
	if !ok {
		return marketing.WorkerLease{}, false, errors.New("WORKER_LEASE_REPOSITORY_NOT_CONFIGURED")
	}
	if cmd.Now.IsZero() {
		cmd.Now = s.now()
	}
	return repository.RenewWorkerLease(ctx, cmd)
}

func (s *Service) ListServiceProviderHealthSummaries(ctx context.Context, query marketing.ServiceProviderHealthSummaryQuery) ([]marketing.ServiceProviderHealthSummary, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	const providerPageSize = 500
	items := make([]marketing.ServiceProviderHealthSummary, 0, limit)
	matched := 0
	for providerOffset := 0; ; providerOffset += providerPageSize {
		providers, err := s.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
			TenantID:     query.TenantID,
			ServiceRef:   query.ServiceRef,
			InterfaceRef: query.InterfaceRef,
			Status:       marketing.ServiceAssetActive,
			Code:         query.ProviderRef,
			Limit:        providerPageSize,
			Offset:       providerOffset,
		})
		if err != nil {
			return nil, err
		}
		if len(providers) == 0 {
			break
		}
		for _, provider := range providers {
			state, err := s.serviceProviderRuntimeState(ctx, provider)
			if err != nil {
				return nil, err
			}
			if query.HealthStatus != "" && state.HealthStatus != query.HealthStatus {
				continue
			}
			if query.CircuitStatus != "" && state.CircuitStatus != query.CircuitStatus {
				continue
			}
			summary, err := s.serviceProviderHealthSummary(ctx, provider, state)
			if err != nil {
				return nil, err
			}
			if query.Status != "" && !strings.EqualFold(summary.Status, query.Status) {
				continue
			}
			if matched < offset {
				matched++
				continue
			}
			if len(items) < limit {
				items = append(items, summary)
			}
			matched++
			if len(items) >= limit {
				return items, nil
			}
		}
		if len(providers) < providerPageSize {
			break
		}
	}
	return items, nil
}

func (s *Service) SaveServiceRateLimitPolicy(ctx context.Context, policy marketing.ServiceRateLimitPolicy) (marketing.ServiceRateLimitPolicy, error) {
	policy = s.normalizeServiceRateLimitPolicy(policy)
	if err := policy.Validate(); err != nil {
		return marketing.ServiceRateLimitPolicy{}, err
	}
	if strings.TrimSpace(policy.InterfaceRef) != "" {
		if err := s.ensureServiceInterfaceActiveIfPresent(ctx, policy.TenantID, policy.ServiceRef, policy.InterfaceRef); err != nil {
			return marketing.ServiceRateLimitPolicy{}, err
		}
	} else if strings.TrimSpace(policy.ServiceRef) != "" {
		if err := s.ensureServiceDefinitionActiveIfPresent(ctx, policy.TenantID, policy.ServiceRef); err != nil {
			return marketing.ServiceRateLimitPolicy{}, err
		}
	}
	return s.repository.SaveServiceRateLimitPolicy(ctx, policy)
}

func (s *Service) ListServiceRateLimitPolicies(ctx context.Context, query marketing.ServiceRateLimitPolicyQuery) ([]marketing.ServiceRateLimitPolicy, error) {
	return s.repository.ListServiceRateLimitPolicies(ctx, query)
}

func (s *Service) normalizeServiceRateLimitPolicy(policy marketing.ServiceRateLimitPolicy) marketing.ServiceRateLimitPolicy {
	now := s.now()
	if policy.ID == "" {
		policy.ID = marketing.NewID("svcratelimit")
	}
	if policy.Name == "" {
		policy.Name = policy.Code
	}
	if policy.Backend == "" {
		policy.Backend = marketing.ServiceRateLimitBackendPostgres
	}
	if policy.Status == "" {
		policy.Status = marketing.ServiceAssetActive
	}
	if len(policy.Metadata) == 0 {
		policy.Metadata = json.RawMessage(`{}`)
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now
	return policy
}

func (s *Service) ResetServiceProviderCircuit(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string) (marketing.ServiceProviderRuntimeState, error) {
	provider, err := s.GetServiceProvider(ctx, tenantID, serviceRef, interfaceRef, providerRef)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	if !serviceAssetStatusActive(provider.Status, provider.DeletedAt) {
		return marketing.ServiceProviderRuntimeState{}, errors.New("SERVICE_PROVIDER_DISABLED")
	}
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	now := s.now()
	state.CircuitStatus = marketing.ServiceProviderCircuitClosed
	state.HealthStatus = marketing.ServiceProviderHealthHealthy
	state.HealthMessage = ""
	state.ConsecutiveFailures = 0
	state.HalfOpenProbeCount = 0
	state.HalfOpenSuccessCount = 0
	state.CircuitOpenedAt = nil
	state.CircuitResetAt = &now
	state.UpdatedAt = now
	return s.repository.SaveServiceProviderRuntimeState(ctx, state)
}

func (s *Service) CheckServiceProviderHealth(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, actor string) (marketing.ServiceProviderRuntimeState, error) {
	provider, err := s.GetServiceProvider(ctx, tenantID, serviceRef, interfaceRef, providerRef)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	if !serviceAssetStatusActive(provider.Status, provider.DeletedAt) {
		return marketing.ServiceProviderRuntimeState{}, errors.New("SERVICE_PROVIDER_DISABLED")
	}
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	resourceRef := serviceProviderHealthCheckResourceRef(provider)
	now := s.now()
	state.UpdatedAt = now
	state.Metadata = mergeServiceProviderRuntimeMetadata(state.Metadata, map[string]any{
		"lastHealthCheckAt": now.Format(time.RFC3339Nano),
		"lastHealthCheckBy": strings.TrimSpace(actor),
	})
	if resourceRef == "" {
		state.HealthStatus = marketing.ServiceProviderHealthHealthy
		state.HealthMessage = "provider has no dry-run target"
		return s.repository.SaveServiceProviderRuntimeState(ctx, state)
	}
	result, err := s.TestResource(ctx, provider.TenantID, resourceRef, actor)
	if err != nil {
		state.HealthStatus = marketing.ServiceProviderHealthUnhealthy
		state.HealthMessage = err.Error()
		return s.repository.SaveServiceProviderRuntimeState(ctx, state)
	}
	if ok, _ := result["ok"].(bool); !ok {
		state.HealthStatus = marketing.ServiceProviderHealthUnhealthy
		state.HealthMessage = firstWorkflowString(workflowConfigString(result, "message"), workflowConfigString(result, "error"))
		return s.repository.SaveServiceProviderRuntimeState(ctx, state)
	}
	state.HealthStatus = marketing.ServiceProviderHealthHealthy
	state.HealthMessage = "health check passed"
	return s.repository.SaveServiceProviderRuntimeState(ctx, state)
}

func (s *Service) markServiceProviderUnhealthy(ctx context.Context, provider marketing.ServiceProvider, message string) (marketing.ServiceProviderRuntimeState, error) {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	state.HealthStatus = marketing.ServiceProviderHealthUnhealthy
	state.HealthMessage = strings.TrimSpace(message)
	state.UpdatedAt = s.now()
	return s.repository.SaveServiceProviderRuntimeState(ctx, state)
}

func (s *Service) normalizeServiceProviderRuntimeState(state marketing.ServiceProviderRuntimeState) marketing.ServiceProviderRuntimeState {
	now := s.now()
	if state.ID == "" {
		state.ID = marketing.NewID("svcprovstate")
	}
	if state.HealthStatus == "" {
		state.HealthStatus = marketing.ServiceProviderHealthHealthy
	}
	if state.CircuitStatus == "" {
		state.CircuitStatus = marketing.ServiceProviderCircuitClosed
	}
	cfg := serviceProviderRuntimeStateDefaults()
	if state.HalfOpenProbeLimit <= 0 {
		state.HalfOpenProbeLimit = cfg.HalfOpenProbeLimit
	}
	if state.HalfOpenSuccessThreshold <= 0 {
		state.HalfOpenSuccessThreshold = cfg.HalfOpenSuccessThreshold
	}
	if len(state.Metadata) == 0 {
		state.Metadata = json.RawMessage(`{}`)
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	return state
}

func (s *Service) serviceProviderRuntimeState(ctx context.Context, provider marketing.ServiceProvider) (marketing.ServiceProviderRuntimeState, error) {
	state, err := s.repository.GetServiceProviderRuntimeState(ctx, provider.TenantID, provider.ServiceRef, provider.InterfaceRef, provider.Code)
	if err == nil {
		return s.normalizeServiceProviderRuntimeState(state), nil
	}
	if !errors.Is(err, marketing.ErrNotFound) {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	state = marketing.ServiceProviderRuntimeState{
		TenantID:      provider.TenantID,
		ServiceRef:    provider.ServiceRef,
		InterfaceRef:  provider.InterfaceRef,
		ProviderRef:   provider.Code,
		HealthStatus:  marketing.ServiceProviderHealthHealthy,
		CircuitStatus: marketing.ServiceProviderCircuitClosed,
		Metadata:      json.RawMessage(`{}`),
	}
	return s.SaveServiceProviderRuntimeState(ctx, state)
}

type serviceProviderGovernanceConfigOption struct {
	Binding marketing.ServiceInterfaceBinding
}

func (s *Service) serviceProviderRuntimeAvailable(ctx context.Context, provider marketing.ServiceProvider, options ...serviceProviderGovernanceConfigOption) (bool, string, string, error) {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return false, "", "", err
	}
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider, options...)
	if err != nil {
		return false, "", "", err
	}
	now := s.now()
	halfOpenEligible := false
	if state.CircuitStatus == marketing.ServiceProviderCircuitOpen {
		if state.CircuitOpenedAt == nil || now.Sub(*state.CircuitOpenedAt) < cfg.CircuitCooldown {
			return false, "SERVICE_PROVIDER_CIRCUIT_OPEN", "provider circuit is open", nil
		}
		halfOpenEligible = true
	}
	if state.CircuitStatus == marketing.ServiceProviderCircuitHalfOpen && cfg.HalfOpenProbeLimit > 0 && state.HalfOpenProbeCount >= cfg.HalfOpenProbeLimit {
		return false, "SERVICE_PROVIDER_HALF_OPEN_PROBE_LIMITED", "provider half-open probe limit exceeded", nil
	}
	if state.HealthStatus == marketing.ServiceProviderHealthUnhealthy && !halfOpenEligible && state.CircuitStatus != marketing.ServiceProviderCircuitHalfOpen {
		return false, "SERVICE_PROVIDER_UNHEALTHY", state.HealthMessage, nil
	}
	if cfg.RateLimitBackend != marketing.ServiceRateLimitBackendRedis && serviceProviderRateLimitExceeded(state, cfg, now) {
		return false, "SERVICE_PROVIDER_RATE_LIMITED", "provider rate limit exceeded", nil
	}
	return true, "", "", nil
}

type serviceProviderRuntimeConsumeOption struct {
	Subject marketing.EventSubject
	Binding marketing.ServiceInterfaceBinding
}

func (s *Service) consumeServiceProviderRuntime(ctx context.Context, provider marketing.ServiceProvider, options ...serviceProviderRuntimeConsumeOption) (marketing.ServiceProviderRuntimeState, error) {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider, serviceProviderRuntimeGovernanceOption(options))
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	now := s.now()
	if state.CircuitStatus == marketing.ServiceProviderCircuitOpen {
		if state.CircuitOpenedAt == nil || now.Sub(*state.CircuitOpenedAt) < cfg.CircuitCooldown {
			return state, errors.New("SERVICE_PROVIDER_CIRCUIT_OPEN")
		}
		state.CircuitStatus = marketing.ServiceProviderCircuitHalfOpen
		state.HalfOpenProbeCount = 0
		state.HalfOpenSuccessCount = 0
	}
	if state.HealthStatus == marketing.ServiceProviderHealthUnhealthy && state.CircuitStatus != marketing.ServiceProviderCircuitHalfOpen {
		return state, errors.New("SERVICE_PROVIDER_UNHEALTHY:" + state.HealthMessage)
	}
	if state.CircuitStatus == marketing.ServiceProviderCircuitHalfOpen {
		state.HalfOpenProbeLimit = cfg.HalfOpenProbeLimit
		state.HalfOpenSuccessThreshold = cfg.HalfOpenSuccessThreshold
		if cfg.HalfOpenProbeLimit > 0 && state.HalfOpenProbeCount >= cfg.HalfOpenProbeLimit {
			return state, errors.New("SERVICE_PROVIDER_HALF_OPEN_PROBE_LIMITED")
		}
		state.HalfOpenProbeCount++
		state.UpdatedAt = now
		saved, err := s.repository.SaveServiceProviderRuntimeState(ctx, state)
		if err != nil {
			return marketing.ServiceProviderRuntimeState{}, err
		}
		state = saved
	}
	subject := serviceProviderRuntimeSubject(options)
	consume := marketing.ServiceProviderRateLimitConsume{
		State:       state,
		PolicyRef:   cfg.RateLimitPolicyRef,
		Limit:       cfg.RateLimitPerWindow,
		Window:      cfg.RateLimitWindow,
		Now:         now,
		Scope:       serviceProviderRateLimitScope(cfg, subject),
		SubjectType: strings.TrimSpace(subject.Type),
		SubjectID:   strings.TrimSpace(subject.ID),
		SubjectHash: serviceProviderRateLimitSubjectHash(cfg, subject),
	}
	var consumed bool
	if cfg.RateLimitBackend == marketing.ServiceRateLimitBackendRedis {
		if s.redisRateLimiter == nil {
			return marketing.ServiceProviderRuntimeState{}, ErrServiceRateLimitRedisNotConfigured
		}
		state, consumed, err = s.redisRateLimiter.ConsumeServiceProviderRateLimit(ctx, consume)
	} else {
		state, consumed, err = s.repository.ConsumeServiceProviderRateLimit(ctx, consume)
	}
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	if !consumed {
		return state, errors.New("SERVICE_PROVIDER_RATE_LIMITED")
	}
	return state, nil
}

func (s *Service) recordServiceProviderSuccess(ctx context.Context, provider marketing.ServiceProvider) error {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return err
	}
	now := s.now()
	state.SuccessCount++
	state.ConsecutiveFailures = 0
	state.HealthStatus = marketing.ServiceProviderHealthHealthy
	state.HealthMessage = ""
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider)
	if err != nil {
		return err
	}
	if state.CircuitStatus == marketing.ServiceProviderCircuitHalfOpen {
		state.HalfOpenSuccessCount++
		state.HalfOpenSuccessThreshold = cfg.HalfOpenSuccessThreshold
		if state.HalfOpenSuccessCount < cfg.HalfOpenSuccessThreshold {
			state.UpdatedAt = now
			_, err = s.repository.SaveServiceProviderRuntimeState(ctx, state)
			return err
		}
	}
	state.CircuitStatus = marketing.ServiceProviderCircuitClosed
	state.CircuitOpenedAt = nil
	state.HalfOpenProbeCount = 0
	state.HalfOpenSuccessCount = 0
	state.LastSuccessAt = &now
	state.UpdatedAt = now
	_, err = s.repository.SaveServiceProviderRuntimeState(ctx, state)
	return err
}

func (s *Service) recordServiceProviderFailure(ctx context.Context, provider marketing.ServiceProvider, message string) error {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return err
	}
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider)
	if err != nil {
		return err
	}
	now := s.now()
	state.FailureCount++
	state.ConsecutiveFailures++
	state.LastFailureAt = &now
	if strings.TrimSpace(message) != "" {
		state.HealthMessage = strings.TrimSpace(message)
	}
	if state.CircuitStatus == marketing.ServiceProviderCircuitHalfOpen || state.ConsecutiveFailures >= cfg.CircuitThreshold {
		state.HealthStatus = marketing.ServiceProviderHealthUnhealthy
		state.CircuitStatus = marketing.ServiceProviderCircuitOpen
		state.CircuitOpenedAt = &now
		state.HalfOpenProbeCount = 0
		state.HalfOpenSuccessCount = 0
	} else {
		state.HealthStatus = marketing.ServiceProviderHealthDegraded
	}
	state.UpdatedAt = now
	_, err = s.repository.SaveServiceProviderRuntimeState(ctx, state)
	return err
}

func (s *Service) serviceProviderHealthSummary(ctx context.Context, provider marketing.ServiceProvider, state marketing.ServiceProviderRuntimeState) (marketing.ServiceProviderHealthSummary, error) {
	status := string(state.HealthStatus)
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderHealthSummary{}, err
	}
	if state.CircuitStatus == marketing.ServiceProviderCircuitOpen {
		status = "CIRCUIT_OPEN"
	} else if cfg.RateLimitBackend != marketing.ServiceRateLimitBackendRedis && serviceProviderRateLimitExceeded(state, cfg, s.now()) {
		status = "RATE_LIMITED"
	}
	lastCheckedAt, lastCheckedBy := serviceProviderLastHealthCheck(state.Metadata)
	rateLimitWindow := (*time.Time)(nil)
	if !state.RateLimitWindow.IsZero() {
		value := state.RateLimitWindow
		rateLimitWindow = &value
	}
	return marketing.ServiceProviderHealthSummary{
		TenantID:         provider.TenantID,
		ServiceRef:       provider.ServiceRef,
		InterfaceRef:     provider.InterfaceRef,
		ProviderRef:      provider.Code,
		ProviderName:     provider.Name,
		ProviderStatus:   provider.Status,
		Protocol:         provider.Protocol,
		HealthStatus:     state.HealthStatus,
		CircuitStatus:    state.CircuitStatus,
		Status:           status,
		HealthMessage:    state.HealthMessage,
		RateLimitCount:   state.RateLimitCount,
		RateLimitWindow:  rateLimitWindow,
		LastCheckedAt:    lastCheckedAt,
		LastCheckedBy:    lastCheckedBy,
		LastSuccessAt:    state.LastSuccessAt,
		LastFailureAt:    state.LastFailureAt,
		CircuitOpenedAt:  state.CircuitOpenedAt,
		CircuitResetAt:   state.CircuitResetAt,
		RuntimeUpdatedAt: state.UpdatedAt,
	}, nil
}

func serviceProviderLastHealthCheck(raw json.RawMessage) (*time.Time, string) {
	metadata := map[string]any{}
	if len(raw) == 0 || json.Unmarshal(raw, &metadata) != nil {
		return nil, ""
	}
	actor, _ := metadata["lastHealthCheckBy"].(string)
	value, _ := metadata["lastHealthCheckAt"].(string)
	if strings.TrimSpace(value) == "" {
		return nil, strings.TrimSpace(actor)
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return nil, strings.TrimSpace(actor)
	}
	parsed = parsed.UTC()
	return &parsed, strings.TrimSpace(actor)
}

func serviceProviderHealthCheckResourceRef(provider marketing.ServiceProvider) string {
	if len(provider.ResourceRefs) > 0 {
		return strings.TrimSpace(provider.ResourceRefs[0])
	}
	var config map[string]any
	if len(provider.Config) == 0 || json.Unmarshal(provider.Config, &config) != nil {
		return ""
	}
	return firstWorkflowString(workflowConfigString(config, "targetResourceRef"), workflowConfigString(config, "resourceRef"))
}

func mergeServiceProviderRuntimeMetadata(raw json.RawMessage, values map[string]any) json.RawMessage {
	metadata := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &metadata)
	}
	for key, value := range values {
		if strings.TrimSpace(key) != "" {
			metadata[key] = value
		}
	}
	out, _ := json.Marshal(metadata)
	return out
}

func serviceProviderRateLimitExceeded(state marketing.ServiceProviderRuntimeState, cfg serviceProviderRuntimeConfig, now time.Time) bool {
	if cfg.RateLimitPerWindow <= 0 {
		return false
	}
	if state.RateLimitWindow.IsZero() || now.Sub(state.RateLimitWindow) >= cfg.RateLimitWindow {
		return false
	}
	return state.RateLimitCount >= cfg.RateLimitPerWindow
}

func serviceProviderRuntimeSubject(options []serviceProviderRuntimeConsumeOption) marketing.EventSubject {
	if len(options) == 0 {
		return marketing.EventSubject{}
	}
	return options[0].Subject
}

func serviceProviderRuntimeGovernanceOption(options []serviceProviderRuntimeConsumeOption) serviceProviderGovernanceConfigOption {
	if len(options) == 0 {
		return serviceProviderGovernanceConfigOption{}
	}
	return serviceProviderGovernanceConfigOption{Binding: options[0].Binding}
}

func serviceProviderRateLimitScope(cfg serviceProviderRuntimeConfig, subject marketing.EventSubject) string {
	if strings.EqualFold(strings.TrimSpace(cfg.RateLimitScope), "GLOBAL") {
		return "GLOBAL"
	}
	if strings.TrimSpace(subject.Type) == "" || strings.TrimSpace(subject.ID) == "" {
		return "ANONYMOUS"
	}
	return "SUBJECT"
}

func serviceProviderRateLimitSubjectHash(cfg serviceProviderRuntimeConfig, subject marketing.EventSubject) string {
	scope := serviceProviderRateLimitScope(cfg, subject)
	switch scope {
	case "GLOBAL":
		return "global"
	case "ANONYMOUS":
		return "anonymous"
	default:
		sum := sha256.Sum256([]byte(strings.TrimSpace(subject.Type) + "\x00" + strings.TrimSpace(subject.ID)))
		return fmt.Sprintf("sha256:%x", sum[:])
	}
}

func (s *Service) serviceProviderGovernanceConfig(ctx context.Context, provider marketing.ServiceProvider, options ...serviceProviderGovernanceConfigOption) (serviceProviderRuntimeConfig, error) {
	cfg := serviceProviderRuntimeConfig{
		RateLimitPerWindow:       defaultProviderRateLimitPerWindow,
		RateLimitWindow:          defaultProviderRateLimitWindow,
		RateLimitBackend:         marketing.ServiceRateLimitBackendPostgres,
		CircuitThreshold:         defaultProviderCircuitThreshold,
		CircuitCooldown:          defaultProviderCircuitCooldown,
		HalfOpenProbeLimit:       defaultProviderHalfOpenProbeLimit,
		HalfOpenSuccessThreshold: defaultProviderHalfOpenSuccessThreshold,
	}
	var raw map[string]any
	if len(provider.Config) > 0 && json.Unmarshal(provider.Config, &raw) == nil {
		cfg.RateLimitPerWindow = serviceProviderConfigInt(raw, cfg.RateLimitPerWindow, "rateLimitPerWindow", "rateLimitPerMinute", "rateLimit")
		if seconds := serviceProviderConfigInt(raw, 0, "rateLimitWindowSeconds", "rateWindowSeconds"); seconds > 0 {
			cfg.RateLimitWindow = time.Duration(seconds) * time.Second
		}
		cfg.CircuitThreshold = serviceProviderConfigInt(raw, cfg.CircuitThreshold, "circuitFailureThreshold", "failureThreshold")
		if seconds := serviceProviderConfigInt(raw, 0, "circuitCooldownSeconds", "circuitResetSeconds", "recoverySeconds"); seconds > 0 {
			cfg.CircuitCooldown = time.Duration(seconds) * time.Second
		}
		cfg.HalfOpenProbeLimit = serviceProviderConfigInt(raw, cfg.HalfOpenProbeLimit, "halfOpenProbeLimit", "probeLimit")
		cfg.HalfOpenSuccessThreshold = serviceProviderConfigInt(raw, cfg.HalfOpenSuccessThreshold, "halfOpenSuccessThreshold", "probeSuccessThreshold")
	}
	binding := serviceProviderGovernanceBinding(options)
	if strings.TrimSpace(binding.RateLimitRef) != "" {
		policy, err := s.matchBindingServiceRateLimitPolicy(ctx, provider, binding)
		if err != nil {
			return cfg, err
		}
		cfg.RateLimitPolicyRef = policy.Code
		cfg.RateLimitPerWindow = policy.Limit
		cfg.RateLimitWindow = time.Duration(policy.WindowSeconds) * time.Second
		cfg.RateLimitBackend = policy.Backend
		cfg.RateLimitScope = serviceRateLimitPolicyRuntimeScope(policy)
	} else if policy, ok, err := s.matchServiceRateLimitPolicy(ctx, provider); err != nil {
		return cfg, err
	} else if ok {
		cfg.RateLimitPolicyRef = policy.Code
		cfg.RateLimitPerWindow = policy.Limit
		cfg.RateLimitWindow = time.Duration(policy.WindowSeconds) * time.Second
		cfg.RateLimitBackend = policy.Backend
		cfg.RateLimitScope = serviceRateLimitPolicyRuntimeScope(policy)
	}
	if cfg.RateLimitPerWindow < 0 {
		cfg.RateLimitPerWindow = 0
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = defaultProviderRateLimitWindow
	}
	if cfg.CircuitThreshold <= 0 {
		cfg.CircuitThreshold = defaultProviderCircuitThreshold
	}
	if cfg.CircuitCooldown <= 0 {
		cfg.CircuitCooldown = defaultProviderCircuitCooldown
	}
	if cfg.HalfOpenProbeLimit <= 0 {
		cfg.HalfOpenProbeLimit = defaultProviderHalfOpenProbeLimit
	}
	if cfg.HalfOpenSuccessThreshold <= 0 {
		cfg.HalfOpenSuccessThreshold = defaultProviderHalfOpenSuccessThreshold
	}
	return cfg, nil
}

func serviceProviderGovernanceBinding(options []serviceProviderGovernanceConfigOption) marketing.ServiceInterfaceBinding {
	if len(options) == 0 {
		return marketing.ServiceInterfaceBinding{}
	}
	return options[0].Binding
}

func serviceRateLimitPolicyRuntimeScope(policy marketing.ServiceRateLimitPolicy) string {
	var metadata map[string]any
	if len(policy.Metadata) > 0 && json.Unmarshal(policy.Metadata, &metadata) == nil {
		for _, key := range []string{"rateLimitScope", "runtimeScope", "redisScope"} {
			if value := workflowOutputString(metadata[key]); strings.EqualFold(strings.TrimSpace(value), "GLOBAL") {
				return "GLOBAL"
			}
		}
	}
	return "SUBJECT"
}

func (s *Service) matchServiceRateLimitPolicy(ctx context.Context, provider marketing.ServiceProvider) (marketing.ServiceRateLimitPolicy, bool, error) {
	policies, err := s.repository.ListServiceRateLimitPolicies(ctx, marketing.ServiceRateLimitPolicyQuery{
		TenantID: provider.TenantID,
		Status:   marketing.ServiceAssetActive,
		Limit:    500,
	})
	if err != nil {
		return marketing.ServiceRateLimitPolicy{}, false, err
	}
	bestScore := -1
	var best marketing.ServiceRateLimitPolicy
	for _, policy := range policies {
		score, ok := serviceRateLimitPolicyMatchScore(policy, provider)
		if !ok {
			continue
		}
		if score > bestScore || (score == bestScore && strings.Compare(policy.Code, best.Code) < 0) {
			bestScore = score
			best = policy
		}
	}
	return best, bestScore >= 0, nil
}

func (s *Service) matchBindingServiceRateLimitPolicy(ctx context.Context, provider marketing.ServiceProvider, binding marketing.ServiceInterfaceBinding) (marketing.ServiceRateLimitPolicy, error) {
	ref := strings.TrimSpace(binding.RateLimitRef)
	if ref == "" {
		return marketing.ServiceRateLimitPolicy{}, errors.New("SERVICE_RATE_LIMIT_POLICY_REF_REQUIRED")
	}
	policies, err := s.repository.ListServiceRateLimitPolicies(ctx, marketing.ServiceRateLimitPolicyQuery{
		Code:           ref,
		IncludeDeleted: true,
		Limit:          500,
	})
	if err != nil {
		return marketing.ServiceRateLimitPolicy{}, err
	}
	var tenantMatched marketing.ServiceRateLimitPolicy
	otherTenant := false
	for _, policy := range policies {
		if policy.Code != ref {
			continue
		}
		if policy.TenantID != provider.TenantID {
			otherTenant = true
			continue
		}
		tenantMatched = policy
		break
	}
	if tenantMatched.Code == "" {
		if otherTenant {
			return marketing.ServiceRateLimitPolicy{}, fmt.Errorf("SERVICE_RATE_LIMIT_POLICY_TENANT_DENIED:%s", ref)
		}
		return marketing.ServiceRateLimitPolicy{}, fmt.Errorf("SERVICE_RATE_LIMIT_POLICY_NOT_FOUND:%s", ref)
	}
	if !serviceAssetStatusActive(tenantMatched.Status, tenantMatched.DeletedAt) {
		return marketing.ServiceRateLimitPolicy{}, fmt.Errorf("SERVICE_RATE_LIMIT_POLICY_DISABLED:%s", ref)
	}
	if _, ok := serviceRateLimitPolicyMatchScore(tenantMatched, provider); !ok {
		return marketing.ServiceRateLimitPolicy{}, fmt.Errorf("SERVICE_RATE_LIMIT_POLICY_SCOPE_DENIED:%s", ref)
	}
	return tenantMatched, nil
}

func serviceRateLimitPolicyMatchScore(policy marketing.ServiceRateLimitPolicy, provider marketing.ServiceProvider) (int, bool) {
	if policy.TenantID != provider.TenantID {
		return 0, false
	}
	score := 0
	if policy.ServiceRef != "" {
		if policy.ServiceRef != provider.ServiceRef {
			return 0, false
		}
		score += 1
	}
	if policy.InterfaceRef != "" {
		if policy.InterfaceRef != provider.InterfaceRef {
			return 0, false
		}
		score += 2
	}
	if policy.ProviderRef != "" {
		if policy.ProviderRef != provider.Code {
			return 0, false
		}
		score += 4
	}
	return score, true
}

func serviceProviderRuntimeStateDefaults() serviceProviderRuntimeConfig {
	return serviceProviderRuntimeConfig{
		RateLimitPerWindow:       defaultProviderRateLimitPerWindow,
		RateLimitWindow:          defaultProviderRateLimitWindow,
		CircuitThreshold:         defaultProviderCircuitThreshold,
		CircuitCooldown:          defaultProviderCircuitCooldown,
		HalfOpenProbeLimit:       defaultProviderHalfOpenProbeLimit,
		HalfOpenSuccessThreshold: defaultProviderHalfOpenSuccessThreshold,
	}
}

func serviceProviderConfigInt(config map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		value, ok := config[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed >= 0 && typed <= float64(math.MaxInt) {
				return int(typed)
			}
		case int:
			return typed
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil && parsed <= int64(math.MaxInt) {
				return int(parsed)
			}
		case string:
			var parsed int
			if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
				return parsed
			}
		}
	}
	return fallback
}
