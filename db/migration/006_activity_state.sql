CREATE TABLE IF NOT EXISTS marketing_activity_state (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    activity_id VARCHAR(64) NOT NULL REFERENCES marketing_activity(id) ON DELETE CASCADE,
    namespace VARCHAR(128) NOT NULL,
    state_key VARCHAR(256) NOT NULL,
    subject_type VARCHAR(64),
    subject_id VARCHAR(128),
    data_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    version BIGINT NOT NULL DEFAULT 1,
    last_event_id VARCHAR(128),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, activity_id, namespace, state_key)
);

CREATE INDEX IF NOT EXISTS idx_activity_state_subject
    ON marketing_activity_state(tenant_id, subject_type, subject_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_state_activity
    ON marketing_activity_state(activity_id, namespace, updated_at DESC);
