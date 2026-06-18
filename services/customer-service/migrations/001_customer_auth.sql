CREATE TABLE IF NOT EXISTS customers (
    id uuid PRIMARY KEY,
    phone text UNIQUE,
    email text UNIQUE,
    first_name text,
    last_name text,
    onboarding_status text NOT NULL DEFAULT 'profile_required',
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (phone IS NOT NULL OR email IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS customer_sessions (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    refresh_token_hash text NOT NULL,
    device_id text,
    user_agent text NOT NULL DEFAULT '',
    ip_address text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_customer_sessions_customer_id ON customer_sessions(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_active ON customer_sessions(customer_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS customer_auth_events (
    id bigserial PRIMARY KEY,
    customer_id uuid REFERENCES customers(id) ON DELETE SET NULL,
    phone text,
    email text,
    event_type text NOT NULL,
    ip_address text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_customer_auth_events_customer_id ON customer_auth_events(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_auth_events_phone ON customer_auth_events(phone);
CREATE INDEX IF NOT EXISTS idx_customer_auth_events_email ON customer_auth_events(email);
