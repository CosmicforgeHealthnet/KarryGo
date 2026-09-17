DROP TABLE IF EXISTS taxi_recovery_cases;
ALTER TABLE taxi_goods_handover_challenges DROP COLUMN IF EXISTS stop_ref;
DROP TABLE IF EXISTS taxi_goods_cargo_requirements;
DROP TABLE IF EXISTS taxi_goods_route_versions;
DROP TABLE IF EXISTS taxi_goods_mission_stops;
DROP INDEX IF EXISTS uq_taxi_goods_plan_slot;
ALTER TABLE taxi_goods_parcels DROP COLUMN IF EXISTS pickup_stop_ref;
ALTER TABLE taxi_goods_missions DROP CONSTRAINT IF EXISTS taxi_goods_missions_quote_or_plan;
DELETE FROM taxi_goods_missions WHERE quote_id IS NULL;
ALTER TABLE taxi_goods_missions ALTER COLUMN quote_id SET NOT NULL;
ALTER TABLE taxi_goods_missions
  DROP COLUMN IF EXISTS recovery_state, DROP COLUMN IF EXISTS provider_replacement_count, DROP COLUMN IF EXISTS movement_version,
  DROP COLUMN IF EXISTS custody_mode, DROP COLUMN IF EXISTS pickup_window_end, DROP COLUMN IF EXISTS pickup_window_start,
  DROP COLUMN IF EXISTS not_before, DROP COLUMN IF EXISTS pickup_count, DROP COLUMN IF EXISTS route_version,
  DROP COLUMN IF EXISTS route_plan_ref, DROP COLUMN IF EXISTS fulfilment_group_ref, DROP COLUMN IF EXISTS eligibility_policy_version,
  DROP COLUMN IF EXISTS service_product_version, DROP COLUMN IF EXISTS service_product_code;
DROP INDEX IF EXISTS idx_taxi_outbox_ready;
CREATE INDEX IF NOT EXISTS idx_taxi_outbox_ready ON taxi_outbox(available_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;
