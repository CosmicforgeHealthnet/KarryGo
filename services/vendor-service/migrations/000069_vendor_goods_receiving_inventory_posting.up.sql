-- Section 15C — Goods Receiving, Inventory Posting & Discrepancies.
--
-- Purchasing owns:
--   receipt workflow
--   inspection results
--   supplier discrepancies
--   supplier delivery document references
--   immutable purchase-cost evidence
--
-- Generic Inventory remains the sole physical stock authority.
--
-- No wallet/ledger movement is created by this migration or workflow.

CREATE UNIQUE INDEX IF NOT EXISTS vendor_purchase_orders_id_business_uq
ON vendor_purchase_orders (id, business_id);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_purchase_order_lines_id_business_po_uq
ON vendor_purchase_order_lines (id, business_id, purchase_order_id);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_suppliers_id_business_uq
ON vendor_suppliers (id, business_id);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_supplier_products_id_business_uq
ON vendor_supplier_products (id, business_id);

CREATE UNIQUE INDEX IF NOT EXISTS vendor_supplier_products_id_business_branch_uq
ON vendor_supplier_products (id, business_id, branch_id);

-- Exactly one generic Inventory receipt may represent a confirmed
-- Purchasing GoodsReceipt. This is defense-in-depth against duplicate confirm.
CREATE UNIQUE INDEX IF NOT EXISTS inventory_receipts_purchasing_source_uq
ON inventory_receipts (business_id, source_ref)
WHERE source_ref IS NOT NULL
  AND source_ref LIKE 'PURCHASING:%';

-- ============================================================
-- EFFECTIVE RECEIVING POLICY
-- ============================================================

CREATE TABLE vendor_goods_receiving_policies (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    branch_id text,

    over_delivery_mode text NOT NULL DEFAULT 'DENY'
        CHECK (over_delivery_mode IN ('DENY','ALLOW_WITH_TOLERANCE')),

    over_delivery_tolerance_bps bigint NOT NULL DEFAULT 0
        CHECK (
            over_delivery_tolerance_bps >= 0
            AND over_delivery_tolerance_bps <= 10000
        ),

    cost_variance_approval_bps bigint NOT NULL DEFAULT 0
        CHECK (
            cost_variance_approval_bps >= 0
            AND cost_variance_approval_bps <= 10000
        ),

    cost_variance_approval_minor bigint NOT NULL DEFAULT 0
        CHECK (cost_variance_approval_minor >= 0),

    require_cost_variance_approval boolean NOT NULL DEFAULT true,

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_goods_receiving_policy_branch_fk
        FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches(id,business_id)
);

CREATE UNIQUE INDEX vendor_goods_receiving_policy_business_default_uq
ON vendor_goods_receiving_policies(business_id)
WHERE branch_id IS NULL;

CREATE UNIQUE INDEX vendor_goods_receiving_policy_branch_uq
ON vendor_goods_receiving_policies(business_id,branch_id)
WHERE branch_id IS NOT NULL;

-- ============================================================
-- GOODS RECEIPT
-- ============================================================

CREATE TABLE vendor_goods_receipts (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    purchase_order_id text NOT NULL,
    supplier_id text NOT NULL,
    branch_id text NOT NULL,

    supplier_delivery_reference text,
    invoice_reference text,
    invoice_total_minor bigint
        CHECK (invoice_total_minor IS NULL OR invoice_total_minor >= 0),

    currency text NOT NULL
        CHECK (char_length(currency) BETWEEN 3 AND 8),

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'INSPECTED',
                'POSTED',
                'REVERSED',
                'CANCELLED'
            )
        ),

    received_at timestamptz NOT NULL,
    inspected_at timestamptz,
    posted_at timestamptz,
    reversed_at timestamptz,

    received_by_staff_id text NOT NULL,
    inspected_by_staff_id text,
    reversed_by_staff_id text,

    inventory_receipt_id text,

    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 4000),

    reversal_reason text
        CHECK (
            reversal_reason IS NULL
            OR char_length(reversal_reason) <= 2000
        ),

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_goods_receipts_po_fk
        FOREIGN KEY (purchase_order_id,business_id)
        REFERENCES vendor_purchase_orders(id,business_id),

    CONSTRAINT vendor_goods_receipts_supplier_fk
        FOREIGN KEY (supplier_id,business_id)
        REFERENCES vendor_suppliers(id,business_id),

    CONSTRAINT vendor_goods_receipts_branch_fk
        FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_goods_receipts_inventory_receipt_fk
        FOREIGN KEY (inventory_receipt_id,business_id,branch_id)
        REFERENCES inventory_receipts(id,business_id,branch_id),

    UNIQUE(id,business_id),
    UNIQUE(id,business_id,branch_id)
);

