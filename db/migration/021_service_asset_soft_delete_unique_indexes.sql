DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT c.conname INTO constraint_name
    FROM pg_constraint c
    WHERE c.conrelid = 'marketing_service_interface'::regclass
      AND c.contype = 'f'
      AND (
          SELECT array_agg(a.attname::TEXT ORDER BY keys.ordinality)
          FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
          JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
      ) = ARRAY['tenant_id', 'service_ref']
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE marketing_service_interface DROP CONSTRAINT %I', constraint_name);
    END IF;

    SELECT c.conname INTO constraint_name
    FROM pg_constraint c
    WHERE c.conrelid = 'marketing_service_provider'::regclass
      AND c.contype = 'f'
      AND (
          SELECT array_agg(a.attname::TEXT ORDER BY keys.ordinality)
          FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
          JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
      ) = ARRAY['tenant_id', 'service_ref', 'interface_ref']
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE marketing_service_provider DROP CONSTRAINT %I', constraint_name);
    END IF;

    SELECT c.conname INTO constraint_name
    FROM pg_constraint c
    WHERE c.conrelid = 'marketing_service_interface_binding'::regclass
      AND c.contype = 'f'
      AND (
          SELECT array_agg(a.attname::TEXT ORDER BY keys.ordinality)
          FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
          JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
      ) = ARRAY['tenant_id', 'service_ref', 'interface_ref']
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE marketing_service_interface_binding DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

DO $$
DECLARE
    item RECORD;
    constraint_name TEXT;
    index_name TEXT;
BEGIN
    FOR item IN
        SELECT table_name, columns
        FROM (VALUES
            ('marketing_service', ARRAY['tenant_id', 'code']::TEXT[]),
            ('marketing_service_interface', ARRAY['tenant_id', 'service_ref', 'code']::TEXT[]),
            ('marketing_service_provider', ARRAY['tenant_id', 'service_ref', 'interface_ref', 'code']::TEXT[]),
            ('marketing_service_schema', ARRAY['tenant_id', 'schema_ref', 'version']::TEXT[]),
            ('marketing_service_interface_binding', ARRAY['tenant_id', 'service_ref', 'interface_ref', 'scope', 'workspace_id', 'folder_id', 'activity_id', 'node_id', 'principal_type', 'principal_id']::TEXT[]),
            ('marketing_service_rate_limit_policy', ARRAY['tenant_id', 'code']::TEXT[]),
            ('marketing_service_rate_limit_policy', ARRAY['tenant_id', 'service_ref', 'interface_ref', 'provider_ref']::TEXT[])
        ) AS targets(table_name, columns)
    LOOP
        SELECT c.conname INTO constraint_name
        FROM pg_constraint c
        WHERE c.conrelid = item.table_name::regclass
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname::TEXT ORDER BY keys.ordinality)
              FROM unnest(c.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = keys.attnum
          ) = item.columns
        LIMIT 1;
        IF constraint_name IS NOT NULL THEN
            EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', item.table_name, constraint_name);
        END IF;

        FOR index_name IN
            SELECT i.relname
            FROM pg_index x
            JOIN pg_class i ON i.oid = x.indexrelid
            WHERE x.indrelid = item.table_name::regclass
              AND x.indisunique
              AND NOT x.indisprimary
              AND x.indpred IS NULL
              AND (
                  SELECT array_agg(a.attname::TEXT ORDER BY keys.ordinality)
                  FROM unnest(string_to_array(x.indkey::TEXT, ' ')) WITH ORDINALITY AS keys(attnum, ordinality)
                  JOIN pg_attribute a ON a.attrelid = x.indrelid AND a.attnum::TEXT = keys.attnum
              ) = item.columns
        LOOP
            EXECUTE format('DROP INDEX IF EXISTS %I', index_name);
        END LOOP;
    END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_active_code_uidx
    ON marketing_service (tenant_id, code)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_interface_active_code_uidx
    ON marketing_service_interface (tenant_id, service_ref, code)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_provider_active_code_uidx
    ON marketing_service_provider (tenant_id, service_ref, interface_ref, code)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_schema_active_ref_version_uidx
    ON marketing_service_schema (tenant_id, schema_ref, version)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_interface_binding_active_scope_uidx
    ON marketing_service_interface_binding (
        tenant_id, service_ref, interface_ref, scope, workspace_id, folder_id,
        activity_id, node_id, principal_type, principal_id
    )
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_rate_limit_policy_active_code_uidx
    ON marketing_service_rate_limit_policy (tenant_id, code)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS marketing_service_rate_limit_policy_active_scope_uidx
    ON marketing_service_rate_limit_policy (tenant_id, service_ref, interface_ref, provider_ref)
    WHERE deleted_at IS NULL;
