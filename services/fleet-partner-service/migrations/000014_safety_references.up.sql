-- WP6.2: fleet-side incident and dispute REFERENCES.
--
-- Fleet does not own incidents or disputes. The support-dispute-service does.
-- What an organization needs on this side is a durable, tenant-scoped pointer
-- saying "this assignment / trip / provider is connected to that case", so an
-- operator reading fleet can find the case and so fleet's own history records
-- that the connection was made.
--
-- Three boundaries are enforced by the schema itself rather than by
-- convention:
--
--   1. external_reference is opaque text with NO foreign key. A cross-service
--      foreign key would make fleet's writes depend on another service's
--      table, which is the coupling this architecture exists to avoid.
--   2. verification_state is NOT NULL and has no default. Every row must state
--      whether the authoritative service confirmed the reference, so an
--      unverified pointer can never be mistaken for a verified one.
--   3. History is append-only. A reference that was silently repointed is
--      worse than no reference at all.

CREATE TABLE IF NOT EXISTS fleet_partner_safety_references (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    reference_type         text NOT NULL CHECK (reference_type IN ('INCIDENT', 'DISPUTE')),
    -- The authoritative owner of the case. Recorded so a later reader knows
    -- which service to ask, rather than assuming.
    authority_service      text NOT NULL DEFAULT 'SUPPORT_DISPUTE',
    -- Opaque identifier owned by the authoritative service. Deliberately not a
    -- foreign key and deliberately not parsed.
    external_reference     text NOT NULL,

    -- Fleet-side associations. All optional individually; at least one must be
    -- present, because a reference attached to nothing cannot be acted on.
    assignment_id          text REFERENCES fleet_partner_provider_assignments (id) ON DELETE RESTRICT,
    request_projection_id  text REFERENCES fleet_partner_marketplace_request_projections (id) ON DELETE RESTRICT,
    trip_projection_id     text REFERENCES fleet_partner_trip_projections (id) ON DELETE RESTRICT,
    -- The canonical trip identifier as the execution service knows it, kept
    -- alongside the projection id so the reference survives a projection that
    -- has not arrived yet.
    platform_trip_id       text,
    executing_provider_id  text,
    vehicle_reference      text,

    -- Verification. UNVERIFIED means no authoritative endpoint exists for this
    -- reference type; VERIFICATION_UNAVAILABLE is never stored — a deployment
    -- that cannot verify a type that CAN be verified refuses the write instead.
    verification_state     text NOT NULL
                           CHECK (verification_state IN ('VERIFIED', 'UNVERIFIED')),
    verified_at            timestamptz,
    -- Projection of the authoritative status, only when it can be obtained
    -- safely. Left NULL rather than guessed.
    status_projection      text,
    status_projection_at   timestamptz,

    created_by_actor_type  text NOT NULL CHECK (created_by_actor_type IN ('ORGANIZATION_USER', 'PROVIDER', 'SYSTEM')),
    created_by_actor_id    text NOT NULL,
    correlation_id         text NOT NULL DEFAULT '',
    causation_id           text NOT NULL DEFAULT '',
    idempotency_key        text NOT NULL,
    request_fingerprint    text NOT NULL,
    version                bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),

    -- A reference that points at nothing on this side is unusable.
    CONSTRAINT chk_fleet_partner_safety_reference_association CHECK (
        assignment_id IS NOT NULL
        OR request_projection_id IS NOT NULL
        OR trip_projection_id IS NOT NULL
        OR platform_trip_id IS NOT NULL
        OR executing_provider_id IS NOT NULL
    ),
    -- One record per external case per organization. A second attempt to
    -- attach the same case to a DIFFERENT assignment is a conflicting
    -- reference, and is rejected rather than duplicated.
    CONSTRAINT uq_fleet_partner_safety_reference
        UNIQUE (organization_id, reference_type, authority_service, external_reference)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_safety_reference_idempotency
    ON fleet_partner_safety_references (organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_safety_reference_org
    ON fleet_partner_safety_references (organization_id, reference_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_safety_reference_assignment
    ON fleet_partner_safety_references (organization_id, assignment_id)
    WHERE assignment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_safety_reference_trip
    ON fleet_partner_safety_references (organization_id, platform_trip_id)
    WHERE platform_trip_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_safety_reference_provider
    ON fleet_partner_safety_references (organization_id, executing_provider_id)
    WHERE executing_provider_id IS NOT NULL;

-- Immutable history of everything that happened to a reference.
CREATE TABLE IF NOT EXISTS fleet_partner_safety_reference_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    reference_id    text NOT NULL,
    action          text NOT NULL CHECK (action IN ('CREATED', 'STATUS_PROJECTED', 'VERIFIED', 'DETACHED')),
    from_state      text NOT NULL DEFAULT '',
    to_state        text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_safety_reference_history
    ON fleet_partner_safety_reference_history (organization_id, reference_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_safety_reference_history_append_only
    ON fleet_partner_safety_reference_history;
CREATE TRIGGER trg_fleet_partner_safety_reference_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_safety_reference_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Permissions. Reading and attaching a case reference are separate
-- capabilities: an operator who may look up an incident is not automatically
-- someone who may assert that one exists.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('safety.references.read',   'safety', 'Read incident and dispute references held by the organization', true),
    ('safety.references.manage', 'safety', 'Attach an incident or dispute reference to organization work', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('safety.references.read', 'safety.references.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin', 'safety.references.read'),
    ('rol_system_admin', 'safety.references.manage'),
    ('rol_system_operations', 'safety.references.read'),
    ('rol_system_operations', 'safety.references.manage'),
    ('rol_system_dispatcher', 'safety.references.read'),
    ('rol_system_dispatcher', 'safety.references.manage'),
    ('rol_system_fleet', 'safety.references.read'),
    -- Compliance reads cases but does not create the operational link.
    ('rol_system_compliance', 'safety.references.read')
ON CONFLICT DO NOTHING;
