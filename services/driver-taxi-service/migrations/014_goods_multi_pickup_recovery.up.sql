-- Section 7: multi-pickup Taxi Goods execution and Taxi recovery cases.
-- Additive; single-pickup missions are unchanged.
--
-- Outbox routing is NOT redefined here: taxi_outbox.target already names the
-- consumer that owns a row, so customer movement rows are routed by the same
-- field the Vendor lifecycle and Fleet completed-work rows use.

-- ─── Goods mission product and plan binding ────────────────────────────────
ALTER TABLE taxi_goods_missions
  ADD COLUMN IF NOT EXISTS service_product_code TEXT,
  ADD COLUMN IF NOT EXISTS service_product_version BIGINT,
  ADD COLUMN IF NOT EXISTS eligibility_policy_version TEXT,
  ADD COLUMN IF NOT EXISTS fulfilment_group_ref TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_plan_ref TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS route_version BIGINT NOT NULL DEFAULT 1 CHECK (route_version >= 1),
  ADD COLUMN IF NOT EXISTS pickup_count INT NOT NULL DEFAULT 1 CHECK (pickup_count >= 1),
  ADD COLUMN IF NOT EXISTS not_before TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS pickup_window_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS pickup_window_end TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS custody_mode TEXT NOT NULL DEFAULT 'LEGACY_SINGLE' CHECK (custody_mode IN ('LEGACY_SINGLE','MULTI_MANIFEST')),
  ADD COLUMN IF NOT EXISTS movement_version BIGINT NOT NULL DEFAULT 1 CHECK (movement_version >= 1),
  ADD COLUMN IF NOT EXISTS provider_replacement_count INT NOT NULL DEFAULT 0 CHECK (provider_replacement_count >= 0),
  ADD COLUMN IF NOT EXISTS recovery_state TEXT NOT NULL DEFAULT '';

-- Multi-pickup missions are committed against an approved fulfilment plan,
-- not a Taxi quote: Taxi never prices them. Single-pickup missions still
-- require their quote.
ALTER TABLE taxi_goods_missions ALTER COLUMN quote_id DROP NOT NULL;
ALTER TABLE taxi_goods_missions ADD CONSTRAINT taxi_goods_missions_quote_or_plan
  CHECK (quote_id IS NOT NULL OR (pickup_count > 1 AND plan_ref <> '' AND plan_version >= 1));
ALTER TABLE taxi_goods_parcels ADD COLUMN IF NOT EXISTS pickup_stop_ref TEXT NOT NULL DEFAULT '';

-- One physical execution per plan slot: a replayed or stale commit can never
-- create a second mission for the same slot of the same plan version.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_goods_plan_slot
  ON taxi_goods_missions(source_service, plan_ref, plan_version, mission_slot_ref)
  WHERE plan_ref <> '' AND mission_slot_ref <> '' AND status NOT IN ('CANCELLED');

-- ─── Ordered stops with per-stop lifecycle ─────────────────────────────────
CREATE TABLE taxi_goods_mission_stops (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
  sequence INT NOT NULL CHECK (sequence >= 1),
  stop_type TEXT NOT NULL CHECK (stop_type IN ('PICKUP','DROPOFF')),
  external_stop_ref TEXT NOT NULL,
  vendor_ref TEXT NOT NULL DEFAULT '',
  vendor_branch_ref TEXT NOT NULL DEFAULT '',
  latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
  longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
  address_snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  readiness_window_start TIMESTAMPTZ,
  readiness_window_end TIMESTAMPTZ,
  service_time_seconds INT CHECK (service_time_seconds IS NULL OR service_time_seconds >= 0),
  secure_handover_required BOOLEAN NOT NULL DEFAULT TRUE,
  handover_policy_version TEXT NOT NULL DEFAULT '',
  handover_challenge_ref TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED','EN_ROUTE','ARRIVED','HANDOVER_PENDING','COLLECTED','DELIVERED','DELAYED','FAILED','CANCELLED_BY_REPLAN')),
  arrived_at TIMESTAMPTZ,
  handover_verified_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (mission_id, sequence),
  UNIQUE (mission_id, external_stop_ref),
  CHECK (readiness_window_start IS NULL OR readiness_window_end IS NULL OR readiness_window_start <= readiness_window_end),
  CHECK (status <> 'COLLECTED' OR secure_handover_required = FALSE OR handover_verified_at IS NOT NULL),
  CHECK (stop_type = 'PICKUP' OR status <> 'COLLECTED'),
  CHECK (stop_type = 'DROPOFF' OR status <> 'DELIVERED')
);
CREATE INDEX idx_taxi_goods_mission_stops_open ON taxi_goods_mission_stops(mission_id, sequence) WHERE status NOT IN ('COLLECTED','DELIVERED','CANCELLED_BY_REPLAN');

