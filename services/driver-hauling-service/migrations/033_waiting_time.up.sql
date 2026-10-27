-- Waiting time: the driver's wait at pickup and drop-off beyond a free period
-- is charged to the customer in fixed blocks.

-- Rate history. A change is a new row, never an edit, so every booking's rates
-- can be traced to the change that set them.
CREATE TABLE IF NOT EXISTS hauling_waiting_policies (
  id              TEXT PRIMARY KEY,
  free_minutes    INTEGER NOT NULL CHECK (free_minutes >= 0 AND free_minutes <= 1440),
  block_minutes   INTEGER NOT NULL CHECK (block_minutes > 0 AND block_minutes <= 1440),
  block_fee_kobo  BIGINT  NOT NULL CHECK (block_fee_kobo >= 0),
  created_by      TEXT NOT NULL CHECK (created_by <> ''),
  reason          TEXT NOT NULL CHECK (reason <> ''),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS hauling_waiting_policies_latest_idx ON hauling_waiting_policies (created_at DESC);

CREATE OR REPLACE FUNCTION hauling_waiting_policies_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'waiting-time rates are append-only: add a new row instead of changing %', OLD.id;
END $$;

DROP TRIGGER IF EXISTS hauling_waiting_policies_append_only ON hauling_waiting_policies;
CREATE TRIGGER hauling_waiting_policies_append_only
  BEFORE UPDATE OR DELETE ON hauling_waiting_policies
  FOR EACH ROW EXECUTE FUNCTION hauling_waiting_policies_append_only();

-- One row per booking that reached a stop: the rates it is charged at (copied
-- when the driver first arrives), when the driver arrived at and left each stop,
-- and what happened to the fee.
CREATE TABLE IF NOT EXISTS hauling_booking_waiting (
  booking_id          UUID PRIMARY KEY REFERENCES haulage_bookings (id),
  policy_id           TEXT,
  free_minutes        INTEGER NOT NULL CHECK (free_minutes >= 0),
  block_minutes       INTEGER NOT NULL CHECK (block_minutes > 0),
  block_fee_kobo      BIGINT  NOT NULL CHECK (block_fee_kobo >= 0),
  pickup_arrived_at   TIMESTAMPTZ,
  pickup_left_at      TIMESTAMPTZ,
  dropoff_arrived_at  TIMESTAMPTZ,
  dropoff_left_at     TIMESTAMPTZ,
  fee_kobo            BIGINT NOT NULL DEFAULT 0 CHECK (fee_kobo >= 0),
  -- NONE: nothing to charge. PENDING: computed, not settled yet. PAID: taken
  -- from the wallet. OWED: the wallet was short. CASH: the driver collects it.
  -- NOT_APPLICABLE: a job settled outside the customer's wallet (fleet, vendor).
  fee_status          TEXT NOT NULL DEFAULT 'PENDING'
                      CHECK (fee_status IN ('NONE', 'PENDING', 'PAID', 'OWED', 'CASH', 'NOT_APPLICABLE')),
  fee_reference       TEXT,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT hauling_booking_waiting_pickup_order CHECK (pickup_left_at IS NULL OR pickup_arrived_at IS NULL OR pickup_left_at >= pickup_arrived_at),
  CONSTRAINT hauling_booking_waiting_dropoff_order CHECK (dropoff_left_at IS NULL OR dropoff_arrived_at IS NULL OR dropoff_left_at >= dropoff_arrived_at)
);

CREATE INDEX IF NOT EXISTS hauling_booking_waiting_owed_idx
  ON hauling_booking_waiting (updated_at) WHERE fee_status = 'OWED';
