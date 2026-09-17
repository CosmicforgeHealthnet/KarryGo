-- Section 7: service-product and capability-driven Taxi. Additive only:
-- legacy seats/vehicle_class stay in place and keep driving matching until
-- TAXI_CAPABILITY_MATCHING_ENABLED is switched on.

-- ─── Unambiguous seat semantics ────────────────────────────────────────────
-- total_seats counts every physical seat; passenger_capacity excludes the
-- driver. Both start NULL (unknown): legacy "seats = 4" is not evidence.
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS total_seats INT CHECK (total_seats IS NULL OR total_seats BETWEEN 1 AND 60);
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS passenger_capacity INT CHECK (passenger_capacity IS NULL OR passenger_capacity BETWEEN 1 AND 59);
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS capability_version BIGINT NOT NULL DEFAULT 0 CHECK (capability_version >= 0);
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS capability_updated_at TIMESTAMPTZ;
ALTER TABLE vehicles ADD CONSTRAINT vehicles_passenger_capacity_below_seats
  CHECK (total_seats IS NULL OR passenger_capacity IS NULL OR passenger_capacity < total_seats);

-- ─── Normalized vehicle capability ─────────────────────────────────────────
-- NULL means unknown, never false. certification-sensitive fields change
-- eligibility back to PENDING when a provider edits them.
CREATE TABLE taxi_vehicle_capabilities (
  vehicle_id UUID PRIMARY KEY REFERENCES vehicles(id) ON DELETE CASCADE,
  body_style TEXT CHECK (body_style IS NULL OR body_style IN ('SEDAN','HATCHBACK','SUV','MINIVAN','VAN','PICKUP','WAGON','OTHER')),
  door_count INT CHECK (door_count IS NULL OR door_count BETWEEN 2 AND 6),
  ac_status TEXT CHECK (ac_status IS NULL OR ac_status IN ('WORKING','NOT_WORKING','NONE')),
  luggage_class TEXT CHECK (luggage_class IS NULL OR luggage_class IN ('NONE','SMALL','STANDARD','LARGE')),
  luggage_volume_ml BIGINT CHECK (luggage_volume_ml IS NULL OR luggage_volume_ml >= 0),
  wheelchair_accessible BOOLEAN,
  child_seat_capable BOOLEAN,
  pet_eligible BOOLEAN,
  goods_max_weight_grams BIGINT CHECK (goods_max_weight_grams IS NULL OR goods_max_weight_grams BETWEEN 0 AND 2000000),
  goods_max_volume_ml BIGINT CHECK (goods_max_volume_ml IS NULL OR goods_max_volume_ml BETWEEN 0 AND 20000000),
  goods_max_length_mm BIGINT CHECK (goods_max_length_mm IS NULL OR goods_max_length_mm BETWEEN 0 AND 6000),
  goods_max_width_mm BIGINT CHECK (goods_max_width_mm IS NULL OR goods_max_width_mm BETWEEN 0 AND 3000),
  goods_max_height_mm BIGINT CHECK (goods_max_height_mm IS NULL OR goods_max_height_mm BETWEEN 0 AND 3000),
  goods_max_package_count BIGINT CHECK (goods_max_package_count IS NULL OR goods_max_package_count BETWEEN 0 AND 500),
  upright_safe BOOLEAN,
  fragile_safe BOOLEAN,
  weather_protected BOOLEAN,
  temperature_controlled BOOLEAN,
  temperature_min_c NUMERIC(5,2),
  temperature_max_c NUMERIC(5,2),
  secure_cargo_area BOOLEAN,
  cargo_separation_supported BOOLEAN,
  observed_at TIMESTAMPTZ,
  verified_at TIMESTAMPTZ,
  verified_by TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (temperature_min_c IS NULL OR temperature_max_c IS NULL OR temperature_min_c <= temperature_max_c)
);

-- Verified goods facts already certified on cargo profiles carry over; nothing
-- unverified is promoted.
INSERT INTO taxi_vehicle_capabilities (vehicle_id, goods_max_weight_grams, goods_max_volume_ml, goods_max_length_mm,
  goods_max_package_count, fragile_safe, upright_safe, verified_at, verified_by, observed_at)
SELECT vehicle_id, max_weight_grams, max_volume_ml, longest_dimension_mm, max_package_count,
       'FRAGILE_CARRY' = ANY(handling_capabilities), 'UPRIGHT_CARRY' = ANY(handling_capabilities),
       verified_at, verified_by, updated_at
  FROM taxi_vehicle_cargo_profiles WHERE verified
ON CONFLICT (vehicle_id) DO NOTHING;

CREATE TABLE taxi_vehicle_product_eligibility (
  vehicle_id UUID NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  product_code TEXT NOT NULL CHECK (product_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
  status TEXT NOT NULL CHECK (status IN ('PENDING','ELIGIBLE','SUSPENDED','EXPIRED','REJECTED')),
  policy_version TEXT NOT NULL,
  certification_ref TEXT NOT NULL DEFAULT '',
  effective_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  reason_code TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (vehicle_id, product_code),
  CHECK (status <> 'ELIGIBLE' OR certification_ref <> '')
);
CREATE INDEX idx_taxi_vehicle_product_eligibility_product ON taxi_vehicle_product_eligibility(product_code, status);

-- ─── Locally applied immutable policy snapshots ────────────────────────────
CREATE TABLE taxi_policy_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_kind TEXT NOT NULL CHECK (policy_kind IN ('SERVICE_PRODUCTS')),
  policy_version TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  checksum TEXT NOT NULL,
  effective_at TIMESTAMPTZ NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (policy_kind, policy_version)
);
CREATE INDEX idx_taxi_policy_snapshots_effective ON taxi_policy_snapshots(policy_kind, effective_at DESC, applied_at DESC);

