-- Corporate passwordless authentication: identities, OTP challenges,
-- sessions, session history and security events.
--
-- Invariants enforced here:
--   * no password column of any kind exists;
--   * OTP codes are stored only as keyed digests (code_digest), never raw;
--   * refresh tokens are stored only as hashes;
--   * email uniqueness is case-insensitive (email_normalized is pre-lowered);
--   * phone is stored E.164 normalized.

CREATE TABLE IF NOT EXISTS fleet_partner_corporate_identities (
    id                          text PRIMARY KEY,
    email_normalized            text NOT NULL,
    email_display               text NOT NULL,
    phone_e164                  text NOT NULL,
    phone_display               text NOT NULL,
    status                      text NOT NULL DEFAULT 'PENDING'
                                CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'LOCKED', 'ARCHIVED')),
    email_verified_at           timestamptz,
    phone_bound_at              timestamptz,
    last_authenticated_at       timestamptz,
    failed_authentication_count integer NOT NULL DEFAULT 0,
    locked_until                timestamptz,
    version                     bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    archived_at                 timestamptz
);

-- Unique across identities that are not archived. Email is already lowered by
-- the application; the index is on the normalized column so comparison is
-- case-insensitive by construction.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_identity_email
    ON fleet_partner_corporate_identities (email_normalized)
    WHERE status <> 'ARCHIVED';

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_identity_phone
    ON fleet_partner_corporate_identities (phone_e164)
    WHERE status <> 'ARCHIVED';

CREATE TABLE IF NOT EXISTS fleet_partner_auth_otp_challenges (
    id                    text PRIMARY KEY,
    identity_id           text REFERENCES fleet_partner_corporate_identities (id) ON DELETE RESTRICT,
    email_normalized      text NOT NULL,
    email_hash            text NOT NULL,
    phone_e164            text NOT NULL,
    phone_hash            text NOT NULL,
    purpose               text NOT NULL
                          CHECK (purpose IN ('SIGN_UP_OR_LOGIN', 'STAFF_INVITATION', 'ORGANIZATION_SELECTION',
                                             'STEP_UP', 'CONTACT_CHANGE', 'PROVIDER_INVITATION')),
    status                text NOT NULL DEFAULT 'PENDING'
                          CHECK (status IN ('PENDING', 'VERIFIED', 'CONSUMED', 'EXPIRED', 'SUPERSEDED', 'LOCKED')),
    -- Keyed HMAC digest of the code. The raw OTP is never stored.
    code_digest           text NOT NULL,
    expires_at            timestamptz NOT NULL,
    resend_available_at   timestamptz NOT NULL DEFAULT now(),
    failed_attempts       integer NOT NULL DEFAULT 0,
    maximum_attempts      integer NOT NULL DEFAULT 5 CHECK (maximum_attempts >= 1),
    delivery_attempts     integer NOT NULL DEFAULT 0,
    delivery_status       text NOT NULL DEFAULT 'PENDING'
                          CHECK (delivery_status IN ('PENDING', 'ACCEPTED', 'FAILED', 'UNAVAILABLE')),
    notification_reference text,
    -- Context bindings for step-up and invitation purposes.
    session_id            text,
    organization_id       text,
    action_class          text,
    invitation_id         text,
    requested_ip_hash     text,
    user_agent_hash       text,
    request_id            text NOT NULL DEFAULT '',
    correlation_id        text NOT NULL DEFAULT '',
    verified_at           timestamptz,
    consumed_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_otp_lookup
    ON fleet_partner_auth_otp_challenges (email_hash, purpose, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_otp_expiry
    ON fleet_partner_auth_otp_challenges (expires_at)
    WHERE status = 'PENDING';

CREATE TABLE IF NOT EXISTS fleet_partner_corporate_sessions (
    id                   text PRIMARY KEY,
    identity_id          text NOT NULL REFERENCES fleet_partner_corporate_identities (id) ON DELETE RESTRICT,
    organization_user_id text,
    organization_id      text,
    session_family_id    text NOT NULL,
    -- HMAC hash of the opaque refresh token; the token itself is never stored.
    refresh_token_hash   text NOT NULL,
    status               text NOT NULL DEFAULT 'ACTIVE'
                         CHECK (status IN ('ACTIVE', 'REVOKED', 'EXPIRED', 'COMPROMISED')),
    access_issued_at     timestamptz NOT NULL DEFAULT now(),
    refresh_expires_at   timestamptz NOT NULL,
    last_seen_at         timestamptz NOT NULL DEFAULT now(),
    ip_hash              text,
    user_agent_hash      text,
    device_label         text,
    role_version         bigint NOT NULL DEFAULT 1,
    revoked_at           timestamptz,
    revocation_reason    text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_session_refresh_hash
    ON fleet_partner_corporate_sessions (refresh_token_hash);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_sessions_identity
    ON fleet_partner_corporate_sessions (identity_id, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_sessions_family
    ON fleet_partner_corporate_sessions (session_family_id);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_sessions_organization
    ON fleet_partner_corporate_sessions (organization_id, status)
    WHERE organization_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_sessions_expiry
    ON fleet_partner_corporate_sessions (refresh_expires_at)
    WHERE status = 'ACTIVE';

-- Append-only session lifecycle history.
CREATE TABLE IF NOT EXISTS fleet_partner_corporate_session_history (
    id             text PRIMARY KEY,
    session_id     text NOT NULL,
    identity_id    text NOT NULL,
    from_status    text NOT NULL,
    to_status      text NOT NULL,
    reason         text NOT NULL,
    request_id     text NOT NULL DEFAULT '',
    correlation_id text NOT NULL DEFAULT '',
    occurred_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_session_history_session
    ON fleet_partner_corporate_session_history (session_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_session_history_append_only ON fleet_partner_corporate_session_history;
CREATE TRIGGER trg_fleet_partner_session_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_corporate_session_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Append-only security events. Deliberately stores hashes, never raw
-- identifiers, so a leaked table cannot enumerate accounts.
CREATE TABLE IF NOT EXISTS fleet_partner_auth_security_events (
    id             text PRIMARY KEY,
    event_type     text NOT NULL,
    severity       text NOT NULL DEFAULT 'INFO'
                   CHECK (severity IN ('INFO', 'WARN', 'CRITICAL')),
    identity_id    text,
    organization_id text,
    email_hash     text,
    phone_hash     text,
    ip_hash        text,
    user_agent_hash text,
    detail         jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id     text NOT NULL DEFAULT '',
    correlation_id text NOT NULL DEFAULT '',
    occurred_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_security_events_type
    ON fleet_partner_auth_security_events (event_type, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_security_events_append_only ON fleet_partner_auth_security_events;
CREATE TRIGGER trg_fleet_partner_security_events_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_auth_security_events
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
