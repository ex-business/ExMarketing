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
}

type ProviderHealthWorkerConfig struct {
	Enabled   bool
	Interval  time.Duration
	Timeout   time.Duration
	BatchSize int
	Actor     string
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
		Enabled:   strings.EqualFold(strings.TrimSpace(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_WORKER")), "true"),
		Interval:  durationEnv("EX_MARKETING_PROVIDER_HEALTH_INTERVAL", defaultProviderHealthInterval),
		Timeout:   durationEnv("EX_MARKETING_PROVIDER_HEALTH_TIMEOUT", defaultProviderHealthTimeout),
		BatchSize: intEnv("EX_MARKETING_PROVIDER_HEALTH_BATCH_SIZE", defaultProviderHealthBatchSize),
		Actor:     firstProviderHealthNonEmpty(os.Getenv("EX_MARKETING_PROVIDER_HEALTH_ACTOR"), defaultProviderHealthActor),
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
	providers, err := w.service.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
		Status: marketing.ServiceAssetActive,
		Limit:  w.config.BatchSize,
	})
	if err != nil {
		w.logger.Warn("provider health worker list failed", "event", "provider_health_list_failed", "error", err)
		return
	}
	for _, provider := range providers {
		if ctx.Err() != nil {
			return
		}
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
			continue
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
