-- Bind protected customer booking creation to the exact serviceability
-- decision that authorized it and make create retries source-safe. Existing
-- legacy bookings remain valid because every new column is nullable.

ALTER TABLE dispatch_bookings
    ADD COLUMN serviceability_quote_id TEXT NULL,
    ADD COLUMN serviceability_quote_version BIGINT NULL
        CHECK (serviceability_quote_version IS NULL OR serviceability_quote_version > 0),
    ADD COLUMN serviceability_quote_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN create_idempotency_key TEXT NULL,
    ADD COLUMN create_request_fingerprint TEXT NOT NULL DEFAULT '';

-- The application performs customer-scoped lookup before mutation; this
-- composite relation also makes cross-customer quote binding impossible at the
-- database boundary.
CREATE UNIQUE INDEX dispatch_serviceability_quote_customer_identity
    ON dispatch_serviceability_quotes(id, customer_id);

ALTER TABLE dispatch_bookings
    ADD CONSTRAINT dispatch_booking_serviceability_customer_fk
    FOREIGN KEY(serviceability_quote_id, customer_id)
    REFERENCES dispatch_serviceability_quotes(id, customer_id)
    ON DELETE RESTRICT;

CREATE UNIQUE INDEX dispatch_booking_create_idempotency
    ON dispatch_bookings(customer_id, create_idempotency_key)
    WHERE create_idempotency_key IS NOT NULL;

CREATE INDEX dispatch_booking_serviceability_quote
    ON dispatch_bookings(serviceability_quote_id)
    WHERE serviceability_quote_id IS NOT NULL;

COMMENT ON COLUMN dispatch_bookings.serviceability_quote_id IS
    'Customer-owned, unexpired serviceability decision used for protected booking creation.';
COMMENT ON COLUMN dispatch_bookings.create_idempotency_key IS
    'Customer-scoped create operation key; same key with a changed fingerprint is rejected.';
