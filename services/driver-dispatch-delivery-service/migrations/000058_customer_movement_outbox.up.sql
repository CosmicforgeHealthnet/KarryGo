-- Canonical customer movement emission schema.
--
-- An execution authority does not rewrite its state machine to publish
-- movement events. It records, per movement, the last state it published and
-- a monotonic source_version, and queues the envelope durably. Emission is
-- therefore a function of committed state: a crash loses nothing, and a
-- replay re-publishes exactly what the authority currently believes.
CREATE TABLE IF NOT EXISTS customer_movement_emissions (
  authority TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  state TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  source_version BIGINT NOT NULL CHECK (source_version >= 1),
  emitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (authority, source_ref)
);

CREATE TABLE IF NOT EXISTS customer_movement_outbox (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  authority TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  source_version BIGINT NOT NULL CHECK (source_version >= 1),
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DELIVERED','DEAD')),
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  leased_until TIMESTAMPTZ,
  leased_by TEXT,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_customer_movement_outbox_claimable
  ON customer_movement_outbox(next_attempt_at)
  WHERE status = 'PENDING';
