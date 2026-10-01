DROP INDEX IF EXISTS idx_hauling_outbox_reconciliation;
DROP INDEX IF EXISTS idx_hauling_outbox_target_ready;
DROP INDEX IF EXISTS uq_hauling_outbox_idempotency;
ALTER TABLE hauling_domain_outbox DROP CONSTRAINT IF EXISTS chk_hauling_outbox_reconciliation_state;
ALTER TABLE hauling_domain_outbox
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS reconciliation_state,
    DROP COLUMN IF EXISTS target;

DROP INDEX IF EXISTS idx_hauling_bookings_organization;
DROP INDEX IF EXISTS uq_hauling_bookings_fleet_assignment;
ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_settlement_mode;
ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_execution_mode;
ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_corporate_complete;
ALTER TABLE haulage_bookings
    DROP COLUMN IF EXISTS fleet_correlation_id,
    DROP COLUMN IF EXISTS completion_version,
    DROP COLUMN IF EXISTS settlement_mode,
    DROP COLUMN IF EXISTS execution_mode,
    DROP COLUMN IF EXISTS fleet_assignment_version,
    DROP COLUMN IF EXISTS fleet_assignment_id,
    DROP COLUMN IF EXISTS membership_id,
    DROP COLUMN IF EXISTS corporate_organization_id,
    DROP COLUMN IF EXISTS is_corporate_managed;
