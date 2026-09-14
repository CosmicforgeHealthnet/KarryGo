DROP TABLE IF EXISTS commerce_delivery_executions;

ALTER TABLE commerce_payment_allocations
    DROP COLUMN IF EXISTS organization_delivery_released_minor,
    DROP COLUMN IF EXISTS provider_delivery_released_minor,
    DROP COLUMN IF EXISTS managed_membership_id,
    DROP COLUMN IF EXISTS managed_organization_id,
    DROP COLUMN IF EXISTS delivery_executor_mode;
