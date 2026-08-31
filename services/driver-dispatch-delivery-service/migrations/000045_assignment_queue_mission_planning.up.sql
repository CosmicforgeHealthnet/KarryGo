-- Additive Dispatch Sections 7-8 assignment, queue and mission-planning state.
-- Legacy request_broadcasts/provider_request_inbox/trips remain unchanged.

CREATE TABLE dispatch_assignment_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK (strategy IN ('BROADCAST_FIRST_ACCEPT','SEQUENTIAL_NEAREST','ORGANIZATION_DISPATCH','ADMIN_ASSISTED')),
    organization_id UUID NULL,
    policy_version TEXT NOT NULL,
    selected_by TEXT NOT NULL DEFAULT 'SERVER_POLICY',
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (mission_id),
    UNIQUE (idempotency_key)
);

CREATE TABLE dispatch_assignment_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    rank INTEGER NOT NULL CHECK (rank > 0),
    distance_meters BIGINT NOT NULL CHECK (distance_meters >= 0),
    eligibility_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    candidate_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (mission_id, provider_id),
    UNIQUE (mission_id, rank)
);

CREATE TABLE dispatch_assignment_offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES dispatch_assignment_candidates(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'OFFERED' CHECK (status IN ('OFFERED','ACCEPTED','DECLINED','EXPIRED','CANCELLED')),
    offered_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ NULL,
    decline_reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NULL,
    request_fingerprint TEXT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > offered_at)
);
CREATE UNIQUE INDEX dispatch_assignment_one_live_offer
    ON dispatch_assignment_offers(mission_id) WHERE status='OFFERED';
CREATE UNIQUE INDEX dispatch_assignment_one_winner
    ON dispatch_assignment_offers(mission_id) WHERE status='ACCEPTED';
CREATE UNIQUE INDEX dispatch_assignment_offer_operation
    ON dispatch_assignment_offers(provider_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX dispatch_assignment_offer_expiry
    ON dispatch_assignment_offers(expires_at) WHERE status='OFFERED';

CREATE TABLE dispatch_queue_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED','RETRYING','WAITING_CUSTOMER','SCHEDULED','ASSIGNED','EXPIRED','CANCELLED','FAILED')),
    priority SMALLINT NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100),
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    scheduled_for TIMESTAMPTZ NULL,
    quote_ref TEXT NOT NULL DEFAULT '',
    quote_expires_at TIMESTAMPTZ NULL,
    customer_option TEXT NULL CHECK (customer_option IS NULL OR customer_option IN ('KEEP_WAITING','SCHEDULE','EXPANDED_SEARCH','REQUOTE','CANCEL')),
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    last_error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    UNIQUE (idempotency_key)
);
CREATE UNIQUE INDEX dispatch_queue_one_active_mission
    ON dispatch_queue_entries(mission_id)
    WHERE status IN ('QUEUED','RETRYING','WAITING_CUSTOMER','SCHEDULED');
CREATE INDEX dispatch_queue_due
    ON dispatch_queue_entries(priority DESC, next_attempt_at, created_at)
    WHERE status IN ('QUEUED','RETRYING','SCHEDULED');

CREATE TABLE dispatch_queue_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id UUID NOT NULL REFERENCES dispatch_queue_entries(id) ON DELETE CASCADE,
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    option TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dispatch_mission_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_group_id UUID NOT NULL REFERENCES dispatch_delivery_groups(id) ON DELETE CASCADE,
    plan_ref TEXT NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUPERSEDED','REJECTED','REVIEW')),
    outcome TEXT NOT NULL CHECK (outcome IN ('CONSOLIDATE_SINGLE_MISSION','SPLIT_MULTIPLE_MISSIONS','WAIT_FOR_READINESS','OPERATIONAL_REVIEW','REJECT_INCOMPATIBLE')),
    policy_version TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    reason_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    plan_snapshot JSONB NOT NULL,
    superseded_by UUID NULL REFERENCES dispatch_mission_plans(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (delivery_group_id, plan_ref, version)
);
CREATE UNIQUE INDEX dispatch_mission_plan_one_active
    ON dispatch_mission_plans(delivery_group_id) WHERE status='ACTIVE';

