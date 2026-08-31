DROP TRIGGER IF EXISTS inventory_items_validate_shop_variant_trigger
ON inventory_items;

DROP FUNCTION IF EXISTS inventory_items_validate_shop_variant();

DROP TABLE IF EXISTS inventory_revisions;
DROP TABLE IF EXISTS inventory_items;