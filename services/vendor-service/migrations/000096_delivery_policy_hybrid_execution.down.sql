DROP TABLE IF EXISTS commerce_delivery_custody_events;
DROP TABLE IF EXISTS commerce_delivery_execution;

DELETE FROM vendor_role_permissions
 WHERE permission_id = (SELECT id FROM vendor_permissions WHERE code = 'vendor.delivery.operate');
DELETE FROM vendor_permissions WHERE code = 'vendor.delivery.operate';

ALTER TABLE commerce_fulfilment_missions
    DROP COLUMN IF EXISTS custody_state,
    DROP COLUMN IF EXISTS customer_price_guarantee_minor,
    DROP COLUMN IF EXISTS policy_version,
    DROP COLUMN IF EXISTS fallback_trigger,
    DROP COLUMN IF EXISTS delivery_mode;

ALTER TABLE commerce_fulfilment_plan_groups
    DROP COLUMN IF EXISTS customer_price_guarantee_minor,
    DROP COLUMN IF EXISTS fallback_trigger,
    DROP COLUMN IF EXISTS delivery_mode;

ALTER TABLE commerce_fulfilment_plans
    DROP COLUMN IF EXISTS delivery_price_guarantee_currency,
    DROP COLUMN IF EXISTS customer_delivery_price_guarantee_minor,
    DROP COLUMN IF EXISTS delivery_policy_snapshot;

ALTER TABLE vendor_delivery_policies
    DROP COLUMN IF EXISTS fallback_triggers,
    DROP COLUMN IF EXISTS supported_vehicle_classes,
    DROP COLUMN IF EXISTS operating_periods,
    DROP COLUMN IF EXISTS distance_tiers,
    DROP COLUMN IF EXISTS service_zones,
    DROP COLUMN IF EXISTS rider_availability,
    DROP COLUMN IF EXISTS max_volume_ml,
    DROP COLUMN IF EXISTS max_package_count,
    DROP COLUMN IF EXISTS operating_timezone,
    DROP COLUMN IF EXISTS fee_rule,
    DROP COLUMN IF EXISTS delivery_mode;
