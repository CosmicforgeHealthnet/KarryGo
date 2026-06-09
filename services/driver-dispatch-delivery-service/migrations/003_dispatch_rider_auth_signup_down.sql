-- Migration 003 rollback: Remove email/purpose columns added for signup flow

BEGIN;

ALTER TABLE dispatch_rider_otps
    DROP COLUMN IF EXISTS purpose,
    DROP COLUMN IF EXISTS email;

DROP INDEX IF EXISTS idx_dispatch_rider_identities_email_unique;

ALTER TABLE dispatch_rider_identities
    DROP COLUMN IF EXISTS email;

COMMIT;
