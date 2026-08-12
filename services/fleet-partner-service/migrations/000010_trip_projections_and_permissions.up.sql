-- Sections 11–12: organization-facing trip projections, projection gap and
-- reconciliation tracking, operating mode, and the marketplace permissions.
--
-- Canonical trip state is owned by the execution service. These tables are
-- read models updated only from source events or reconciliation — never by a
-- corporate user action.

CREATE TABLE IF NOT EXISTS fleet_partner_trip_projections (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    source_service         text NOT NULL CHECK (source_service IN ('DISPATCH', 'TAXI', 'HAULING')),
    platform_trip_id       text NOT NULL,
    platform_request_id    text,
    platform_booking_id    text,
    executing_provider_id  text NOT NULL,
    organization_membership_id text,
    settlement_organization_id text,
    assignment_id          text REFERENCES fleet_partner_provider_assignments (id) ON DELETE RESTRICT,
    service_type           text NOT NULL DEFAULT '',
    -- Canonical status string as reported by the owning service. It is
    -- deliberately not constrained here: each execution service owns its own
    -- trip vocabulary and this is a projection, not the source of truth.
    trip_status            text NOT NULL,
    source_version         bigint NOT NULL DEFAULT 0,
    last_event_id          text,
    last_event_at          timestamptz,
    last_reconciled_at     timestamptz,
    freshness_state        text NOT NULL DEFAULT 'CURRENT'
                           CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'RECONCILING', 'UNKNOWN')),
    completed_at           timestamptz,
    cancelled_at           timestamptz,
    version                bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_trip_projection
        UNIQUE (organization_id, source_service, platform_trip_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_trip_projections_org
    ON fleet_partner_trip_projections (organization_id, trip_status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_trip_projections_provider
    ON fleet_partner_trip_projections (organization_id, executing_provider_id);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_trip_projections_stale
    ON fleet_partner_trip_projections (freshness_state, last_event_at)
    WHERE freshness_state <> 'CURRENT';

CREATE TABLE IF NOT EXISTS fleet_partner_trip_projection_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    trip_projection_id text NOT NULL,
    from_status     text NOT NULL,
    to_status       text NOT NULL,
    source_version  bigint NOT NULL DEFAULT 0,
    source_event_id text,
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_trip_history
    ON fleet_partner_trip_projection_history (organization_id, trip_projection_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_trip_history_append_only ON fleet_partner_trip_projection_history;
CREATE TRIGGER trg_fleet_partner_trip_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_trip_projection_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Detected source-version gaps. While a material gap is open, state-changing
-- commands on the affected aggregate are blocked.
CREATE TABLE IF NOT EXISTS fleet_partner_projection_gaps (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL,
    source_service    text NOT NULL,
    aggregate_type    text NOT NULL CHECK (aggregate_type IN ('REQUEST', 'TRIP')),
    aggregate_source_id text NOT NULL,
    expected_version  bigint NOT NULL,
    received_version  bigint NOT NULL,
    status            text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED')),
    detected_at       timestamptz NOT NULL DEFAULT now(),
    resolved_at       timestamptz,
    CONSTRAINT uq_fleet_partner_open_gap
        UNIQUE (source_service, aggregate_type, aggregate_source_id, expected_version)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_gaps_open
    ON fleet_partner_projection_gaps (status, detected_at)
    WHERE status = 'OPEN';

CREATE TABLE IF NOT EXISTS fleet_partner_projection_reconciliation_cases (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    source_service      text NOT NULL,
    aggregate_type      text NOT NULL CHECK (aggregate_type IN ('REQUEST', 'TRIP', 'COMMAND')),
    aggregate_source_id text NOT NULL,
    reason              text NOT NULL,
    status              text NOT NULL DEFAULT 'OPEN'
                        CHECK (status IN ('OPEN', 'IN_PROGRESS', 'RESOLVED', 'FAILED_PERMANENT')),
    attempts            integer NOT NULL DEFAULT 0,
    last_error_code     text,
    next_attempt_at     timestamptz,
    resolved_at         timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_reconciliation_due
    ON fleet_partner_projection_reconciliation_cases (status, next_attempt_at)
    WHERE status IN ('OPEN', 'IN_PROGRESS');

-- Organization operating mode. Additive; defaults to CENTRALIZED so existing
-- organizations keep dispatcher-controlled behaviour until they opt in.
--
-- It can never disable provider decline: that invariant lives in
-- fleet_partner_provider_policies with a CHECK constraint.
ALTER TABLE fleet_partner_organization_settings
    ADD COLUMN IF NOT EXISTS operating_mode text NOT NULL DEFAULT 'CENTRALIZED';

ALTER TABLE fleet_partner_organization_settings
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_operating_mode;

ALTER TABLE fleet_partner_organization_settings
    ADD CONSTRAINT chk_fleet_partner_operating_mode
    CHECK (operating_mode IN ('CENTRALIZED', 'DELEGATED', 'HYBRID'));

-- Marketplace, assignment, trip and reconciliation permissions.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('marketplace.read',       'marketplace',    'Read organization marketplace opportunities', true),
    ('marketplace.accept',     'marketplace',    'Accept a marketplace opportunity for the organization', true),
    ('marketplace.reject',     'marketplace',    'Decline a marketplace opportunity for the organization', true),
    ('dispatch.assign',        'dispatch',       'Assign work to a managed provider', true),
    ('assignments.read',       'assignments',    'Read provider assignments', true),
    ('assignments.cancel',     'assignments',    'Cancel a provider assignment', true),
    ('assignments.reassign',   'assignments',    'Reassign work to another provider', true),
    ('trips.read',             'trips',          'Read organization trip projections', true),
    ('reconciliation.read',    'reconciliation', 'Read projection reconciliation state', true),
    ('reconciliation.manage',  'reconciliation', 'Trigger projection reconciliation and repair', true),
    -- Provider-only capability. It is intentionally NOT granted to any system
    -- role: only the authenticated provider bound to an assignment may respond,
    -- so granting it to corporate staff must never become a way to impersonate
    -- a provider.
    ('assignments.respond',    'assignments',    'Provider-only: respond to an assignment (never grantable to corporate staff)', true)
ON CONFLICT (code) DO NOTHING;

-- Grant the new corporate permissions to the seeded system roles.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('marketplace.read', 'marketplace.accept', 'marketplace.reject', 'dispatch.assign',
               'assignments.read', 'assignments.cancel', 'assignments.reassign', 'trips.read',
               'reconciliation.read', 'reconciliation.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin', 'marketplace.read'), ('rol_system_admin', 'marketplace.accept'),
    ('rol_system_admin', 'marketplace.reject'), ('rol_system_admin', 'dispatch.assign'),
    ('rol_system_admin', 'assignments.read'), ('rol_system_admin', 'assignments.cancel'),
    ('rol_system_admin', 'assignments.reassign'), ('rol_system_admin', 'trips.read'),
    ('rol_system_admin', 'reconciliation.read'),

    ('rol_system_operations', 'marketplace.read'), ('rol_system_operations', 'marketplace.accept'),
    ('rol_system_operations', 'marketplace.reject'), ('rol_system_operations', 'dispatch.assign'),
    ('rol_system_operations', 'assignments.read'), ('rol_system_operations', 'assignments.cancel'),
    ('rol_system_operations', 'assignments.reassign'), ('rol_system_operations', 'trips.read'),

    -- The dispatcher role is the primary marketplace and assignment actor.
    ('rol_system_dispatcher', 'marketplace.read'), ('rol_system_dispatcher', 'marketplace.accept'),
    ('rol_system_dispatcher', 'marketplace.reject'), ('rol_system_dispatcher', 'dispatch.assign'),
    ('rol_system_dispatcher', 'assignments.read'), ('rol_system_dispatcher', 'assignments.cancel'),
    ('rol_system_dispatcher', 'assignments.reassign'), ('rol_system_dispatcher', 'trips.read'),

    ('rol_system_fleet', 'marketplace.read'), ('rol_system_fleet', 'assignments.read'),
    ('rol_system_fleet', 'assignments.reassign'), ('rol_system_fleet', 'trips.read'),

    ('rol_system_compliance', 'assignments.read'), ('rol_system_compliance', 'trips.read'),
    ('rol_system_analyst', 'marketplace.read'), ('rol_system_analyst', 'assignments.read'),
    ('rol_system_analyst', 'trips.read'),
    ('rol_system_support', 'assignments.read'), ('rol_system_support', 'trips.read'),
    ('rol_system_view_only', 'marketplace.read'), ('rol_system_view_only', 'assignments.read'),
    ('rol_system_view_only', 'trips.read')
ON CONFLICT DO NOTHING;
