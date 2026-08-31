-- Route-aware, multi-pickup Commerce delivery on Dispatch's existing quote,
-- booking, marketplace, trip and transactional-outbox authorities.

ALTER TABLE commerce_delivery_quotes
    ADD COLUMN IF NOT EXISTS contract_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS route_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS supply_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS service_class TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS route_provider TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS route_evidence_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS distance_meters BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS estimated_seconds BIGINT NOT NULL DEFAULT 0;

ALTER TABLE commerce_delivery_quotes
    DROP CONSTRAINT IF EXISTS commerce_delivery_quotes_serviceable_chk;
ALTER TABLE commerce_delivery_quotes
    ADD CONSTRAINT commerce_delivery_quotes_serviceable_chk CHECK (
        NOT serviceable OR (
            reason_code = 'SERVICEABLE' AND currency <> '' AND
            (contract_version = 1 OR available_providers > 0)
        )
    );
ALTER TABLE commerce_delivery_quotes
    ADD CONSTRAINT commerce_delivery_quotes_v1_route_chk CHECK (
        contract_version <> 1 OR NOT serviceable OR (
            distance_meters > 0 AND estimated_seconds > 0 AND
            length(btrim(route_provider)) > 0 AND
            length(btrim(route_evidence_version)) > 0 AND
            mode IN ('FASTEST','BALANCED','SAVER') AND capability_version > 0
        )
    );

ALTER TABLE commerce_missions
    ADD COLUMN IF NOT EXISTS caller_service TEXT NOT NULL DEFAULT 'vendor-service',
    ADD COLUMN IF NOT EXISTS operation_ref TEXT,
    ADD COLUMN IF NOT EXISTS request_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS canonical_ref TEXT,
    ADD COLUMN IF NOT EXISTS runtime_state TEXT NOT NULL DEFAULT 'SEARCHING',
    ADD COLUMN IF NOT EXISTS queue_state TEXT NOT NULL DEFAULT 'SEARCHING',
    ADD COLUMN IF NOT EXISTS assignment_attempt INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS plan_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS plan_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS plan_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mission_slot_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS quote_revision BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS quote_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS service_class TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS funding_reference TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS trip_id UUID NULL REFERENCES trips(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS current_provider_id UUID NULL REFERENCES providers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS promised_by TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS reason_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL;

UPDATE commerce_missions
SET operation_ref = mission_ref
WHERE operation_ref IS NULL;
UPDATE commerce_missions
SET canonical_ref = 'dcm_' || replace(gen_random_uuid()::text, '-', '')
WHERE canonical_ref IS NULL;
ALTER TABLE commerce_missions ALTER COLUMN operation_ref SET NOT NULL;
ALTER TABLE commerce_missions ALTER COLUMN canonical_ref SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_mission_caller_operation
    ON commerce_missions(caller_service, operation_ref);
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_mission_canonical_ref
    ON commerce_missions(canonical_ref);
CREATE INDEX IF NOT EXISTS idx_commerce_mission_runtime_queue
    ON commerce_missions(runtime_state, queue_state, promised_by);

CREATE TABLE IF NOT EXISTS commerce_mission_stops (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE CASCADE,
    stop_ref TEXT NOT NULL,
    physical_stop_ref TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK(sequence > 0),
    vendor_ref TEXT NOT NULL,
    vendor_display TEXT NOT NULL DEFAULT '',
    branch_ref TEXT NOT NULL,
    manifest_ref TEXT NOT NULL,
    location_snapshot JSONB NOT NULL,
    ready_from TIMESTAMPTZ NOT NULL,
    ready_to TIMESTAMPTZ NOT NULL,
    handling_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'AWAITING_PICKUP',
    handover_status TEXT NOT NULL DEFAULT 'REQUIRED',
    handover_code TEXT NOT NULL DEFAULT '',
    handover_code_hash TEXT NOT NULL DEFAULT '',
    handover_code_expires_at TIMESTAMPTZ NULL,
    handover_code_attempts INTEGER NOT NULL DEFAULT 0,
    handover_code_resend_count INTEGER NOT NULL DEFAULT 0,
    handover_code_last_sent_at TIMESTAMPTZ NULL,
    handover_verified_at TIMESTAMPTZ NULL,
    handover_evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_version BIGINT NOT NULL DEFAULT 1,
    arrived_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(commerce_mission_id, stop_ref),
    UNIQUE(commerce_mission_id, sequence),
    CHECK(ready_to > ready_from),
    CHECK(status IN ('AWAITING_PICKUP','ARRIVED','READY_FOR_HANDOVER','PICKED_UP','SKIPPED','RETURNED')),
    CHECK(handover_status IN ('REQUIRED','ISSUED','VERIFIED','CONSUMED','DEPENDENCY_UNAVAILABLE'))
);

CREATE INDEX IF NOT EXISTS idx_commerce_mission_stops_sequence
    ON commerce_mission_stops(commerce_mission_id, sequence);

CREATE TABLE IF NOT EXISTS commerce_mission_parcels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE CASCADE,
    stop_id UUID NOT NULL REFERENCES commerce_mission_stops(id) ON DELETE RESTRICT,
    parcel_ref TEXT NOT NULL,
    vendor_ref TEXT NOT NULL,
    branch_ref TEXT NOT NULL,
    fulfilment_unit_ref TEXT NOT NULL,
    vendor_order_ref TEXT NOT NULL,
    manifest_ref TEXT NOT NULL,
    manifest_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    weight_grams BIGINT NOT NULL CHECK(weight_grams >= 0),
    volume_ml BIGINT NOT NULL DEFAULT 0 CHECK(volume_ml >= 0),
    length_mm BIGINT NOT NULL DEFAULT 0 CHECK(length_mm >= 0),
    width_mm BIGINT NOT NULL DEFAULT 0 CHECK(width_mm >= 0),
    height_mm BIGINT NOT NULL DEFAULT 0 CHECK(height_mm >= 0),
    package_count BIGINT NOT NULL CHECK(package_count > 0),
    handling_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'AWAITING_PICKUP',
    custodian_type TEXT NOT NULL DEFAULT 'VENDOR',
    custodian_ref TEXT NOT NULL DEFAULT '',
    source_version BIGINT NOT NULL DEFAULT 1,
    picked_up_at TIMESTAMPTZ NULL,
    delivered_at TIMESTAMPTZ NULL,
    returned_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(commerce_mission_id, parcel_ref),
    CHECK(status IN ('AWAITING_PICKUP','PICKED_UP','IN_TRANSIT','DELIVERED','RETURN_REQUIRED','RETURNED','TRANSFER_REQUIRED','CANCELLED')),
    CHECK(custodian_type IN ('VENDOR','PROVIDER','RECEIVER','TRANSFER','RETURN'))
);

