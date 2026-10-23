-- Native processing evidence, separate from the customer's immutable decision.
ALTER TABLE commerce_substitution_proposals
 ADD COLUMN admin_inventory_processing_state text NOT NULL DEFAULT 'NOT_RECORDED' CHECK(admin_inventory_processing_state IN ('NOT_RECORDED','APPLIED','NOT_REQUIRED')),
 ADD COLUMN admin_inventory_processed_at timestamptz,
 ADD COLUMN admin_inventory_operation_key text NOT NULL DEFAULT '',
 ADD CONSTRAINT commerce_substitution_inventory_processing_consistency CHECK(
 (admin_inventory_processing_state='NOT_RECORDED' AND admin_inventory_processed_at IS NULL AND admin_inventory_operation_key='') OR
 (admin_inventory_processing_state IN ('APPLIED','NOT_REQUIRED') AND admin_inventory_processed_at IS NOT NULL AND admin_inventory_operation_key='substitution-inventory-'||id));
