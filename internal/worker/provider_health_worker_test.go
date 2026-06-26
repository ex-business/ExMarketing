package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
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
	if service.acquireCalls != 1 || service.releaseCalls != 1 {
		t.Fatalf("expected lease acquire/release, got acquire=%d release=%d", service.acquireCalls, service.releaseCalls)
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

func TestProviderHealthWorkerSkipsWhenLeaseHeld(t *testing.T) {
	service := newProviderHealthTestService([]marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary",
			Status:       marketing.ServiceAssetActive,
		},
	})
	service.lease = marketing.WorkerLease{
		Name:       defaultProviderHealthActor,
		Owner:      "other-worker",
		LeaseUntil: time.Now().UTC().Add(time.Minute),
	}
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled: true,
		Timeout: time.Second,
	})

	worker.tick(context.Background())

	if service.acquireCalls != 1 {
		t.Fatalf("expected one lease acquire attempt, got %d", service.acquireCalls)
	}
	if service.listCalls != 0 || service.checkCalls != 0 || service.releaseCalls != 0 {
		t.Fatalf("expected held lease to skip work, list=%d checks=%d releases=%d", service.listCalls, service.checkCalls, service.releaseCalls)
	}
}

func TestProviderHealthWorkerAcquiresExpiredLease(t *testing.T) {
	service := newProviderHealthTestService([]marketing.ServiceProvider{
		{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary",
			Status:       marketing.ServiceAssetActive,
		},
	})
	service.lease = marketing.WorkerLease{
		Name:       defaultProviderHealthActor,
		Owner:      "other-worker",
		LeaseUntil: time.Now().UTC().Add(-time.Second),
	}
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled: true,
		Timeout: time.Second,
	})

	worker.tick(context.Background())

	if service.checkCalls != 1 {
		t.Fatalf("expected expired lease to allow work, got checks=%d", service.checkCalls)
	}
	if service.lease.Owner != worker.config.LeaseOwner {
		t.Fatalf("expected lease owner takeover, got %q", service.lease.Owner)
	}
}

func TestProviderHealthWorkerScansBeyondSingleBatch(t *testing.T) {
	providers := make([]marketing.ServiceProvider, 0, 5)
	for i := 0; i < 5; i++ {
		providers = append(providers, marketing.ServiceProvider{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary-" + strconv.Itoa(i),
			Status:       marketing.ServiceAssetActive,
		})
	}
	service := newProviderHealthTestService(providers)
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled:   true,
		Timeout:   time.Second,
		BatchSize: 2,
	})

	worker.tick(context.Background())

	if service.listCalls != 3 {
		t.Fatalf("expected three paged list calls, got %d", service.listCalls)
	}
	if service.checkCalls != 5 {
		t.Fatalf("expected all providers checked, got %d", service.checkCalls)
	}
	if service.renewCalls != 2 {
		t.Fatalf("expected lease renew after full batches, got %d", service.renewCalls)
	}
}

func TestProviderHealthWorkerStopsWhenLeaseRenewLosesOwner(t *testing.T) {
	providers := make([]marketing.ServiceProvider, 0, 5)
	for i := 0; i < 5; i++ {
		providers = append(providers, marketing.ServiceProvider{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary-" + strconv.Itoa(i),
			Status:       marketing.ServiceAssetActive,
		})
	}
	service := newProviderHealthTestService(providers)
	service.loseLeaseOnRenew = true
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled:   true,
		Timeout:   time.Second,
		BatchSize: 2,
	})

	worker.tick(context.Background())

	if service.renewCalls != 1 {
		t.Fatalf("expected one failed renew, got %d", service.renewCalls)
	}
	if service.listCalls != 1 {
		t.Fatalf("expected worker to stop before second page, got list calls=%d", service.listCalls)
	}
	if service.checkCalls != 2 {
		t.Fatalf("expected only first batch checked after lease loss, got %d", service.checkCalls)
	}
}