CREATE INDEX vendor_goods_receipts_po_idx
ON vendor_goods_receipts(
    business_id,
    purchase_order_id,
    status,
    received_at DESC,
    id
);

CREATE INDEX vendor_goods_receipts_branch_idx
ON vendor_goods_receipts(
    business_id,
    branch_id,
    status,
    received_at DESC,
    id
);

-- ============================================================
-- INSPECTED RECEIPT LINES
-- ============================================================

CREATE TABLE vendor_goods_receipt_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    goods_receipt_id text NOT NULL,
    purchase_order_id text NOT NULL,
    purchase_order_line_id text,

    supplier_product_id text,
    inventory_item_id text,

    purchase_unit_code text NOT NULL
        CHECK (char_length(purchase_unit_code) BETWEEN 1 AND 80),

    inventory_units_per_purchase_unit bigint NOT NULL
        CHECK (inventory_units_per_purchase_unit > 0),

    ordered_quantity_snapshot bigint
        CHECK (
            ordered_quantity_snapshot IS NULL
            OR ordered_quantity_snapshot > 0
        ),

    received_before_snapshot bigint
        CHECK (
            received_before_snapshot IS NULL
            OR received_before_snapshot >= 0
        ),

    delivered_quantity bigint NOT NULL
        CHECK (delivered_quantity >= 0),

    accepted_quantity bigint NOT NULL
        CHECK (accepted_quantity >= 0),

    damaged_quantity bigint NOT NULL DEFAULT 0
        CHECK (damaged_quantity >= 0),

    rejected_quantity bigint NOT NULL DEFAULT 0
        CHECK (rejected_quantity >= 0),

    short_quantity bigint NOT NULL DEFAULT 0
        CHECK (short_quantity >= 0),

    inventory_quantity bigint NOT NULL DEFAULT 0
        CHECK (inventory_quantity >= 0),

    wrong_item boolean NOT NULL DEFAULT false,

    wrong_item_reference text
        CHECK (
            wrong_item_reference IS NULL
            OR char_length(wrong_item_reference) <= 300
        ),

    quality_issue boolean NOT NULL DEFAULT false,

    quality_notes text NOT NULL DEFAULT ''
        CHECK (char_length(quality_notes) <= 2000),

    expected_unit_cost_minor bigint
        CHECK (
            expected_unit_cost_minor IS NULL
            OR expected_unit_cost_minor >= 0
        ),

    actual_unit_cost_minor bigint
        CHECK (
            actual_unit_cost_minor IS NULL
            OR actual_unit_cost_minor >= 0
        ),

    currency text NOT NULL
        CHECK (char_length(currency) BETWEEN 3 AND 8),

    lot_number text,
    batch_number text,
    manufactured_at timestamptz,
    best_before_at timestamptz,
    expires_at timestamptz,

    inventory_receipt_line_id text,
    inventory_movement_id text,
    inventory_lot_id text,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_goods_receipt_lines_receipt_fk
        FOREIGN KEY (goods_receipt_id,business_id)
        REFERENCES vendor_goods_receipts(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_goods_receipt_lines_po_line_fk
        FOREIGN KEY (
            purchase_order_line_id,
            business_id,
            purchase_order_id
        )
        REFERENCES vendor_purchase_order_lines(
            id,
            business_id,
            purchase_order_id
        ),

    CONSTRAINT vendor_goods_receipt_lines_supplier_product_fk
        FOREIGN KEY (supplier_product_id,business_id)
        REFERENCES vendor_supplier_products(id,business_id),

    CONSTRAINT vendor_goods_receipt_lines_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id
        )
        REFERENCES inventory_items(id,business_id),

    CONSTRAINT vendor_goods_receipt_lines_quantity_ck
        CHECK (
            delivered_quantity =
                accepted_quantity +
                damaged_quantity +
                rejected_quantity
        ),

    CONSTRAINT vendor_goods_receipt_lines_wrong_item_ck
        CHECK (
            NOT wrong_item
            OR (
                accepted_quantity = 0
                AND inventory_quantity = 0
                AND purchase_order_line_id IS NULL
                AND inventory_item_id IS NULL
            )
        ),

    CONSTRAINT vendor_goods_receipt_lines_linked_ck
        CHECK (
            wrong_item
            OR (
                purchase_order_line_id IS NOT NULL
                AND supplier_product_id IS NOT NULL
                AND inventory_item_id IS NOT NULL
                AND ordered_quantity_snapshot IS NOT NULL
                AND received_before_snapshot IS NOT NULL
                AND expected_unit_cost_minor IS NOT NULL
            )
        ),

    UNIQUE(id,business_id),
    UNIQUE(goods_receipt_id,purchase_order_line_id)
);

