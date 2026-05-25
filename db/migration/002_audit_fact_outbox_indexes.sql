CREATE TABLE IF NOT EXISTS marketing_audit (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  actor VARCHAR(128) NOT NULL,
  action VARCHAR(64) NOT NULL,
  resource VARCHAR(64) NOT NULL,
  resource_id VARCHAR(128) NOT NULL,
  payload_json JSON NOT NULL,
  created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_marketing_audit_tenant_created
  ON marketing_audit (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_audit_resource
  ON marketing_audit (tenant_id, resource, resource_id, created_at DESC);

CREATE TABLE IF NOT EXISTS marketing_fact_provider (
  fact_code VARCHAR(64) PRIMARY KEY,
  provider VARCHAR(32) NOT NULL,
  endpoint TEXT NOT NULL,
  method VARCHAR(16) NOT NULL,
  headers_json JSON,
  cache_ttl_seconds INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_marketing_outbox_pending
  ON marketing_outbox (send_status, next_retry_at, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS uk_marketing_quota_scope
  ON marketing_quota (activity_id, quota_type, scope, scope_key);
