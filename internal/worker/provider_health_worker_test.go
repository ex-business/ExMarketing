package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestProviderHealthWorkerTickChecksActiveProviders(t *testing.T) {
	service := newProviderHealthTestService([]marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary",
			Status:       marketing.ServiceAssetActive,
		},
	})
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled:  true,
		Interval: time.Hour,
		Timeout:  time.Second,
		Actor:    "health-agent",
	})

	worker.tick(context.Background())

	if service.listCalls != 1 {
		t.Fatalf("expected one provider list call, got %d", service.listCalls)
	}
	if service.checkCalls != 1 {
		t.Fatalf("expected one health check call, got %d", service.checkCalls)
	}
	if service.lastActor != "health-agent" {
		t.Fatalf("expected worker actor propagated, got %q", service.lastActor)
	}
	state := service.state("site_001", "wallet-service", "query-wallet", "wallet-primary")
	if state.HealthStatus != marketing.ServiceProviderHealthHealthy {
		t.Fatalf("expected healthy state, got %+v", state)
	}
}

func TestProviderHealthWorkerDisabledDoesNotRun(t *testing.T) {
	service := newProviderHealthTestService([]marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary",
			Status:       marketing.ServiceAssetActive,
		},
	})
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled: false,
	})

	worker.tick(context.Background())
	worker.Run(context.Background())

	if service.listCalls != 0 || service.checkCalls != 0 {
		t.Fatalf("expected disabled worker to do nothing, list=%d checks=%d", service.listCalls, service.checkCalls)
	}
}

func TestProviderHealthWorkerFailureUpdatesState(t *testing.T) {
	service := newProviderHealthTestService([]marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary",
			Status:       marketing.ServiceAssetActive,
		},
	})
	service.checkErr = errors.New("resource probe failed")
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled: true,
		Timeout: time.Second,
	})

	worker.tick(context.Background())

	state := service.state("site_001", "wallet-service", "query-wallet", "wallet-primary")
	if state.HealthStatus != marketing.ServiceProviderHealthUnhealthy {
		t.Fatalf("expected unhealthy state after failed check, got %+v", state)
	}
	if state.HealthMessage != "resource probe failed" {
		t.Fatalf("expected health message from failed check, got %q", state.HealthMessage)
	}
	if state.Metadata == nil || string(state.Metadata) == "" {
		t.Fatalf("expected metadata to be set")
	}
}

func newTestProviderHealthWorker(service *providerHealthTestService, config ProviderHealthWorkerConfig) *ProviderHealthWorker {
	return NewProviderHealthWorker(service, slog.New(slog.NewTextHandler(io.Discard, nil)), config)
}

type providerHealthTestService struct {
	mu         sync.Mutex
	providers  []marketing.ServiceProvider
	states     map[string]marketing.ServiceProviderRuntimeState
	checkErr   error
	listCalls  int
	checkCalls int
	lastActor  string
}

func newProviderHealthTestService(providers []marketing.ServiceProvider) *providerHealthTestService {
	return &providerHealthTestService{
		providers: providers,
		states:    map[string]marketing.ServiceProviderRuntimeState{},
	}
}

func (s *providerHealthTestService) ListServiceProviders(ctx context.Context, query marketing.ServiceProviderQuery) ([]marketing.ServiceProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	out := []marketing.ServiceProvider{}
	limit := query.Limit
	if limit <= 0 {
		limit = len(s.providers)
	}
	for _, provider := range s.providers {
		if query.Status != "" && provider.Status != query.Status {
			continue
		}
		out = append(out, provider)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *providerHealthTestService) CheckServiceProviderHealth(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, actor string) (marketing.ServiceProviderRuntimeState, error) {
	if err := ctx.Err(); err != nil {
		return marketing.ServiceProviderRuntimeState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkCalls++
	s.lastActor = actor
	state := marketing.ServiceProviderRuntimeState{
		TenantID:      tenantID,
		ServiceRef:    serviceRef,
		InterfaceRef:  interfaceRef,
		ProviderRef:   providerRef,
		CircuitStatus: marketing.ServiceProviderCircuitClosed,
		Metadata:      []byte(`{"lastHealthCheckBy":"` + actor + `"}`),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if s.checkErr != nil {
		state.HealthStatus = marketing.ServiceProviderHealthUnhealthy
		state.HealthMessage = s.checkErr.Error()
		s.states[providerHealthStateKey(tenantID, serviceRef, interfaceRef, providerRef)] = state
		return state, s.checkErr
	}
	state.HealthStatus = marketing.ServiceProviderHealthHealthy
	state.HealthMessage = "health check passed"
	s.states[providerHealthStateKey(tenantID, serviceRef, interfaceRef, providerRef)] = state
	return state, nil
}

func (s *providerHealthTestService) state(tenantID string, serviceRef string, interfaceRef string, providerRef string) marketing.ServiceProviderRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[providerHealthStateKey(tenantID, serviceRef, interfaceRef, providerRef)]
}

func providerHealthStateKey(tenantID string, serviceRef string, interfaceRef string, providerRef string) string {
	return tenantID + "|" + serviceRef + "|" + interfaceRef + "|" + providerRef
}
