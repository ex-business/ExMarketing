package worker

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	defaultProviderHealthInterval  = 30 * time.Second
	defaultProviderHealthTimeout   = 5 * time.Second
	defaultProviderHealthBatchSize = 200
	defaultProviderHealthActor     = "provider-health-worker"
)

type ProviderHealthService interface {
	ListServiceProviders(ctx context.Context, query marketing.ServiceProviderQuery) ([]marketing.ServiceProvider, error)
	CheckServiceProviderHealth(ctx context.Context, tenantID string, serviceRef string, interfaceRef string, providerRef string, actor string) (marketing.ServiceProviderRuntimeState, error)
	AcquireWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error)
	RenewWorkerLease(ctx context.Context, cmd marketing.WorkerLeaseAcquireCommand) (marketing.WorkerLease, bool, error)
	ReleaseWorkerLease(ctx context.Context, name string, owner string) error
}

type ProviderHealthWorkerConfig struct {
	Enabled    bool
	Interval   time.Duration
	Timeout    time.Duration
	BatchSize  int
	Actor      string
	LeaseName  string
	LeaseOwner string
	LeaseTTL   time.Duration
}

type ProviderHealthWorker struct {
	service ProviderHealthService
	logger  *slog.Logger
	config  ProviderHealthWorkerConfig
}

func NewProviderHealthWorker(service ProviderHealthService, logger *slog.Logger, config ProviderHealthWorkerConfig) *ProviderHealthWorker {
	config = normalizeProviderHealthWorkerConfig(config)
	return &ProviderHealthWorker{
		service: service,
		logger:  logger,
		config:  config,
	}
}

