DROP TRIGGER IF EXISTS vendor_purchase_cost_history_immutable_trg
ON vendor_purchase_cost_history;

DROP FUNCTION IF EXISTS vendor_purchase_cost_history_immutable();

DROP TABLE IF EXISTS vendor_goods_receipt_commands;
DROP TABLE IF EXISTS vendor_purchase_cost_history;
DROP TABLE IF EXISTS vendor_supplier_discrepancies;
DROP TABLE IF EXISTS vendor_goods_receipt_documents;
DROP TABLE IF EXISTS vendor_goods_receipt_lines;
DROP TABLE IF EXISTS vendor_goods_receipts;
DROP TABLE IF EXISTS vendor_goods_receiving_policies;

DROP INDEX IF EXISTS inventory_receipts_purchasing_source_uq;

DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
    FROM vendor_permissions
    WHERE code IN (
        'vendor.purchasing.receive',
        'vendor.purchasing.discrepancies.manage',
        'vendor.purchasing.cost.read',
        'vendor.purchasing.cost_variance.approve'
    )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.purchasing.receive',
    'vendor.purchasing.discrepancies.manage',
    'vendor.purchasing.cost.read',
    'vendor.purchasing.cost_variance.approve'
);