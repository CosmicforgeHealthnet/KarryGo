-- Section 15B — Purchase Orders, Approval Policies & Lifecycle.
--
-- This migration implements purchasing authority only.
--
-- It DOES NOT:
--   post Inventory stock
--   implement goods-receipt documents
--   implement supplier invoices/payments
--   implement customer commerce Orders
--
-- Section 15C will own goods receiving and call the PO receiving-progress seam.
--
-- Approval policy notes:
--   * thresholds are Vendor-configurable
--   * branch-specific rules override business-wide fallback rules
--   * upper bounds are exclusive: [minimum, maximum)
--   * maximum NULL means no upper limit
--   * permission codes remain authorization authority; role names are not used
--     by purchasing business logic

-- Additional approval authorities.
--
-- Existing vendor.purchasing.approve remains the normal/branch-level approval
-- authority. The two additional codes let a Vendor configure policies that
-- require business-wide or high-value approval without hard-coding role names.
INSERT INTO vendor_permissions (
    id,
    code,
    description,
    category,
    is_high_risk
)
VALUES
(
    'perm_vendor_purchasing_approve_business',
    'vendor.purchasing.approve.business',
    'Approve purchase orders whose configured policy requires business-wide approval.',
    'purchasing',
    false
),
(
    'perm_vendor_purchasing_approve_high_value',
    'vendor.purchasing.approve.high_value',
    'Approve purchase orders whose configured policy requires high-value approval.',
    'purchasing',
    true
)
ON CONFLICT (id) DO NOTHING;

-- Default role mapping. Assignment scope remains authoritative.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id
FROM vendor_permissions
WHERE code='vendor.purchasing.approve'
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_general_manager',id
FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.approve',
    'vendor.purchasing.approve.business'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_procurement',id
FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.approve',
    'vendor.purchasing.approve.business'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_finance',id
FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.approve.business',
    'vendor.purchasing.approve.high_value'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_owner',id
FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.approve',
    'vendor.purchasing.approve.business',
    'vendor.purchasing.approve.high_value'
)
ON CONFLICT DO NOTHING;

-- Existing SupplierProduct identity becomes a safe FK target for PO snapshots.
CREATE UNIQUE INDEX IF NOT EXISTS
vendor_supplier_products_id_business_uq
ON vendor_supplier_products(id,business_id);

-- ============================================================
-- APPROVAL POLICY
-- ============================================================

CREATE TABLE vendor_purchase_approval_policies (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    name text NOT NULL
        CHECK (char_length(btrim(name)) BETWEEN 1 AND 160),

    allow_self_approval boolean NOT NULL DEFAULT false,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE(id,business_id)
);

CREATE UNIQUE INDEX vendor_purchase_one_active_policy_uq
ON vendor_purchase_approval_policies(business_id)
WHERE status='ACTIVE';

