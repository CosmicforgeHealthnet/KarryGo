-- Section 15 (WP14): private orders and order stops.
--
-- A private order is company-owned work: manual, converted from an accepted
-- Section 14 quotation, generated from a contract or a recurring template, or
-- imported in bulk. It is never a marketplace request, and confirming one
-- creates no assignment, trip or wallet movement by itself — that begins only
-- once Section 15's later slices (shipments, orchestration) wire it forward.
--
-- Confirmation is a snapshot boundary. The catalogue definition and the
-- customer/contact/location facts an order was confirmed against are frozen
-- into service_catalog_snapshot and confirmation_snapshot at that moment, so a
-- later edit to the customer or the catalogue item never rewrites what was
-- already confirmed.

CREATE TABLE IF NOT EXISTS fleet_partner_private_orders (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                   text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    order_number                text NOT NULL,
    customer_id                 text NOT NULL REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    customer_reference          text NOT NULL DEFAULT '',
    source                      text NOT NULL
                                CHECK (source IN ('MANUAL', 'QUOTATION', 'CONTRACT', 'RECURRING_TEMPLATE',
                                                   'BULK_IMPORT', 'PARTNER_INTEGRATION')),
    source_reference            text NOT NULL DEFAULT '',
    service_catalog_item_id     text NOT NULL
                                REFERENCES fleet_partner_service_catalog_items (id) ON DELETE RESTRICT,
    -- Frozen at confirmation. Never re-derived from the live catalogue row, so
    -- a later catalogue edit cannot silently restate what this order agreed to.
    service_catalog_snapshot    jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Caller-supplied values for the catalogue item's required_fields_schema,
    -- validated against that schema at creation and re-validated at confirmation.
    service_fields              jsonb NOT NULL DEFAULT '{}'::jsonb,
    contract_id                 text REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    contract_version_id         text REFERENCES fleet_partner_contract_versions (id) ON DELETE RESTRICT,
    quotation_id                 text REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    quotation_version_id         text REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT,
    conversion_intent_id         text
                                REFERENCES fleet_partner_quotation_conversion_intents (id) ON DELETE RESTRICT,
    requested_schedule_start     timestamptz,
    requested_schedule_end       timestamptz,
    timezone                     text NOT NULL,
    priority                     text NOT NULL DEFAULT 'STANDARD'
                                 CHECK (priority IN ('LOW', 'STANDARD', 'HIGH', 'URGENT')),
    special_instructions         text NOT NULL DEFAULT '',
    status                       text NOT NULL DEFAULT 'DRAFT'
                                 CHECK (status IN ('DRAFT', 'AWAITING_APPROVAL', 'CONFIRMED', 'PLANNED',
                                                    'ASSIGNED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED',
                                                    'FAILED', 'PARTIALLY_COMPLETED')),
    -- Frozen at confirmation: customer/contact/location facts as they stood
    -- when the order was confirmed.
    confirmation_snapshot        jsonb NOT NULL DEFAULT '{}'::jsonb,
    approval_request_id          text,
    cancel_reason_code           text NOT NULL DEFAULT '',
    version                      bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                   text NOT NULL,
    updated_by                   text NOT NULL DEFAULT '',
    created_at                   timestamptz NOT NULL DEFAULT now(),
    updated_at                   timestamptz NOT NULL DEFAULT now(),
    confirmed_at                 timestamptz,
    cancelled_at                 timestamptz,
    completed_at                 timestamptz,
    CHECK (requested_schedule_start IS NULL OR requested_schedule_end IS NULL
           OR requested_schedule_end >= requested_schedule_start)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_orders_number
    ON fleet_partner_private_orders (organization_id, order_number);
CREATE INDEX IF NOT EXISTS idx_fleet_orders_cursor
    ON fleet_partner_private_orders (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_orders_status
    ON fleet_partner_private_orders (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_orders_customer
    ON fleet_partner_private_orders (organization_id, customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_orders_branch
    ON fleet_partner_private_orders (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_private_order_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    order_id         text NOT NULL REFERENCES fleet_partner_private_orders (id) ON DELETE RESTRICT,
    previous_status  text NOT NULL,
    new_status       text NOT NULL,
    reason_code      text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_order_history
    ON fleet_partner_private_order_history (order_id, occurred_at);

CREATE TRIGGER trg_fleet_order_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_private_order_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Order stops
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_order_stops (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    order_id             text NOT NULL REFERENCES fleet_partner_private_orders (id) ON DELETE RESTRICT,
    sequence             integer NOT NULL CHECK (sequence >= 0),
    stop_type            text NOT NULL
                         CHECK (stop_type IN ('PICKUP', 'DELIVERY', 'WAYPOINT', 'DEPOT', 'WAREHOUSE',
                                              'CUSTOMS', 'INSPECTION', 'SERVICE', 'RETURN')),
    customer_location_id text REFERENCES fleet_partner_customer_locations (id) ON DELETE RESTRICT,
    address_snapshot     jsonb NOT NULL DEFAULT '{}'::jsonb,
    latitude             numeric(9, 6) CHECK (latitude IS NULL OR (latitude >= -90 AND latitude <= 90)),
    longitude            numeric(9, 6) CHECK (longitude IS NULL OR (longitude >= -180 AND longitude <= 180)),
    time_window_start    timestamptz,
    time_window_end      timestamptz,
    timezone             text NOT NULL,
    contact_snapshot     jsonb NOT NULL DEFAULT '{}'::jsonb,
    instructions         text NOT NULL DEFAULT '',
    status               text NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN ('PENDING', 'CONFIRMED', 'COMPLETED', 'SKIPPED', 'FAILED')),
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, sequence),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK (time_window_start IS NULL OR time_window_end IS NULL OR time_window_end >= time_window_start)
);

CREATE INDEX IF NOT EXISTS idx_fleet_order_stops_order
    ON fleet_partner_order_stops (organization_id, order_id, sequence);

-- ---------------------------------------------------------------------------
-- Conversion-intent consumption
-- ---------------------------------------------------------------------------
--
-- consumed_at IS NULL is the whole "still convertible" gate. The status
-- column and its single-literal CHECK from migration 000023 are left
-- untouched: this is additive, not a rewrite of Section 14's contract.

ALTER TABLE fleet_partner_quotation_conversion_intents
    ADD COLUMN IF NOT EXISTS consumed_at timestamptz,
    ADD COLUMN IF NOT EXISTS private_order_id text;

ALTER TABLE fleet_partner_quotation_conversion_intents
    ADD CONSTRAINT fleet_partner_conversion_intent_order_fkey
    FOREIGN KEY (private_order_id)
    REFERENCES fleet_partner_private_orders (id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_conversion_intent_order
    ON fleet_partner_quotation_conversion_intents (private_order_id)
    WHERE private_order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_conversion_intent_unconsumed
    ON fleet_partner_quotation_conversion_intents (organization_id)
    WHERE consumed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('orders.read',        'orders', 'Read private orders and their stops', true),
    ('orders.manage',      'orders', 'Create and update draft private orders', true),
    ('orders.confirm',     'orders', 'Confirm a private order',              true),
    ('orders.cancel',      'orders', 'Cancel a private order',               true),
    ('orders.stops.manage','orders', 'Add and update private order stops',   true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code LIKE 'orders.%'
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('orders.read', 'orders.manage', 'orders.confirm', 'orders.cancel', 'orders.stops.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_admin', code FROM fleet_partner_permissions
WHERE code IN ('orders.read', 'orders.cancel')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_crm', code FROM fleet_partner_permissions
WHERE code IN ('orders.read', 'orders.manage', 'orders.confirm')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_analyst', code FROM fleet_partner_permissions
WHERE code IN ('orders.read')
ON CONFLICT DO NOTHING;
