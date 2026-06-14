CREATE TABLE IF NOT EXISTS marketing_storage_state (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    namespace VARCHAR(128) NOT NULL,
    storage_key VARCHAR(512) NOT NULL,
    value_type VARCHAR(32) NOT NULL,
    data_json JSONB NOT NULL DEFAULT 'null'::jsonb,
    version BIGINT NOT NULL DEFAULT 1,
    last_event_id VARCHAR(128),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, namespace, storage_key)
);

CREATE INDEX IF NOT EXISTS idx_storage_state_namespace
    ON marketing_storage_state(tenant_id, namespace, updated_at DESC);
