-- 002_partner_auth.sql
-- Partner dashboard authentication: password login + browser sessions.
-- Additive and safe for existing partner_accounts data.

-- Password hash for dashboard (email + password) login. Existing rows default to
-- an empty hash (login is impossible until a password is set); bcrypt hashes are
-- stored here, never plaintext.
ALTER TABLE partner_accounts ADD COLUMN IF NOT EXISTS password_hash text NOT NULL DEFAULT '';

-- Widen the status CHECK to include 'disabled' (keeps existing 'active',
-- 'suspended', 'pending') so internal status updates can disable an account.
ALTER TABLE partner_accounts DROP CONSTRAINT IF EXISTS partner_accounts_status_check;
ALTER TABLE partner_accounts ADD CONSTRAINT partner_accounts_status_check
    CHECK (status IN ('active', 'suspended', 'pending', 'disabled'));

-- Partner dashboard sessions. Only the SHA-256 hash of the session JWT is stored
-- (token_hash) — never the raw JWT.
CREATE TABLE IF NOT EXISTS partner_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id),
    token_hash text NOT NULL UNIQUE,
    ip_address text,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_sessions_partner ON partner_sessions(partner_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON partner_sessions(token_hash);