-- Ordered route history: re-sequencing is versioned, never destructive.
CREATE TABLE taxi_goods_route_versions (
  mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
  route_version BIGINT NOT NULL CHECK (route_version >= 1),
  stop_order TEXT[] NOT NULL CHECK (cardinality(stop_order) >= 2),
  reason_code TEXT NOT NULL,
  operation_ref TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (mission_id, route_version),
  UNIQUE (mission_id, operation_ref)
);

-- Aggregate (stop_id NULL) and per-stop cargo requirements.
CREATE TABLE taxi_goods_cargo_requirements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
  stop_id UUID REFERENCES taxi_goods_mission_stops(id) ON DELETE CASCADE,
  weight_grams BIGINT NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
  volume_ml BIGINT NOT NULL DEFAULT 0 CHECK (volume_ml >= 0),
  max_length_mm BIGINT NOT NULL DEFAULT 0 CHECK (max_length_mm >= 0),
  max_width_mm BIGINT NOT NULL DEFAULT 0 CHECK (max_width_mm >= 0),
  max_height_mm BIGINT NOT NULL DEFAULT 0 CHECK (max_height_mm >= 0),
  package_count BIGINT NOT NULL CHECK (package_count >= 1),
  fragile BOOLEAN NOT NULL DEFAULT FALSE,
  upright_required BOOLEAN NOT NULL DEFAULT FALSE,
  weather_protection_required BOOLEAN NOT NULL DEFAULT FALSE,
  temperature_class TEXT NOT NULL DEFAULT 'AMBIENT' CHECK (temperature_class IN ('AMBIENT','HOT','CHILLED','FROZEN')),
  temperature_min_c NUMERIC(5,2),
  temperature_max_c NUMERIC(5,2),
  secure_cargo_required BOOLEAN NOT NULL DEFAULT FALSE,
  handling_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_taxi_goods_cargo_aggregate ON taxi_goods_cargo_requirements(mission_id) WHERE stop_id IS NULL;
CREATE UNIQUE INDEX uq_taxi_goods_cargo_stop ON taxi_goods_cargo_requirements(mission_id, stop_id) WHERE stop_id IS NOT NULL;

-- Each pickup stop owns its own handover challenge series.
ALTER TABLE taxi_goods_handover_challenges ADD COLUMN IF NOT EXISTS stop_ref TEXT NOT NULL DEFAULT '';

-- ─── Recovery cases (passenger bookings and goods missions) ────────────────
CREATE TABLE taxi_recovery_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type TEXT NOT NULL CHECK (subject_type IN ('BOOKING','GOODS_MISSION')),
  booking_id UUID REFERENCES taxi_bookings(id),
  goods_mission_id UUID REFERENCES taxi_goods_missions(id),
  trigger_code TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('DETECTED','EVALUATING','REPLACEMENT_SEARCH','TRANSFER_REQUIRED','IN_PROGRESS','RESOLVED','OPS_REQUIRED','FAILED')),
  original_provider_id UUID REFERENCES taxi_providers(id),
  original_vehicle_id UUID REFERENCES vehicles(id),
  replacement_provider_id UUID REFERENCES taxi_providers(id),
  replacement_vehicle_id UUID REFERENCES vehicles(id),
  custody_transfer_required BOOLEAN NOT NULL DEFAULT FALSE,
  custody_transfer_ref TEXT,
  operation_ref TEXT NOT NULL UNIQUE,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  audit_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(audit_json) = 'array'),
  CHECK ((subject_type = 'BOOKING') = (booking_id IS NOT NULL) AND (subject_type = 'GOODS_MISSION') = (goods_mission_id IS NOT NULL)),
  CHECK (replacement_provider_id IS NULL OR replacement_provider_id IS DISTINCT FROM original_provider_id),
  CHECK ((state IN ('RESOLVED','FAILED')) = (resolved_at IS NOT NULL))
);
-- At most one open recovery per subject.
CREATE UNIQUE INDEX uq_taxi_recovery_open_booking ON taxi_recovery_cases(booking_id) WHERE booking_id IS NOT NULL AND state NOT IN ('RESOLVED','FAILED');
CREATE UNIQUE INDEX uq_taxi_recovery_open_goods ON taxi_recovery_cases(goods_mission_id) WHERE goods_mission_id IS NOT NULL AND state NOT IN ('RESOLVED','FAILED');