func LoadProviderHealthWorkerConfigFromEnv() ProviderHealthWorkerConfig {
	return normalizeProviderHealthWorkerConfig(ProviderHealthWorkerConfig{
		Enabled:    strings.EqualFold(strings.TrimSpace(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_WORKER")), "true"),
		Interval:   durationEnv("EX_MARKETING_PROVIDER_HEALTH_INTERVAL", defaultProviderHealthInterval),
		Timeout:    durationEnv("EX_MARKETING_PROVIDER_HEALTH_TIMEOUT", defaultProviderHealthTimeout),
		BatchSize:  intEnv("EX_MARKETING_PROVIDER_HEALTH_BATCH_SIZE", defaultProviderHealthBatchSize),
		Actor:      firstProviderHealthNonEmpty(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_ACTOR"), defaultProviderHealthActor),
		LeaseName:  firstProviderHealthNonEmpty(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_LEASE_NAME"), defaultProviderHealthActor),
		LeaseOwner: firstProviderHealthNonEmpty(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_LEASE_OWNER"), defaultProviderHealthLeaseOwner()),
		LeaseTTL:   durationEnv("EX_MARKETING_PROVIDER_HEALTH_LEASE_TTL", 0),
	})
}

func (w *ProviderHealthWorker) Run(ctx context.Context) {
	if w == nil || !w.config.Enabled {
		return
	}
	if w.logger == nil {
		w.logger = slog.Default()
	}
	w.logger.Info("provider health worker started",
		"event", "provider_health_worker_started",
		"interval_ms", w.config.Interval.Milliseconds(),
		"timeout_ms", w.config.Timeout.Milliseconds(),
		"batch_size", w.config.BatchSize,
		"lease_name", w.config.LeaseName,
		"lease_owner", w.config.LeaseOwner,
		"lease_ttl_ms", w.config.LeaseTTL.Milliseconds(),
	)
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			w.logger.Info("provider health worker stopped", "event", "provider_health_worker_stopped")
			return
		case <-ticker.C:
		}
	}
}

func (w *ProviderHealthWorker) tick(ctx context.Context) {
	if w == nil || !w.config.Enabled {
		return
	}
	if w.logger == nil {
		w.logger = slog.Default()
	}
	lease, acquired, err := w.service.AcquireWorkerLease(ctx, marketing.WorkerLeaseAcquireCommand{
		Name:  w.config.LeaseName,
		Owner: w.config.LeaseOwner,
		TTL:   w.config.LeaseTTL,
	})
	if err != nil {
		w.logger.Warn("provider health worker lease failed", "event", "provider_health_lease_failed", "error", err)
		return
	}
	if !acquired {
		w.logger.Debug("provider health worker lease held",
			"event", "provider_health_lease_held",
			"lease_name", lease.Name,
			"lease_owner", lease.Owner,
			"lease_until", lease.LeaseUntil,
		)
		return
	}
	defer func() {
		if err := w.service.ReleaseWorkerLease(context.Background(), w.config.LeaseName, w.config.LeaseOwner); err != nil {
			w.logger.Warn("provider health worker lease release failed", "event", "provider_health_lease_release_failed", "error", err)
		}
	}()
	for offset := 0; ; offset += w.config.BatchSize {
		providers, err := w.service.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
			Status: marketing.ServiceAssetActive,
			Limit:  w.config.BatchSize,
			Offset: offset,
		})
		if err != nil {
			w.logger.Warn("provider health worker list failed", "event", "provider_health_list_failed", "error", err, "offset", offset)
			return
		}
		if len(providers) == 0 {
			return
		}
		for _, provider := range providers {
			if ctx.Err() != nil {
				return
			}
			w.checkProvider(ctx, provider)
		}
		if len(providers) < w.config.BatchSize {
			return
		}
		if !w.renewLease(ctx) {
			return
		}
	}
}

func (w *ProviderHealthWorker) renewLease(ctx context.Context) bool {
	lease, renewed, err := w.service.RenewWorkerLease(ctx, marketing.WorkerLeaseAcquireCommand{
		Name:  w.config.LeaseName,
		Owner: w.config.LeaseOwner,
		TTL:   w.config.LeaseTTL,
	})
	if err != nil {
		w.logger.Warn("provider health worker lease renew failed", "event", "provider_health_lease_renew_failed", "error", err)
		return false
	}
	if !renewed {
		w.logger.Warn("provider health worker lease lost",
			"event", "provider_health_lease_lost",
			"lease_name", lease.Name,
			"lease_owner", lease.Owner,
			"lease_until", lease.LeaseUntil,
		)
		return false
	}
	return true
}

func (w *ProviderHealthWorker) checkProvider(ctx context.Context, provider marketing.ServiceProvider) {
	checkCtx := ctx
	cancel := func() {}
	if w.config.Timeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, w.config.Timeout)
	}
	state, err := w.service.CheckServiceProviderHealth(
		checkCtx,
		provider.TenantID,
		provider.ServiceRef,
		provider.InterfaceRef,
		provider.Code,
		w.config.Actor,
	)
	cancel()
	if err != nil {
		w.logger.Warn("provider health check failed",
			"event", "provider_health_check_failed",
			"tenant_id", provider.TenantID,
			"service_ref", provider.ServiceRef,
			"interface_ref", provider.InterfaceRef,
			"provider_ref", provider.Code,
			"error", err,
		)
		return
	}
	w.logger.Debug("provider health check completed",
		"event", "provider_health_check_completed",
		"tenant_id", provider.TenantID,
		"service_ref", provider.ServiceRef,
		"interface_ref", provider.InterfaceRef,
		"provider_ref", provider.Code,
		"health_status", state.HealthStatus,
		"circuit_status", state.CircuitStatus,
	)
}

func normalizeProviderHealthWorkerConfig(config ProviderHealthWorkerConfig) ProviderHealthWorkerConfig {
	if config.Interval <= 0 {
		config.Interval = defaultProviderHealthInterval
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultProviderHealthTimeout
	}
	if config.BatchSize <= 0 || config.BatchSize > 500 {
		config.BatchSize = defaultProviderHealthBatchSize
	}
	if strings.TrimSpace(config.Actor) == "" {
		config.Actor = defaultProviderHealthActor
	}
	if strings.TrimSpace(config.LeaseName) == "" {
		config.LeaseName = defaultProviderHealthActor
	}
	if strings.TrimSpace(config.LeaseOwner) == "" {
		config.LeaseOwner = defaultProviderHealthLeaseOwner()
	}
	if config.LeaseTTL <= 0 {
		config.LeaseTTL = config.Interval * 2
		if config.LeaseTTL < time.Minute {
			config.LeaseTTL = time.Minute
		}
	}
	return config
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstProviderHealthNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultProviderHealthLeaseOwner() string {
	hostname, _ := os.Hostname()
	return firstProviderHealthNonEmpty(os.Getenv("HOSTNAME"), hostname, defaultProviderHealthActor)
}
