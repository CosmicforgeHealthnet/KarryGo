-- 007_partner_booking_details.sql
-- Partner delivery detail fields for the partner frontend/Figma delivery page.
-- Additive and safe: existing rows keep working; new columns are nullable (or
-- have defaults). Also renames the legacy dest_* columns to dropoff_* so the
-- api-gateway matches the dispatch service naming (dropoff_address/lat/lng).

-- 1. Rename dest_* -> dropoff_* (safe: only when dest_* still exists and
--    dropoff_* does not; re-running is a no-op).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dest_address')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_address') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dest_address TO dropoff_address;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dest_lat')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_lat') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dest_lat TO dropoff_lat;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dest_lng')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_lng') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dest_lng TO dropoff_lng;
    END IF;
END $$;

-- 2. Fallback: if neither dest_* nor dropoff_* existed (fresh non-standard
--    schema), create the dropoff_* columns with the same types 001 used for
--    dest_* (text / numeric(10,7)). No-op after the rename above.
ALTER TABLE partner_bookings
    ADD COLUMN IF NOT EXISTS dropoff_address text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dropoff_lat numeric(10,7) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS dropoff_lng numeric(10,7) NOT NULL DEFAULT 0;

-- 3. New delivery detail columns. currency already exists from 001 (kept here
--    per contract; IF NOT EXISTS makes it a no-op).
ALTER TABLE partner_bookings
    ADD COLUMN IF NOT EXISTS distance_km          numeric(8,2),
    ADD COLUMN IF NOT EXISTS package_desc         text,
    ADD COLUMN IF NOT EXISTS package_weight       numeric(6,2),
    ADD COLUMN IF NOT EXISTS currency             text NOT NULL DEFAULT 'NGN',
    ADD COLUMN IF NOT EXISTS pickup_contact_name  text,
    ADD COLUMN IF NOT EXISTS pickup_contact_phone text,
    ADD COLUMN IF NOT EXISTS rider_id             uuid,
    ADD COLUMN IF NOT EXISTS rider_name           text,
    ADD COLUMN IF NOT EXISTS rider_phone          text,
    ADD COLUMN IF NOT EXISTS rider_rating         numeric(3,2),
    ADD COLUMN IF NOT EXISTS vehicle_color        text,
    ADD COLUMN IF NOT EXISTS vehicle_model        text,
    ADD COLUMN IF NOT EXISTS plate_number         text;

-- 4. Provider-arrived callback support. 001 created the original CHECK
--    constraint; replace it safely so partner_bookings can store the additive
--    arrived lifecycle state.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint
               WHERE conrelid = 'partner_bookings'::regclass
                 AND conname = 'partner_bookings_status_check') THEN
        ALTER TABLE partner_bookings DROP CONSTRAINT partner_bookings_status_check;
    END IF;

    ALTER TABLE partner_bookings
        ADD CONSTRAINT partner_bookings_status_check
        CHECK (status IN ('pending', 'dispatching', 'accepted', 'arrived', 'in_progress', 'completed', 'cancelled', 'failed', 'no_provider'));
END $$;
