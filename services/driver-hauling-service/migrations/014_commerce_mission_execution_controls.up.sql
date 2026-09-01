-- Commerce hauling mission execution controls.
--
-- Section 30 extends the existing commerce mission seam so Vendor Service can
-- hand Hauling the exact committed fulfilment plan: operation ref,
-- multi-pickup stops, Parcel/manifest evidence, scheduled pickup windows and a
-- request fingerprint. Hauling still owns the canonical booking/matching
-- lifecycle and still executes for the already-approved commerce fee.

ALTER TABLE commerce_missions
  ADD COLUMN IF NOT EXISTS operation_ref TEXT,
  ADD COLUMN IF NOT EXISTS request_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS plan_ref TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS plan_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS plan_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS mission_slot_ref TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS vehicle_class TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS pickup_stops JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS parcels JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS manifest_ref TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS manifest_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS handling_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS pickup_window_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS pickup_window_end TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS pickup_timezone TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE commerce_missions
   SET operation_ref = mission_ref
 WHERE operation_ref IS NULL OR length(btrim(operation_ref)) = 0;

ALTER TABLE commerce_missions
  ALTER COLUMN operation_ref SET NOT NULL;

ALTER TABLE commerce_missions
  DROP CONSTRAINT IF EXISTS commerce_missions_status_check;

ALTER TABLE commerce_missions
  ADD CONSTRAINT commerce_missions_status_check
  CHECK (status IN ('accepted', 'refused', 'cancelled'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_commerce_mission_operation_ref
  ON commerce_missions (operation_ref);

CREATE INDEX IF NOT EXISTS idx_hauling_commerce_missions_plan
  ON commerce_missions (plan_ref, plan_version);

CREATE INDEX IF NOT EXISTS idx_hauling_commerce_missions_manifest
  ON commerce_missions (manifest_ref, manifest_version);

COMMENT ON COLUMN commerce_missions.request_fingerprint IS
'Stable semantic fingerprint for create idempotency; same operation_ref with different payload conflicts.';