CREATE INDEX IF NOT EXISTS idx_commerce_mission_parcels_stop
    ON commerce_mission_parcels(stop_id, status);

CREATE TABLE IF NOT EXISTS commerce_mission_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    caller_service TEXT NOT NULL,
    operation_ref TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    commerce_mission_id UUID NULL REFERENCES commerce_missions(id) ON DELETE CASCADE,
    result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(caller_service, operation_ref)
);

CREATE TABLE IF NOT EXISTS commerce_mission_assignment_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK(attempt_number > 0),
    booking_id UUID NOT NULL REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE SET NULL,
    provider_id UUID NULL REFERENCES providers(id) ON DELETE SET NULL,
    state TEXT NOT NULL,
    reason_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ NULL,
    UNIQUE(commerce_mission_id, attempt_number),
    UNIQUE(booking_id)
);

CREATE TABLE IF NOT EXISTS commerce_mission_legs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE CASCADE,
    leg_ref TEXT NOT NULL,
    leg_type TEXT NOT NULL CHECK(leg_type IN ('DELIVERY','RETURN','TRANSFER')),
    state TEXT NOT NULL,
    parcel_refs TEXT[] NOT NULL,
    evidence_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    UNIQUE(commerce_mission_id, leg_ref)
);

ALTER TABLE dispatch_marketplace_outbox
    ADD COLUMN IF NOT EXISTS target_service TEXT NOT NULL DEFAULT 'fleet-partner-service';
DROP INDEX IF EXISTS idx_dispatch_outbox_pending;
CREATE INDEX idx_dispatch_outbox_pending
    ON dispatch_marketplace_outbox(target_service, next_attempt_at)
    WHERE status IN ('PENDING','FAILED');

COMMENT ON TABLE commerce_mission_stops IS
'Canonical Dispatch logical pickup stops. Co-located physical stops may share physical_stop_ref while vendor manifests and handovers remain independent.';
COMMENT ON TABLE commerce_mission_parcels IS
'Operational projections of opaque Vendor parcel identities; Dispatch does not replace or own the commercial parcel identifier.';
