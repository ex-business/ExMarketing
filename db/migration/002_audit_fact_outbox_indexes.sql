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
  tenant_id VARCHAR(64) NOT NULL,
  fact_code VARCHAR(64) NOT NULL,
  provider_type VARCHAR(32) NOT NULL,
  endpoint TEXT NOT NULL,
  method VARCHAR(16) NOT NULL,
  auth_ref VARCHAR(128),
  mapping_json JSON NOT NULL,
  cache_ttl_seconds INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  timeout_ms INT NOT NULL DEFAULT 5000,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  PRIMARY KEY (tenant_id, fact_code),
  CONSTRAINT ck_marketing_fact_provider_type CHECK (provider_type = 'HTTP'),
  CONSTRAINT ck_marketing_fact_provider_method CHECK (method IN ('GET', 'POST')),
  CONSTRAINT ck_marketing_fact_provider_cache_ttl CHECK (cache_ttl_seconds >= 0 AND cache_ttl_seconds <= 3600)
);

CREATE INDEX IF NOT EXISTS idx_marketing_outbox_pending
  ON marketing_outbox (send_status, next_retry_at, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS uk_marketing_quota_scope
  ON marketing_quota (activity_id, quota_type, scope, scope_key);