CREATE FUNCTION reject_taxi_policy_snapshot_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'taxi policy snapshots are immutable';
END $$;
CREATE TRIGGER taxi_policy_snapshots_immutable BEFORE UPDATE OR DELETE ON taxi_policy_snapshots
FOR EACH ROW EXECUTE FUNCTION reject_taxi_policy_snapshot_mutation();

-- Conservative bootstrap policy. Goods products permit a single pickup until
-- operations publishes a snapshot that raises max_pickups deliberately.
INSERT INTO taxi_policy_snapshots (policy_kind, policy_version, payload_json, checksum, effective_at)
VALUES ('SERVICE_PRODUCTS', 'taxi-bootstrap-1', '{
  "policy_version": "taxi-bootstrap-1",
  "effective_at": "2026-09-17T00:00:00Z",
  "products": [
    {"code":"TAXI_STANDARD","version":1,"purpose":"PASSENGER","label":"Standard","active":true,"max_passengers":4,"legacy_vehicle_class":"economy"},
    {"code":"TAXI_COMFORT","version":1,"purpose":"PASSENGER","label":"Comfort","active":true,"max_passengers":4,"legacy_vehicle_class":"comfort"},
    {"code":"TAXI_XL","version":1,"purpose":"PASSENGER","label":"XL","active":true,"min_passenger_capacity":5,"max_passengers":6,"legacy_vehicle_class":"xl"},
    {"code":"TAXI_GOODS_STANDARD","version":1,"purpose":"GOODS","label":"Goods","active":true,"max_pickups":1,"max_dropoffs":1,"secure_pickup_handover":true,"secure_delivery_handover":true},
    {"code":"TAXI_GOODS_LARGE","version":1,"purpose":"GOODS","label":"Goods Large","active":true,"max_pickups":1,"max_dropoffs":1,"secure_pickup_handover":true,"secure_delivery_handover":true}
  ],
  "certification_required_fields": ["passenger_capacity","total_seats","wheelchair_accessible","child_seat_capable","goods_max_weight_grams","goods_max_volume_ml","goods_max_length_mm","goods_max_width_mm","goods_max_height_mm","temperature_controlled","temperature_min_c","temperature_max_c","secure_cargo_area","cargo_separation_supported","fragile_safe","upright_safe"]
}'::jsonb, 'bootstrap', '2026-09-17T00:00:00Z');

-- Legacy compatibility: every active vehicle gets a PENDING eligibility row
-- for the product its legacy class maps to. Nothing becomes ELIGIBLE
-- without certification evidence.
INSERT INTO taxi_vehicle_product_eligibility (vehicle_id, product_code, status, policy_version, reason_code)
SELECT id, CASE vehicle_class WHEN 'economy' THEN 'TAXI_STANDARD' WHEN 'comfort' THEN 'TAXI_COMFORT' WHEN 'xl' THEN 'TAXI_XL' END,
       'PENDING', 'taxi-bootstrap-1', 'LEGACY_CLASS_UNVERIFIED'
  FROM vehicles WHERE vehicle_class IN ('economy','comfort','xl')
ON CONFLICT DO NOTHING;

-- Idempotent inbox for certification events from verification-compliance.
CREATE TABLE taxi_certification_events (
  event_id TEXT PRIMARY KEY,
  vehicle_id UUID NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  product_code TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('CERTIFIED','REVOKED','SUSPENDED','EXPIRED','REJECTED')),
  source_version BIGINT NOT NULL CHECK (source_version >= 1),
  certification_ref TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('APPLIED','STALE')),
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE taxi_vehicle_product_eligibility ADD COLUMN source_version BIGINT NOT NULL DEFAULT 0 CHECK (source_version >= 0);

-- ─── Booking product snapshot ──────────────────────────────────────────────
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS service_product_code TEXT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS service_product_version BIGINT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS eligibility_policy_version TEXT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS guarantee_profile_version TEXT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS requirement_snapshot_json JSONB CHECK (requirement_snapshot_json IS NULL OR jsonb_typeof(requirement_snapshot_json) = 'object');
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS route_plan_ref TEXT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS legacy_vehicle_class TEXT;
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS movement_version BIGINT NOT NULL DEFAULT 1 CHECK (movement_version >= 1);
ALTER TABLE taxi_bookings ADD COLUMN IF NOT EXISTS provider_replacement_count INT NOT NULL DEFAULT 0 CHECK (provider_replacement_count >= 0);

-- ─── Match decision evidence (extends the existing candidate audit) ───────
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS vehicle_id UUID REFERENCES vehicles(id);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS eligibility_result TEXT CHECK (eligibility_result IS NULL OR eligibility_result IN ('ELIGIBLE','REJECTED'));
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS rejection_reason_code TEXT;
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS evaluation_mode TEXT CHECK (evaluation_mode IS NULL OR evaluation_mode IN ('SHADOW','ENFORCED'));
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS legacy_decision_eligible BOOLEAN;
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS reliability_score NUMERIC(10,4);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS idle_fairness_score NUMERIC(10,4);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS direction_score NUMERIC(10,4);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS cancellation_risk_score NUMERIC(10,4);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS safety_score NUMERIC(10,4);
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS selected BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE taxi_match_candidates ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_taxi_match_candidates_shadow_disagreement
  ON taxi_match_candidates(created_at) WHERE evaluation_mode = 'SHADOW' AND legacy_decision_eligible IS DISTINCT FROM (eligibility_result = 'ELIGIBLE');
