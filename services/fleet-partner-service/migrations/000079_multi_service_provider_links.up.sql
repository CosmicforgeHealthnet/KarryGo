-- Section 10: one organization member, several provider identities.
--
-- Fleet has always modelled the RELATIONSHIP between an organization and a
-- provider, one row per service category, in
-- fleet_partner_organization_provider_memberships. That table is already the
-- service link the specification asks for: it carries the authority
-- (service_category), the opaque provider_id, the source version, the status
-- and the suspension timestamp.
--
-- What it has never carried is the fact that three of those rows can be the
-- same human. An organization employing a driver who rides a bike in the
-- morning and drives a taxi in the afternoon holds two unrelated memberships
-- and no way to say so. That is what this migration adds, and only that: the
-- member layer on top, not a replacement underneath.
--
-- Nothing here takes ownership of an asset or a provider. provider_id and
-- asset_id remain opaque references into the driver services, deliberately not
-- foreign keys, exactly as the existing membership table already treats them.

-- ---------------------------------------------------------------------------
-- The member
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_managed_provider_members (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL
                        REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,

    -- member_ref is the organization's own identifier for this person: a staff
    -- number, a payroll id, whatever they already use. It is not a platform
    -- identity and is never used to authenticate anyone.
    member_ref        text NOT NULL,

    -- A deliberately minimal display projection. Canonical provider identity
    -- (legal name, phone, documents) belongs to the owning driver service, and
    -- duplicating it here would create a second copy to keep correct and a
    -- second place for personal data to leak from.
    display_name      text NOT NULL DEFAULT '',

    status            text NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE', 'SUSPENDED', 'ENDED')),

    -- Organization-level consent, separate from each service link's own
    -- qualification. A member may consent to work for the organization while
    -- one particular authority has not yet qualified them.
    consent_status    text NOT NULL DEFAULT 'PENDING'
                        CHECK (consent_status IN ('PENDING', 'GRANTED', 'WITHDRAWN')),

    suspended_at      timestamptz,
    ended_at          timestamptz,
    version           bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),

    UNIQUE (organization_id, member_ref)
);

CREATE INDEX IF NOT EXISTS idx_fleet_managed_members_org
    ON fleet_partner_managed_provider_members (organization_id, status);

-- ---------------------------------------------------------------------------
-- The link from member to the existing service membership
-- ---------------------------------------------------------------------------
-- A nullable column rather than a new join table. The membership row IS the
-- service link; giving it an owner is a smaller and more honest change than
-- introducing a second table that would have to be kept in step with it.
--
-- Nullable because every existing membership predates the member layer and
-- must keep working unlinked. A deployment with the feature off simply never
-- populates it.
ALTER TABLE fleet_partner_organization_provider_memberships
    ADD COLUMN IF NOT EXISTS member_id text
        REFERENCES fleet_partner_managed_provider_members (id) ON DELETE RESTRICT;

-- One member holds at most one live membership per authority. Without this a
-- member could accumulate two live TAXI links and "which taxi identity is
-- this person" would depend on row order.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_member_live_authority
    ON fleet_partner_organization_provider_memberships (member_id, service_category)
    WHERE member_id IS NOT NULL
      AND status IN ('PENDING_PROVIDER_ACCEPTANCE', 'PENDING_SETUP', 'ACTIVE', 'SUSPENDED', 'ENDING');

CREATE INDEX IF NOT EXISTS idx_fleet_memberships_member
    ON fleet_partner_organization_provider_memberships (member_id)
    WHERE member_id IS NOT NULL;

-- A membership and its member must belong to the same organization. Without
-- the composite key below the FK above would permit one organization's
-- membership to be attached to another organization's member, which is a
-- tenant boundary crossed by a single mistyped id.
ALTER TABLE fleet_partner_managed_provider_members
    DROP CONSTRAINT IF EXISTS uq_fleet_managed_member_tenant;
ALTER TABLE fleet_partner_managed_provider_members
    ADD CONSTRAINT uq_fleet_managed_member_tenant UNIQUE (id, organization_id);

ALTER TABLE fleet_partner_organization_provider_memberships
    DROP CONSTRAINT IF EXISTS fk_fleet_membership_member_tenant;
