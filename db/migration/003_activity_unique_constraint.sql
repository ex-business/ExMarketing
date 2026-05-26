DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    WHERE t.relname = 'marketing_activity'
      AND c.conname = 'marketing_activity_tenant_code_key'
  ) THEN
    ALTER TABLE marketing_activity
    ADD CONSTRAINT marketing_activity_tenant_code_key UNIQUE (tenant_id, activity_code);
  END IF;
END
$$;
