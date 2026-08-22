-- Section 15D — Purchasing operations, alerts and performance.
--
-- 15A/15B/15C own the purchasing record: suppliers, purchase orders, goods
-- receipts, discrepancies and immutable purchase-cost evidence. This migration
-- adds only the operational layer on top of that record:
--
--   * a durable purchasing alert store, so overdue and expected-delivery
--     signals survive a restart and do not re-notify on every worker pass
--   * indexes for the operational hot paths 15D introduces (overdue scan,
--     upcoming deliveries, approval queue, dashboards, supplier performance)
--   * the two read permissions the operational surface needs
--
-- No purchasing truth is duplicated here. Every dashboard and performance
-- figure is derived from the 15A/15B/15C tables at read time; this migration
-- adds no projection table, because indexed queries over those tables are
-- sufficient at the scale certified in 15D.
--
-- Generic Inventory remains the sole physical stock authority, and nothing in
-- this migration creates a wallet, ledger or payment effect.

-- ============================================================
-- PURCHASING OPERATIONAL ALERTS
-- ============================================================
--
-- Mirrors the certified inventory_alerts shape rather than inventing a second
-- alerting model: one open alert per (business, dedupe_key), moved through an
-- operator lifecycle instead of being re-created on every sweep.

CREATE TABLE vendor_purchasing_alerts (
    id text PRIMARY KEY,

    business_id text NOT NULL REFERENCES vendor_businesses(id),

    -- Branch is empty for business-wide alerts. Branch-scoped staff only see
    -- alerts for branches they hold, plus business-wide ones.
    branch_id text NOT NULL DEFAULT '',

    alert_type text NOT NULL
        CHECK (
            alert_type IN (
                'PO_EXPECTED_SOON',
                'PO_EXPECTED_TODAY',
                'PO_OVERDUE',
                'PO_MATERIALLY_OVERDUE',
                'SUPPLIER_DELIVERY_INCOMPLETE',
                'DISCREPANCY_UNRESOLVED',
                'DISCREPANCY_ESCALATED'
            )
        ),

    severity text NOT NULL DEFAULT 'WARNING'
        CHECK (severity IN ('INFO', 'WARNING', 'CRITICAL')),

    status text NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN', 'ACKNOWLEDGED', 'RESOLVED')),

    -- Subject references. Exactly which one is populated depends on
    -- alert_type; all are plain references, never foreign keys into another
    -- tenant's rows, and the composite tenant guards below enforce ownership.
    purchase_order_id text NOT NULL DEFAULT '',
    supplier_id text NOT NULL DEFAULT '',
    discrepancy_id text NOT NULL DEFAULT '',

    -- The window an alert belongs to. Re-running the sweep inside the same
    -- window must not raise a second alert, which is what dedupe_key encodes.
    dedupe_key text NOT NULL
        CHECK (char_length(btrim(dedupe_key)) > 0),

    -- Bounded operational context for an operator screen. No supplier contact
    -- details, no PII.
    details jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    resolved_at timestamptz,

    CONSTRAINT vendor_purchasing_alerts_po_fk
        FOREIGN KEY (purchase_order_id, business_id)
        REFERENCES vendor_purchase_orders (id, business_id)
        DEFERRABLE INITIALLY IMMEDIATE,

    CONSTRAINT vendor_purchasing_alerts_supplier_fk
        FOREIGN KEY (supplier_id, business_id)
        REFERENCES vendor_suppliers (id, business_id)
        DEFERRABLE INITIALLY IMMEDIATE
);

-- One alert per dedupe key per tenant. This is the idempotency guarantee the
-- overdue and expected-delivery workers rely on: a repeated pass conflicts
-- rather than inserting a duplicate.
CREATE UNIQUE INDEX vendor_purchasing_alerts_dedupe_uq
ON vendor_purchasing_alerts (business_id, dedupe_key);

CREATE INDEX vendor_purchasing_alerts_open_idx
ON vendor_purchasing_alerts (business_id, branch_id, status, created_at DESC);

CREATE INDEX vendor_purchasing_alerts_po_idx
ON vendor_purchasing_alerts (business_id, purchase_order_id)
WHERE purchase_order_id <> '';

CREATE INDEX vendor_purchasing_alerts_discrepancy_idx
ON vendor_purchasing_alerts (business_id, discrepancy_id)
WHERE discrepancy_id <> '';

-- ============================================================
-- OPERATIONAL INDEXES
-- ============================================================
--
-- Every index below exists because 15D introduces a query that would otherwise
-- scan. They are tenant-leading so a large platform never scans across
-- vendors, and partial where the operational query is itself partial.

