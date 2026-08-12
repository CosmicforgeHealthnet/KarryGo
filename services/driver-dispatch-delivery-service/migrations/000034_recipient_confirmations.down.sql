-- Reverses WP6.3 recipient confirmation.

ALTER TABLE trips DROP COLUMN IF EXISTS recipient_confirmation_method;
ALTER TABLE trips DROP COLUMN IF EXISTS recipient_confirmed_at;

DROP INDEX IF EXISTS idx_recipient_confirmation_confirmed_at;
DROP INDEX IF EXISTS uq_recipient_confirmation_idempotency;
DROP INDEX IF EXISTS uq_recipient_confirmation_trip;

DROP TABLE IF EXISTS recipient_confirmations;
