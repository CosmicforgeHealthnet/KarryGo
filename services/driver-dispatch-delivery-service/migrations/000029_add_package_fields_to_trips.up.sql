ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS package_type TEXT,
    ADD COLUMN IF NOT EXISTS package_size TEXT,
    ADD COLUMN IF NOT EXISTS is_fragile BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE trips
    ADD CONSTRAINT trips_package_size_check
    CHECK (package_size IS NULL OR package_size IN ('small', 'medium', 'large'));