CREATE TABLE vendor_purchase_approval_rules (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    policy_id text NOT NULL,

    -- NULL is the business-wide fallback.
    -- An exact branch rule wins over a fallback rule.
    branch_id text,

    minimum_amount_minor bigint NOT NULL
        CHECK (minimum_amount_minor >= 0),

    maximum_amount_minor bigint
        CHECK (
            maximum_amount_minor IS NULL
            OR maximum_amount_minor > minimum_amount_minor
        ),

    required_approval_count integer NOT NULL
        CHECK (
            required_approval_count >= 1
            AND required_approval_count <= 10
        ),

    approval_scope text NOT NULL
        CHECK (
            approval_scope IN (
                'BRANCH_OR_BUSINESS',
                'BUSINESS_ONLY'
            )
        ),

    required_permission_code text NOT NULL
        CHECK (
            required_permission_code IN (
                'vendor.purchasing.approve',
                'vendor.purchasing.approve.business',
                'vendor.purchasing.approve.high_value'
            )
        ),

    step_up_required boolean NOT NULL DEFAULT false,

    position integer NOT NULL
        CHECK (position >= 0),

    policy_version bigint NOT NULL
        CHECK (policy_version > 0),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_approval_rules_policy_fk
        FOREIGN KEY(policy_id,business_id)
        REFERENCES vendor_purchase_approval_policies(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_purchase_approval_rules_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    UNIQUE(id,business_id,policy_id)
);

CREATE INDEX vendor_purchase_approval_rules_lookup_idx
ON vendor_purchase_approval_rules(
    business_id,
    policy_id,
    status,
    branch_id,
    minimum_amount_minor,
    position
);

-- ============================================================
-- PURCHASE ORDERS
-- ============================================================

CREATE TABLE vendor_purchase_orders (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    branch_id text NOT NULL,
    supplier_id text NOT NULL,

    purchase_order_number text NOT NULL,

    -- Historical Supplier snapshot.
    supplier_code_snapshot text NOT NULL DEFAULT '',
    supplier_name_snapshot text NOT NULL,
    supplier_legal_name_snapshot text NOT NULL DEFAULT '',

    payment_terms_days_snapshot integer NOT NULL DEFAULT 0
        CHECK (payment_terms_days_snapshot >= 0),

    delivery_terms_snapshot text NOT NULL DEFAULT '',

    currency_code text NOT NULL
        CHECK (
            char_length(currency_code)=3
            AND currency_code=upper(currency_code)
        ),

    subtotal_minor bigint NOT NULL DEFAULT 0
        CHECK (subtotal_minor >= 0),

    total_amount_minor bigint NOT NULL DEFAULT 0
        CHECK (total_amount_minor >= 0),

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'SUBMITTED',
                'APPROVED',
                'ORDERED',
                'PARTIALLY_RECEIVED',
                'RECEIVED',
                'CLOSED',
                'REJECTED',
                'CANCELLED'
            )
        ),

    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 8000),

    expected_delivery_date date,

    created_by_staff_id text NOT NULL,

    submitted_by_staff_id text NOT NULL DEFAULT '',
    ordered_by_staff_id text NOT NULL DEFAULT '',
    cancelled_by_staff_id text NOT NULL DEFAULT '',
    closed_by_staff_id text NOT NULL DEFAULT '',

    -- Approval policy snapshot.
    -- These are copied at SUBMITTED and never resolved dynamically afterward.
    approval_policy_id text NOT NULL DEFAULT '',
    approval_policy_version bigint NOT NULL DEFAULT 0
        CHECK (approval_policy_version >= 0),

    approval_rule_id text NOT NULL DEFAULT '',
    approval_rule_version bigint NOT NULL DEFAULT 0
        CHECK (approval_rule_version >= 0),

    required_approval_count integer NOT NULL DEFAULT 0
        CHECK (
            required_approval_count >= 0
            AND required_approval_count <= 10
        ),

    approval_permission_code text NOT NULL DEFAULT '',

    approval_scope text NOT NULL DEFAULT ''
        CHECK (
            approval_scope IN (
                '',
                'BRANCH_OR_BUSINESS',
                'BUSINESS_ONLY'
            )
        ),

    self_approval_allowed boolean NOT NULL DEFAULT false,
    approval_step_up_required boolean NOT NULL DEFAULT false,

    -- Old approval rows remain historical when a draft is changed.
    -- Only rows matching this generation count toward the active decision.
    approval_generation bigint NOT NULL DEFAULT 1
        CHECK (approval_generation > 0),

    submitted_at timestamptz,
    approved_at timestamptz,
    ordered_at timestamptz,
    received_at timestamptz,
    closed_at timestamptz,
    rejected_at timestamptz,
    cancelled_at timestamptz,

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_orders_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_purchase_orders_supplier_fk
        FOREIGN KEY(supplier_id,business_id)
        REFERENCES vendor_suppliers(id,business_id),

    UNIQUE(id,business_id)
);

CREATE UNIQUE INDEX vendor_purchase_orders_number_uq
ON vendor_purchase_orders(
    business_id,
    purchase_order_number
);

CREATE INDEX vendor_purchase_orders_branch_status_idx
ON vendor_purchase_orders(
    business_id,
    branch_id,
    status,
    updated_at DESC,
    id
);

CREATE INDEX vendor_purchase_orders_supplier_idx
ON vendor_purchase_orders(
    business_id,
    supplier_id,
    created_at DESC
);

-- ============================================================
-- PURCHASE ORDER LINES
-- ============================================================

