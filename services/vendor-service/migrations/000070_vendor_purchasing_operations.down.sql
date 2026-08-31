-- Reverse Section 15D's operational layer.
--
-- Only 15D-owned objects are dropped. The 15A/15B/15C purchasing record is
-- untouched, because none of it was created here.

DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id FROM vendor_permissions
    WHERE code IN (
        'vendor.purchasing.performance.read',
        'vendor.purchasing.dashboard.read'
    )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.performance.read',
    'vendor.purchasing.dashboard.read'
);

DROP INDEX IF EXISTS vendor_purchase_cost_history_recent_idx;
DROP INDEX IF EXISTS vendor_purchase_cost_history_supplier_idx;

DROP INDEX IF EXISTS vendor_supplier_discrepancies_type_idx;
DROP INDEX IF EXISTS vendor_supplier_discrepancies_supplier_status_idx;
DROP INDEX IF EXISTS vendor_supplier_discrepancies_followup_idx;

DROP INDEX IF EXISTS vendor_goods_receipts_supplier_time_idx;
DROP INDEX IF EXISTS vendor_goods_receipts_status_time_idx;

DROP INDEX IF EXISTS vendor_purchase_orders_supplier_history_idx;
DROP INDEX IF EXISTS vendor_purchase_orders_status_counts_idx;
DROP INDEX IF EXISTS vendor_purchase_orders_approval_queue_idx;
DROP INDEX IF EXISTS vendor_purchase_orders_delivery_sweep_idx;
DROP INDEX IF EXISTS vendor_purchase_orders_outstanding_delivery_idx;

DROP TABLE IF EXISTS vendor_purchasing_alerts;
