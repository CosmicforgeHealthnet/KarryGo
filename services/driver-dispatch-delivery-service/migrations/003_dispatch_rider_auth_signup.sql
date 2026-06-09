-- Migration 003: Add email support for signup flow
--
-- Changes:
--   dispatch_rider_identities: add nullable email column with partial unique index
--   dispatch_rider_otps:       add nullable email column (stores email for signup OTPs)
--                              add purpose column (login | signup)
--
-- Safe to run multiple times: ADD COLUMN IF NOT EXISTS, CREATE INDEX IF NOT EXISTS.

BEGIN;

-- dispatch_rider_identities ─────────────────────────────────────────────────
ALTER TABLE dispatch_rider_identities
    ADD COLUMN IF NOT EXISTS email TEXT;

-- Only non-NULL emails must be unique (partial unique index allows multiple NULLs).
CREATE UNIQUE INDEX IF NOT EXISTS idx_dispatch_rider_identities_email_unique
    ON dispatch_rider_identities (email)
    WHERE email IS NOT NULL;

-- dispatch_rider_otps ────────────────────────────────────────────────────────
ALTER TABLE dispatch_rider_otps
    ADD COLUMN IF NOT EXISTS email TEXT,
    ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT 'login';

COMMIT;