CREATE INDEX vendor_goods_receipt_lines_po_idx
ON vendor_goods_receipt_lines(
    business_id,
    purchase_order_id,
    purchase_order_line_id
);

CREATE INDEX vendor_goods_receipt_lines_inventory_idx
ON vendor_goods_receipt_lines(
    business_id,
    inventory_item_id
)
WHERE inventory_item_id IS NOT NULL;

-- ============================================================
-- SUPPLIER DELIVERY DOCUMENT / MEDIA REFERENCES
-- ============================================================

CREATE TABLE vendor_goods_receipt_documents (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    goods_receipt_id text NOT NULL,

    document_type text NOT NULL
        CHECK (
            document_type IN (
                'DELIVERY_NOTE',
                'INVOICE',
                'PACKING_LIST',
                'QUALITY_CERTIFICATE',
                'OTHER'
            )
        ),

    media_ref text NOT NULL
        CHECK (char_length(media_ref) BETWEEN 1 AND 300),

    external_reference text
        CHECK (
            external_reference IS NULL
            OR char_length(external_reference) <= 300
        ),

    attached_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_goods_receipt_documents_receipt_fk
        FOREIGN KEY (goods_receipt_id,business_id)
        REFERENCES vendor_goods_receipts(id,business_id)
        ON DELETE CASCADE,

    UNIQUE(business_id,goods_receipt_id,media_ref)
);

CREATE INDEX vendor_goods_receipt_documents_receipt_idx
ON vendor_goods_receipt_documents(
    business_id,
    goods_receipt_id,
    created_at DESC
);

-- ============================================================
-- SUPPLIER DISCREPANCIES
-- ============================================================

