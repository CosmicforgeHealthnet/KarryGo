DROP INDEX IF EXISTS idx_hauling_commerce_missions_manifest;
DROP INDEX IF EXISTS idx_hauling_commerce_missions_plan;
DROP INDEX IF EXISTS uq_hauling_commerce_mission_operation_ref;

ALTER TABLE commerce_missions
  DROP CONSTRAINT IF EXISTS commerce_missions_status_check;

ALTER TABLE commerce_missions
  ADD CONSTRAINT commerce_missions_status_check
  CHECK (status IN ('accepted', 'refused'));

ALTER TABLE commerce_missions
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS source_version,
  DROP COLUMN IF EXISTS pickup_timezone,
  DROP COLUMN IF EXISTS pickup_window_end,
  DROP COLUMN IF EXISTS pickup_window_start,
  DROP COLUMN IF EXISTS handling_requirements,
  DROP COLUMN IF EXISTS manifest_version,
  DROP COLUMN IF EXISTS manifest_ref,
  DROP COLUMN IF EXISTS parcels,
  DROP COLUMN IF EXISTS pickup_stops,
  DROP COLUMN IF EXISTS vehicle_class,
  DROP COLUMN IF EXISTS mission_slot_ref,
  DROP COLUMN IF EXISTS plan_fingerprint,
  DROP COLUMN IF EXISTS plan_version,
  DROP COLUMN IF EXISTS plan_ref,
  DROP COLUMN IF EXISTS request_fingerprint,
  DROP COLUMN IF EXISTS operation_ref;
