DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM commerce_substitution_proposals WHERE admin_inventory_processing_state<>'NOT_RECORDED') THEN
 RAISE EXCEPTION 'Cannot discard authoritative substitution processing evidence'; END IF;
END $$;
ALTER TABLE commerce_substitution_proposals DROP CONSTRAINT commerce_substitution_inventory_processing_consistency,
 DROP COLUMN admin_inventory_operation_key,DROP COLUMN admin_inventory_processed_at,DROP COLUMN admin_inventory_processing_state;
