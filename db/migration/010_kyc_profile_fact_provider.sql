CREATE TABLE IF NOT EXISTS marketing_kyc_profile (
    tenant_id VARCHAR(64) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id VARCHAR(128) NOT NULL,
    kyc_status VARCHAR(32) NOT NULL DEFAULT 'UNVERIFIED',
    kyc_level VARCHAR(32) NOT NULL DEFAULT 'BASIC',
    country VARCHAR(32),
    provider VARCHAR(64) NOT NULL DEFAULT 'INTERNAL_USER_PROFILE',
    verified_at TIMESTAMP,
    attributes_json JSON NOT NULL DEFAULT '{}'::json,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, subject_type, subject_id),
    CONSTRAINT ck_marketing_kyc_profile_status CHECK (kyc_status IN ('VERIFIED', 'UNVERIFIED', 'PENDING', 'REJECTED', 'EXPIRED')),
    CONSTRAINT ck_marketing_kyc_profile_level CHECK (kyc_level IN ('BASIC', 'STANDARD', 'ENHANCED'))
);

CREATE INDEX IF NOT EXISTS idx_marketing_kyc_profile_tenant_status
    ON marketing_kyc_profile (tenant_id, kyc_status);

INSERT INTO marketing_kyc_profile (
    tenant_id, subject_type, subject_id, kyc_status, kyc_level, country, provider,
    verified_at, attributes_json, created_at, updated_at
)
SELECT
    tenant_id,
    subject_type,
    subject_id,
    CASE WHEN status = 'ACTIVE' THEN 'VERIFIED' ELSE 'UNVERIFIED' END,
    CASE WHEN vip_level >= 3 THEN 'STANDARD' ELSE 'BASIC' END,
    country,
    'INTERNAL_USER_PROFILE',
    CASE WHEN status = 'ACTIVE' THEN COALESCE(registered_at, updated_at) ELSE NULL END,
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