CREATE TABLE vendor_supplier_discrepancies (
    id text PRIMARY KEY,
    business_id text NOT NULL,

    supplier_id text NOT NULL,
    purchase_order_id text NOT NULL,
    purchase_order_line_id text,

    goods_receipt_id text NOT NULL,
    goods_receipt_line_id text,

    branch_id text NOT NULL,

    discrepancy_type text NOT NULL
        CHECK (
            discrepancy_type IN (
                'SHORT_DELIVERY',
                'OVER_DELIVERY',
                'DAMAGED',
                'REJECTED',
                'WRONG_ITEM',
                'QUALITY_ISSUE',
                'COST_VARIANCE'
            )
        ),

    status text NOT NULL DEFAULT 'OPEN'
        CHECK (
            status IN (
                'OPEN',
                'APPROVAL_REQUIRED',
                'APPROVED',
                'REJECTED',
                'RESOLVED',
                'VOIDED'
            )
        ),

    quantity bigint
        CHECK (quantity IS NULL OR quantity >= 0),

    expected_cost_minor bigint
        CHECK (
            expected_cost_minor IS NULL
            OR expected_cost_minor >= 0
        ),

    actual_cost_minor bigint
        CHECK (
            actual_cost_minor IS NULL
            OR actual_cost_minor >= 0
        ),

    variance_minor bigint,
    variance_bps bigint,

    currency text
        CHECK (
            currency IS NULL
            OR char_length(currency) BETWEEN 3 AND 8
        ),

    reason_code text,
    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 3000),

    opened_by_staff_id text NOT NULL,
    decided_by_staff_id text,
    resolved_by_staff_id text,

    decided_at timestamptz,
    resolved_at timestamptz,

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_supplier_discrepancies_supplier_fk
        FOREIGN KEY (supplier_id,business_id)
        REFERENCES vendor_suppliers(id,business_id),

    CONSTRAINT vendor_supplier_discrepancies_receipt_fk
        FOREIGN KEY (goods_receipt_id,business_id)
        REFERENCES vendor_goods_receipts(id,business_id),

    CONSTRAINT vendor_supplier_discrepancies_receipt_line_fk
        FOREIGN KEY (goods_receipt_line_id,business_id)
        REFERENCES vendor_goods_receipt_lines(id,business_id),

    CONSTRAINT vendor_supplier_discrepancies_branch_fk
        FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_supplier_discrepancies_open_idx
ON vendor_supplier_discrepancies(
    business_id,
    branch_id,
    status,
    discrepancy_type,
    created_at DESC
);

CREATE INDEX vendor_supplier_discrepancies_supplier_idx
ON vendor_supplier_discrepancies(
    business_id,
    supplier_id,
    status,
    created_at DESC
);

-- ============================================================
-- IMMUTABLE PURCHASE COST HISTORY
-- ============================================================

CREATE TABLE vendor_purchase_cost_history (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,

    supplier_id text NOT NULL,
    supplier_product_id text NOT NULL,
    inventory_item_id text NOT NULL,

    purchase_order_id text NOT NULL,
    purchase_order_line_id text NOT NULL,
    goods_receipt_id text NOT NULL,
    goods_receipt_line_id text NOT NULL,

    evidence_type text NOT NULL
        CHECK (evidence_type IN ('OBSERVED','REVERSAL')),

    purchase_unit_code text NOT NULL,
    inventory_units_per_purchase_unit bigint NOT NULL
        CHECK (inventory_units_per_purchase_unit > 0),

    purchase_quantity bigint NOT NULL
        CHECK (purchase_quantity > 0),

    inventory_quantity bigint NOT NULL
        CHECK (inventory_quantity > 0),

    purchase_unit_cost_minor bigint NOT NULL
        CHECK (purchase_unit_cost_minor >= 0),

    total_cost_minor bigint NOT NULL
        CHECK (total_cost_minor >= 0),

    inventory_unit_cost_minor bigint
        CHECK (
            inventory_unit_cost_minor IS NULL
            OR inventory_unit_cost_minor >= 0
        ),

    currency text NOT NULL,

    observed_at timestamptz NOT NULL,
    recorded_by_staff_id text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_purchase_cost_history_supplier_fk
        FOREIGN KEY (supplier_id,business_id)
        REFERENCES vendor_suppliers(id,business_id),

    CONSTRAINT vendor_purchase_cost_history_supplier_product_fk
        FOREIGN KEY (supplier_product_id,business_id)
        REFERENCES vendor_supplier_products(id,business_id),

    CONSTRAINT vendor_purchase_cost_history_item_fk
        FOREIGN KEY (inventory_item_id,business_id)
        REFERENCES inventory_items(id,business_id),

    CONSTRAINT vendor_purchase_cost_history_receipt_fk
        FOREIGN KEY (goods_receipt_id,business_id)
        REFERENCES vendor_goods_receipts(id,business_id),

    CONSTRAINT vendor_purchase_cost_history_receipt_line_fk
        FOREIGN KEY (goods_receipt_line_id,business_id)
        REFERENCES vendor_goods_receipt_lines(id,business_id),

    UNIQUE(
        business_id,
        goods_receipt_line_id,
        evidence_type
    )
);