CREATE TABLE vendor_purchase_order_lines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    purchase_order_id text NOT NULL,

    supplier_product_id text NOT NULL,
    supplier_product_version bigint NOT NULL
        CHECK (supplier_product_version > 0),

    inventory_item_id text NOT NULL,

    -- Immutable commercial/item snapshot.
    supplier_sku_snapshot text NOT NULL DEFAULT '',

    purchase_unit_code_snapshot text NOT NULL,
    purchase_unit_name_snapshot text NOT NULL,

    conversion_mode_snapshot text NOT NULL
        CHECK (
            conversion_mode_snapshot IN (
                'SAME_AS_INVENTORY',
                'EXACT_INTEGER_MULTIPLE'
            )
        ),

    inventory_units_per_purchase_unit_snapshot bigint NOT NULL
        CHECK (inventory_units_per_purchase_unit_snapshot > 0),

    inventory_measure_ref_snapshot text NOT NULL,
    inventory_measure_owner_key_snapshot text NOT NULL,

    inventory_unit_kind_snapshot text NOT NULL
        CHECK (
            inventory_unit_kind_snapshot IN (
                'COUNT',
                'MASS',
                'VOLUME',
                'CONFIGURED_MEASURE'
            )
        ),

    purchase_quantity bigint NOT NULL
        CHECK (purchase_quantity > 0),

    converted_inventory_quantity bigint NOT NULL
        CHECK (converted_inventory_quantity > 0),

    unit_cost_minor bigint NOT NULL
        CHECK (unit_cost_minor >= 0),

    line_total_minor bigint NOT NULL
        CHECK (line_total_minor >= 0),

    currency_code_snapshot text NOT NULL
        CHECK (
            char_length(currency_code_snapshot)=3
            AND currency_code_snapshot=upper(currency_code_snapshot)
        ),

    -- Section 15C updates this using absolute receipt progress.
    received_purchase_quantity bigint NOT NULL DEFAULT 0
        CHECK (
            received_purchase_quantity >= 0
            AND received_purchase_quantity <= purchase_quantity
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_order_lines_order_fk
        FOREIGN KEY(purchase_order_id,business_id)
        REFERENCES vendor_purchase_orders(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_purchase_order_lines_supplier_product_fk
        FOREIGN KEY(supplier_product_id,business_id)
        REFERENCES vendor_supplier_products(id,business_id),

    CONSTRAINT vendor_purchase_order_lines_inventory_fk
        FOREIGN KEY(inventory_item_id,business_id)
        REFERENCES inventory_items(id,business_id),

    UNIQUE(id,business_id,purchase_order_id)
);

CREATE INDEX vendor_purchase_order_lines_order_idx
ON vendor_purchase_order_lines(
    business_id,
    purchase_order_id,
    id
);

-- Historical commercial snapshots cannot be silently rewritten.
-- Before ordering, the application may replace entire line rows as a new
-- DRAFT revision. After ordering, only received quantity/version/timestamp
-- may advance.
CREATE OR REPLACE FUNCTION
vendor_purchase_order_line_history_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_status text;
BEGIN
    SELECT status
      INTO parent_status
      FROM vendor_purchase_orders
     WHERE id=OLD.purchase_order_id
       AND business_id=OLD.business_id;

    IF TG_OP='DELETE' THEN
        IF parent_status NOT IN ('DRAFT','SUBMITTED','APPROVED') THEN
            RAISE EXCEPTION
                'ordered purchase-order history is immutable';
        END IF;

        RETURN OLD;
    END IF;

    IF parent_status NOT IN ('DRAFT','SUBMITTED','APPROVED') THEN
        IF NEW.id <> OLD.id
           OR NEW.business_id <> OLD.business_id
           OR NEW.purchase_order_id <> OLD.purchase_order_id
           OR NEW.supplier_product_id <> OLD.supplier_product_id
           OR NEW.supplier_product_version <> OLD.supplier_product_version
           OR NEW.inventory_item_id <> OLD.inventory_item_id
           OR NEW.supplier_sku_snapshot <> OLD.supplier_sku_snapshot
           OR NEW.purchase_unit_code_snapshot <> OLD.purchase_unit_code_snapshot
           OR NEW.purchase_unit_name_snapshot <> OLD.purchase_unit_name_snapshot
           OR NEW.conversion_mode_snapshot <> OLD.conversion_mode_snapshot
           OR NEW.inventory_units_per_purchase_unit_snapshot <>
              OLD.inventory_units_per_purchase_unit_snapshot
           OR NEW.inventory_measure_ref_snapshot <>
              OLD.inventory_measure_ref_snapshot
           OR NEW.inventory_measure_owner_key_snapshot <>
              OLD.inventory_measure_owner_key_snapshot
           OR NEW.inventory_unit_kind_snapshot <>
              OLD.inventory_unit_kind_snapshot
           OR NEW.purchase_quantity <> OLD.purchase_quantity
           OR NEW.converted_inventory_quantity <>
              OLD.converted_inventory_quantity
           OR NEW.unit_cost_minor <> OLD.unit_cost_minor
           OR NEW.line_total_minor <> OLD.line_total_minor
           OR NEW.currency_code_snapshot <> OLD.currency_code_snapshot THEN

            RAISE EXCEPTION
                'ordered purchase-order commercial snapshot is immutable';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER vendor_purchase_order_line_history_guard_trigger
BEFORE UPDATE OR DELETE
ON vendor_purchase_order_lines
FOR EACH ROW
EXECUTE FUNCTION vendor_purchase_order_line_history_guard();

-- ============================================================
-- APPROVAL HISTORY
-- ============================================================

CREATE TABLE vendor_purchase_order_approvals (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    purchase_order_id text NOT NULL,

    approval_generation bigint NOT NULL
        CHECK (approval_generation > 0),

    approver_staff_id text NOT NULL,

    decision text NOT NULL
        CHECK (decision IN ('APPROVED','REJECTED')),

    permission_code_snapshot text NOT NULL,
    approval_scope_snapshot text NOT NULL,

    amount_minor_snapshot bigint NOT NULL
        CHECK (amount_minor_snapshot >= 0),

    purchase_order_version_before bigint NOT NULL
        CHECK (purchase_order_version_before > 0),

    step_up_consumed boolean NOT NULL DEFAULT false,

    note text NOT NULL DEFAULT ''
        CHECK (char_length(note) <= 2000),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_order_approvals_order_fk
        FOREIGN KEY(purchase_order_id,business_id)
        REFERENCES vendor_purchase_orders(id,business_id),

    UNIQUE(
        business_id,
        purchase_order_id,
        approval_generation,
        approver_staff_id
    )
);

CREATE INDEX vendor_purchase_order_approvals_history_idx
ON vendor_purchase_order_approvals(
    business_id,
    purchase_order_id,
    approval_generation,
    created_at,
    id
);

-- ============================================================
-- STATE HISTORY
-- ============================================================

CREATE TABLE vendor_purchase_order_status_history (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    purchase_order_id text NOT NULL,

    from_status text NOT NULL,
    to_status text NOT NULL,

    actor_ref text NOT NULL,

    purchase_order_version bigint NOT NULL
        CHECK (purchase_order_version > 0),

    reason text NOT NULL DEFAULT ''
        CHECK (char_length(reason) <= 2000),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_order_status_history_order_fk
        FOREIGN KEY(purchase_order_id,business_id)
        REFERENCES vendor_purchase_orders(id,business_id)
);

CREATE INDEX vendor_purchase_order_status_history_idx
ON vendor_purchase_order_status_history(
    business_id,
    purchase_order_id,
    created_at,
    id
);

-- ============================================================
-- DURABLE DOMAIN-COMMAND IDEMPOTENCY
-- ============================================================

CREATE TABLE vendor_purchase_order_commands (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    command_ref text NOT NULL,

    purchase_order_id text NOT NULL,
    command_type text NOT NULL,

    request_hash text NOT NULL,

    result_snapshot jsonb NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_order_commands_order_fk
        FOREIGN KEY(purchase_order_id,business_id)
        REFERENCES vendor_purchase_orders(id,business_id),

    UNIQUE(business_id,command_ref)
);

CREATE INDEX vendor_purchase_order_commands_order_idx
ON vendor_purchase_order_commands(
    business_id,
    purchase_order_id,
    created_at
);