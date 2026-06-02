CREATE TABLE IF NOT EXISTS marketing_workflow_draft (
    activity_id VARCHAR(64) PRIMARY KEY REFERENCES marketing_activity(id) ON DELETE CASCADE,
    tenant_id VARCHAR(64) NOT NULL,
    definition_json JSONB NOT NULL,
    updated_by VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS marketing_workflow_version (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    activity_id VARCHAR(64) NOT NULL REFERENCES marketing_activity(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    definition_json JSONB NOT NULL,
    status VARCHAR(32) NOT NULL,
    published_by VARCHAR(128),
    published_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (activity_id, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_workflow_active_version
    ON marketing_workflow_version(activity_id)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_workflow_version_activity
    ON marketing_workflow_version(activity_id, version DESC);

CREATE TABLE IF NOT EXISTS marketing_workflow_execution (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    activity_id VARCHAR(64) NOT NULL REFERENCES marketing_activity(id) ON DELETE CASCADE,
    workflow_version_id VARCHAR(64) NOT NULL REFERENCES marketing_workflow_version(id),
    workflow_version INTEGER NOT NULL,
    event_inbox_id VARCHAR(64),
    event_id VARCHAR(128),
    status VARCHAR(32) NOT NULL,
    input_json JSONB,
    output_json JSONB,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_execution_activity
    ON marketing_workflow_execution(activity_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_workflow_execution_event
    ON marketing_workflow_execution(tenant_id, event_id, created_at DESC);

CREATE TABLE IF NOT EXISTS marketing_workflow_node_execution (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    execution_id VARCHAR(64) NOT NULL REFERENCES marketing_workflow_execution(id) ON DELETE CASCADE,
    node_id VARCHAR(128) NOT NULL,
    node_type VARCHAR(64) NOT NULL,
    node_title VARCHAR(256),
    status VARCHAR(32) NOT NULL,
    input_json JSONB,
    output_json JSONB,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_node_execution_execution
    ON marketing_workflow_node_execution(execution_id, started_at ASC);
