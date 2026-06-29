-- Rollback 002: Restore original column types from migration 001
BEGIN;

ALTER TABLE dispatch_rider_otps
    ALTER COLUMN phone_number TYPE VARCHAR(30),
    ALTER COLUMN expires_at   TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN locked_until TYPE TIMESTAMP USING locked_until AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE dispatch_rider_identities
    ALTER COLUMN phone_number TYPE VARCHAR(30),
    ALTER COLUMN created_at   TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE dispatch_rider_sessions
    ALTER COLUMN phone_number TYPE VARCHAR(30),
    ALTER COLUMN device_id    TYPE VARCHAR(255),
    ALTER COLUMN device_type  TYPE VARCHAR(100),
    ALTER COLUMN ip_address   TYPE VARCHAR(100),
    ALTER COLUMN expires_at   TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at   TYPE TIMESTAMP USING revoked_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

COMMIT;
