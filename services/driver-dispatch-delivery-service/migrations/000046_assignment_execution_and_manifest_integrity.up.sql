-- Close the additive Sections 7-8 execution, queue, and manifest integrity
-- gaps. The canonical dispatch_bookings -> request_broadcasts -> trips chain
-- remains the only rider execution authority.

CREATE TABLE dispatch_mission_execution_links (
    mission_id UUID PRIMARY KEY REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    booking_id UUID NOT NULL UNIQUE REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,
    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'READY' CHECK (state IN ('READY','TRIP_CREATED','RECONCILIATION_REQUIRED')),
    trip_id UUID NULL UNIQUE REFERENCES trips(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_type, source_ref, source_version)
);
CREATE UNIQUE INDEX dispatch_delivery_mission_one_booking
    ON dispatch_delivery_missions(booking_id) WHERE booking_id IS NOT NULL;

ALTER TABLE dispatch_queue_entries
    ADD COLUMN customer_decision_deadline TIMESTAMPTZ NULL,
    ADD COLUMN strategy TEXT NOT NULL DEFAULT 'SEQUENTIAL_NEAREST' CHECK (strategy IN ('SEQUENTIAL_NEAREST','ORGANIZATION_DISPATCH','ADMIN_ASSISTED')),
    ADD COLUMN initial_radius_km NUMERIC(7,2) NOT NULL DEFAULT 10 CHECK (initial_radius_km > 0),
    ADD COLUMN current_radius_km NUMERIC(7,2) NOT NULL DEFAULT 10 CHECK (current_radius_km > 0),
    ADD COLUMN max_radius_km NUMERIC(7,2) NOT NULL DEFAULT 50 CHECK (max_radius_km >= current_radius_km),
    ADD COLUMN service_tier TEXT NOT NULL DEFAULT 'standard' CHECK (service_tier IN ('standard','express')),
    ADD COLUMN priority_reason TEXT NOT NULL DEFAULT 'NORMAL',
    ADD COLUMN priority_policy_version TEXT NOT NULL DEFAULT 'dispatch_queue_priority_v1',
    ADD COLUMN search_policy_version TEXT NOT NULL DEFAULT 'dispatch_queue_search_v1',
    ADD COLUMN serviceability_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN wallet_operation_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN wallet_state TEXT NOT NULL DEFAULT 'NOT_REQUIRED' CHECK (wallet_state IN ('NOT_REQUIRED','REQUESTED','ACKNOWLEDGED','UNKNOWN','FAILED')),
    ADD COLUMN wallet_error_code TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX dispatch_queue_wallet_operation_unique
    ON dispatch_queue_entries(wallet_operation_ref) WHERE wallet_operation_ref <> '';

CREATE TABLE dispatch_assignment_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    provider_id UUID NULL REFERENCES providers(id) ON DELETE RESTRICT,
    strategy TEXT NOT NULL CHECK (strategy IN ('ORGANIZATION_DISPATCH','ADMIN_ASSISTED')),
    actor_ref TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    safe_note TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION dispatch_reject_assignment_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'dispatch assignment audit is append-only';
END;
$$;
CREATE TRIGGER dispatch_assignment_audit_append_only
    BEFORE UPDATE OR DELETE ON dispatch_assignment_audit
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_assignment_audit_mutation();

ALTER TABLE dispatch_parcels
    ADD COLUMN karrygo_parcel_ref TEXT,
    ADD COLUMN scannable_code TEXT;
UPDATE dispatch_parcels
SET karrygo_parcel_ref='KGP-' || upper(replace(id::text,'-','')),
    scannable_code='kgp_scan-' || replace(id::text,'-','')
WHERE karrygo_parcel_ref IS NULL OR scannable_code IS NULL;
ALTER TABLE dispatch_parcels
    ALTER COLUMN karrygo_parcel_ref SET NOT NULL,
    ALTER COLUMN scannable_code SET NOT NULL;
CREATE UNIQUE INDEX dispatch_parcel_karrygo_ref_unique ON dispatch_parcels(karrygo_parcel_ref);
CREATE UNIQUE INDEX dispatch_parcel_scannable_unique ON dispatch_parcels(scannable_code);

ALTER TABLE dispatch_delivery_missions
    ADD COLUMN logical_mission_ref TEXT NOT NULL DEFAULT '';
UPDATE dispatch_delivery_missions SET logical_mission_ref=id::text WHERE logical_mission_ref='';
CREATE INDEX dispatch_delivery_mission_logical_ref ON dispatch_delivery_missions(delivery_group_id,logical_mission_ref);
CREATE UNIQUE INDEX dispatch_manifest_logical_version_unique ON dispatch_parcel_manifests(manifest_ref,version);
CREATE UNIQUE INDEX dispatch_manifest_one_active_logical
    ON dispatch_parcel_manifests(manifest_ref) WHERE status IN ('ACTIVE','CUSTODY_ACTIVE');

CREATE TABLE dispatch_replan_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_group_id UUID NOT NULL REFERENCES dispatch_delivery_groups(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    expected_source_version BIGINT NOT NULL CHECK (expected_source_version > 0),
    expected_plan_version BIGINT NOT NULL CHECK (expected_plan_version > 0),
    result_plan_id UUID NULL REFERENCES dispatch_mission_plans(id) ON DELETE RESTRICT,
    result_plan_version BIGINT NULL CHECK (result_plan_version IS NULL OR result_plan_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dispatch_manifest_custody_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    manifest_id UUID NOT NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('ACTIVATE','RELEASE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE dispatch_mission_execution_links IS 'Idempotent bridge from planned delivery missions into the canonical Booking/Request/Trip execution chain.';
COMMENT ON COLUMN dispatch_parcels.scannable_code IS 'Opaque parcel identity only; never an OTP or handover credential.';
