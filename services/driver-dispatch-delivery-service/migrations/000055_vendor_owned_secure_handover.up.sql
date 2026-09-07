-- Section 32: authoritative secure custody for vendor-owned physical delivery.
-- Vendor Commerce owns neither this state machine nor any credential digest.
CREATE TABLE dispatch_vendor_delivery_missions (
    mission_ref text PRIMARY KEY,
    fulfilment_ref text NOT NULL UNIQUE,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    provider_ref text NOT NULL,
    receiver_ref text NOT NULL,
    receiver_phone text NOT NULL DEFAULT '',
    manifest_ref text NOT NULL,
    manifest_version bigint NOT NULL CHECK (manifest_version > 0),
    parcel_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(parcel_refs) = 'array'),
    status text NOT NULL CHECK (status IN ('ASSIGNED','PICKUP_REQUIRED','PICKED_UP','DELIVERY_REQUIRED','DELIVERED','COMPLETED','CANCELLED','FAILED')),
    source_version bigint NOT NULL DEFAULT 1 CHECK (source_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (length(btrim(mission_ref)) > 0 AND length(btrim(fulfilment_ref)) > 0),
    CHECK (length(btrim(business_id)) > 0 AND length(btrim(branch_id)) > 0),
    CHECK (length(btrim(provider_ref)) > 0 AND length(btrim(receiver_ref)) > 0),
    CHECK (length(btrim(manifest_ref)) > 0)
);

CREATE TABLE dispatch_vendor_handover_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_ref text NOT NULL UNIQUE,
    mission_ref text NOT NULL REFERENCES dispatch_vendor_delivery_missions(mission_ref) ON DELETE RESTRICT,
    stage text NOT NULL CHECK (stage IN ('PICKUP','DELIVERY')),
    version bigint NOT NULL CHECK (version > 0),
    provider_ref text NOT NULL,
    receiver_ref text NOT NULL,
    manifest_ref text NOT NULL,
    manifest_version bigint NOT NULL CHECK (manifest_version > 0),
    otp_keyed_hash text NOT NULL,
    qr_keyed_hash text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CONSUMED','INVALIDATED','EXPIRED','LOCKED')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts integer NOT NULL CHECK (max_attempts > 0),
    resend_count integer NOT NULL DEFAULT 0 CHECK (resend_count >= 0),
    max_resends integer NOT NULL CHECK (max_resends >= 0),
    resend_cooldown_until timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    invalidated_at timestamptz,
    invalidated_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (otp_keyed_hash <> qr_keyed_hash),
    CHECK ((status = 'CONSUMED') = (consumed_at IS NOT NULL))
);

CREATE UNIQUE INDEX dispatch_vendor_one_active_stage
    ON dispatch_vendor_handover_challenges(mission_ref, stage) WHERE status='ACTIVE';
CREATE UNIQUE INDEX dispatch_vendor_challenge_generation
    ON dispatch_vendor_handover_challenges(mission_ref, stage, version);

CREATE TABLE dispatch_vendor_handover_operations (
    caller text NOT NULL,
    operation_ref text NOT NULL,
    request_fingerprint text NOT NULL,
    mission_ref text NOT NULL REFERENCES dispatch_vendor_delivery_missions(mission_ref) ON DELETE RESTRICT,
    challenge_ref text NOT NULL,
    result_status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(caller, operation_ref),
    CHECK (length(btrim(caller)) > 0 AND length(btrim(operation_ref)) > 0)
);

COMMENT ON TABLE dispatch_vendor_handover_challenges IS
    'Dispatch-owned VendorMission custody challenges. Contains keyed hashes only; plaintext OTP and raw QR are forbidden.';
