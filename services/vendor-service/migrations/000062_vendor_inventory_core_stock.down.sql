DROP TRIGGER IF EXISTS inventory_movements_immutable_trigger
ON inventory_movements;

DROP FUNCTION IF EXISTS inventory_movements_immutable();

DROP TRIGGER IF EXISTS inventory_movements_insert_guard
ON inventory_movements;

DROP TRIGGER IF EXISTS inventory_lots_mutation_guard
ON inventory_lots;

DROP TRIGGER IF EXISTS inventory_balances_mutation_guard
ON inventory_balances;

DROP FUNCTION IF EXISTS inventory_authorized_mutation_required();

DROP VIEW IF EXISTS inventory_balance_availability;

DROP TABLE IF EXISTS inventory_receipt_lines;
DROP TABLE IF EXISTS inventory_receipts;
DROP TABLE IF EXISTS inventory_movements;
DROP TABLE IF EXISTS inventory_lots;
DROP TABLE IF EXISTS inventory_balances;

DROP INDEX IF EXISTS inventory_items_scope_identity_uq;