ALTER TABLE dispatch_delivery_missions
    ADD COLUMN plan_id UUID NULL REFERENCES dispatch_mission_plans(id) ON DELETE RESTRICT,
    ADD COLUMN plan_mission_index INTEGER NULL CHECK (plan_mission_index IS NULL OR plan_mission_index > 0),
    ADD COLUMN vehicle_class TEXT NOT NULL DEFAULT '',
    ADD COLUMN capacity_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE dispatch_mission_stops
    ADD COLUMN stop_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN readiness_from TIMESTAMPTZ NULL,
    ADD COLUMN readiness_to TIMESTAMPTZ NULL;
CREATE UNIQUE INDEX dispatch_mission_stop_ref_unique
    ON dispatch_mission_stops(mission_id, stop_ref) WHERE stop_ref <> '';

ALTER TABLE dispatch_parcel_manifests
    ADD COLUMN status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUPERSEDED','CUSTODY_ACTIVE','COMPLETED')),
    ADD COLUMN superseded_by UUID NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    ADD COLUMN policy_version TEXT NOT NULL DEFAULT 'dispatch_manifest_v1';
-- Keep manifest identity/version immutable while allowing the lifecycle-only
-- ACTIVE -> SUPERSEDED transition introduced by this migration. Membership
-- rows remain fully immutable.
CREATE OR REPLACE FUNCTION dispatch_reject_manifest_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME = 'dispatch_manifest_parcels' OR TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'parcel manifests are immutable; create a new manifest version';
    END IF;
    IF NEW.id <> OLD.id OR NEW.mission_id <> OLD.mission_id OR
       NEW.manifest_ref <> OLD.manifest_ref OR NEW.version <> OLD.version OR
       NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'parcel manifest identity and version are immutable';
    END IF;
    RETURN NEW;
END;
$$;

-- Migration 44 permitted multiple immutable versions. Preserve every row and
-- mark only each mission's newest version active before enforcing the new
-- one-active-manifest invariant.
WITH ranked AS (
    SELECT id, row_number() OVER (PARTITION BY mission_id ORDER BY version DESC, created_at DESC, id DESC) AS position
    FROM dispatch_parcel_manifests
)
UPDATE dispatch_parcel_manifests manifest
SET status='SUPERSEDED'
FROM ranked
WHERE manifest.id=ranked.id AND ranked.position>1;

CREATE UNIQUE INDEX dispatch_manifest_one_active_per_mission
    ON dispatch_parcel_manifests(mission_id)
    WHERE status IN ('ACTIVE','CUSTODY_ACTIVE');

CREATE TABLE dispatch_stop_expected_parcels (
    mission_stop_id UUID NOT NULL REFERENCES dispatch_mission_stops(id) ON DELETE CASCADE,
    parcel_id UUID NOT NULL REFERENCES dispatch_parcels(id) ON DELETE RESTRICT,
    manifest_id UUID NOT NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    PRIMARY KEY (mission_stop_id, parcel_id)
);

CREATE TABLE dispatch_active_custody_manifest_parcels (
    parcel_id UUID PRIMARY KEY REFERENCES dispatch_parcels(id) ON DELETE RESTRICT,
    manifest_id UUID NOT NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (manifest_id, parcel_id)
);

COMMENT ON TABLE dispatch_assignment_offers IS 'Exclusive sequential offers; partial indexes enforce one live offer and one winner per mission.';
COMMENT ON TABLE dispatch_queue_entries IS 'Durable operational queue; never a financial ledger or authorization to change a quote.';
COMMENT ON TABLE dispatch_mission_plans IS 'Immutable versioned planning snapshots; execution remains the canonical Trip lifecycle.';
