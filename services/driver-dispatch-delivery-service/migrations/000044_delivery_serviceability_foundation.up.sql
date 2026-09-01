-- Additive Sections 4-6 Dispatch domain and serviceability foundation.
-- The existing dispatch_bookings -> delivery_requests -> trips execution chain
-- remains authoritative and no existing row is backfilled or made dependent on
-- these tables.

CREATE TABLE IF NOT EXISTS dispatch_delivery_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NULL,
    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    source_version BIGINT NOT NULL DEFAULT 1 CHECK(source_version > 0),
    booking_id UUID NULL REFERENCES dispatch_bookings(id) ON DELETE SET NULL,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE SET NULL,
    partner_id UUID NULL,
    organization_id UUID NULL,
    state TEXT NOT NULL DEFAULT 'DRAFT',
    version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK(source_type IN ('CUSTOMER','PARTNER_API','VENDOR_COMMERCE','FLEET_PRIVATE','ADMIN_ASSISTED')),
    CHECK(state IN ('DRAFT','SERVICEABILITY_CHECK','QUOTED','PAYMENT_PENDING','READY_FOR_DISPATCH','ACTIVE','PARTIALLY_DELIVERED','DELIVERED','SETTLEMENT_PENDING','COMPLETED','CANCELLED','FAILED','RETURNING','RETURNED','UNDER_REVIEW')),
    UNIQUE(source_type, source_ref, source_version)
);
CREATE INDEX IF NOT EXISTS idx_dispatch_delivery_groups_customer ON dispatch_delivery_groups(customer_id, created_at DESC);

CREATE TABLE IF NOT EXISTS dispatch_delivery_missions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_group_id UUID NOT NULL REFERENCES dispatch_delivery_groups(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    source_version BIGINT NOT NULL DEFAULT 1 CHECK(source_version > 0),
    booking_id UUID NULL REFERENCES dispatch_bookings(id) ON DELETE SET NULL,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE SET NULL,
    state TEXT NOT NULL DEFAULT 'PLANNED',
    version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK(source_type IN ('CUSTOMER','PARTNER_API','VENDOR_COMMERCE','FLEET_PRIVATE','ADMIN_ASSISTED')),
    CHECK(state IN ('PLANNED','QUEUED','OFFERING','ASSIGNED','EN_ROUTE_PICKUP','AT_PICKUP','PICKUP_VERIFICATION','PICKED_UP','IN_TRANSIT','AT_DELIVERY','DELIVERY_VERIFICATION','DELIVERED','COMPLETED','FAILED','REASSIGNING','RETURNING','RETURNED','HUB_HOLD','CANCELLED')),
    UNIQUE(delivery_group_id, source_ref, source_version)
);

CREATE TABLE IF NOT EXISTS dispatch_mission_stops (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK(sequence > 0),
    stop_type TEXT NOT NULL CHECK(stop_type IN ('PICKUP','PROVIDER_TRANSFER','DELIVERY','HUB_DROP','HUB_PICKUP','RETURN')),
    location_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(mission_id, sequence)
);

CREATE TABLE IF NOT EXISTS dispatch_parcels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_group_id UUID NOT NULL REFERENCES dispatch_delivery_groups(id) ON DELETE CASCADE,
    parcel_ref TEXT NOT NULL,
    declaration JSONB NOT NULL DEFAULT '{}'::jsonb,
    admission_result TEXT NOT NULL DEFAULT 'ACCEPTED',
    admission_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    package_policy_version TEXT NOT NULL,
    measured_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    mismatch_classification TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'DECLARED',
    version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK(admission_result IN ('ACCEPTED','ACCEPTED_WITH_CONDITIONS','REQUIRES_REPACK','REQUIRES_REPRICE','REFUSED_PROHIBITED','REFUSED_UNSAFE','OPERATIONAL_REVIEW')),
    CHECK(state IN ('DECLARED','READY_FOR_PICKUP','PICKUP_VERIFIED','IN_CUSTODY','IN_TRANSIT','DELIVERY_VERIFIED','DELIVERED','DISCREPANCY','REFUSED','DAMAGED','MISSING','RETURNING','RETURNED','HUB_HOLD','CANCELLED')),
    UNIQUE(delivery_group_id, parcel_ref)
);

