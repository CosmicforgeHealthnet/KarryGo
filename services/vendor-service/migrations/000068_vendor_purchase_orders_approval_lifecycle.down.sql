DROP TABLE IF EXISTS vendor_purchase_order_commands;
DROP TABLE IF EXISTS vendor_purchase_order_status_history;
DROP TABLE IF EXISTS vendor_purchase_order_approvals;

DROP TRIGGER IF EXISTS
vendor_purchase_order_line_history_guard_trigger
ON vendor_purchase_order_lines;

DROP FUNCTION IF EXISTS
vendor_purchase_order_line_history_guard();

DROP TABLE IF EXISTS vendor_purchase_order_lines;
DROP TABLE IF EXISTS vendor_purchase_orders;

DROP TABLE IF EXISTS vendor_purchase_approval_rules;
DROP TABLE IF EXISTS vendor_purchase_approval_policies;

DROP INDEX IF EXISTS vendor_supplier_products_id_business_uq;

DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
      FROM vendor_permissions
     WHERE code IN (
        'vendor.purchasing.approve.business',
        'vendor.purchasing.approve.high_value'
     )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.approve.business',
    'vendor.purchasing.approve.high_value'
);