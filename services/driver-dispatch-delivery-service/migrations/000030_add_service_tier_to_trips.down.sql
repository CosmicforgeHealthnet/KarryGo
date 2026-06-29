ALTER TABLE trips
    DROP CONSTRAINT IF EXISTS trips_service_tier_check;

ALTER TABLE trips
    DROP COLUMN IF EXISTS service_tier;