func TestProviderHealthWorkerStopsWhenLeaseRenewErrors(t *testing.T) {
	providers := make([]marketing.ServiceProvider, 0, 5)
	for i := 0; i < 5; i++ {
		providers = append(providers, marketing.ServiceProvider{
			TenantID:     "site_001",
			ServiceRef:   "wallet-service",
			InterfaceRef: "query-wallet",
			Code:         "wallet-primary-" + strconv.Itoa(i),
			Status:       marketing.ServiceAssetActive,
		})
	}
	service := newProviderHealthTestService(providers)
	service.renewErr = errors.New("lease store unavailable")
	worker := newTestProviderHealthWorker(service, ProviderHealthWorkerConfig{
		Enabled:   true,
		Timeout:   time.Second,
		BatchSize: 2,
	})

	worker.tick(context.Background())

	if service.renewCalls != 1 {
		t.Fatalf("expected one failed renew, got %d", service.renewCalls)
	}
	if service.listCalls != 1 {
		t.Fatalf("expected renew error to stop before second page, got list calls=%d", service.listCalls)
	}
	if service.checkCalls != 2 {
		t.Fatalf("expected only first batch checked after renew error, got %d", service.checkCalls)
	}
}

func newTestProviderHealthWorker(service *providerHealthTestService, config ProviderHealthWorkerConfig) *ProviderHealthWorker {
	return NewProviderHealthWorker(service, slog.New(slog.NewTextHandler(io.Discard, nil)), config)
}

type providerHealthTestService struct {
	mu               sync.Mutex
	providers        []marketing.ServiceProvider
	states           map[string]marketing.ServiceProviderRuntimeState
	checkErr         error
	listCalls        int
	checkCalls       int
	lastActor        string
	lease            marketing.WorkerLease
	acquireCalls     int
	renewCalls       int
	releaseCalls     int
	loseLeaseOnRenew bool
	renewErr         error
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
	skipped := 0
	for _, provider := range s.providers {
		if query.Status != "" && provider.Status != query.Status {
			continue
		}
		if query.Offset > 0 && skipped < query.Offset {
			skipped++
			continue
		}
		out = append(out, provider)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *providerHealthTestService) AcquireWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return marketing.WorkerLease{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acquireCalls++
	now := time.Now().UTC()
	if cmd.Now.IsZero() {
		cmd.Now = now
	}
	if cmd.TTL <= 0 {
		cmd.TTL = time.Minute
	}
	if s.lease.Name != "" && s.lease.Owner != cmd.Owner && s.lease.LeaseUntil.After(cmd.Now) {
		held := s.lease
		held.Status = marketing.WorkerLeaseHeld
		return held, false, nil
	}
	s.lease = marketing.WorkerLease{
		Name:       cmd.Name,
		Owner:      cmd.Owner,
		Status:     marketing.WorkerLeaseAcquired,
		LeaseTTL:   cmd.TTL,
		LeaseUntil: cmd.Now.Add(cmd.TTL),
		AcquiredAt: cmd.Now,
		UpdatedAt:  cmd.Now,
	}
	return s.lease, true, nil
}

func (s *providerHealthTestService) RenewWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return marketing.WorkerLease{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewCalls++
	if s.renewErr != nil {
		return marketing.WorkerLease{}, false, s.renewErr
	}
	now := time.Now().UTC()
	if cmd.Now.IsZero() {
		cmd.Now = now
	}
	if cmd.TTL <= 0 {
		cmd.TTL = time.Minute
	}
	if s.loseLeaseOnRenew {
		s.lease.Owner = "other-worker"
		s.lease.Status = marketing.WorkerLeaseHeld
		return s.lease, false, nil
	}
	if s.lease.Name == "" {
		return marketing.WorkerLease{}, false, marketing.ErrNotFound
	}
	if s.lease.Owner != cmd.Owner {
		held := s.lease
		held.Status = marketing.WorkerLeaseHeld
		return held, false, nil
	}
	s.lease.Status = marketing.WorkerLeaseAcquired
	s.lease.LeaseTTL = cmd.TTL
	s.lease.LeaseUntil = cmd.Now.Add(cmd.TTL)
	s.lease.UpdatedAt = cmd.Now
	return s.lease, true, nil
}

func (s *providerHealthTestService) ReleaseWorkerLease(ctx context.Context, name string, owner string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseCalls++
	if s.lease.Name == name && s.lease.Owner == owner {
		s.lease.LeaseUntil = time.Now().UTC()
		s.lease.UpdatedAt = s.lease.LeaseUntil
	}
	return nil
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
