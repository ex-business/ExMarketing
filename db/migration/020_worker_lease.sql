CREATE TABLE IF NOT EXISTS marketing_worker_lease (
    name TEXT PRIMARY KEY,
    owner TEXT NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT marketing_worker_lease_name_check CHECK (length(trim(name)) > 0),
    CONSTRAINT marketing_worker_lease_owner_check CHECK (length(trim(owner)) > 0),
    CONSTRAINT marketing_worker_lease_time_check CHECK (lease_until >= acquired_at)
);

CREATE INDEX IF NOT EXISTS marketing_worker_lease_expiry_idx
    ON marketing_worker_lease (lease_until);
