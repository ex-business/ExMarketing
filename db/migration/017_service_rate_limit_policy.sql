CREATE TABLE IF NOT EXISTS marketing_service_rate_limit_policy (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    service_ref TEXT NOT NULL DEFAULT '',
    interface_ref TEXT NOT NULL DEFAULT '',
    provider_ref TEXT NOT NULL DEFAULT '',
    limit_count INTEGER NOT NULL,
    window_seconds INTEGER NOT NULL,
    backend TEXT NOT NULL DEFAULT 'POSTGRES',
    status TEXT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT marketing_service_rate_limit_policy_code_unique UNIQUE (tenant_id, code),
    CONSTRAINT marketing_service_rate_limit_policy_scope_unique UNIQUE (tenant_id, service_ref, interface_ref, provider_ref),
    CONSTRAINT marketing_service_rate_limit_policy_limit_check CHECK (limit_count >= 0),
    CONSTRAINT marketing_service_rate_limit_policy_window_check CHECK (window_seconds > 0),
    CONSTRAINT marketing_service_rate_limit_policy_backend_check CHECK (backend IN ('POSTGRES', 'REDIS')),
    CONSTRAINT marketing_service_rate_limit_policy_status_check CHECK (status IN ('ACTIVE', 'DRAFT', 'DISABLED', 'DELETED')),
    CONSTRAINT marketing_service_rate_limit_policy_metadata_check CHECK (jsonb_typeof(metadata_json) = 'object')
);

CREATE INDEX IF NOT EXISTS marketing_service_rate_limit_policy_lookup_idx
    ON marketing_service_rate_limit_policy (tenant_id, service_ref, interface_ref, provider_ref, status);

CREATE INDEX IF NOT EXISTS marketing_service_rate_limit_policy_status_idx
    ON marketing_service_rate_limit_policy (tenant_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS marketing_service_rate_limit_policy_metadata_gin_idx
    ON marketing_service_rate_limit_policy USING GIN (metadata_json);
