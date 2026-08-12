-- Reverses WP6.4 failed delivery.

ALTER TABLE trips DROP COLUMN IF EXISTS failed_at;

DROP INDEX IF EXISTS idx_delivery_failure_retry;
DROP INDEX IF EXISTS idx_delivery_failure_category;
DROP INDEX IF EXISTS uq_delivery_failure_idempotency;
DROP INDEX IF EXISTS uq_delivery_failure_trip;

DROP TABLE IF EXISTS delivery_failures;
