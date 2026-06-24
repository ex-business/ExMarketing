CREATE TABLE IF NOT EXISTS marketing_service_provider_runtime_state (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    service_ref TEXT NOT NULL,
    interface_ref TEXT NOT NULL,
    provider_ref TEXT NOT NULL,
    health_status TEXT NOT NULL,
    health_message TEXT NOT NULL DEFAULT '',
    circuit_status TEXT NOT NULL,
    failure_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    rate_limit_window TIMESTAMPTZ,
    rate_limit_count INTEGER NOT NULL DEFAULT 0,
    half_open_probe_count INTEGER NOT NULL DEFAULT 0,
    half_open_probe_limit INTEGER NOT NULL DEFAULT 1,
    half_open_success_count INTEGER NOT NULL DEFAULT 0,
    half_open_success_threshold INTEGER NOT NULL DEFAULT 1,
    last_failure_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    circuit_opened_at TIMESTAMPTZ,
    circuit_reset_at TIMESTAMPTZ,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT marketing_service_provider_runtime_state_unique UNIQUE (tenant_id, service_ref, interface_ref, provider_ref),
    CONSTRAINT marketing_service_provider_runtime_state_health_check CHECK (health_status IN ('UNKNOWN', 'HEALTHY', 'DEGRADED', 'UNHEALTHY')),
    CONSTRAINT marketing_service_provider_runtime_state_circuit_check CHECK (circuit_status IN ('CLOSED', 'OPEN', 'HALF_OPEN')),
    CONSTRAINT marketing_service_provider_runtime_state_failure_count_check CHECK (failure_count >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_success_count_check CHECK (success_count >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_consecutive_failures_check CHECK (consecutive_failures >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_rate_limit_count_check CHECK (rate_limit_count >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_half_open_probe_count_check CHECK (half_open_probe_count >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_half_open_probe_limit_check CHECK (half_open_probe_limit >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_half_open_success_count_check CHECK (half_open_success_count >= 0),
    CONSTRAINT marketing_service_provider_runtime_state_half_open_success_threshold_check CHECK (half_open_success_threshold >= 0)
);

CREATE INDEX IF NOT EXISTS marketing_service_provider_runtime_state_lookup_idx
    ON marketing_service_provider_runtime_state (tenant_id, service_ref, interface_ref, provider_ref);

CREATE INDEX IF NOT EXISTS marketing_service_provider_runtime_state_health_idx
    ON marketing_service_provider_runtime_state (tenant_id, health_status, circuit_status, updated_at DESC);

CREATE INDEX IF NOT EXISTS marketing_service_provider_runtime_state_metadata_gin_idx
    ON marketing_service_provider_runtime_state USING GIN (metadata_json);
