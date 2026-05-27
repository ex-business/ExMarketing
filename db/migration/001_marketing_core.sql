CREATE TABLE marketing_activity (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  activity_code VARCHAR(64) NOT NULL,
  activity_name VARCHAR(128) NOT NULL,
  activity_type VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  trigger_events JSON NOT NULL,
  start_time TIMESTAMP NOT NULL,
  end_time TIMESTAMP NOT NULL,
  timezone VARCHAR(64) NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  exclusive_group VARCHAR(64) DEFAULT '',
  stackable BOOLEAN NOT NULL DEFAULT FALSE,
  plugin_code VARCHAR(64) DEFAULT '',
  version INT NOT NULL DEFAULT 1,
  config_json JSON,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  CONSTRAINT marketing_activity_tenant_code_key UNIQUE (tenant_id, activity_code)
);

CREATE TABLE marketing_rule (
  id VARCHAR(64) PRIMARY KEY,
  activity_id VARCHAR(64) NOT NULL,
  rule_code VARCHAR(64) NOT NULL,
  rule_phase VARCHAR(32) NOT NULL,
  rule_type VARCHAR(32) NOT NULL,
  rule_config_json JSON NOT NULL,
  sort INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_reward (
  id VARCHAR(64) PRIMARY KEY,
  activity_id VARCHAR(64) NOT NULL,
  reward_code VARCHAR(64) NOT NULL,
  reward_type VARCHAR(64) NOT NULL,
  calculate_type VARCHAR(64) NOT NULL,
  reward_config_json JSON NOT NULL,
  grant_mode VARCHAR(32) NOT NULL,
  grant_channel VARCHAR(32) NOT NULL,
  flow_multiple DECIMAL(18,4) DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_event_inbox (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  event_id VARCHAR(128) NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  source_system VARCHAR(64) NOT NULL,
  subject_type VARCHAR(32) NOT NULL,
  subject_id VARCHAR(128) NOT NULL,
  biz_type VARCHAR(64) NOT NULL,
  biz_id VARCHAR(128) NOT NULL,
  occurred_at TIMESTAMP NOT NULL,
  payload_json JSON NOT NULL,
  idempotency_key VARCHAR(256) NOT NULL UNIQUE,
  process_status VARCHAR(32) NOT NULL,
  error_message TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_participation (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  activity_id VARCHAR(64) NOT NULL,
  event_inbox_id VARCHAR(64) NOT NULL,
  subject_type VARCHAR(32) NOT NULL,
  subject_id VARCHAR(128) NOT NULL,
  biz_type VARCHAR(64) NOT NULL,
  biz_id VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  reject_reason VARCHAR(256),
  snapshot_json JSON NOT NULL,
  idempotency_key VARCHAR(256) NOT NULL UNIQUE,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_grant_order (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  activity_id VARCHAR(64) NOT NULL,
  participation_id VARCHAR(64) NOT NULL,
  subject_type VARCHAR(32) NOT NULL,
  subject_id VARCHAR(128) NOT NULL,
  reward_type VARCHAR(64) NOT NULL,
  reward_amount DECIMAL(18,4) DEFAULT 0,
  reward_payload_json JSON NOT NULL,
  bill_no VARCHAR(128) NOT NULL,
  grant_status VARCHAR(32) NOT NULL,
  grant_channel VARCHAR(32) NOT NULL,
  retry_count INT NOT NULL DEFAULT 0,
  error_message TEXT,
  external_grant_id VARCHAR(128),
  finished_at TIMESTAMP,
  idempotency_key VARCHAR(256) NOT NULL UNIQUE,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_quota (
  id VARCHAR(64) PRIMARY KEY,
  activity_id VARCHAR(64) NOT NULL,
  quota_type VARCHAR(32) NOT NULL,
  scope VARCHAR(32) NOT NULL,
  scope_key VARCHAR(128) NOT NULL,
  total_value DECIMAL(18,4) NOT NULL,
  used_value DECIMAL(18,4) NOT NULL DEFAULT 0,
  frozen_value DECIMAL(18,4) NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_quota_reservation (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  activity_id VARCHAR(64) NOT NULL,
  grant_order_id VARCHAR(64) NOT NULL,
  command_id VARCHAR(128) NOT NULL,
  quota_type VARCHAR(32) NOT NULL,
  scope VARCHAR(32) NOT NULL,
  scope_key VARCHAR(128) NOT NULL,
  reserved_amount DECIMAL(18,4) NOT NULL,
  reservation_status VARCHAR(32) NOT NULL,
  confirmed_at TIMESTAMP,
  released_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE (tenant_id, grant_order_id, quota_type, scope, scope_key)
);

CREATE TABLE marketing_outbox (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  message_type VARCHAR(64) NOT NULL,
  target_channel VARCHAR(64) NOT NULL,
  target_ref VARCHAR(128) NOT NULL,
  payload_json JSON NOT NULL,
  send_status VARCHAR(32) NOT NULL,
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMP,
  error_message TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_manual_grant_export_file (
  id VARCHAR(64) PRIMARY KEY,
  tenant_id VARCHAR(64) NOT NULL,
  file_name VARCHAR(256) NOT NULL,
  file_format VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  operator VARCHAR(128) NOT NULL,
  reason VARCHAR(512) NOT NULL,
  filter_snapshot_json JSON NOT NULL,
  row_count INT NOT NULL DEFAULT 0,
  file_checksum VARCHAR(128),
  storage_ref VARCHAR(512),
  error_message TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE marketing_manual_grant_export_detail (
  id VARCHAR(64) PRIMARY KEY,
  export_id VARCHAR(64) NOT NULL,
  tenant_id VARCHAR(64) NOT NULL,
  grant_order_id VARCHAR(64) NOT NULL,
  bill_no VARCHAR(128) NOT NULL,
  row_number INT NOT NULL,
  export_grant_status VARCHAR(32) NOT NULL,
  detail_status VARCHAR(32) NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE (export_id, grant_order_id)
);
