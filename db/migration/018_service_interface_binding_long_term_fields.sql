ALTER TABLE marketing_service_interface_binding
    ADD COLUMN IF NOT EXISTS include_subfolders BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS min_role VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS environment VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_limit_ref VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_environment
    ON marketing_service_interface_binding(tenant_id, environment, status);

CREATE INDEX IF NOT EXISTS idx_marketing_service_interface_binding_deleted
    ON marketing_service_interface_binding(tenant_id, deleted_at)
    WHERE deleted_at IS NOT NULL;
