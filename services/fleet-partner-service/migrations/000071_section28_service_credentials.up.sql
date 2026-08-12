-- Section 28: service-to-service credential lifecycle and route/method/
-- event scoping, layered on top of the existing shared/go/serviceauth HMAC
-- signature mechanism (untouched -- this table never stores a secret; see
-- below) and internal/platform/noncestore's replay protection (also
-- untouched).
--
-- Today every internal-route handler group (documents, finance, marketplace,
-- backgroundops/worker-operations) enforces its OWN caller allowlist
-- (FLEET_*_ALLOWED_CALLERS), independently, with no route/method
-- granularity: a caller in the allowlist may call EVERY route that handler
-- group registers, forever, with no expiry or revocation short of an
-- operator editing an env var and redeploying. This table adds a single,
-- closed, versioned, auditable, rotatable declaration of what a caller
-- may actually do, checked centrally (internal/platform/serviceauth,
-- internal/middleware's new scope-enforcement middleware) rather than
-- scattered per-handler.
--
-- The secret itself is deliberately NEVER a column here. A credential row
-- is pure metadata: identity (service_name, version), lifecycle
-- (active_from/expires_at/revoked_at), scope (allowed_methods,
-- allowed_route_patterns, allowed_event_types) and last_used_at. The actual
-- HMAC secret bytes continue to live only in configuration
-- (FLEET_PARTNER_SERVICE_SECRETS, extended by
-- internal/platform/serviceauth.ParseVersionedSecrets to accept an optional
-- "serviceName:vN=secret" form; a bare "serviceName=secret" entry is
-- version 1, preserving every existing deployment's configuration
-- unchanged) -- so this migration cannot leak a secret no matter how the
-- table is read, backed up, or replicated, and satisfies "no keys in the
-- database" for this credential system without waiting on the general
-- encryption-at-rest work.
--
-- allowed_route_patterns entries are route identifiers exactly as
-- internal/platform/riskregistry already encodes them ("METHOD /path/with/
-- :params"), the same closed table this session's rate-limit classification
-- work already built and keeps in sync with what is actually mounted
-- (TestRouteRiskRegistry_CoversEveryMountedRoute) -- reused here rather
-- than inventing a second, parallel route-identifier scheme. A single
-- literal "*" entry in allowed_route_patterns or allowed_methods means "any
-- route"/"any method" -- an explicit, closed-registry-sanctioned wildcard
-- token, never a substring or prefix match. Existing callers are seeded
-- wildcard-scoped below so this migration changes zero enforced behavior on
-- its own; narrowing a specific caller's scope is a deliberate follow-up
-- rotation action through the new administration usecase, not something
-- this migration decides on an operator's behalf.
CREATE TABLE IF NOT EXISTS fleet_partner_service_credentials (
    id                      text PRIMARY KEY,
    service_name            text NOT NULL,
    version                 integer NOT NULL CHECK (version >= 1),
    allowed_methods         jsonb NOT NULL DEFAULT '[]'::jsonb,
    allowed_route_patterns  jsonb NOT NULL DEFAULT '[]'::jsonb,
    allowed_event_types     jsonb NOT NULL DEFAULT '[]'::jsonb,
    active_from             timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz,
    revoked_at              timestamptz,
    revoked_reason          text,
    last_used_at            timestamptz,
    created_by              text NOT NULL,
    version_note            text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (service_name, version)
);

-- The hot lookup path (InternalServiceAuth resolving candidate credentials
-- for a caller name) only ever wants live rows -- not revoked, and only
-- rarely more than a handful of versions deep -- so the partial index
-- carries the whole table's realistic working set.
CREATE INDEX IF NOT EXISTS idx_fleet_partner_service_credentials_lookup
    ON fleet_partner_service_credentials (service_name, version DESC)
    WHERE revoked_at IS NULL;

-- fleet_partner_forbid_mutation (migration 000001) intentionally does NOT
-- cover this table: unlike audit records, a credential's last_used_at must
-- update on every successful call, and revocation/expiry are legitimate
-- in-place transitions of a row's own lifecycle, not falsification of
-- history. Every meaningful transition (create/rotate/revoke) is instead
-- separately recorded through the existing append-only audit log
-- (internal/platform/audit) by the administration usecase -- see
-- internal/platform/serviceauth's own doc comment.

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('security.service_credentials.read', 'security', 'Read service credential metadata (never raw secrets)', true),
    ('security.service_credentials.manage', 'security', 'Create, rotate and revoke service credentials', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

-- Deliberately NOT granted to any organization role below (unlike every
-- other Section 28 permission this session added): service credentials are
-- a platform-wide concern with no organization_id on the row at all, not a
-- per-tenant one, so gating them behind a tenant role binding would be the
-- wrong authorization model entirely -- see internal/platform/serviceauth's
-- doc comment for the administration surface's actual authorization
-- boundary (platform-operator, not organization-scoped).

-- Seed rows for every caller name this codebase's OWN internal route
-- registrations currently reference (internal/config/config.go's
-- *AllowedCallers defaults, cmd/marketplace_routes.go): wildcard-scoped
-- (allowed_methods=["*"], allowed_route_patterns=["*"]) so this migration
-- changes zero enforced behavior for any already-configured deployment --
-- exactly today's actual behavior (any caller in a handler group's
-- allowlist may call any route that group registers). Narrowing these to
-- the caller's real, minimal route set is real, disclosed follow-up work
-- through the rotation administration usecase, not silently done here.
INSERT INTO fleet_partner_service_credentials
    (id, service_name, version, allowed_methods, allowed_route_patterns, allowed_event_types,
     created_by, version_note)
VALUES
    ('svccred_media_file_service_v1', 'media-file-service', 1,
     '["*"]'::jsonb, '["*"]'::jsonb, '[]'::jsonb,
     'migration_000071', 'Grandfathered from FLEET_DOCUMENTS_ALLOWED_CALLERS default at introduction of scoped credentials.'),
    ('svccred_payment_wallet_service_v1', 'payment-wallet-service', 1,
     '["*"]'::jsonb, '["*"]'::jsonb, '["finance.operation.posted","finance.projection.updated"]'::jsonb,
     'migration_000071', 'Grandfathered from FLEET_FINANCE_ALLOWED_CALLERS default at introduction of scoped credentials.'),
    ('svccred_driver_dispatch_delivery_service_v1', 'driver-dispatch-delivery-service', 1,
     '["*"]'::jsonb, '["*"]'::jsonb, '[]'::jsonb,
     'migration_000071', 'Grandfathered from marketplace/documents-provider allowed-caller defaults at introduction of scoped credentials.'),
    ('svccred_driver_taxi_service_v1', 'driver-taxi-service', 1,
     '["*"]'::jsonb, '["*"]'::jsonb, '[]'::jsonb,
     'migration_000071', 'Grandfathered from documents-provider allowed-caller default at introduction of scoped credentials.'),
    ('svccred_driver_hauling_service_v1', 'driver-hauling-service', 1,
     '["*"]'::jsonb, '["*"]'::jsonb, '[]'::jsonb,
     'migration_000071', 'Grandfathered from marketplace/documents-provider allowed-caller defaults at introduction of scoped credentials.')
ON CONFLICT (service_name, version) DO NOTHING;
