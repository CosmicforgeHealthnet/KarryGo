DROP TRIGGER IF EXISTS commerce_mission_stop_manifest_frozen ON commerce_mission_stops;
DROP FUNCTION IF EXISTS enforce_frozen_commerce_stop_manifest();
DROP TRIGGER IF EXISTS commerce_mission_parcel_manifest_frozen ON commerce_mission_parcels;
DROP FUNCTION IF EXISTS enforce_frozen_commerce_manifest();
DROP TRIGGER IF EXISTS commerce_mission_evidence_immutable ON commerce_mission_evidence;
DROP FUNCTION IF EXISTS reject_commerce_evidence_mutation();
DROP TRIGGER IF EXISTS commerce_mission_carrier_history_append_only ON commerce_mission_carrier_history;
DROP FUNCTION IF EXISTS enforce_commerce_carrier_history();
DROP TABLE IF EXISTS commerce_mission_carrier_history;
DROP TABLE IF EXISTS commerce_provider_transfers;
DROP TABLE IF EXISTS commerce_return_operations;
DROP TABLE IF EXISTS commerce_delivery_exceptions;
DROP TABLE IF EXISTS commerce_mission_evidence;
ALTER TABLE commerce_missions
    DROP CONSTRAINT IF EXISTS commerce_missions_replacement_state_chk,
    DROP COLUMN IF EXISTS receiver_challenge_generation,
    DROP COLUMN IF EXISTS replacement_reason,
    DROP COLUMN IF EXISTS replacement_trip_id,
    DROP COLUMN IF EXISTS replacement_provider_id,
    DROP COLUMN IF EXISTS replacement_state;
ALTER TABLE commerce_mission_parcels
    DROP COLUMN IF EXISTS manifest_frozen_at,
    DROP COLUMN IF EXISTS manifest_created_at,
    DROP COLUMN IF EXISTS manifest_version;
ALTER TABLE commerce_mission_stops
    DROP COLUMN IF EXISTS manifest_frozen_at,
    DROP COLUMN IF EXISTS manifest_version;
