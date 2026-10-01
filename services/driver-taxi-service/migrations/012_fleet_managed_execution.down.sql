DROP INDEX IF EXISTS idx_taxi_outbox_reconciliation;
DROP INDEX IF EXISTS idx_taxi_outbox_target_ready;
DROP INDEX IF EXISTS uq_taxi_outbox_idempotency;
ALTER TABLE taxi_outbox DROP CONSTRAINT IF EXISTS chk_taxi_outbox_reconciliation_state;
ALTER TABLE taxi_outbox
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS reconciliation_state,
    DROP COLUMN IF EXISTS target;

DROP INDEX IF EXISTS idx_taxi_reservation_source_request;
DROP INDEX IF EXISTS uq_taxi_reservation_live_booking;
DROP TABLE IF EXISTS taxi_fleet_execution_reservations;

DROP INDEX IF EXISTS idx_taxi_bookings_organization;
DROP INDEX IF EXISTS uq_taxi_bookings_fleet_assignment;
ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_source_context;
ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_settlement_mode;
ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_corporate_complete;
ALTER TABLE taxi_bookings
    DROP COLUMN IF EXISTS fleet_correlation_id,
    DROP COLUMN IF EXISTS source_version,
    DROP COLUMN IF EXISTS settlement_mode,
    DROP COLUMN IF EXISTS source_context,
    DROP COLUMN IF EXISTS fleet_assignment_version,
    DROP COLUMN IF EXISTS fleet_assignment_id,
    DROP COLUMN IF EXISTS membership_id,
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS is_corporate_managed;
