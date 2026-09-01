DROP INDEX IF EXISTS dispatch_booking_serviceability_quote;
DROP INDEX IF EXISTS dispatch_booking_create_idempotency;

ALTER TABLE dispatch_bookings
    DROP CONSTRAINT IF EXISTS dispatch_booking_serviceability_customer_fk;

ALTER TABLE dispatch_bookings
    DROP COLUMN IF EXISTS create_request_fingerprint,
    DROP COLUMN IF EXISTS create_idempotency_key,
    DROP COLUMN IF EXISTS serviceability_quote_fingerprint,
    DROP COLUMN IF EXISTS serviceability_quote_version,
    DROP COLUMN IF EXISTS serviceability_quote_id;

DROP INDEX IF EXISTS dispatch_serviceability_quote_customer_identity;
