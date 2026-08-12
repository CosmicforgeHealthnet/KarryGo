-- Section 15 (WP14), slice A4: shipments and cargo items.
--
-- A shipment belongs to exactly one private order and is never a canonical
-- trip: confirming or creating a shipment moves no money, reserves no
-- capacity and starts no execution. That begins only once a later Section 15
-- slice (private assignments and execution orchestration) wires a shipment
-- forward through an accepted provider.
--
-- Cargo items carry a restricted_item_status that defaults to UNKNOWN, not
-- to a clean bill of health. A cargo description that merely claims to be
-- ordinary is not a compliance decision, and treating silence as approval
-- would let regulated cargo move on nothing but free text.

CREATE TABLE IF NOT EXISTS fleet_partner_shipments (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                   text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    order_id                    text NOT NULL REFERENCES fleet_partner_private_orders (id) ON DELETE RESTRICT,
    origin_stop_id              text NOT NULL REFERENCES fleet_partner_order_stops (id) ON DELETE RESTRICT,
    destination_stop_id         text NOT NULL REFERENCES fleet_partner_order_stops (id) ON DELETE RESTRICT,
    cargo_profile               jsonb NOT NULL DEFAULT '{}'::jsonb,
    passenger_profile           jsonb NOT NULL DEFAULT '{}'::jsonb,
    required_vehicle_class      text NOT NULL DEFAULT '',
    required_service_capability text NOT NULL DEFAULT '',
    temperature_requirements    jsonb NOT NULL DEFAULT '{}'::jsonb,
    declared_value_amount_minor bigint NOT NULL DEFAULT 0 CHECK (declared_value_amount_minor >= 0),
    declared_value_currency     char(3) NOT NULL CHECK (declared_value_currency ~ '^[A-Z]{3}$'),
    -- Projection only, maintained by the A5 package feature. Zero here is
    -- honest: no packages exist on a shipment until that slice is built.
    package_count                integer NOT NULL DEFAULT 0 CHECK (package_count >= 0),
    status                       text NOT NULL DEFAULT 'DRAFT'
                                 CHECK (status IN ('DRAFT', 'READY', 'PLANNED', 'ASSIGNED', 'IN_TRANSIT',
                                                    'COMPLETED', 'FAILED', 'PARTIALLY_COMPLETED', 'CANCELLED')),
    version                      bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                   text NOT NULL,
    updated_by                   text NOT NULL DEFAULT '',
    created_at                   timestamptz NOT NULL DEFAULT now(),
    updated_at                   timestamptz NOT NULL DEFAULT now(),
    completed_at                 timestamptz,
    cancelled_at                 timestamptz,
    CHECK (origin_stop_id <> destination_stop_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_shipments_cursor
    ON fleet_partner_shipments (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_shipments_order
    ON fleet_partner_shipments (organization_id, order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_shipments_status
    ON fleet_partner_shipments (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_shipments_branch
    ON fleet_partner_shipments (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_shipment_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    shipment_id      text NOT NULL REFERENCES fleet_partner_shipments (id) ON DELETE RESTRICT,
    previous_status  text NOT NULL,
    new_status       text NOT NULL,
    reason_code      text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_shipment_history
    ON fleet_partner_shipment_history (shipment_id, occurred_at);

CREATE TRIGGER trg_fleet_shipment_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_shipment_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Cargo items
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_cargo_items (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    shipment_id                 text NOT NULL REFERENCES fleet_partner_shipments (id) ON DELETE RESTRICT,
    description                 text NOT NULL CHECK (length(btrim(description)) > 0),
    -- Decimal, never float: a quantity of 0.1 tonnes must stay 0.1.
    quantity                    numeric(18, 6) NOT NULL CHECK (quantity > 0),
    unit                        text NOT NULL,
    handling_category           text NOT NULL DEFAULT 'GENERAL'
                                CHECK (handling_category IN ('GENERAL', 'DANGEROUS_GOODS', 'MEDICAL',
                                                              'COLD_CHAIN', 'FRAGILE', 'HIGH_VALUE',
                                                              'FOOD_AGRICULTURE', 'RESTRICTED')),
    hazardous_classification    text NOT NULL DEFAULT '',
    temperature_min             numeric(6, 2),
    temperature_max             numeric(6, 2),
    temperature_unit            text NOT NULL DEFAULT '',
    declared_value_amount_minor bigint NOT NULL DEFAULT 0 CHECK (declared_value_amount_minor >= 0),
    declared_value_currency     char(3) NOT NULL CHECK (declared_value_currency ~ '^[A-Z]{3}$'),
    -- UNKNOWN is the default and the fail-closed state. A cargo item whose
    -- restriction status has not been decided is treated exactly like one
    -- pending review, never like one cleared.
    restricted_item_status      text NOT NULL DEFAULT 'UNKNOWN'
                                CHECK (restricted_item_status IN ('NOT_RESTRICTED', 'REQUIRES_COMPLIANCE_REVIEW',
                                                                   'COMPLIANCE_APPROVED', 'COMPLIANCE_REJECTED',
                                                                   'UNKNOWN')),
    compliance_reference         text NOT NULL DEFAULT '',
    compliance_decision_version  bigint NOT NULL DEFAULT 0,
    version                      bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                   text NOT NULL,
    updated_by                   text NOT NULL DEFAULT '',
    created_at                   timestamptz NOT NULL DEFAULT now(),
    updated_at                   timestamptz NOT NULL DEFAULT now(),
    CHECK (temperature_min IS NULL OR temperature_max IS NULL OR temperature_max >= temperature_min)
);

CREATE INDEX IF NOT EXISTS idx_fleet_cargo_items_shipment
    ON fleet_partner_cargo_items (organization_id, shipment_id, created_at);
CREATE INDEX IF NOT EXISTS idx_fleet_cargo_items_restricted
    ON fleet_partner_cargo_items (organization_id, restricted_item_status)
    WHERE restricted_item_status IN ('REQUIRES_COMPLIANCE_REVIEW', 'UNKNOWN');

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('shipments.read',   'shipments', 'Read shipments and cargo items',        true),
    ('shipments.manage', 'shipments', 'Create and update shipments and cargo', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code LIKE 'shipments.%'
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('shipments.read', 'shipments.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_crm', code FROM fleet_partner_permissions
WHERE code IN ('shipments.read')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_analyst', code FROM fleet_partner_permissions
WHERE code IN ('shipments.read')
ON CONFLICT DO NOTHING;
