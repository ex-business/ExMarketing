CREATE TABLE IF NOT EXISTS marketing_service (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    code VARCHAR(128) NOT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    service_type VARCHAR(32) NOT NULL,
    scope VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    owner VARCHAR(128) NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_scope
    ON marketing_service(tenant_id, workspace_id, folder_id, status);

CREATE TABLE IF NOT EXISTS marketing_service_interface (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    service_ref VARCHAR(128) NOT NULL,
    code VARCHAR(128) NOT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    operation VARCHAR(64) NOT NULL,
    input_schema_ref VARCHAR(128) NOT NULL DEFAULT '',
    output_schema_ref VARCHAR(128) NOT NULL DEFAULT '',
    error_schema_ref VARCHAR(128) NOT NULL DEFAULT '',
    idempotency_key_template TEXT NOT NULL DEFAULT '',
    timeout_ms INTEGER NOT NULL DEFAULT 0,
    retry_policy VARCHAR(64) NOT NULL DEFAULT '',
    lua_callable BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(32) NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, service_ref, code),
    FOREIGN KEY (tenant_id, service_ref) REFERENCES marketing_service(tenant_id, code) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_service
    ON marketing_service_interface(tenant_id, service_ref, status);

CREATE TABLE IF NOT EXISTS marketing_service_provider (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    service_ref VARCHAR(128) NOT NULL,
    interface_ref VARCHAR(128) NOT NULL,
    code VARCHAR(128) NOT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    protocol VARCHAR(64) NOT NULL,
    endpoint TEXT NOT NULL DEFAULT '',
    topic VARCHAR(256) NOT NULL DEFAULT '',
    method VARCHAR(64) NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    auth_ref VARCHAR(256) NOT NULL DEFAULT '',
    resource_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    weight INTEGER NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 0,
    environment VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, service_ref, interface_ref, code),
    FOREIGN KEY (tenant_id, service_ref, interface_ref) REFERENCES marketing_service_interface(tenant_id, service_ref, code) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_provider_interface
    ON marketing_service_provider(tenant_id, service_ref, interface_ref, status);
