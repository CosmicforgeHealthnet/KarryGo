-- Company staff memberships, invitations, roles, permissions and role
-- bindings. Authorization is deny-by-default: authority exists only where an
-- unexpired, unrevoked binding grants it.

CREATE TABLE IF NOT EXISTS fleet_partner_organization_users (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    identity_id         text NOT NULL REFERENCES fleet_partner_corporate_identities (id) ON DELETE RESTRICT,
    full_name           text NOT NULL,
    job_title           text,
    status              text NOT NULL DEFAULT 'INVITED'
                        CHECK (status IN ('INVITED', 'ACTIVE', 'SUSPENDED', 'ENDED', 'EXPIRED', 'CANCELLED')),
    is_owner            boolean NOT NULL DEFAULT false,
    permitted_branch_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    permitted_department_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    -- Bumped whenever role bindings change so cached authorization and live
    -- sessions can detect staleness without a cache invalidation broadcast.
    role_version        bigint NOT NULL DEFAULT 1,
    invited_by          text,
    invited_at          timestamptz,
    activated_at        timestamptz,
    suspended_at        timestamptz,
    ended_at            timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- One identity holds at most one live membership per organization; the same
-- identity may belong to many organizations.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_org_user_identity
    ON fleet_partner_organization_users (organization_id, identity_id)
    WHERE status IN ('INVITED', 'ACTIVE', 'SUSPENDED');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_org_users_org
    ON fleet_partner_organization_users (organization_id, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_org_users_identity
    ON fleet_partner_organization_users (identity_id, status);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_staff_invitations (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    organization_user_id  text REFERENCES fleet_partner_organization_users (id) ON DELETE RESTRICT,
    full_name             text NOT NULL,
    email_normalized      text NOT NULL,
    phone_e164            text NOT NULL,
    -- Only the hash of the single-use invitation token is stored.
    invitation_token_hash text NOT NULL,
    role_ids              jsonb NOT NULL DEFAULT '[]'::jsonb,
    permitted_branch_ids  jsonb NOT NULL DEFAULT '[]'::jsonb,
    permitted_department_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    status                text NOT NULL DEFAULT 'PENDING'
                          CHECK (status IN ('PENDING', 'ACCEPTED', 'EXPIRED', 'CANCELLED')),
    expires_at            timestamptz NOT NULL,
    delivery_status       text NOT NULL DEFAULT 'PENDING'
                          CHECK (delivery_status IN ('PENDING', 'ACCEPTED', 'FAILED', 'UNAVAILABLE')),
    notification_reference text,
    attempts              integer NOT NULL DEFAULT 0,
    created_by            text NOT NULL,
    accepted_at           timestamptz,
    consumed_at           timestamptz,
    version               bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_staff_invitation_token
    ON fleet_partner_organization_staff_invitations (invitation_token_hash);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_staff_invitation_pending
    ON fleet_partner_organization_staff_invitations (organization_id, email_normalized)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_staff_invitations_expiry
    ON fleet_partner_organization_staff_invitations (expires_at)
    WHERE status = 'PENDING';

-- Platform permission catalogue (global, not tenant owned).
CREATE TABLE IF NOT EXISTS fleet_partner_permissions (
    code        text PRIMARY KEY,
    category    text NOT NULL,
    description text NOT NULL DEFAULT '',
    -- Permissions seeded ahead of their business module are marked so they
    -- can be granted but are not yet claimed to be enforced anywhere.
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_partner_roles (
    id              text PRIMARY KEY,
    -- NULL organization_id marks an immutable system role template.
    organization_id text REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    code            text NOT NULL,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    is_system       boolean NOT NULL DEFAULT false,
    status          text NOT NULL DEFAULT 'ACTIVE'
                    CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_system_role_code
    ON fleet_partner_roles (code)
    WHERE organization_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_org_role_code
    ON fleet_partner_roles (organization_id, code)
    WHERE organization_id IS NOT NULL AND status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS fleet_partner_role_permissions (
    role_id         text NOT NULL REFERENCES fleet_partner_roles (id) ON DELETE RESTRICT,
    permission_code text NOT NULL REFERENCES fleet_partner_permissions (code) ON DELETE RESTRICT,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE IF NOT EXISTS fleet_partner_role_bindings (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    organization_user_id text NOT NULL REFERENCES fleet_partner_organization_users (id) ON DELETE RESTRICT,
    role_id              text NOT NULL REFERENCES fleet_partner_roles (id) ON DELETE RESTRICT,
    scope_type           text NOT NULL DEFAULT 'ORGANIZATION'
                         CHECK (scope_type IN ('ORGANIZATION', 'BRANCH', 'DEPARTMENT')),
    scope_id             text,
    effective_from       timestamptz NOT NULL DEFAULT now(),
    expires_at           timestamptz,
    granted_by           text NOT NULL,
    revoked_at           timestamptz,
    revocation_reason    text,
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    -- A non-organization scope must name the scoped resource.
    CONSTRAINT chk_fleet_partner_binding_scope
        CHECK ((scope_type = 'ORGANIZATION' AND scope_id IS NULL) OR (scope_type <> 'ORGANIZATION' AND scope_id IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_role_binding_live
    ON fleet_partner_role_bindings (organization_user_id, role_id, scope_type, COALESCE(scope_id, ''))
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_role_bindings_org
    ON fleet_partner_role_bindings (organization_id, organization_user_id)
    WHERE revoked_at IS NULL;

-- Seed the platform permission catalogue. Idempotent so replay is safe.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('organization.read',        'organization', 'Read organization profile and onboarding state', true),
    ('organization.update',      'organization', 'Update organization profile', true),
    ('organization.submit',      'organization', 'Submit organization for verification', true),
    ('organization.close',       'organization', 'Request organization closure', true),
    ('structure.read',           'structure',    'Read branches and departments', true),
    ('structure.manage',         'structure',    'Manage branches and departments', true),
    ('staff.read',               'staff',        'Read company staff', true),
    ('staff.invite',             'staff',        'Invite company staff', true),
    ('staff.manage',             'staff',        'Manage company staff', true),
    ('staff.suspend',            'staff',        'Suspend company staff', true),
    ('roles.read',               'roles',        'Read roles and permissions', true),
    ('roles.manage',             'roles',        'Manage roles and role bindings', true),
    ('approvals.read',           'approvals',    'Read approval requests', true),
    ('approvals.decide',         'approvals',    'Approve or reject approval requests', true),
    ('approval_policies.manage', 'approvals',    'Manage approval policies', true),
    ('providers.read',           'providers',    'Read managed providers', true),
    ('providers.create',         'providers',    'Create or link managed providers', true),
    ('providers.manage',         'providers',    'Manage managed providers', true),
    ('providers.suspend',        'providers',    'Suspend managed providers', true),
    ('providers.remove',         'providers',    'Remove managed providers', true),
    ('providers.manage_capabilities', 'providers', 'Manage provider capabilities', true),
    ('providers.manage_policies',     'providers', 'Manage provider policies', true),
    ('security.sessions.read',   'security',     'Read organization sessions', true),
    ('security.sessions.revoke', 'security',     'Revoke organization sessions', true),
    ('security.audit.read',      'security',     'Read audit records', true),
    -- Seeded ahead of their business modules (not yet enforced anywhere).
    ('wallets.read',             'finance',      'Read organization wallet projections', false),
    ('wallets.allocate',         'finance',      'Allocate organization wallet funds', false),
    ('orders.read',              'orders',       'Read corporate orders', false),
    ('orders.manage',            'orders',       'Manage corporate orders', false),
    ('assignments.manage',       'assignments',  'Manage work assignments', false),
    ('reports.read',             'reports',      'Read corporate reports', false),
    ('reports.export',           'reports',      'Export corporate reports', false)
ON CONFLICT (code) DO NOTHING;

-- Seed immutable system role templates (organization_id IS NULL).
INSERT INTO fleet_partner_roles (id, organization_id, code, name, description, is_system) VALUES
    ('rol_system_owner',        NULL, 'ORGANIZATION_OWNER',           'Organization Owner',            'Full authority over the organization', true),
    ('rol_system_admin',        NULL, 'COMPANY_ADMIN',                'Company Administrator',         'Administers company configuration and staff', true),
    ('rol_system_operations',   NULL, 'OPERATIONS_MANAGER',           'Operations Manager',            'Runs day-to-day operations', true),
    ('rol_system_dispatcher',   NULL, 'DISPATCHER',                   'Dispatcher',                    'Coordinates work allocation', true),
    ('rol_system_fleet',        NULL, 'FLEET_MANAGER',                'Fleet Manager',                 'Manages providers and assets', true),
    ('rol_system_finance',      NULL, 'FINANCE_MANAGER',              'Finance Manager',               'Manages financial operations', true),
    ('rol_system_compliance',   NULL, 'COMPLIANCE_SAFETY_MANAGER',    'Compliance and Safety Manager', 'Manages compliance and safety', true),
    ('rol_system_warehouse',    NULL, 'WAREHOUSE_MANAGER',            'Warehouse Manager',             'Manages warehouse operations', true),
    ('rol_system_crm',          NULL, 'CUSTOMER_RELATIONSHIP_MANAGER','Customer Relationship Manager', 'Manages customer relationships', true),
    ('rol_system_analyst',      NULL, 'ANALYST',                      'Analyst',                       'Reads reports and analytics', true),
    ('rol_system_support',      NULL, 'SUPPORT_USER',                 'Support User',                  'Handles support interactions', true),
    ('rol_system_view_only',    NULL, 'VIEW_ONLY',                    'View Only',                     'Read-only access', true)
ON CONFLICT (id) DO NOTHING;

-- System role permission templates.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions WHERE is_active = true
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin', 'organization.read'), ('rol_system_admin', 'organization.update'),
    ('rol_system_admin', 'structure.read'), ('rol_system_admin', 'structure.manage'),
    ('rol_system_admin', 'staff.read'), ('rol_system_admin', 'staff.invite'),
    ('rol_system_admin', 'staff.manage'), ('rol_system_admin', 'staff.suspend'),
    ('rol_system_admin', 'roles.read'), ('rol_system_admin', 'roles.manage'),
    ('rol_system_admin', 'approvals.read'), ('rol_system_admin', 'approvals.decide'),
    ('rol_system_admin', 'providers.read'), ('rol_system_admin', 'providers.create'),
    ('rol_system_admin', 'providers.manage'), ('rol_system_admin', 'security.sessions.read'),
    ('rol_system_admin', 'security.audit.read'),

    ('rol_system_operations', 'organization.read'), ('rol_system_operations', 'structure.read'),
    ('rol_system_operations', 'staff.read'), ('rol_system_operations', 'providers.read'),
    ('rol_system_operations', 'providers.manage'), ('rol_system_operations', 'approvals.read'),

    ('rol_system_dispatcher', 'organization.read'), ('rol_system_dispatcher', 'structure.read'),
    ('rol_system_dispatcher', 'providers.read'),

    ('rol_system_fleet', 'organization.read'), ('rol_system_fleet', 'structure.read'),
    ('rol_system_fleet', 'providers.read'), ('rol_system_fleet', 'providers.create'),
    ('rol_system_fleet', 'providers.manage'), ('rol_system_fleet', 'providers.suspend'),
    ('rol_system_fleet', 'providers.manage_capabilities'), ('rol_system_fleet', 'providers.manage_policies'),

    ('rol_system_finance', 'organization.read'), ('rol_system_finance', 'approvals.read'),
    ('rol_system_finance', 'approvals.decide'),

    ('rol_system_compliance', 'organization.read'), ('rol_system_compliance', 'providers.read'),
    ('rol_system_compliance', 'security.audit.read'), ('rol_system_compliance', 'approvals.read'),

    ('rol_system_warehouse', 'organization.read'), ('rol_system_warehouse', 'structure.read'),

    ('rol_system_crm', 'organization.read'),

    ('rol_system_analyst', 'organization.read'), ('rol_system_analyst', 'structure.read'),
    ('rol_system_analyst', 'providers.read'), ('rol_system_analyst', 'staff.read'),

    ('rol_system_support', 'organization.read'), ('rol_system_support', 'providers.read'),

    ('rol_system_view_only', 'organization.read'), ('rol_system_view_only', 'structure.read'),
    ('rol_system_view_only', 'staff.read'), ('rol_system_view_only', 'providers.read')
ON CONFLICT DO NOTHING;
