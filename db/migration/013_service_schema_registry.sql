CREATE TABLE IF NOT EXISTS marketing_service_schema (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    schema_ref VARCHAR(128) NOT NULL,
    schema_type VARCHAR(32) NOT NULL,
    scope VARCHAR(32) NOT NULL,
    version INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    schema_json JSONB NOT NULL,
    examples_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    description TEXT NOT NULL DEFAULT '',
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, schema_ref, version),
    CHECK (version > 0),
    CHECK (jsonb_typeof(schema_json) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_schema_lookup
    ON marketing_service_schema(tenant_id, schema_ref, schema_type, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_schema_scope
    ON marketing_service_schema(tenant_id, workspace_id, folder_id, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_schema_json
    ON marketing_service_schema USING GIN(schema_json);
