CREATE TABLE IF NOT EXISTS marketing_user_profile (
    tenant_id VARCHAR(64) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id VARCHAR(128) NOT NULL,
    user_level INTEGER NOT NULL DEFAULT 0,
    vip_level INTEGER NOT NULL DEFAULT 0,
    country VARCHAR(32),
    channel VARCHAR(64),
    registered_at TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    attributes_json JSON NOT NULL DEFAULT '{}'::json,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, subject_type, subject_id),
    CONSTRAINT ck_marketing_user_profile_status CHECK (status IN ('ACTIVE', 'DISABLED', 'DELETED')),
    CONSTRAINT ck_marketing_user_profile_level CHECK (user_level >= 0 AND vip_level >= 0)
);

CREATE INDEX IF NOT EXISTS idx_marketing_user_profile_tenant_status
    ON marketing_user_profile (tenant_id, status);
