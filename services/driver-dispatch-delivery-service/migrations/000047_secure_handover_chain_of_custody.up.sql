-- Secure two-stage handover and immutable chain-of-custody foundation.
-- Existing Trip/recipient-confirmation/proof flows remain available for
-- deployments where the new feature flags are disabled.

CREATE TABLE dispatch_handover_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_ref TEXT NOT NULL UNIQUE,
    context_key TEXT NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY','PROVIDER_TRANSFER')),
    delivery_group_id UUID NULL REFERENCES dispatch_delivery_groups(id) ON DELETE RESTRICT,
    mission_id UUID NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    mission_ref TEXT NOT NULL,
    stop_id UUID NULL REFERENCES dispatch_mission_stops(id) ON DELETE RESTRICT,
    stop_ref TEXT NOT NULL,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    booking_id UUID NOT NULL REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,
    manifest_ref TEXT NOT NULL,
    manifest_version BIGINT NOT NULL CHECK (manifest_version > 0),
    assigned_provider_id UUID NULL REFERENCES providers(id) ON DELETE RESTRICT,
    intended_recipient_ref TEXT NOT NULL,
    recipient_type TEXT NOT NULL CHECK (recipient_type IN ('SENDER','RECEIVER','PROVIDER')),
    masked_destination TEXT NOT NULL DEFAULT '',
    otp_hash TEXT NOT NULL,
    qr_token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    resend_count INTEGER NOT NULL DEFAULT 0 CHECK (resend_count >= 0),
    resend_cooldown_until TIMESTAMPTZ NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','CONSUMED','INVALIDATED','EXPIRED','LOCKED')),
    consumed_at TIMESTAMPTZ NULL,
    invalidated_at TIMESTAMPTZ NULL,
    invalidated_reason TEXT NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ NULL,
    notification_state TEXT NOT NULL DEFAULT 'NOT_REQUESTED'
        CHECK (notification_state IN ('NOT_REQUESTED','REQUESTED','ACCEPTED','UNAVAILABLE','FAILED')),
    notification_channels TEXT[] NOT NULL DEFAULT '{}',
    notification_ref TEXT NOT NULL DEFAULT '',
    created_by_source TEXT NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    optimistic_version BIGINT NOT NULL DEFAULT 1 CHECK (optimistic_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK ((status='CONSUMED') = (consumed_at IS NOT NULL)),
    CHECK ((status='LOCKED') = (locked_at IS NOT NULL)),
    CHECK ((status='INVALIDATED') = (invalidated_at IS NOT NULL)),
    UNIQUE (context_key, version)
);

CREATE UNIQUE INDEX dispatch_handover_one_active_context
    ON dispatch_handover_challenges(context_key) WHERE status='ACTIVE';
CREATE UNIQUE INDEX dispatch_handover_one_active_stage_context
    ON dispatch_handover_challenges(mission_ref, stop_ref, stage) WHERE status='ACTIVE';
CREATE INDEX dispatch_handover_trip_stage
    ON dispatch_handover_challenges(trip_id, stage, version DESC);
CREATE INDEX dispatch_handover_expiry
    ON dispatch_handover_challenges(expires_at) WHERE status='ACTIVE';

CREATE TABLE dispatch_handover_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_id UUID NOT NULL REFERENCES dispatch_handover_challenges(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    method TEXT NOT NULL CHECK (method IN ('OTP','QR')),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('CUSTOMER','PROVIDER','VENDOR','PARTNER','SYSTEM')),
    actor_ref TEXT NOT NULL,
    result TEXT NOT NULL CHECK (result IN ('SUCCEEDED','FAILED','EXPIRED','LOCKED','INVALIDATED','SCOPE_MISMATCH')),
    reason_code TEXT NOT NULL DEFAULT '',
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_handover_attempt_challenge
    ON dispatch_handover_attempts(challenge_id, attempted_at);

CREATE TABLE dispatch_handover_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    caller_scope TEXT NOT NULL,
    operation_ref TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    challenge_id UUID NULL REFERENCES dispatch_handover_challenges(id) ON DELETE RESTRICT,
    result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (caller_scope, operation_ref)
);

CREATE TABLE dispatch_evidence_requirements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    context_key TEXT NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY','PACKAGE_DISCREPANCY','FAILED_ATTEMPT','PROVIDER_TRANSFER','RETURN')),
    policy_version TEXT NOT NULL,
    parcel_scans_required BOOLEAN NOT NULL DEFAULT false,
    photo_required BOOLEAN NOT NULL DEFAULT true,
    signature_required BOOLEAN NOT NULL DEFAULT false,
    gps_required BOOLEAN NOT NULL DEFAULT false,
    min_evidence_count INTEGER NOT NULL DEFAULT 1 CHECK (min_evidence_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (context_key, stage, policy_version)
);

