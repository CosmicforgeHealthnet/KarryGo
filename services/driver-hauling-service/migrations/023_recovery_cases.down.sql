-- Reverse of 023.
--
-- The booking columns go first: active_recovery_id references the case table,
-- so dropping the table while the foreign key still exists would fail.

ALTER TABLE haulage_bookings
    DROP COLUMN IF EXISTS active_recovery_id,
    DROP COLUMN IF EXISTS recovery_count;

DROP TABLE IF EXISTS hauling_recovery_cases;
