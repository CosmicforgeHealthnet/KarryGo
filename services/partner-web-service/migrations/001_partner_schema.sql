-- 001_partner_schema.sql
-- Partner API schema owned by api-gateway.
-- api-gateway owns its own PostgreSQL database (API_GATEWAY_DATABASE_URL).
-- This migration only creates schema. No partner routes, auth, payment, webhook
-- delivery, rate limiting, or idempotency service logic is implemented here.

-- gen_random_uuid() is provided by pgcrypto (core since PG13, extension before).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Table 1 — partner_accounts
CREATE TABLE IF NOT EXISTS partner_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_name text NOT NULL,
    business_email text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'pending')),
    plan text NOT NULL DEFAULT 'sandbox' CHECK (plan IN ('sandbox', 'live')),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Table 2 — partner_api_keys
-- Security: never store plaintext API keys. Only key_hash (bcrypt, later) and
-- key_prefix are persisted. The full key is shown once at creation in a later phase.
CREATE TABLE IF NOT EXISTS partner_api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    environment text NOT NULL CHECK (environment IN ('sandbox', 'live')),
    label text,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_partner_api_keys_key_prefix ON partner_api_keys(key_prefix);
CREATE INDEX IF NOT EXISTS idx_partner_api_keys_partner_id ON partner_api_keys(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_api_keys_status ON partner_api_keys(status);

-- Table 3 — partner_bookings
-- Idempotency is partner-scoped (UNIQUE(partner_id, idempotency_key)), not global:
-- different partners may reuse the same idempotency key string.
CREATE TABLE IF NOT EXISTS partner_bookings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_ref text NOT NULL UNIQUE,
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    external_ref text,
    pickup_address text NOT NULL,
    pickup_lat numeric(10,7) NOT NULL,
    pickup_lng numeric(10,7) NOT NULL,
    dest_address text NOT NULL,
    dest_lat numeric(10,7) NOT NULL,
    dest_lng numeric(10,7) NOT NULL,
    receiver_name text NOT NULL,
    receiver_phone text NOT NULL,
    package_type text CHECK (package_type IN ('documents', 'food', 'electronics', 'clothing', 'medical', 'other')),
    package_size text CHECK (package_size IN ('small', 'medium', 'large')),
    is_fragile boolean NOT NULL DEFAULT false,
    service_tier text NOT NULL DEFAULT 'standard' CHECK (service_tier IN ('standard', 'express')),
    fare_amount_kobo bigint NOT NULL,
    currency text NOT NULL DEFAULT 'NGN',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatching', 'accepted', 'in_progress', 'completed', 'cancelled', 'failed', 'no_provider')),
    payment_intent_id text,
    dispatch_booking_id uuid,
    idempotency_key text NOT NULL,
    event_published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (partner_id, idempotency_key)
);

-- booking_ref is already indexed by its UNIQUE constraint, so no separate index.
CREATE INDEX IF NOT EXISTS idx_partner_bookings_partner_id ON partner_bookings(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_bookings_dispatch_booking_id ON partner_bookings(dispatch_booking_id);
CREATE INDEX IF NOT EXISTS idx_partner_bookings_status ON partner_bookings(status);
CREATE INDEX IF NOT EXISTS idx_partner_bookings_external_ref ON partner_bookings(external_ref);
CREATE INDEX IF NOT EXISTS idx_partner_bookings_partner_external_ref ON partner_bookings(partner_id, external_ref);

-- Table 4 — partner_webhooks
-- GW-G decision: signing_secret stores AES-GCM ENCRYPTED secret material, NOT a
-- bcrypt hash. A hash cannot be recovered to sign outgoing webhook payloads, so
-- the secret is encrypted at rest (key derived from WEBHOOK_SIGNING_SECRET) and
-- decrypted in memory only to compute the HMAC. Plaintext is shown once at
-- creation and never stored or logged.
CREATE TABLE IF NOT EXISTS partner_webhooks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    url text NOT NULL,
    signing_secret text NOT NULL, -- AES-GCM encrypted; never plaintext
    events text[] NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_partner_webhooks_partner_id ON partner_webhooks(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_webhooks_status ON partner_webhooks(status);

-- Table 5 — partner_webhook_deliveries
CREATE TABLE IF NOT EXISTS partner_webhook_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id uuid NOT NULL REFERENCES partner_webhooks(id) ON DELETE CASCADE,
    booking_id uuid NOT NULL REFERENCES partner_bookings(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    attempts int NOT NULL DEFAULT 0,
    last_attempt_at timestamptz,
    next_attempt_at timestamptz,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed_permanent')),
    response_status int,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_partner_webhook_deliveries_webhook_id ON partner_webhook_deliveries(webhook_id);
CREATE INDEX IF NOT EXISTS idx_partner_webhook_deliveries_booking_id ON partner_webhook_deliveries(booking_id);
CREATE INDEX IF NOT EXISTS idx_partner_webhook_deliveries_status ON partner_webhook_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_partner_webhook_deliveries_next_attempt_at ON partner_webhook_deliveries(next_attempt_at);

-- Table 6 — partner_idempotency_keys
-- key is later stored as the composite string "partner_id:idempotency_key".
-- partner_id is kept as its own column for partner isolation and cleanup.
CREATE TABLE IF NOT EXISTS partner_idempotency_keys (
    key text PRIMARY KEY,
    partner_id uuid NOT NULL REFERENCES partner_accounts(id) ON DELETE CASCADE,
    request_hash text,
    response jsonb NOT NULL,
    status_code int NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_partner_idempotency_keys_partner_id ON partner_idempotency_keys(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_idempotency_keys_expires_at ON partner_idempotency_keys(expires_at);
