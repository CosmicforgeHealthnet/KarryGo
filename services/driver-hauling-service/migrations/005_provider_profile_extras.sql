-- Migration 005: provider + truck metadata surfaced by the provider profile screens.
-- Profile Info (language), Verification & Documents (driver's license no + expiry),
-- and the richer Truck Information form (license type, axles, experience, goods, insurance).

ALTER TABLE truck_providers
  ADD COLUMN IF NOT EXISTS language               TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS driver_license_number  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS license_expiry_year    TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS license_expiry_date    TEXT NOT NULL DEFAULT '';

ALTER TABLE trucks
  ADD COLUMN IF NOT EXISTS license_type        TEXT   NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS number_of_axles     TEXT   NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS years_of_experience TEXT   NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS goods_types         TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS has_insurance       BOOLEAN NOT NULL DEFAULT FALSE;
