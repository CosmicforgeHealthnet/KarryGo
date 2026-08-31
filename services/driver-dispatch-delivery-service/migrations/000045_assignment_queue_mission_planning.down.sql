DROP TABLE IF EXISTS dispatch_active_custody_manifest_parcels;
DROP TABLE IF EXISTS dispatch_stop_expected_parcels;
DROP INDEX IF EXISTS dispatch_manifest_one_active_per_mission;
CREATE OR REPLACE FUNCTION dispatch_reject_manifest_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'parcel manifests are immutable; create a new manifest version'; END; $$;
ALTER TABLE dispatch_parcel_manifests DROP COLUMN IF EXISTS policy_version, DROP COLUMN IF EXISTS superseded_by, DROP COLUMN IF EXISTS status;
DROP INDEX IF EXISTS dispatch_mission_stop_ref_unique;
ALTER TABLE dispatch_mission_stops DROP COLUMN IF EXISTS readiness_to, DROP COLUMN IF EXISTS readiness_from, DROP COLUMN IF EXISTS stop_ref;
ALTER TABLE dispatch_delivery_missions DROP COLUMN IF EXISTS capacity_snapshot, DROP COLUMN IF EXISTS vehicle_class, DROP COLUMN IF EXISTS plan_mission_index, DROP COLUMN IF EXISTS plan_id;
DROP TABLE IF EXISTS dispatch_mission_plans;
DROP TABLE IF EXISTS dispatch_queue_operations;
DROP TABLE IF EXISTS dispatch_queue_entries;
DROP TABLE IF EXISTS dispatch_assignment_offers;
DROP TABLE IF EXISTS dispatch_assignment_candidates;
DROP TABLE IF EXISTS dispatch_assignment_policies;
