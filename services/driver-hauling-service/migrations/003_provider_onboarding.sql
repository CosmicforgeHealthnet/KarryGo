ALTER TABLE truck_providers
  ADD COLUMN IF NOT EXISTS location_state              TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS location_city               TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS operation_mode              TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS service_type                TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS gov_id_url                  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS driver_license_url          TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS vehicle_reg_url             TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS guarantor_name              TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS guarantor_phone             TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS emergency_contact_name      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS emergency_contact_phone     TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS emergency_contact_relationship TEXT NOT NULL DEFAULT '';