CREATE TABLE IF NOT EXISTS dispatch_parcel_manifests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    manifest_ref TEXT NOT NULL,
    version BIGINT NOT NULL CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(mission_id, manifest_ref, version)
);

CREATE TABLE IF NOT EXISTS dispatch_manifest_parcels (
    manifest_id UUID NOT NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    parcel_id UUID NOT NULL REFERENCES dispatch_parcels(id) ON DELETE RESTRICT,
    ordinal INTEGER NOT NULL CHECK(ordinal > 0),
    PRIMARY KEY(manifest_id, parcel_id),
    UNIQUE(manifest_id, ordinal)
);

CREATE OR REPLACE FUNCTION dispatch_reject_manifest_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'parcel manifests are immutable; create a new manifest version';
END;
$$;
DROP TRIGGER IF EXISTS dispatch_parcel_manifest_immutable ON dispatch_parcel_manifests;
CREATE TRIGGER dispatch_parcel_manifest_immutable
    BEFORE UPDATE OR DELETE ON dispatch_parcel_manifests
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_manifest_mutation();
DROP TRIGGER IF EXISTS dispatch_manifest_membership_immutable ON dispatch_manifest_parcels;
CREATE TRIGGER dispatch_manifest_membership_immutable
    BEFORE UPDATE OR DELETE ON dispatch_manifest_parcels
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_manifest_mutation();

CREATE TABLE IF NOT EXISTS dispatch_serviceability_quotes (
    id TEXT PRIMARY KEY,
    customer_id UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    source_type TEXT NOT NULL DEFAULT 'CUSTOMER',
    source_ref TEXT NOT NULL DEFAULT '',
    source_version BIGINT NOT NULL DEFAULT 1 CHECK(source_version > 0),
    pickup_latitude DOUBLE PRECISION NOT NULL CHECK(pickup_latitude BETWEEN -90 AND 90),
    pickup_longitude DOUBLE PRECISION NOT NULL CHECK(pickup_longitude BETWEEN -180 AND 180),
    dropoff_latitude DOUBLE PRECISION NOT NULL CHECK(dropoff_latitude BETWEEN -90 AND 90),
    dropoff_longitude DOUBLE PRECISION NOT NULL CHECK(dropoff_longitude BETWEEN -180 AND 180),
    vehicle_class TEXT NOT NULL,
    decision TEXT NOT NULL CHECK(decision IN ('SERVICEABLE','SERVICEABLE_WITH_CONDITIONS','OUTSIDE_SERVICE_AREA','OPERATIONAL_REVIEW')),
    reason_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    policy_version TEXT NOT NULL,
    route_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    package_declaration JSONB NOT NULL,
    package_admission JSONB NOT NULL,
    pricing_inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
    fare_estimate_kobo BIGINT NOT NULL DEFAULT 0 CHECK(fare_estimate_kobo >= 0),
    currency TEXT NOT NULL DEFAULT '',
    degraded BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(customer_id, idempotency_key),
    CHECK(expires_at > created_at),
    CHECK(source_type = 'CUSTOMER'),
    CHECK((decision IN ('SERVICEABLE','SERVICEABLE_WITH_CONDITIONS') AND fare_estimate_kobo > 0 AND currency='NGN') OR (decision NOT IN ('SERVICEABLE','SERVICEABLE_WITH_CONDITIONS') AND fare_estimate_kobo=0))
);
CREATE INDEX IF NOT EXISTS idx_dispatch_serviceability_customer ON dispatch_serviceability_quotes(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dispatch_serviceability_expiry ON dispatch_serviceability_quotes(expires_at);

COMMENT ON TABLE dispatch_delivery_missions IS 'Additive mission semantics linked to, never replacing, the canonical Dispatch trip.';
COMMENT ON TABLE dispatch_serviceability_quotes IS 'Non-financial route/package admission snapshots; creates no booking, wallet hold, match or trip.';
