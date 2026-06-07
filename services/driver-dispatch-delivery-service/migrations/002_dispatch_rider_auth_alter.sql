-- Migration 002: Align dispatch rider auth column types with spec
--
-- Changes from 001:
--   - phone_number VARCHAR(30) → TEXT   (no artificial length cap)
--   - expires_at   TIMESTAMP   → TIMESTAMPTZ   (timezone-aware)
--   - locked_until TIMESTAMP   → TIMESTAMPTZ
--   - created_at   TIMESTAMP   → TIMESTAMPTZ
--   - updated_at   TIMESTAMP   → TIMESTAMPTZ
--   - revoked_at   TIMESTAMP   → TIMESTAMPTZ
--   - ip_address   VARCHAR(100) → TEXT
--   - user_agent   TEXT (no change)
--   - device_id    VARCHAR(255) → TEXT
--   - device_type  VARCHAR(100) → TEXT
--
-- Safe to run multiple times: ALTER COLUMN TYPE is idempotent when types match.

BEGIN;

-- dispatch_rider_otps ─────────────────────────────────────────────────────────
ALTER TABLE dispatch_rider_otps
    ALTER COLUMN phone_number TYPE TEXT,
    ALTER COLUMN expires_at   TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN locked_until TYPE TIMESTAMPTZ USING locked_until AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- dispatch_rider_identities ───────────────────────────────────────────────────
ALTER TABLE dispatch_rider_identities
    ALTER COLUMN phone_number TYPE TEXT,
    ALTER COLUMN created_at   TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- dispatch_rider_sessions ─────────────────────────────────────────────────────
ALTER TABLE dispatch_rider_sessions
    ALTER COLUMN phone_number TYPE TEXT,
    ALTER COLUMN device_id    TYPE TEXT,
    ALTER COLUMN device_type  TYPE TEXT,
    ALTER COLUMN ip_address   TYPE TEXT,
    ALTER COLUMN expires_at   TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at   TYPE TIMESTAMPTZ USING revoked_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

COMMIT;
