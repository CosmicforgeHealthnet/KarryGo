ALTER TABLE trips
    DROP CONSTRAINT IF EXISTS trips_package_size_check;

ALTER TABLE trips
    DROP COLUMN IF EXISTS package_type,
    DROP COLUMN IF EXISTS package_size,
    DROP COLUMN IF EXISTS is_fragile;