-- Overdue and upcoming-delivery scans read only outstanding orders. A partial
-- index keeps the scan proportional to outstanding work rather than to
-- purchase-order history, which is dominated by terminal states.
CREATE INDEX vendor_purchase_orders_outstanding_delivery_idx
ON vendor_purchase_orders (business_id, expected_delivery_date, branch_id)
WHERE status IN ('ORDERED', 'PARTIALLY_RECEIVED')
  AND expected_delivery_date IS NOT NULL;

-- The cross-tenant worker sweep orders by expected date across all vendors,
-- so it needs a leading date rather than a leading business.
CREATE INDEX vendor_purchase_orders_delivery_sweep_idx
ON vendor_purchase_orders (expected_delivery_date, id)
WHERE status IN ('ORDERED', 'PARTIALLY_RECEIVED')
  AND expected_delivery_date IS NOT NULL;

-- Approval queue: orders waiting on a decision.
CREATE INDEX vendor_purchase_orders_approval_queue_idx
ON vendor_purchase_orders (business_id, branch_id, created_at DESC)
WHERE status = 'SUBMITTED';

-- Dashboard status counts and supplier performance both group by status.
CREATE INDEX vendor_purchase_orders_status_counts_idx
ON vendor_purchase_orders (business_id, status, branch_id);

-- Supplier performance reads a supplier's order history newest-first.
CREATE INDEX vendor_purchase_orders_supplier_history_idx
ON vendor_purchase_orders (business_id, supplier_id, created_at DESC);

-- Receiving dashboards and supplier lead-time both scan receipts by status
-- and time.
CREATE INDEX vendor_goods_receipts_status_time_idx
ON vendor_goods_receipts (business_id, status, received_at DESC);

CREATE INDEX vendor_goods_receipts_supplier_time_idx
ON vendor_goods_receipts (business_id, supplier_id, received_at DESC);

-- Discrepancy follow-up: the operational view is always "still outstanding,
-- oldest first", so age ordering belongs in the index.
CREATE INDEX vendor_supplier_discrepancies_followup_idx
ON vendor_supplier_discrepancies (business_id, status, created_at)
WHERE status IN ('OPEN', 'APPROVAL_REQUIRED');

CREATE INDEX vendor_supplier_discrepancies_supplier_status_idx
ON vendor_supplier_discrepancies (business_id, supplier_id, status);

CREATE INDEX vendor_supplier_discrepancies_type_idx
ON vendor_supplier_discrepancies (business_id, discrepancy_type, status);

-- Cost history reads: by supplier, and newest-first within a tenant.
CREATE INDEX vendor_purchase_cost_history_supplier_idx
ON vendor_purchase_cost_history (business_id, supplier_id, observed_at DESC);

CREATE INDEX vendor_purchase_cost_history_recent_idx
ON vendor_purchase_cost_history (business_id, observed_at DESC);

-- ============================================================
-- OPERATIONAL PERMISSIONS
-- ============================================================
--
-- Two read permissions only. Operational visibility is separated from
-- vendor.purchasing.read because a supplier performance summary and a
-- purchasing dashboard aggregate a whole business, which is a strictly wider
-- disclosure than reading one purchase order. No new write permission is
-- added: 15D introduces no new mutation that existing permissions do not
-- already govern.

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk)
VALUES
(
    'perm_vendor_purchasing_performance_read',
    'vendor.purchasing.performance.read',
    'View supplier performance summaries derived from purchasing evidence.',
    'purchasing',
    false
),
(
    'perm_vendor_purchasing_dashboard_read',
    'vendor.purchasing.dashboard.read',
    'View purchasing operational dashboards, overdue and upcoming deliveries.',
    'purchasing',
    false
)
ON CONFLICT (code) DO NOTHING;

-- Owner, General Manager and Procurement run purchasing operations.
INSERT INTO vendor_role_permissions(role_id, permission_id)
SELECT r.role_id, p.id
FROM (
    VALUES
        ('role_owner'),
        ('role_general_manager'),
        ('role_procurement')
) AS r(role_id)
JOIN vendor_permissions p
  ON p.code IN (
      'vendor.purchasing.performance.read',
      'vendor.purchasing.dashboard.read'
  )
ON CONFLICT DO NOTHING;

-- A Branch Manager sees the operational picture for their own branches; the
-- scope resolver, not this grant, is what limits them to those branches.
INSERT INTO vendor_role_permissions(role_id, permission_id)
SELECT 'role_branch_manager', p.id
FROM vendor_permissions p
WHERE p.code = 'vendor.purchasing.dashboard.read'
ON CONFLICT DO NOTHING;

-- Finance and Analyst/Auditor already read purchase cost; supplier
-- performance is the same class of commercial reporting.
INSERT INTO vendor_role_permissions(role_id, permission_id)
SELECT r.role_id, p.id
FROM (
    VALUES
        ('role_finance'),
        ('role_analyst_auditor')
) AS r(role_id)
JOIN vendor_permissions p
  ON p.code = 'vendor.purchasing.performance.read'
ON CONFLICT DO NOTHING;
