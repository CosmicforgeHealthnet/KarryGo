-- Section 55: organization API clients and outbound webhooks.
-- Additive only: credentials are hashed/encrypted; no plaintext secret column exists.

CREATE TABLE IF NOT EXISTS fleet_partner_organization_api_clients (
    id                              text PRIMARY KEY,
    organization_id                 text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    name                            text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    client_id                       text NOT NULL UNIQUE,
    secret_hash                     bytea NOT NULL,
    secret_version                  integer NOT NULL DEFAULT 1 CHECK (secret_version > 0),
    previous_secret_hash            bytea,
    previous_secret_version         integer,
    previous_secret_valid_until     timestamptz,
    scopes                          text[] NOT NULL CHECK (cardinality(scopes) > 0),
    ip_allowlist                    cidr[] NOT NULL DEFAULT '{}',
    status                          text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','REVOKED')),
    expires_at                      timestamptz,
    created_by                      text NOT NULL,
    last_used_at                    timestamptz,
    rotated_at                      timestamptz,
    revoked_at                      timestamptz,
    version                         bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                      timestamptz NOT NULL DEFAULT now(),
    updated_at                      timestamptz NOT NULL DEFAULT now(),
    CHECK ((previous_secret_hash IS NULL AND previous_secret_version IS NULL AND previous_secret_valid_until IS NULL)
        OR (previous_secret_hash IS NOT NULL AND previous_secret_version IS NOT NULL AND previous_secret_valid_until IS NOT NULL)),
    CHECK ((status = 'ACTIVE' AND revoked_at IS NULL) OR (status = 'REVOKED' AND revoked_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_api_clients_org_status
    ON fleet_partner_organization_api_clients(organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_api_clients_expiry
    ON fleet_partner_organization_api_clients(expires_at)
    WHERE status = 'ACTIVE' AND expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_webhook_subscriptions (
    id                                  text PRIMARY KEY,
    organization_id                     text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    endpoint_url                        text NOT NULL,
    event_types                         text[] NOT NULL CHECK (cardinality(event_types) > 0),
    secret_ciphertext                   bytea NOT NULL,
    secret_version                      integer NOT NULL DEFAULT 1 CHECK (secret_version > 0),
    previous_secret_ciphertext          bytea,
    previous_secret_version             integer,
    previous_secret_valid_until         timestamptz,
    status                              text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DISABLED')),
    failure_count                       integer NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    disabled_at                         timestamptz,
    created_by                          text NOT NULL,
    version                             bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                          timestamptz NOT NULL DEFAULT now(),
    updated_at                          timestamptz NOT NULL DEFAULT now(),
    CHECK ((previous_secret_ciphertext IS NULL AND previous_secret_version IS NULL AND previous_secret_valid_until IS NULL)
        OR (previous_secret_ciphertext IS NOT NULL AND previous_secret_version IS NOT NULL AND previous_secret_valid_until IS NOT NULL)),
    CHECK ((status = 'ACTIVE' AND disabled_at IS NULL) OR (status = 'DISABLED' AND disabled_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_webhooks_org_status
    ON fleet_partner_webhook_subscriptions(organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_webhooks_event_types
    ON fleet_partner_webhook_subscriptions USING gin(event_types);

CREATE TABLE IF NOT EXISTS fleet_partner_webhook_deliveries (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    subscription_id             text NOT NULL REFERENCES fleet_partner_webhook_subscriptions(id) ON DELETE RESTRICT,
    source_event_id             text NOT NULL,
    event_type                  text NOT NULL,
    attempt_no                  integer NOT NULL DEFAULT 1 CHECK (attempt_no > 0),
    status                      text NOT NULL DEFAULT 'PENDING'
                                CHECK (status IN ('PENDING','DELIVERING','RETRY_SCHEDULED','SUCCEEDED','FAILED','DEAD_LETTERED')),
    response_status             integer,
    response_body_redacted      text,
    scheduled_at                timestamptz NOT NULL DEFAULT now(),
    delivered_at                timestamptz,
    next_attempt_at             timestamptz,
    error_class                 text,
    error_code                  text,
    correlation_id              text NOT NULL,
    job_id                      text,
    replay_of_delivery_id       text REFERENCES fleet_partner_webhook_deliveries(id) ON DELETE RESTRICT,
    version                     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(subscription_id, source_event_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_webhook_delivery_claim
    ON fleet_partner_webhook_deliveries(status, next_attempt_at, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_webhook_delivery_subscription
    ON fleet_partner_webhook_deliveries(organization_id, subscription_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_webhook_delivery_event
    ON fleet_partner_webhook_deliveries(subscription_id, source_event_id);

-- Idempotency metadata for secret-returning commands. The result deliberately
-- contains only a resource reference; secret-bearing response bodies are never
-- persisted in the generic HTTP idempotency cache.
CREATE TABLE IF NOT EXISTS fleet_partner_integration_commands (
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    command_type        text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    resource_id         text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(organization_id, command_type, idempotency_key)
);

-- Durable fanout checkpoint: a committed outbox event is expanded into tenant
-- webhook deliveries exactly once; delivery itself remains at-least-once.
CREATE TABLE IF NOT EXISTS fleet_partner_webhook_fanout_events (
    source_event_id     text PRIMARY KEY,
    organization_id     text NOT NULL,
    event_type          text NOT NULL,
    processed_at        timestamptz NOT NULL DEFAULT now()
);

INSERT INTO fleet_partner_permissions(code, category, description, is_active) VALUES
    ('integrations.read', 'integrations', 'Read organization API client and webhook metadata', true),
    ('integrations.manage', 'integrations', 'Create, rotate and revoke organization API clients', true),
    ('webhooks.manage', 'integrations', 'Create, update, rotate, disable and replay organization webhooks', true)
ON CONFLICT(code) DO UPDATE SET category=EXCLUDED.category, description=EXCLUDED.description, is_active=true;

INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code IN ('integrations.read','integrations.manage','webhooks.manage')
ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_admin', code FROM fleet_partner_permissions
WHERE code IN ('integrations.read','integrations.manage','webhooks.manage')
ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('integrations.read','webhooks.manage')
ON CONFLICT DO NOTHING;
