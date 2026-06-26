ALTER TABLE marketing_service
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE marketing_service_interface
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE marketing_service_provider
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE marketing_service_schema
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE marketing_service_interface_binding
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '';

ALTER TABLE marketing_service_rate_limit_policy
    ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS catalog_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_marketing_service_deleted
    ON marketing_service(tenant_id, deleted_at, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_deleted
    ON marketing_service_interface(tenant_id, service_ref, deleted_at, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_provider_deleted
    ON marketing_service_provider(tenant_id, service_ref, interface_ref, deleted_at, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_schema_deleted
    ON marketing_service_schema(tenant_id, schema_ref, deleted_at, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_deleted
    ON marketing_service_interface_binding(tenant_id, service_ref, interface_ref, deleted_at, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_rate_limit_policy_deleted
    ON marketing_service_rate_limit_policy(tenant_id, service_ref, interface_ref, provider_ref, deleted_at, status);