CREATE INDEX vendor_purchase_cost_history_item_idx
ON vendor_purchase_cost_history(
    business_id,
    inventory_item_id,
    observed_at DESC,
    id
);

CREATE INDEX vendor_purchase_cost_history_supplier_product_idx
ON vendor_purchase_cost_history(
    business_id,
    supplier_product_id,
    observed_at DESC,
    id
);

-- Cost history is commercial evidence. It is append-only.
CREATE OR REPLACE FUNCTION vendor_purchase_cost_history_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'vendor_purchase_cost_history is immutable'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER vendor_purchase_cost_history_immutable_trg
BEFORE UPDATE OR DELETE
ON vendor_purchase_cost_history
FOR EACH ROW
EXECUTE FUNCTION vendor_purchase_cost_history_immutable();

-- ============================================================
-- DURABLE CONFIRM / REVERSAL COMMAND DEDUPE
-- ============================================================

CREATE TABLE vendor_goods_receipt_commands (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),

    command_ref text NOT NULL,
    operation text NOT NULL
        CHECK (operation IN ('CONFIRM','REVERSE')),

    request_hash text NOT NULL
        CHECK (char_length(request_hash) = 64),

    goods_receipt_id text NOT NULL,
    result_version bigint NOT NULL CHECK (result_version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_goods_receipt_commands_receipt_fk
        FOREIGN KEY(goods_receipt_id,business_id)
        REFERENCES vendor_goods_receipts(id,business_id),

    UNIQUE(business_id,command_ref)
);

-- ============================================================
-- 15C IAM
-- ============================================================

INSERT INTO vendor_permissions(
    id,
    code,
    description,
    category,
    is_high_risk
)
VALUES
(
    'perm_vendor_purchasing_receive',
    'vendor.purchasing.receive',
    'Inspect and confirm supplier goods receipts into Inventory.',
    'purchasing',
    false
),
(
    'perm_vendor_purchasing_discrepancies_manage',
    'vendor.purchasing.discrepancies.manage',
    'Manage supplier receiving discrepancies.',
    'purchasing',
    false
),
(
    'perm_vendor_purchasing_cost_read',
    'vendor.purchasing.cost.read',
    'Read immutable purchase cost history.',
    'purchasing',
    false
),
(
    'perm_vendor_purchasing_cost_variance_approve',
    'vendor.purchasing.cost_variance.approve',
    'Approve supplier purchase cost variances.',
    'purchasing',
    true
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.role_id,p.id
FROM (
    VALUES
        ('role_owner'),
        ('role_general_manager'),
        ('role_procurement')
) AS r(role_id)
JOIN vendor_permissions p
  ON p.code IN (
      'vendor.purchasing.receive',
      'vendor.purchasing.discrepancies.manage',
      'vendor.purchasing.cost.read',
      'vendor.purchasing.cost_variance.approve'
  )
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_shop_inventory',p.id
FROM vendor_permissions p
WHERE p.code = 'vendor.purchasing.receive'
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_finance',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.purchasing.cost.read',
    'vendor.purchasing.cost_variance.approve'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',p.id
FROM vendor_permissions p
WHERE p.code = 'vendor.purchasing.cost.read'
ON CONFLICT DO NOTHING;