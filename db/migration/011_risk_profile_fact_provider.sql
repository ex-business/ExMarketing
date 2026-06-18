CREATE TABLE IF NOT EXISTS marketing_risk_profile (
    tenant_id VARCHAR(64) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id VARCHAR(128) NOT NULL,
    risk_score INTEGER NOT NULL DEFAULT 0,
    risk_level VARCHAR(32) NOT NULL DEFAULT 'LOW',
    risk_status VARCHAR(32) NOT NULL DEFAULT 'CLEAR',
    blacklisted BOOLEAN NOT NULL DEFAULT FALSE,
    country VARCHAR(32),
    provider VARCHAR(64) NOT NULL DEFAULT 'INTERNAL_USER_PROFILE',
    assessed_at TIMESTAMP,
    attributes_json JSON NOT NULL DEFAULT '{}'::json,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, subject_type, subject_id),
    CONSTRAINT ck_marketing_risk_profile_score CHECK (risk_score >= 0 AND risk_score <= 100),
    CONSTRAINT ck_marketing_risk_profile_level CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH')),
    CONSTRAINT ck_marketing_risk_profile_status CHECK (risk_status IN ('CLEAR', 'REVIEW', 'BLOCKED'))
);

CREATE INDEX IF NOT EXISTS idx_marketing_risk_profile_tenant_status
    ON marketing_risk_profile (tenant_id, risk_status);

CREATE INDEX IF NOT EXISTS idx_marketing_risk_profile_tenant_level
    ON marketing_risk_profile (tenant_id, risk_level);

INSERT INTO marketing_risk_profile (
    tenant_id, subject_type, subject_id, risk_score, risk_level, risk_status, blacklisted,
    country, provider, assessed_at, attributes_json, created_at, updated_at
)
SELECT
    tenant_id,
    subject_type,
    subject_id,
    CASE WHEN status = 'ACTIVE' THEN 0 ELSE 80 END,
    CASE WHEN status = 'ACTIVE' THEN 'LOW' ELSE 'HIGH' END,
    CASE WHEN status = 'ACTIVE' THEN 'CLEAR' ELSE 'REVIEW' END,
    FALSE,
    country,
    'INTERNAL_USER_PROFILE',
    updated_at,
    json_build_object(
        'sourceStatus', status,
        'sourceUserLevel', user_level,
        'sourceVipLevel', vip_level,
        'initializedFrom', 'marketing_user_profile'
    )::json,
    created_at,
    updated_at
FROM marketing_user_profile
ON CONFLICT (tenant_id, subject_type, subject_id) DO NOTHING;
