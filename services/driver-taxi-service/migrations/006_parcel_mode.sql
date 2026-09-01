-- Parcel mode: a taxi may carry a package instead of a passenger, and the
-- passenger app chooses between the two when opening the ride flow.
--
-- Additive only. Note payment_method deliberately gets NO check constraint
-- even though wallet is now the only accepted value: a CHECK would fail to
-- apply on any database that already holds a 'cash' row from before that
-- change, and rewriting those rows to 'wallet' would falsify history.
-- Wallet-only is enforced in CreateBooking.

-- ─── Booking: purpose + package/receiver detail ─────────────────────────────
-- ride_purpose is a SEPARATE column from booking_mode, not a third
-- booking_mode value: booking_mode answers "standard or hourly reservation",
-- ride_purpose answers "passenger or package". The two are orthogonal, and
-- every existing booking_mode reader keeps its exact current meaning.
ALTER TABLE taxi_bookings
  ADD COLUMN IF NOT EXISTS ride_purpose   TEXT NOT NULL DEFAULT 'passenger',
  ADD COLUMN IF NOT EXISTS receiver_name  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receiver_phone TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS package_desc   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS package_type   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS package_size   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS is_fragile     BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS delivery_verification_status TEXT NOT NULL DEFAULT 'not_required';

ALTER TABLE taxi_bookings
  DROP CONSTRAINT IF EXISTS taxi_bookings_ride_purpose_chk;
ALTER TABLE taxi_bookings
  ADD CONSTRAINT taxi_bookings_ride_purpose_chk
  CHECK (ride_purpose IN ('passenger', 'parcel'));

ALTER TABLE taxi_bookings
  DROP CONSTRAINT IF EXISTS taxi_bookings_delivery_verification_chk;
ALTER TABLE taxi_bookings
  ADD CONSTRAINT taxi_bookings_delivery_verification_chk
  CHECK (delivery_verification_status IN ('not_required', 'pending', 'verified'));

CREATE INDEX IF NOT EXISTS idx_taxi_bookings_parcel
  ON taxi_bookings(ride_purpose)
  WHERE ride_purpose = 'parcel';

-- ─── Driver opt-in ──────────────────────────────────────────────────────────
-- An explicit boolean, NOT a reuse of the free-text operation_mode/service_type
-- columns from 001: those are unvalidated pass-through strings the profile
-- screen writes with preserve-on-empty semantics, so making matching
-- eligibility depend on one would hinge a real dispatch decision on a value
-- nothing validates.
--
-- Default false: a driver is opted OUT until they choose, so nobody is ever
-- matched to a package they never agreed to carry. The consequence is that on
-- a fresh database no driver can take a parcel until one opts in via
-- PUT /provider/parcel-preference.
ALTER TABLE taxi_providers
  ADD COLUMN IF NOT EXISTS accepts_parcels BOOLEAN NOT NULL DEFAULT false;

-- ─── Handover verification: one row per (booking, step) ─────────────────────
-- The table keeps its taxi_pickup_verifications name -- renaming it to
-- taxi_handover_verifications would churn every SQL string and integration
-- test for no functional gain. Its meaning is now widened to both handover
-- steps; see the verification package doc.
--
-- DEFAULT 'pickup' makes the primary-key swap safe on rows that already
-- exist. The dropped constraint name was confirmed against a live database
-- ('taxi_pickup_verifications_pkey', Postgres' default, which 003 did not
-- override) -- DROP CONSTRAINT IF EXISTS on a wrong name is a silent no-op
-- that would leave the ADD PRIMARY KEY below failing.
ALTER TABLE taxi_pickup_verifications
  ADD COLUMN IF NOT EXISTS step TEXT NOT NULL DEFAULT 'pickup';

ALTER TABLE taxi_pickup_verifications
  DROP CONSTRAINT IF EXISTS taxi_pickup_verifications_pkey;
ALTER TABLE taxi_pickup_verifications
  ADD PRIMARY KEY (booking_id, step);

ALTER TABLE taxi_pickup_verifications
  DROP CONSTRAINT IF EXISTS taxi_pickup_verifications_step_chk;
ALTER TABLE taxi_pickup_verifications
  ADD CONSTRAINT taxi_pickup_verifications_step_chk
  CHECK (step IN ('pickup', 'delivery'));
