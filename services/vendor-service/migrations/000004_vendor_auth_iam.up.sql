-- Section 5: Authentication, Sessions and Staff IAM.
--
-- Two distinct security domains, deliberately never sharing a table, key
-- namespace, or challenge: Vendor AUTHENTICATION (this migration) vs the
-- platform's logistics pickup/delivery custody handover (owned elsewhere,
-- PENDING_EXTERNAL_DEPENDENCY per docs/architecture/07). vendor_otp_challenges
-- exists only for login and step-up purposes and can never confirm a delivery.
--
-- vendor_id and branch_id throughout are opaque text references, not foreign
-- keys: Section 6 (Business/Brand/Branch) does not exist yet. This migration
-- builds the scope infrastructure those future tables will populate, per the
-- architecture's "safe nullable references for future Section 6
-- relationships" instruction. No cross-service foreign keys either.

CREATE TABLE IF NOT EXISTS vendor_users (
    id text PRIMARY KEY,
    phone text,
    email text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vendor_users_identifier_present CHECK (phone IS NOT NULL OR email IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_users_phone_unique ON vendor_users (phone) WHERE phone IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS vendor_users_email_unique ON vendor_users (email) WHERE email IS NOT NULL;

-- vendor_staff binds a vendor_user to one vendor business (vendor_id). A user
-- may be staff at more than one vendor business (one row each).
CREATE TABLE IF NOT EXISTS vendor_staff (
    id text PRIMARY KEY,
    vendor_id text NOT NULL,
    user_id text NOT NULL REFERENCES vendor_users (id),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    suspended_at timestamptz,
    suspended_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_staff_vendor_user_unique ON vendor_staff (vendor_id, user_id);
CREATE INDEX IF NOT EXISTS vendor_staff_vendor_idx ON vendor_staff (vendor_id);
CREATE INDEX IF NOT EXISTS vendor_staff_user_idx ON vendor_staff (user_id);

-- vendor_otp_challenges: AUTHENTICATION OTP only (purpose IN login, step_up).
-- otp_hash is an HMAC over (challenge id, identifier key, code) via
-- shared/go/auth.HashOTP — the raw code is never persisted. A resend
-- overwrites otp_hash/expires_at/generation in place so the previous code
-- becomes immediately unusable without allocating a second concurrently-valid
-- row.
CREATE TABLE IF NOT EXISTS vendor_otp_challenges (
    id text PRIMARY KEY,
    purpose text NOT NULL CHECK (purpose IN ('login', 'step_up')),
    identifier_type text NOT NULL CHECK (identifier_type IN ('phone', 'email')),
    identifier_value text NOT NULL,
    identifier_key text NOT NULL,
    otp_hash text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed', 'expired', 'locked')),
    attempts integer NOT NULL DEFAULT 0,
    resend_count integer NOT NULL DEFAULT 0,
    generation integer NOT NULL DEFAULT 1,
    -- step_up-only binding: which already-authenticated session/staff/vendor
    -- this elevation is for, and what operation it will unlock. NULL for
    -- purpose='login' (no session exists yet at login time).
    session_id text,
    staff_id text,
    vendor_id text,
    step_up_purpose text,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    locked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vendor_otp_challenges_step_up_binding CHECK (
        (purpose = 'login' AND session_id IS NULL AND staff_id IS NULL AND step_up_purpose IS NULL)
        OR (purpose = 'step_up' AND session_id IS NOT NULL AND staff_id IS NOT NULL AND vendor_id IS NOT NULL AND step_up_purpose IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS vendor_otp_challenges_identifier_active_idx
    ON vendor_otp_challenges (identifier_key, purpose)
    WHERE status = 'active';
CREATE INDEX IF NOT EXISTS vendor_otp_challenges_expiry_idx ON vendor_otp_challenges (expires_at) WHERE status = 'active';

-- vendor_sessions: revocable refresh sessions. Each session is anchored to
-- exactly one vendor_staff row, which transitively fixes the session's
-- vendor_id/branch scope — request bodies are never trusted for tenant scope
-- (see docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md).
CREATE TABLE IF NOT EXISTS vendor_sessions (
    id text PRIMARY KEY,
    staff_id text NOT NULL REFERENCES vendor_staff (id),
    refresh_token_hash text NOT NULL,
    device_id text,
    user_agent text,
    ip_address text,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE INDEX IF NOT EXISTS vendor_sessions_staff_idx ON vendor_sessions (staff_id);
CREATE INDEX IF NOT EXISTS vendor_sessions_active_expiry_idx ON vendor_sessions (expires_at) WHERE revoked_at IS NULL;

-- vendor_step_up_grants: the result of a verified step_up OTP. A short-lived,
-- single-purpose elevation, bound to session+actor+vendor+purpose and
-- consumed exactly once.
CREATE TABLE IF NOT EXISTS vendor_step_up_grants (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES vendor_sessions (id),
    staff_id text NOT NULL REFERENCES vendor_staff (id),
    vendor_id text NOT NULL,
    purpose text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed', 'expired', 'revoked')),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS vendor_step_up_grants_session_idx ON vendor_step_up_grants (session_id, purpose) WHERE status = 'active';

-- vendor_permissions: global capability catalogue (never vendor-scoped).
CREATE TABLE IF NOT EXISTS vendor_permissions (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    description text NOT NULL,
    category text NOT NULL,
    is_high_risk boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- vendor_roles: vendor_id NULL = a system role template (the 11 default role
-- families); vendor_id set = a vendor's own custom role.
CREATE TABLE IF NOT EXISTS vendor_roles (
    id text PRIMARY KEY,
    vendor_id text,
    code text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_system boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_roles_system_code_unique ON vendor_roles (code) WHERE vendor_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS vendor_roles_vendor_code_unique ON vendor_roles (vendor_id, code) WHERE vendor_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS vendor_role_permissions (
    role_id text NOT NULL REFERENCES vendor_roles (id),
    permission_id text NOT NULL REFERENCES vendor_permissions (id),
    PRIMARY KEY (role_id, permission_id)
);

-- vendor_staff_role_assignments: branch_id NULL = vendor-wide grant of the
-- role; branch_id set = the grant applies only within that branch. Soft
-- revocation (revoked_at) preserves assignment history for audit.
CREATE TABLE IF NOT EXISTS vendor_staff_role_assignments (
    id text PRIMARY KEY,
    staff_id text NOT NULL REFERENCES vendor_staff (id),
    role_id text NOT NULL REFERENCES vendor_roles (id),
    branch_id text,
    assigned_by text,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    revoked_by text
);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_staff_role_assignments_active_unique
    ON vendor_staff_role_assignments (staff_id, role_id, COALESCE(branch_id, ''))
    WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS vendor_staff_role_assignments_staff_idx ON vendor_staff_role_assignments (staff_id) WHERE revoked_at IS NULL;

-- vendor_audit_log: real persistence for internal/audit.Recorder. Append-only
-- in practice (no UPDATE/DELETE usecase exists); never stores raw OTP,
-- tokens, or step-up secrets.
CREATE TABLE IF NOT EXISTS vendor_audit_log (
    id text PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    vendor_id text,
    actor_type text NOT NULL,
    actor_id text,
    action text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    branch_id text,
    correlation_id text,
    before_data jsonb,
    after_data jsonb,
    reason text
);

CREATE INDEX IF NOT EXISTS vendor_audit_log_vendor_idx ON vendor_audit_log (vendor_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS vendor_audit_log_aggregate_idx ON vendor_audit_log (aggregate_type, aggregate_id);
