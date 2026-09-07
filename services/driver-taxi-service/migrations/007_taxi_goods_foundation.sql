-- Section 29: distinct Vendor Commerce goods execution. Passenger bookings
-- and the direct-customer ride_purpose=parcel compatibility tables are not
-- reused by this aggregate.

CREATE TABLE taxi_goods_provider_settings (
    provider_id UUID PRIMARY KEY REFERENCES taxi_providers(id) ON DELETE CASCADE,
    capability_status TEXT NOT NULL DEFAULT 'NOT_APPROVED'
        CHECK (capability_status IN ('NOT_APPROVED','APPROVED','SUSPENDED')),
    goods_available BOOLEAN NOT NULL DEFAULT FALSE,
    approved_at TIMESTAMPTZ,
    approved_by TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE taxi_vehicle_cargo_profiles (
    vehicle_id UUID PRIMARY KEY REFERENCES vehicles(id) ON DELETE CASCADE,
    max_weight_grams BIGINT NOT NULL CHECK (max_weight_grams BETWEEN 0 AND 1000000),
    max_volume_ml BIGINT NOT NULL CHECK (max_volume_ml BETWEEN 0 AND 10000000),
    max_package_count INT NOT NULL CHECK (max_package_count BETWEEN 0 AND 100),
    longest_dimension_mm INT NOT NULL CHECK (longest_dimension_mm BETWEEN 0 AND 5000),
    handling_capabilities TEXT[] NOT NULL DEFAULT '{}',
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    verified_by TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (handling_capabilities <@ ARRAY['FRAGILE_CARRY','UPRIGHT_CARRY','LEAK_CONTAINMENT','INSULATED','HOT_HOLD','COLD_CHAIN','FROZEN_CHAIN']::TEXT[])
);

CREATE TABLE taxi_goods_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_service TEXT NOT NULL,
    quote_reference TEXT NOT NULL,
    revision BIGINT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    request_payload JSONB NOT NULL,
    serviceable BOOLEAN NOT NULL,
    reason_code TEXT NOT NULL,
    market_id UUID REFERENCES markets(id),
    operating_zone_id UUID REFERENCES operating_zones(id),
    distance_km NUMERIC(12,3) NOT NULL DEFAULT 0,
    estimated_minutes INT NOT NULL DEFAULT 0,
    fee_minor BIGINT NOT NULL DEFAULT 0 CHECK (fee_minor >= 0),
    currency CHAR(3) NOT NULL,
    available_providers INT NOT NULL DEFAULT 0 CHECK (available_providers >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_service, quote_reference, revision),
    UNIQUE (source_service, quote_reference, request_fingerprint)
);

CREATE TABLE taxi_goods_missions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_service TEXT NOT NULL,
    mission_ref TEXT NOT NULL,
    operation_ref TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    parent_order_ref TEXT NOT NULL,
    vendor_order_refs TEXT[] NOT NULL,
    customer_ref TEXT NOT NULL,
    quote_id UUID NOT NULL REFERENCES taxi_goods_quotes(id),
    plan_ref TEXT NOT NULL DEFAULT '',
    plan_version BIGINT NOT NULL DEFAULT 0,
    plan_fingerprint TEXT NOT NULL DEFAULT '',
    mission_slot_ref TEXT NOT NULL DEFAULT '',
    provider_id UUID REFERENCES taxi_providers(id),
    vehicle_id UUID REFERENCES vehicles(id),
    pickup JSONB NOT NULL,
    dropoff JSONB NOT NULL,
    receiver_ref TEXT NOT NULL DEFAULT '',
    receiver_name TEXT NOT NULL DEFAULT '',
    receiver_phone TEXT NOT NULL DEFAULT '',
    load JSONB NOT NULL,
    handling TEXT[] NOT NULL DEFAULT '{}',
    manifest_ref TEXT NOT NULL DEFAULT '',
    manifest_version BIGINT NOT NULL DEFAULT 0,
    manifest_frozen_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'MATCHING' CHECK (status IN (
        'MATCHING','OFFERED','ASSIGNED','EN_ROUTE_PICKUP','AT_PICKUP',
        'PICKUP_HANDOVER_REQUIRED','PICKED_UP','IN_TRANSIT','AT_DROPOFF',
        'DELIVERY_HANDOVER_REQUIRED','DELIVERED','COMPLETED','CANCELLED','UNMATCHED','FAILED')),
    version BIGINT NOT NULL DEFAULT 1,
    agreed_fee_minor BIGINT NOT NULL CHECK (agreed_fee_minor >= 0),
    currency CHAR(3) NOT NULL,
    pickup_evidence_required BOOLEAN NOT NULL DEFAULT FALSE,
    delivery_evidence_required BOOLEAN NOT NULL DEFAULT TRUE,
    pickup_evidence_ref TEXT NOT NULL DEFAULT '',
    delivery_evidence_ref TEXT NOT NULL DEFAULT '',
    settlement_status TEXT NOT NULL DEFAULT 'NOT_READY' CHECK (settlement_status IN ('NOT_READY','PENDING','CONFIRMED','FAILED')),
    settlement_attempts INT NOT NULL DEFAULT 0,
    settlement_next_attempt_at TIMESTAMPTZ,
    cancel_reason TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL,
    offered_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    picked_up_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_service, mission_ref),
    UNIQUE (source_service, operation_ref)
);

CREATE TABLE taxi_goods_parcels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
    parcel_ref TEXT NOT NULL,
    vendor_ref TEXT NOT NULL DEFAULT '',
    branch_ref TEXT NOT NULL DEFAULT '',
    vendor_order_ref TEXT NOT NULL DEFAULT '',
    manifest_ref TEXT NOT NULL,
    manifest_version BIGINT NOT NULL CHECK (manifest_version >= 1),
    weight_grams BIGINT NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
    length_mm BIGINT NOT NULL DEFAULT 0 CHECK (length_mm >= 0),
    width_mm BIGINT NOT NULL DEFAULT 0 CHECK (width_mm >= 0),
    height_mm BIGINT NOT NULL DEFAULT 0 CHECK (height_mm >= 0),
    package_count BIGINT NOT NULL DEFAULT 1 CHECK (package_count > 0),
    handling TEXT[] NOT NULL DEFAULT '{}',
    custody_status TEXT NOT NULL DEFAULT 'PLANNED' CHECK (custody_status IN ('PLANNED','PICKED_UP','DELIVERED','RETURNED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (mission_id, parcel_ref)
);

ALTER TABLE provider_operational_state ADD COLUMN goods_mission_id UUID REFERENCES taxi_goods_missions(id);
ALTER TABLE provider_operational_state_history ADD COLUMN goods_mission_id UUID REFERENCES taxi_goods_missions(id);
ALTER TABLE provider_operational_state ADD CONSTRAINT provider_one_execution_ref CHECK (booking_id IS NULL OR goods_mission_id IS NULL);
ALTER TABLE provider_operational_state_history ADD CONSTRAINT provider_history_one_execution_ref CHECK (booking_id IS NULL OR goods_mission_id IS NULL);

CREATE INDEX idx_taxi_goods_quotes_reference ON taxi_goods_quotes(source_service, quote_reference, revision DESC);
CREATE INDEX idx_taxi_goods_missions_provider_status ON taxi_goods_missions(provider_id, status);
CREATE INDEX idx_taxi_goods_missions_settlement ON taxi_goods_missions(settlement_next_attempt_at) WHERE settlement_status IN ('PENDING','FAILED');
CREATE INDEX idx_taxi_goods_parcels_manifest ON taxi_goods_parcels(mission_id, manifest_ref, manifest_version);

