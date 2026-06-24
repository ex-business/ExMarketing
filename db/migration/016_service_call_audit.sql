CREATE TABLE IF NOT EXISTS marketing_service_call_audit (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  folder_id TEXT NOT NULL DEFAULT '',
  activity_id TEXT NOT NULL DEFAULT '',
  workflow_id TEXT NOT NULL DEFAULT '',
  node_id TEXT NOT NULL DEFAULT '',
  node_type TEXT NOT NULL DEFAULT '',
  service_ref TEXT NOT NULL,
  interface_ref TEXT NOT NULL,
  provider_ref TEXT NOT NULL DEFAULT '',
  resource_ref TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  attempt INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL CHECK (status IN ('SUCCESS', 'FAILED', 'DENIED')),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
  request_hash TEXT NOT NULL DEFAULT '',
  response_hash TEXT NOT NULL DEFAULT '',
  provider_trace_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  schema_trace_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  permission_trace_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  actor TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT marketing_service_call_audit_attempt_check CHECK (attempt >= 0)
);

CREATE INDEX IF NOT EXISTS idx_marketing_service_call_audit_tenant_created
  ON marketing_service_call_audit (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_service_call_audit_activity_node
  ON marketing_service_call_audit (tenant_id, activity_id, node_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_service_call_audit_interface
  ON marketing_service_call_audit (tenant_id, service_ref, interface_ref, provider_ref, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_service_call_audit_error
  ON marketing_service_call_audit (tenant_id, error_code, created_at DESC)
  WHERE error_code <> '';

CREATE INDEX IF NOT EXISTS idx_marketing_service_call_audit_provider_trace_gin
  ON marketing_service_call_audit USING GIN (provider_trace_json);
