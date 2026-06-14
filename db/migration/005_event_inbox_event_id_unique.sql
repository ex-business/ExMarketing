CREATE UNIQUE INDEX IF NOT EXISTS marketing_event_inbox_tenant_event_id_key
  ON marketing_event_inbox (tenant_id, event_id);
