-- Section 15 (WP14): organization service catalogue.
--
-- A catalogue item is a definition, not a price: pricing, approval and proof
-- policy are opaque references an operator's own system resolves. Fleet does
-- not calculate fares, commissions or tax here or anywhere in Section 15.
--
-- Catalogue codes are permanently reserved once used, even after archival, so
-- an archived code can never be silently repurposed for a different meaning.
-- Material redefinition of an active item is expected to happen through a new
-- code or a new item, not a quiet edit of what an existing code means.

CREATE TABLE IF NOT EXISTS fleet_partner_service_catalog_items (
    id                         text PRIMARY KEY,
    organization_id            text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                  text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    name                       text NOT NULL CHECK (length(btrim(name)) > 0),
    code                       text NOT NULL CHECK (length(btrim(code)) > 0),
    category                   text NOT NULL
                               CHECK (category IN ('PACKAGE_DELIVERY', 'COURIER_LAST_MILE', 'SAME_DAY_DELIVERY',
                                                    'SCHEDULED_DELIVERY', 'TAXI', 'PASSENGER_TRANSPORT', 'HAULING',
                                                    'TRUCKING', 'HEAVY_HAUL', 'FREIGHT_FORWARDING',
                                                    'MOVING_RELOCATION', 'COLD_CHAIN', 'MEDICAL_LOGISTICS',
                                                    'AGRICULTURE_FOOD', 'TOWING_RECOVERY', 'WAREHOUSING',
                                                    'FULFILMENT', 'PRIVATE_LOGISTICS', 'OTHER')),
    description                text NOT NULL DEFAULT '',
    capability_reference       text NOT NULL DEFAULT '',
    -- Declarative only: an allowlisted set of field descriptors, never an
    -- executable expression. The domain layer rejects unsupported keywords,
    -- unbounded depth and oversized payloads before this is ever stored.
    required_fields_schema     jsonb NOT NULL DEFAULT '{}'::jsonb,
    supported_vehicle_classes  jsonb NOT NULL DEFAULT '[]'::jsonb,
    supported_package_types    jsonb NOT NULL DEFAULT '[]'::jsonb,
    pricing_policy_reference   text NOT NULL DEFAULT '',
    approval_policy_reference  text NOT NULL DEFAULT '',
    proof_policy_reference     text NOT NULL DEFAULT '',
    status                     text NOT NULL DEFAULT 'DRAFT'
                               CHECK (status IN ('DRAFT', 'ACTIVE', 'INACTIVE', 'ARCHIVED')),
    version                    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                 text NOT NULL,
    updated_by                 text NOT NULL DEFAULT '',
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    archived_at                timestamptz
);

-- Permanent, not partial: a code stays reserved after archival so it can never
-- be silently reused for a different meaning.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_servicecatalog_code
    ON fleet_partner_service_catalog_items (organization_id, code);

CREATE INDEX IF NOT EXISTS idx_fleet_servicecatalog_cursor
    ON fleet_partner_service_catalog_items (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_servicecatalog_status
    ON fleet_partner_service_catalog_items (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_servicecatalog_category
    ON fleet_partner_service_catalog_items (organization_id, category);
CREATE INDEX IF NOT EXISTS idx_fleet_servicecatalog_branch
    ON fleet_partner_service_catalog_items (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('servicecatalog.items.read',       'servicecatalog', 'Read organization service catalogue items',   true),
    ('servicecatalog.items.manage',     'servicecatalog', 'Create and update service catalogue items',   true),
    ('servicecatalog.items.activate',   'servicecatalog', 'Activate a service catalogue item',            true),
    ('servicecatalog.items.deactivate', 'servicecatalog', 'Deactivate a service catalogue item',          true),
    ('servicecatalog.items.archive',    'servicecatalog', 'Archive a service catalogue item permanently', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code LIKE 'servicecatalog.%'
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('servicecatalog.items.read', 'servicecatalog.items.manage',
               'servicecatalog.items.activate', 'servicecatalog.items.deactivate')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_admin', code FROM fleet_partner_permissions
WHERE code IN ('servicecatalog.items.read', 'servicecatalog.items.activate',
               'servicecatalog.items.deactivate', 'servicecatalog.items.archive')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_analyst', code FROM fleet_partner_permissions
WHERE code IN ('servicecatalog.items.read')
ON CONFLICT DO NOTHING;
