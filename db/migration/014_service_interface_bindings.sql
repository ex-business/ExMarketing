CREATE TABLE IF NOT EXISTS marketing_service_interface_binding (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    service_ref VARCHAR(128) NOT NULL,
    interface_ref VARCHAR(128) NOT NULL,
    scope VARCHAR(32) NOT NULL,
    workspace_id VARCHAR(64) NOT NULL DEFAULT '',
    folder_id VARCHAR(64) NOT NULL DEFAULT '',
    activity_id VARCHAR(64) NOT NULL DEFAULT '',
    node_id VARCHAR(128) NOT NULL DEFAULT '',
    principal_type VARCHAR(32) NOT NULL,
    principal_id VARCHAR(128) NOT NULL,
    allowed_node_types_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_operations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_provider_policies_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allow_explicit_provider_ref BOOLEAN NOT NULL DEFAULT false,
    allowed_provider_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    lua_callable BOOLEAN NOT NULL DEFAULT false,
    require_approval BOOLEAN NOT NULL DEFAULT false,
    deny BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(32) NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    updated_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id, activity_id, node_id, principal_type, principal_id),
    FOREIGN KEY (tenant_id, service_ref, interface_ref) REFERENCES marketing_service_interface(tenant_id, service_ref, code) ON DELETE CASCADE,
    CHECK (jsonb_typeof(allowed_node_types_json) = 'array'),
    CHECK (jsonb_typeof(allowed_operations_json) = 'array'),
    CHECK (jsonb_typeof(allowed_provider_policies_json) = 'array'),
    CHECK (jsonb_typeof(allowed_provider_refs_json) = 'array'),
    CHECK (jsonb_typeof(metadata_json) = 'object'),
    CHECK (allow_explicit_provider_ref = false OR jsonb_array_length(allowed_provider_refs_json) > 0)
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_interface
    ON marketing_service_interface_binding(tenant_id, service_ref, interface_ref, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_scope
    ON marketing_service_interface_binding(tenant_id, scope, workspace_id, folder_id, activity_id, node_id, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_nodes
    ON marketing_service_interface_binding USING GIN(allowed_node_types_json);
