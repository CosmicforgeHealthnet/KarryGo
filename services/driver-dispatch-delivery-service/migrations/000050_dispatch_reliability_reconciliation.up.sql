-- Durable reconciliation and scheduled activation for existing Dispatch
-- workflows. This extends the canonical queue, handover, recovery and outbox
-- tables; it intentionally creates no second outbox or remote-service ledger.

ALTER TABLE trips
    ADD COLUMN wallet_settlement_state TEXT NOT NULL DEFAULT 'PENDING' CHECK (wallet_settlement_state IN ('PENDING','UNKNOWN','ACKNOWLEDGED','DEAD_LETTER')),
    ADD COLUMN wallet_settlement_attempts INTEGER NOT NULL DEFAULT 0 CHECK (wallet_settlement_attempts >= 0),
    ADD COLUMN wallet_settlement_next_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN wallet_settlement_lease_owner TEXT,
    ADD COLUMN wallet_settlement_lease_expires_at TIMESTAMPTZ,
    ADD COLUMN wallet_settlement_error_code TEXT NOT NULL DEFAULT '';
CREATE INDEX dispatch_trip_wallet_settlement_due ON trips(wallet_settlement_next_at)
    WHERE status='completed' AND wallet_settlement_state IN ('PENDING','UNKNOWN');

ALTER TABLE dispatch_bookings
    ADD COLUMN hold_reconciliation_state TEXT NOT NULL DEFAULT 'PENDING' CHECK (hold_reconciliation_state IN ('PENDING','UNKNOWN','ACKNOWLEDGED','DEAD_LETTER')),
    ADD COLUMN hold_reconciliation_attempts INTEGER NOT NULL DEFAULT 0 CHECK (hold_reconciliation_attempts >= 0),
    ADD COLUMN hold_reconciliation_next_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN hold_reconciliation_lease_owner TEXT,
    ADD COLUMN hold_reconciliation_lease_expires_at TIMESTAMPTZ;
CREATE INDEX dispatch_booking_hold_reconciliation_due ON dispatch_bookings(hold_reconciliation_next_at)
    WHERE payment_status='unpaid' AND hold_reconciliation_state IN ('PENDING','UNKNOWN');

ALTER TABLE dispatch_queue_entries
    ADD COLUMN wallet_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (wallet_attempt_count >= 0),
    ADD COLUMN wallet_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN wallet_lease_owner TEXT NULL,
    ADD COLUMN wallet_lease_expires_at TIMESTAMPTZ NULL,
    ADD COLUMN wallet_terminal_at TIMESTAMPTZ NULL;

CREATE INDEX dispatch_queue_wallet_reconciliation_due
    ON dispatch_queue_entries(wallet_next_attempt_at, updated_at)
    WHERE wallet_state IN ('REQUESTED','UNKNOWN');

ALTER TABLE dispatch_handover_challenges
    DROP CONSTRAINT dispatch_handover_challenges_notification_state_check,
    ADD CONSTRAINT dispatch_handover_challenges_notification_state_check
        CHECK (notification_state IN ('NOT_REQUESTED','REQUESTED','ACCEPTED','UNAVAILABLE','FAILED','DEAD_LETTER')),
    ADD COLUMN notification_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (notification_attempt_count >= 0),
    ADD COLUMN notification_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN notification_lease_owner TEXT NULL,
    ADD COLUMN notification_lease_expires_at TIMESTAMPTZ NULL,
    ADD COLUMN notification_last_error_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN notification_terminal_at TIMESTAMPTZ NULL;

CREATE INDEX dispatch_handover_notification_reconciliation_due
    ON dispatch_handover_challenges(notification_next_attempt_at, updated_at)
    WHERE notification_state IN ('REQUESTED','FAILED');

ALTER TABLE dispatch_return_workflows
    ADD COLUMN assignment_state TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (assignment_state IN ('PENDING','DELIVERING','ACKNOWLEDGED','UNKNOWN','DEAD_LETTER')),
    ADD COLUMN assignment_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (assignment_attempt_count >= 0),
    ADD COLUMN assignment_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN assignment_lease_owner TEXT NULL,
    ADD COLUMN assignment_lease_expires_at TIMESTAMPTZ NULL,
    ADD COLUMN assignment_last_error_code TEXT NOT NULL DEFAULT '';

CREATE INDEX dispatch_return_assignment_reconciliation_due
    ON dispatch_return_workflows(assignment_next_attempt_at, updated_at)
    WHERE assignment_state IN ('PENDING','UNKNOWN','DELIVERING');

-- Generic outbox payloads are transport metadata, never a secret transport.
-- Recurse through objects and arrays so nested credentials cannot bypass the
-- guard. Secret-bearing handover delivery continues over the tightly scoped
-- Notification Service contract.
CREATE OR REPLACE FUNCTION dispatch_outbox_payload_is_safe(value JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    item RECORD;
BEGIN
    IF jsonb_typeof(value) = 'object' THEN
        FOR item IN SELECT key, val FROM jsonb_each(value) AS e(key, val)
        LOOP
            IF lower(item.key) IN (
                'otp','otp_hash','otphash','qr_token','qr_token_hash','qrtoken','qrtokenhash',
                'delivery_code','delivery_code_hash','password','secret','api_key','apikey',
                'access_token','accesstoken','refresh_token','refreshtoken','cvv','card_number',
                'receiver_phone','receiverphone','receiver_email','receiveremail',
                'recipient_phone','recipientphone','recipient_email','recipientemail'
            ) THEN
                RETURN FALSE;
            END IF;
            IF NOT dispatch_outbox_payload_is_safe(item.val) THEN
                RETURN FALSE;
            END IF;
        END LOOP;
    ELSIF jsonb_typeof(value) = 'array' THEN
        FOR item IN SELECT element AS val FROM jsonb_array_elements(value) AS a(element)
        LOOP
            IF NOT dispatch_outbox_payload_is_safe(item.val) THEN
                RETURN FALSE;
            END IF;
        END LOOP;
    END IF;
    RETURN TRUE;
END;
$$;

ALTER TABLE dispatch_marketplace_outbox
    ADD CONSTRAINT dispatch_marketplace_outbox_secret_safe
    CHECK (dispatch_outbox_payload_is_safe(payload));
