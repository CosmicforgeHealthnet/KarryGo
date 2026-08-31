DROP TRIGGER IF EXISTS commerce_order_events_immutable ON commerce_order_events;
DROP TRIGGER IF EXISTS commerce_order_allocations_immutable ON commerce_order_allocations;
DROP TRIGGER IF EXISTS commerce_vendor_order_lines_immutable ON commerce_vendor_order_lines;

DROP TABLE IF EXISTS commerce_order_events;
DROP TABLE IF EXISTS commerce_order_allocations;
DROP TABLE IF EXISTS commerce_vendor_order_lines;
DROP TABLE IF EXISTS commerce_vendor_orders;
DROP TABLE IF EXISTS commerce_parent_orders;

DROP FUNCTION IF EXISTS commerce_order_reject_mutation();