ALTER TABLE fleet_partner_organization_provider_memberships
    ADD CONSTRAINT fk_fleet_membership_member_tenant
        FOREIGN KEY (member_id, organization_id)
        REFERENCES fleet_partner_managed_provider_members (id, organization_id)
        ON DELETE RESTRICT;

-- ---------------------------------------------------------------------------
-- Managed asset links
-- ---------------------------------------------------------------------------
-- Which vehicles, bikes and trucks an organization considers its own. Fleet
-- references them; it never owns their state. Capability, certification,
-- roadworthiness and availability all remain the owning service's answer, and
-- a link here asserts none of them.
CREATE TABLE IF NOT EXISTS fleet_partner_managed_asset_links (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL
                         REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    authority          text NOT NULL CHECK (authority IN ('DISPATCH', 'TAXI', 'HAULING')),

    -- Opaque identifier owned by the driver service.
    asset_id           text NOT NULL,

    -- The member who normally operates it, when the organization assigns one.
    -- Nullable: a pool vehicle belongs to the organization, not to a person.
    member_id          text REFERENCES fleet_partner_managed_provider_members (id) ON DELETE RESTRICT,

    link_status        text NOT NULL DEFAULT 'ACTIVE'
                         CHECK (link_status IN ('ACTIVE', 'SUSPENDED', 'REMOVED')),

    -- The owning service's version of the asset facts this link was made
    -- against, so a stale report cannot overwrite a newer one.
    source_version     bigint NOT NULL DEFAULT 0 CHECK (source_version >= 0),

    linked_at          timestamptz NOT NULL DEFAULT now(),
    suspended_at       timestamptz,
    removed_at         timestamptz,
    version            bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_fleet_managed_asset_link UNIQUE (organization_id, authority, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_managed_asset_links_member
    ON fleet_partner_managed_asset_links (member_id)
    WHERE member_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Admit Taxi to the projections that already carry Dispatch and Haulage
-- ---------------------------------------------------------------------------
-- Three CHECK constraints named exactly two source services. Fleet could
-- therefore never have PROJECTED a taxi lifecycle even once its adapter was
-- enabled: the first event would have failed on insert. Widening them is what
-- makes the control tower able to show a taxi trip alongside the others.

ALTER TABLE fleet_partner_tracking_event_inbox
    DROP CONSTRAINT IF EXISTS fleet_partner_tracking_event_inbox_source_service_check;
ALTER TABLE fleet_partner_tracking_event_inbox
    ADD CONSTRAINT fleet_partner_tracking_event_inbox_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service',
                                  'driver-taxi-service',
                                  'driver-hauling-service'));

ALTER TABLE fleet_partner_tracking_projections
    DROP CONSTRAINT IF EXISTS fleet_partner_tracking_projections_source_service_check;
ALTER TABLE fleet_partner_tracking_projections
    ADD CONSTRAINT fleet_partner_tracking_projections_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service',
                                  'driver-taxi-service',
                                  'driver-hauling-service'));

ALTER TABLE fleet_partner_private_execution_projections
    DROP CONSTRAINT IF EXISTS fleet_partner_private_execution_projections_source_service_check;
ALTER TABLE fleet_partner_private_execution_projections
    ADD CONSTRAINT fleet_partner_private_execution_projections_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service',
                                  'driver-taxi-service',
                                  'driver-hauling-service'));

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------
INSERT INTO fleet_partner_permissions (code, category, description, is_active)
VALUES ('managed_members.read', 'providers', 'Read organization managed provider members', true),
       ('managed_members.manage', 'providers', 'Create and maintain organization managed provider members', true),
       ('managed_assets.read', 'providers', 'Read organization managed asset links', true),
       ('managed_assets.manage', 'providers', 'Link and unlink organization managed assets', true)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
VALUES ('rol_system_owner', 'managed_members.read'),
       ('rol_system_owner', 'managed_members.manage'),
       ('rol_system_owner', 'managed_assets.read'),
       ('rol_system_owner', 'managed_assets.manage'),
       ('rol_system_operations', 'managed_members.read'),
       ('rol_system_operations', 'managed_members.manage'),
       ('rol_system_operations', 'managed_assets.read'),
       ('rol_system_operations', 'managed_assets.manage')
ON CONFLICT DO NOTHING;
