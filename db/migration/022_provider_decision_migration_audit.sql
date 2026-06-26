CREATE TABLE IF NOT EXISTS marketing_service_provider_decision_audit (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  folder_id TEXT NOT NULL DEFAULT '',
  activity_id TEXT NOT NULL DEFAULT '',
  node_id TEXT NOT NULL DEFAULT '',
  node_type TEXT NOT NULL DEFAULT '',
  service_ref TEXT NOT NULL,
  interface_ref TEXT NOT NULL,
  provider_policy TEXT NOT NULL DEFAULT '',
  candidate_provider_ref TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL CHECK (decision IN ('selected', 'skipped', 'fallback', 'rate_limited', 'unhealthy', 'circuit_open', 'schema_failed')),
  reason TEXT NOT NULL DEFAULT '',
  latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
  weight INTEGER NOT NULL DEFAULT 0,
  environment TEXT NOT NULL DEFAULT '',
  mock BOOLEAN NOT NULL DEFAULT false,
  trace_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  details_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_marketing_provider_decision_audit_tenant_created
  ON marketing_service_provider_decision_audit (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_provider_decision_audit_activity
  ON marketing_service_provider_decision_audit (tenant_id, activity_id, node_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_provider_decision_audit_provider
  ON marketing_service_provider_decision_audit (tenant_id, service_ref, interface_ref, candidate_provider_ref, decision, created_at DESC);

CREATE TABLE IF NOT EXISTS marketing_service_migration_audit (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  folder_id TEXT NOT NULL DEFAULT '',
  activity_id TEXT NOT NULL DEFAULT '',
  migration_type TEXT NOT NULL,
  operation TEXT NOT NULL,
  dry_run BOOLEAN NOT NULL DEFAULT false,
  actor TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  break_glass_ticket TEXT NOT NULL DEFAULT '',
  summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  items_count INTEGER NOT NULL DEFAULT 0 CHECK (items_count >= 0),
  blockers_count INTEGER NOT NULL DEFAULT 0 CHECK (blockers_count >= 0),
  result TEXT NOT NULL CHECK (result IN ('SUCCESS', 'FAILED', 'DENIED')),
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_marketing_migration_audit_tenant_created
  ON marketing_service_migration_audit (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_migration_audit_activity
  ON marketing_service_migration_audit (tenant_id, activity_id, migration_type, operation, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_marketing_migration_audit_type
  ON marketing_service_migration_audit (tenant_id, migration_type, operation, result, created_at DESC);
