-- Append-only operational evidence, controlled delivery recovery, and
-- provider-to-provider custody transfer for canonical Commerce missions.

ALTER TABLE commerce_mission_stops
    ADD COLUMN manifest_version BIGINT NOT NULL DEFAULT 1 CHECK (manifest_version > 0),
    ADD COLUMN manifest_frozen_at TIMESTAMPTZ NULL;

ALTER TABLE commerce_mission_parcels
    ADD COLUMN manifest_version BIGINT NOT NULL DEFAULT 1 CHECK (manifest_version > 0),
    ADD COLUMN manifest_created_at TIMESTAMPTZ NULL,
    ADD COLUMN manifest_frozen_at TIMESTAMPTZ NULL;

ALTER TABLE commerce_missions
    ADD COLUMN replacement_state TEXT NOT NULL DEFAULT 'NONE',
    ADD COLUMN replacement_provider_id UUID NULL REFERENCES providers(id) ON DELETE RESTRICT,
    ADD COLUMN replacement_trip_id UUID NULL REFERENCES trips(id) ON DELETE RESTRICT,
    ADD COLUMN replacement_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN receiver_challenge_generation BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT commerce_missions_replacement_state_chk CHECK (
        replacement_state IN ('NONE','REQUIRED','MATCHING','ASSIGNED','TRANSFER_PENDING','TRANSFER_IN_PROGRESS')
    );

CREATE TABLE commerce_mission_evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE RESTRICT,
    stop_id UUID NULL REFERENCES commerce_mission_stops(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL,
    evidence_type TEXT NOT NULL CHECK (evidence_type IN (
        'PICKUP','DELIVERY','DAMAGE','DISCREPANCY','FAILED_ATTEMPT','RETURN','PROVIDER_TRANSFER'
    )),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('PROVIDER','VENDOR','INTERNAL_SERVICE','SUPPORT')),
    actor_ref TEXT NOT NULL,
    parcel_refs TEXT[] NOT NULL,
    manifest_ref TEXT NOT NULL DEFAULT '',
    manifest_version BIGINT NOT NULL DEFAULT 0 CHECK (manifest_version >= 0),
    package_count BIGINT NOT NULL DEFAULT 0 CHECK (package_count >= 0),
    captured_at TIMESTAMPTZ NOT NULL,
    latitude DOUBLE PRECISION NULL,
    longitude DOUBLE PRECISION NULL,
    media_refs TEXT[] NOT NULL DEFAULT '{}',
    description TEXT NOT NULL DEFAULT '',
    reason_code TEXT NOT NULL DEFAULT '',
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    correction_of UUID NULL REFERENCES commerce_mission_evidence(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (commerce_mission_id, operation_ref),
    CHECK (cardinality(parcel_refs) BETWEEN 1 AND 100),
    CHECK (cardinality(media_refs) <= 12),
    CHECK (length(description) <= 2000),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

CREATE INDEX commerce_mission_evidence_mission_idx
    ON commerce_mission_evidence(commerce_mission_id, created_at, id);
CREATE INDEX commerce_mission_evidence_parcels_idx
    ON commerce_mission_evidence USING gin(parcel_refs);

CREATE TABLE commerce_delivery_exceptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL,
    reason_code TEXT NOT NULL CHECK (reason_code IN (
        'VENDOR_NOT_READY','PACKAGE_MISMATCH','PROHIBITED_ITEM','RESTRICTED_ITEM',
        'DAMAGED_PACKAGE','RECEIVER_UNAVAILABLE','RECEIVER_REFUSED','WRONG_ADDRESS',
        'ADDRESS_INACCESSIBLE','VEHICLE_BREAKDOWN','PROVIDER_ILLNESS','WEATHER_ROAD',
        'SECURITY_RISK','SAFETY_INCIDENT'
    )),
    state TEXT NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN','RECOVERY_SELECTED','RESOLVED')),
    recovery_action TEXT NOT NULL DEFAULT '' CHECK (recovery_action IN (
        '','WAIT','RETRY_PICKUP','RETRY_DELIVERY','RESCHEDULE','REPLAN',
        'ALTERNATIVE_RECEIVER','RETURN_TO_SENDER','RETURN_TO_VENDOR','PROVIDER_REPLACEMENT','SUPPORT'
    )),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('PROVIDER','VENDOR','INTERNAL_SERVICE','SUPPORT')),
    actor_ref TEXT NOT NULL,
    parcel_refs TEXT[] NOT NULL,
    evidence_id UUID NULL REFERENCES commerce_mission_evidence(id) ON DELETE RESTRICT,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ NULL,
    UNIQUE (commerce_mission_id, operation_ref),
    CHECK (cardinality(parcel_refs) BETWEEN 1 AND 100)
);

CREATE INDEX commerce_delivery_exceptions_mission_idx
    ON commerce_delivery_exceptions(commerce_mission_id, created_at, id);

CREATE TABLE commerce_return_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL,
    product_class TEXT NOT NULL CHECK (product_class IN ('PREPARED_FOOD','SHOP_RETAIL')),
    state TEXT NOT NULL DEFAULT 'REQUIRED' CHECK (state IN ('REQUIRED','IN_RETURN','RETURNED','SUPPORT_REQUIRED')),
    disposition TEXT NOT NULL CHECK (disposition IN (
        'RETURN_TO_VENDOR','FOOD_DISPOSAL_POLICY','SHOP_INSPECTION','SUPPORT_REVIEW'
    )),
    parcel_refs TEXT[] NOT NULL,
    origin_leg_ref TEXT NOT NULL DEFAULT '',
    return_leg_ref TEXT NOT NULL,
    evidence_id UUID NULL REFERENCES commerce_mission_evidence(id) ON DELETE RESTRICT,
    completion_evidence_id UUID NULL REFERENCES commerce_mission_evidence(id) ON DELETE RESTRICT,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    UNIQUE (commerce_mission_id, operation_ref),
    UNIQUE (commerce_mission_id, return_leg_ref),
    CHECK (cardinality(parcel_refs) BETWEEN 1 AND 100),
    CHECK (product_class <> 'PREPARED_FOOD' OR disposition <> 'SHOP_INSPECTION')
);

