DROP TABLE IF EXISTS commerce_vendor_hybrid_fallbacks;
DROP TABLE IF EXISTS commerce_vendor_mission_operations;
DROP TABLE IF EXISTS commerce_vendor_mission_runtime;
DROP TABLE IF EXISTS commerce_vendor_mission_assignments;
DROP TABLE IF EXISTS commerce_vendor_rider_availability_operations;
DROP TABLE IF EXISTS commerce_vendor_rider_availability;

DROP TRIGGER IF EXISTS commerce_vendor_order_effective_delivery_init ON commerce_vendor_orders;
DROP FUNCTION IF EXISTS initialize_commerce_vendor_order_effective_delivery();
ALTER TABLE commerce_vendor_orders
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_effective_delivery_check,
    DROP COLUMN IF EXISTS effective_logistics_mission_ref,
    DROP COLUMN IF EXISTS effective_delivery_fee_beneficiary,
    DROP COLUMN IF EXISTS effective_delivery_responsibility;
