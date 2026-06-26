package engine

import (
	"context"
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
	CircuitThreshold         int
	CircuitCooldown          time.Duration
	HalfOpenProbeLimit       int
	HalfOpenSuccessThreshold int
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

func (s *Service) SaveServiceRateLimitPolicy(ctx context.Context, policy marketing.ServiceRateLimitPolicy) (marketing.ServiceRateLimitPolicy, error) {
	policy = s.normalizeServiceRateLimitPolicy(policy)
	if err := policy.Validate(); err != nil {
		return marketing.ServiceRateLimitPolicy{}, err
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

func (s *Service) serviceProviderRuntimeAvailable(ctx context.Context, provider marketing.ServiceProvider) (bool, string, string, error) {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return false, "", "", err
	}
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider)
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
	if serviceProviderRateLimitExceeded(state, cfg, now) {
		return false, "SERVICE_PROVIDER_RATE_LIMITED", "provider rate limit exceeded", nil
	}
	return true, "", "", nil
}

func (s *Service) consumeServiceProviderRuntime(ctx context.Context, provider marketing.ServiceProvider) (marketing.ServiceProviderRuntimeState, error) {
	state, err := s.serviceProviderRuntimeState(ctx, provider)
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	cfg, err := s.serviceProviderGovernanceConfig(ctx, provider)
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
	state, consumed, err := s.repository.ConsumeServiceProviderRateLimit(ctx, marketing.ServiceProviderRateLimitConsume{
		State:  state,
		Limit:  cfg.RateLimitPerWindow,
		Window: cfg.RateLimitWindow,
		Now:    now,
	})
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

func (s *Service) serviceProviderGovernanceConfig(ctx context.Context, provider marketing.ServiceProvider) (serviceProviderRuntimeConfig, error) {
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
	if policy, ok, err := s.matchServiceRateLimitPolicy(ctx, provider); err != nil {
		return cfg, err
	} else if ok {
		cfg.RateLimitPolicyRef = policy.Code
		cfg.RateLimitPerWindow = policy.Limit
		cfg.RateLimitWindow = time.Duration(policy.WindowSeconds) * time.Second
		cfg.RateLimitBackend = policy.Backend
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
