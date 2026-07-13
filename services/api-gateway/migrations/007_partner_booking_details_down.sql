-- 007_partner_booking_details_down.sql
-- Reverts 003: drops the delivery-detail columns added by 003 and renames the
-- dropoff_* columns back to dest_*. currency is NOT dropped because it was
-- created by 001, not 003.

ALTER TABLE partner_bookings
    DROP COLUMN IF EXISTS distance_km,
    DROP COLUMN IF EXISTS package_desc,
    DROP COLUMN IF EXISTS package_weight,
    DROP COLUMN IF EXISTS pickup_contact_name,
    DROP COLUMN IF EXISTS pickup_contact_phone,
    DROP COLUMN IF EXISTS rider_id,
    DROP COLUMN IF EXISTS rider_name,
    DROP COLUMN IF EXISTS rider_phone,
    DROP COLUMN IF EXISTS rider_rating,
    DROP COLUMN IF EXISTS vehicle_color,
    DROP COLUMN IF EXISTS vehicle_model,
    DROP COLUMN IF EXISTS plate_number;

-- Revert the additive arrived lifecycle state before restoring the original
-- 001 status constraint.
UPDATE partner_bookings SET status = 'accepted' WHERE status = 'arrived';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint
               WHERE conrelid = 'partner_bookings'::regclass
                 AND conname = 'partner_bookings_status_check') THEN
        ALTER TABLE partner_bookings DROP CONSTRAINT partner_bookings_status_check;
    END IF;

    ALTER TABLE partner_bookings
        ADD CONSTRAINT partner_bookings_status_check
        CHECK (status IN ('pending', 'dispatching', 'accepted', 'in_progress', 'completed', 'cancelled', 'failed', 'no_provider'));
END $$;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_address')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dest_address') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dropoff_address TO dest_address;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_lat')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dest_lat') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dropoff_lat TO dest_lat;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'partner_bookings' AND column_name = 'dropoff_lng')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name = 'partner_bookings' AND column_name = 'dest_lng') THEN
        ALTER TABLE partner_bookings RENAME COLUMN dropoff_lng TO dest_lng;
    END IF;
END $$;
