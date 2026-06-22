-- Migration 002: add preferred_truck_type and scheduled_at to haulage_bookings.
-- cargo_type is kept for historical data; preferred_truck_type is the UI-facing field.

ALTER TABLE haulage_bookings
  ADD COLUMN IF NOT EXISTS preferred_truck_type TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ;

ALTER TABLE haulage_bookings
  ALTER COLUMN cargo_type SET DEFAULT '';