CREATE TABLE dispatch_evidence_capture_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    capture_session_ref TEXT NOT NULL UNIQUE,
    challenge_id UUID NOT NULL REFERENCES dispatch_handover_challenges(id) ON DELETE RESTRICT,
    mission_id UUID NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    stop_id UUID NULL REFERENCES dispatch_mission_stops(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    manifest_ref TEXT NOT NULL,
    manifest_version BIGINT NOT NULL CHECK (manifest_version > 0),
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY','PACKAGE_DISCREPANCY','FAILED_ATTEMPT','PROVIDER_TRANSFER','RETURN')),
    evidence_type TEXT NOT NULL CHECK (evidence_type IN ('PHOTO','SIGNATURE','PACKAGE_DISCREPANCY','FAILED_ATTEMPT','PROVIDER_TRANSFER','RETURN')),
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CONSUMED','EXPIRED','INVALIDATED')),
    max_uploads INTEGER NOT NULL DEFAULT 1 CHECK (max_uploads > 0),
    upload_count INTEGER NOT NULL DEFAULT 0 CHECK (upload_count >= 0 AND upload_count <= max_uploads),
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE UNIQUE INDEX dispatch_evidence_one_active_session
    ON dispatch_evidence_capture_sessions(challenge_id, provider_id, stage, evidence_type)
    WHERE status='ACTIVE';

CREATE TABLE dispatch_parcel_scan_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mission_id UUID NOT NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    stop_id UUID NOT NULL REFERENCES dispatch_mission_stops(id) ON DELETE RESTRICT,
    manifest_id UUID NOT NULL REFERENCES dispatch_parcel_manifests(id) ON DELETE RESTRICT,
    parcel_id UUID NOT NULL REFERENCES dispatch_parcels(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (mission_id, stop_id, manifest_id, parcel_id)
);

CREATE TABLE dispatch_evidence_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_ref TEXT NOT NULL UNIQUE,
    capture_session_id UUID NOT NULL REFERENCES dispatch_evidence_capture_sessions(id) ON DELETE RESTRICT,
    challenge_id UUID NOT NULL REFERENCES dispatch_handover_challenges(id) ON DELETE RESTRICT,
    mission_id UUID NULL REFERENCES dispatch_delivery_missions(id) ON DELETE RESTRICT,
    stop_id UUID NULL REFERENCES dispatch_mission_stops(id) ON DELETE RESTRICT,
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    manifest_ref TEXT NOT NULL,
    manifest_version BIGINT NOT NULL CHECK (manifest_version > 0),
    stage TEXT NOT NULL,
    evidence_type TEXT NOT NULL,
    media_ref TEXT NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL,
    latitude DOUBLE PRECISION NULL CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NULL CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180),
    reason_code TEXT NOT NULL DEFAULT '',
    supersedes_evidence_ref TEXT NOT NULL DEFAULT '',
    operation_ref TEXT NOT NULL UNIQUE,
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_evidence_trip_stage
    ON dispatch_evidence_refs(trip_id, stage, created_at);

CREATE OR REPLACE FUNCTION dispatch_reject_handover_terminal_reactivation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'ACTIVE' AND NEW.status = 'ACTIVE' THEN
        RAISE EXCEPTION 'terminal handover challenge cannot be reactivated';
    END IF;
    IF NEW.context_key <> OLD.context_key OR NEW.version <> OLD.version OR
       NEW.stage <> OLD.stage OR NEW.trip_id <> OLD.trip_id OR
       NEW.manifest_ref <> OLD.manifest_ref OR NEW.manifest_version <> OLD.manifest_version OR
       NEW.otp_hash <> OLD.otp_hash OR NEW.qr_token_hash <> OLD.qr_token_hash THEN
        RAISE EXCEPTION 'handover challenge identity and credentials are immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER dispatch_handover_terminal_guard
    BEFORE UPDATE ON dispatch_handover_challenges
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_handover_terminal_reactivation();

CREATE OR REPLACE FUNCTION dispatch_invalidate_handover_on_trip_identity_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.provider_id IS DISTINCT FROM OLD.provider_id THEN
        UPDATE dispatch_handover_challenges
           SET status='INVALIDATED', invalidated_at=now(), invalidated_reason='PROVIDER_REASSIGNED',
               locked_at=NULL, updated_at=now(), optimistic_version=optimistic_version+1
         WHERE trip_id=NEW.id AND status='ACTIVE';
    ELSIF NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.receiver_phone IS DISTINCT FROM OLD.receiver_phone THEN
        UPDATE dispatch_handover_challenges
           SET status='INVALIDATED', invalidated_at=now(), invalidated_reason='RECEIVER_CHANGED',
               locked_at=NULL, updated_at=now(), optimistic_version=optimistic_version+1
         WHERE trip_id=NEW.id AND status='ACTIVE';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER dispatch_trip_handover_identity_guard
    AFTER UPDATE OF provider_id, customer_id, receiver_phone ON trips
    FOR EACH ROW EXECUTE FUNCTION dispatch_invalidate_handover_on_trip_identity_change();

CREATE OR REPLACE FUNCTION dispatch_reject_chain_of_custody_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'chain-of-custody evidence is append-only';
END;
$$;
CREATE TRIGGER dispatch_handover_attempt_append_only
    BEFORE UPDATE OR DELETE ON dispatch_handover_attempts
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_chain_of_custody_mutation();
CREATE TRIGGER dispatch_parcel_scan_append_only
    BEFORE UPDATE OR DELETE ON dispatch_parcel_scan_events
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_chain_of_custody_mutation();
CREATE TRIGGER dispatch_evidence_ref_append_only
    BEFORE UPDATE OR DELETE ON dispatch_evidence_refs
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_chain_of_custody_mutation();

COMMENT ON TABLE dispatch_handover_challenges IS 'Versioned OTP/QR authority. Plaintext credentials are never stored.';
COMMENT ON TABLE dispatch_evidence_refs IS 'Append-only media-file references; Dispatch stores no evidence bytes.';
