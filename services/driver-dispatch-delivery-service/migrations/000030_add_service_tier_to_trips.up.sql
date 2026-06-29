ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS service_tier TEXT DEFAULT 'standard';

UPDATE trips
SET service_tier = 'standard'
WHERE service_tier IS NULL OR service_tier = '';

ALTER TABLE trips
    DROP CONSTRAINT IF EXISTS trips_service_tier_check;

ALTER TABLE trips
    ADD CONSTRAINT trips_service_tier_check
    CHECK (service_tier IS NULL OR service_tier IN ('standard', 'express'));
