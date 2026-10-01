DROP INDEX IF EXISTS idx_taxi_match_candidates_shadow_disagreement;
ALTER TABLE taxi_match_candidates
  DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS selected, DROP COLUMN IF EXISTS safety_score,
  DROP COLUMN IF EXISTS cancellation_risk_score, DROP COLUMN IF EXISTS direction_score, DROP COLUMN IF EXISTS idle_fairness_score,
  DROP COLUMN IF EXISTS reliability_score, DROP COLUMN IF EXISTS legacy_decision_eligible, DROP COLUMN IF EXISTS evaluation_mode,
  DROP COLUMN IF EXISTS rejection_reason_code, DROP COLUMN IF EXISTS eligibility_result, DROP COLUMN IF EXISTS vehicle_id;
ALTER TABLE taxi_bookings
  DROP COLUMN IF EXISTS provider_replacement_count, DROP COLUMN IF EXISTS movement_version, DROP COLUMN IF EXISTS legacy_vehicle_class,
  DROP COLUMN IF EXISTS route_plan_ref, DROP COLUMN IF EXISTS requirement_snapshot_json, DROP COLUMN IF EXISTS guarantee_profile_version,
  DROP COLUMN IF EXISTS eligibility_policy_version, DROP COLUMN IF EXISTS service_product_version, DROP COLUMN IF EXISTS service_product_code;
DROP TABLE IF EXISTS taxi_certification_events;
DROP TRIGGER IF EXISTS taxi_policy_snapshots_immutable ON taxi_policy_snapshots;
DROP FUNCTION IF EXISTS reject_taxi_policy_snapshot_mutation();
DROP TABLE IF EXISTS taxi_policy_snapshots;
DROP TABLE IF EXISTS taxi_vehicle_product_eligibility;
DROP TABLE IF EXISTS taxi_vehicle_capabilities;
ALTER TABLE vehicles DROP CONSTRAINT IF EXISTS vehicles_passenger_capacity_below_seats;
ALTER TABLE vehicles
  DROP COLUMN IF EXISTS capability_updated_at, DROP COLUMN IF EXISTS capability_version,
  DROP COLUMN IF EXISTS passenger_capacity, DROP COLUMN IF EXISTS total_seats;
