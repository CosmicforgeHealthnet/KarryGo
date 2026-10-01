-- Section 29 secure two-stage goods handover. Only keyed hashes and opaque
-- references are durable; plaintext OTP and QR bearer values never enter SQL.

CREATE TABLE taxi_goods_handover_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_ref TEXT NOT NULL UNIQUE,
    mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY')),
    provider_id UUID NOT NULL REFERENCES taxi_providers(id),
    receiver_ref TEXT NOT NULL DEFAULT '',
    otp_hash BYTEA NOT NULL,
    qr_hash BYTEA NOT NULL,
    version BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CONSUMED','EXPIRED','INVALIDATED','LOCKED')),
    attempts INT NOT NULL DEFAULT 0,
    resend_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    resend_available_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    invalidated_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (mission_id, stage, version)
);

CREATE UNIQUE INDEX idx_taxi_goods_handover_active_stage
    ON taxi_goods_handover_challenges(mission_id, stage)
    WHERE status = 'ACTIVE';

CREATE TABLE taxi_goods_evidence_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_ref TEXT NOT NULL UNIQUE,
    mission_id UUID NOT NULL REFERENCES taxi_goods_missions(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY')),
    media_ref TEXT NOT NULL,
    parcel_refs TEXT[] NOT NULL DEFAULT '{}',
    captured_by_provider_id UUID NOT NULL REFERENCES taxi_providers(id),
    captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (mission_id, stage, media_ref)
);

CREATE TABLE taxi_service_nonces (
    caller_service TEXT NOT NULL,
    nonce TEXT NOT NULL,
    request_path TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (caller_service, nonce)
);

CREATE INDEX idx_taxi_service_nonces_expiry ON taxi_service_nonces(expires_at);

-- Binding changes are security events, not ordinary profile edits. Enforce
-- invalidation below every application path so a future recovery/admin flow
-- cannot accidentally leave an old provider or receiver credential usable.
CREATE OR REPLACE FUNCTION invalidate_taxi_goods_challenges_on_binding_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.provider_id IS DISTINCT FROM NEW.provider_id THEN
        UPDATE taxi_goods_handover_challenges
           SET status='INVALIDATED',invalidated_reason='PROVIDER_REASSIGNED',updated_at=now()
         WHERE mission_id=NEW.id AND status='ACTIVE';
    ELSIF OLD.receiver_ref IS DISTINCT FROM NEW.receiver_ref THEN
        UPDATE taxi_goods_handover_challenges
           SET status='INVALIDATED',invalidated_reason='RECEIVER_CHANGED',updated_at=now()
         WHERE mission_id=NEW.id AND stage='DELIVERY' AND status='ACTIVE';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER taxi_goods_mission_binding_challenge_invalidation
AFTER UPDATE OF provider_id,receiver_ref ON taxi_goods_missions
FOR EACH ROW EXECUTE FUNCTION invalidate_taxi_goods_challenges_on_binding_change();
