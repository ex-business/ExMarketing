CREATE TABLE IF NOT EXISTS marketing_resource (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    activity_id VARCHAR(64) NOT NULL DEFAULT '',
    code VARCHAR(128) NOT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type VARCHAR(64) NOT NULL,
    driver VARCHAR(64) NOT NULL,
    scope VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    environment VARCHAR(32) NOT NULL DEFAULT '',
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    schema_version INTEGER NOT NULL DEFAULT 1,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_resource_scope
    ON marketing_resource(tenant_id, workspace_id, folder_id, activity_id, status);

CREATE INDEX IF NOT EXISTS idx_resource_type
    ON marketing_resource(tenant_id, type, status);

CREATE TABLE IF NOT EXISTS marketing_resource_schema (
    id VARCHAR(64) PRIMARY KEY,
    resource_id VARCHAR(64) NOT NULL REFERENCES marketing_resource(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    schema_type VARCHAR(64) NOT NULL,
    schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (resource_id, version)
);

CREATE TABLE IF NOT EXISTS marketing_resource_binding (
    id VARCHAR(64) PRIMARY KEY,
    resource_id VARCHAR(64) NOT NULL REFERENCES marketing_resource(id) ON DELETE CASCADE,
    tenant_id VARCHAR(64) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    activity_id VARCHAR(64) NOT NULL DEFAULT '',
    node_type VARCHAR(64) NOT NULL DEFAULT '',
    node_id VARCHAR(128) NOT NULL DEFAULT '',
    principal_type VARCHAR(32) NOT NULL DEFAULT '',
    principal_id VARCHAR(128) NOT NULL DEFAULT '',
    min_role VARCHAR(32) NOT NULL DEFAULT '',
    allowed_operations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_namespaces_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    key_policy_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    include_subfolders BOOLEAN NOT NULL DEFAULT false,
    require_approval BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_resource_binding_resource
    ON marketing_resource_binding(resource_id, status);

CREATE INDEX IF NOT EXISTS idx_resource_binding_scope
    ON marketing_resource_binding(tenant_id, workspace_id, folder_id, activity_id, node_type, status);

CREATE TABLE IF NOT EXISTS marketing_resource_secret (
    id VARCHAR(64) PRIMARY KEY,
    resource_id VARCHAR(64) NOT NULL REFERENCES marketing_resource(id) ON DELETE CASCADE,
    secret_ref VARCHAR(256) NOT NULL,
    secret_type VARCHAR(64) NOT NULL,
    encrypted_payload BYTEA,
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS marketing_resource_audit (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    activity_id VARCHAR(64) NOT NULL DEFAULT '',
    workflow_execution_id VARCHAR(64) NOT NULL DEFAULT '',
    node_id VARCHAR(128) NOT NULL DEFAULT '',
    node_type VARCHAR(64) NOT NULL DEFAULT '',
    resource_id VARCHAR(64) NOT NULL,
    resource_code VARCHAR(128) NOT NULL,
    operation VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    actor_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_resource_audit_resource
    ON marketing_resource_audit(resource_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_resource_audit_scope
    ON marketing_resource_audit(tenant_id, workspace_id, activity_id, created_at DESC);

