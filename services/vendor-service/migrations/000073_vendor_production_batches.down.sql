DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
    FROM vendor_permissions
    WHERE code IN (
        'vendor.production.waste.record',
        'vendor.production.prepared_stock'
    )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.production.waste.record',
    'vendor.production.prepared_stock'
);

DROP TABLE IF EXISTS inventory_production_postings;
DROP TABLE IF EXISTS vendor_production_batch_outputs;
DROP TABLE IF EXISTS vendor_production_batch_inventory_splits;

DROP TRIGGER IF EXISTS vendor_production_waste_evidence_immutable_trigger
ON vendor_production_batch_waste_events;

DROP FUNCTION IF EXISTS vendor_production_waste_evidence_immutable();

DROP TABLE IF EXISTS vendor_production_batch_waste_events;
DROP TABLE IF EXISTS vendor_production_batch_reservations;
DROP TABLE IF EXISTS vendor_production_batch_packaging;
DROP TABLE IF EXISTS vendor_production_batch_components;
DROP TABLE IF EXISTS vendor_production_batches;
DROP TABLE IF EXISTS vendor_production_stock_policies;

ALTER TABLE vendor_packaging_branch_configs
DROP COLUMN IF EXISTS production_consumption_point;