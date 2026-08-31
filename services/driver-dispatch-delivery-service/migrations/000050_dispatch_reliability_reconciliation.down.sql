DROP INDEX IF EXISTS dispatch_booking_hold_reconciliation_due;
ALTER TABLE dispatch_bookings
    DROP COLUMN IF EXISTS hold_reconciliation_state,
    DROP COLUMN IF EXISTS hold_reconciliation_attempts,
    DROP COLUMN IF EXISTS hold_reconciliation_next_at,
    DROP COLUMN IF EXISTS hold_reconciliation_lease_owner,
    DROP COLUMN IF EXISTS hold_reconciliation_lease_expires_at;

DROP INDEX IF EXISTS dispatch_trip_wallet_settlement_due;
ALTER TABLE trips
    DROP COLUMN IF EXISTS wallet_settlement_state,
    DROP COLUMN IF EXISTS wallet_settlement_attempts,
    DROP COLUMN IF EXISTS wallet_settlement_next_at,
    DROP COLUMN IF EXISTS wallet_settlement_lease_owner,
    DROP COLUMN IF EXISTS wallet_settlement_lease_expires_at,
    DROP COLUMN IF EXISTS wallet_settlement_error_code;

ALTER TABLE dispatch_marketplace_outbox
    DROP CONSTRAINT IF EXISTS dispatch_marketplace_outbox_secret_safe;
DROP FUNCTION IF EXISTS dispatch_outbox_payload_is_safe(JSONB);

DROP INDEX IF EXISTS dispatch_return_assignment_reconciliation_due;
ALTER TABLE dispatch_return_workflows
    DROP COLUMN IF EXISTS assignment_last_error_code,
    DROP COLUMN IF EXISTS assignment_lease_expires_at,
    DROP COLUMN IF EXISTS assignment_lease_owner,
    DROP COLUMN IF EXISTS assignment_next_attempt_at,
    DROP COLUMN IF EXISTS assignment_attempt_count,
    DROP COLUMN IF EXISTS assignment_state;

DROP INDEX IF EXISTS dispatch_handover_notification_reconciliation_due;
UPDATE dispatch_handover_challenges SET notification_state='FAILED'
WHERE notification_state='DEAD_LETTER';
ALTER TABLE dispatch_handover_challenges
    DROP COLUMN IF EXISTS notification_terminal_at,
    DROP COLUMN IF EXISTS notification_last_error_code,
    DROP COLUMN IF EXISTS notification_lease_expires_at,
    DROP COLUMN IF EXISTS notification_lease_owner,
    DROP COLUMN IF EXISTS notification_next_attempt_at,
    DROP COLUMN IF EXISTS notification_attempt_count,
    DROP CONSTRAINT IF EXISTS dispatch_handover_challenges_notification_state_check,
    ADD CONSTRAINT dispatch_handover_challenges_notification_state_check
        CHECK (notification_state IN ('NOT_REQUESTED','REQUESTED','ACCEPTED','UNAVAILABLE','FAILED'));

DROP INDEX IF EXISTS dispatch_queue_wallet_reconciliation_due;
ALTER TABLE dispatch_queue_entries
    DROP COLUMN IF EXISTS wallet_terminal_at,
    DROP COLUMN IF EXISTS wallet_lease_expires_at,
    DROP COLUMN IF EXISTS wallet_lease_owner,
    DROP COLUMN IF EXISTS wallet_next_attempt_at,
    DROP COLUMN IF EXISTS wallet_attempt_count;