CREATE TABLE commerce_provider_transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_ref TEXT NOT NULL UNIQUE,
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('REQUIRED','CHALLENGE_ISSUED','COMPLETED','CANCELLED')),
    old_provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    new_provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    old_trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    new_trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    parcel_refs TEXT[] NOT NULL,
    manifest_binding JSONB NOT NULL CHECK (jsonb_typeof(manifest_binding) = 'array'),
    package_count BIGINT NOT NULL CHECK (package_count > 0),
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    challenge_hash TEXT NOT NULL,
    challenge_expires_at TIMESTAMPTZ NOT NULL,
    challenge_attempts INTEGER NOT NULL DEFAULT 0 CHECK (challenge_attempts >= 0),
    evidence_id UUID NULL REFERENCES commerce_mission_evidence(id) ON DELETE RESTRICT,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    UNIQUE (commerce_mission_id, operation_ref),
    CHECK (old_provider_id <> new_provider_id),
    CHECK (cardinality(parcel_refs) BETWEEN 1 AND 100)
);

CREATE UNIQUE INDEX commerce_provider_transfers_one_open_idx
    ON commerce_provider_transfers(commerce_mission_id)
    WHERE state IN ('REQUIRED','CHALLENGE_ISSUED');

CREATE TABLE commerce_mission_carrier_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commerce_mission_id UUID NOT NULL REFERENCES commerce_missions(id) ON DELETE RESTRICT,
    transfer_id UUID NULL REFERENCES commerce_provider_transfers(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE RESTRICT,
    custody_started_at TIMESTAMPTZ NOT NULL,
    custody_ended_at TIMESTAMPTZ NULL,
    end_reason TEXT NOT NULL DEFAULT '',
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (commerce_mission_id, provider_id, custody_started_at)
);

CREATE UNIQUE INDEX commerce_mission_carrier_history_one_active_idx
    ON commerce_mission_carrier_history(commerce_mission_id)
    WHERE custody_ended_at IS NULL;

CREATE FUNCTION enforce_commerce_carrier_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'commerce carrier history is append-only';
    END IF;
    IF NEW.commerce_mission_id IS DISTINCT FROM OLD.commerce_mission_id OR
       NEW.transfer_id IS DISTINCT FROM OLD.transfer_id OR
       NEW.provider_id IS DISTINCT FROM OLD.provider_id OR
       NEW.trip_id IS DISTINCT FROM OLD.trip_id OR
       NEW.custody_started_at IS DISTINCT FROM OLD.custody_started_at OR
       OLD.custody_ended_at IS NOT NULL OR
       NEW.custody_ended_at IS NULL THEN
        RAISE EXCEPTION 'commerce carrier history permits only one responsibility closure';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_mission_carrier_history_append_only
    BEFORE UPDATE OR DELETE ON commerce_mission_carrier_history
    FOR EACH ROW EXECUTE FUNCTION enforce_commerce_carrier_history();

CREATE FUNCTION reject_commerce_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce operational evidence is append-only';
END $$;

CREATE TRIGGER commerce_mission_evidence_immutable
    BEFORE UPDATE OR DELETE ON commerce_mission_evidence
    FOR EACH ROW EXECUTE FUNCTION reject_commerce_evidence_mutation();

CREATE FUNCTION enforce_frozen_commerce_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.manifest_frozen_at IS NOT NULL AND (
        NEW.manifest_ref IS DISTINCT FROM OLD.manifest_ref OR
        NEW.manifest_version IS DISTINCT FROM OLD.manifest_version OR
        NEW.manifest_snapshot IS DISTINCT FROM OLD.manifest_snapshot OR
        NEW.package_count IS DISTINCT FROM OLD.package_count
    ) THEN
        RAISE EXCEPTION 'commerce parcel manifest is frozen';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_mission_parcel_manifest_frozen
    BEFORE UPDATE ON commerce_mission_parcels
    FOR EACH ROW EXECUTE FUNCTION enforce_frozen_commerce_manifest();

CREATE FUNCTION enforce_frozen_commerce_stop_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.manifest_frozen_at IS NOT NULL AND (
        NEW.manifest_ref IS DISTINCT FROM OLD.manifest_ref OR
        NEW.manifest_version IS DISTINCT FROM OLD.manifest_version
    ) THEN
        RAISE EXCEPTION 'commerce stop manifest is frozen';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_mission_stop_manifest_frozen
    BEFORE UPDATE ON commerce_mission_stops
    FOR EACH ROW EXECUTE FUNCTION enforce_frozen_commerce_stop_manifest();

COMMENT ON TABLE commerce_mission_evidence IS
'Append-only metadata and opaque media references for pickup, delivery, exceptions, returns and provider transfer. Media bytes remain media-file-service owned.';
COMMENT ON TABLE commerce_provider_transfers IS
'Two-provider, exact-parcel custody handover on one canonical Commerce mission; simple provider_id reassignment after custody is forbidden.';
