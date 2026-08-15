DROP TABLE IF EXISTS production_operational_alerts;

DROP INDEX IF EXISTS cloud_kitchen_quantity_bands_active_idx;
ALTER TABLE cloud_kitchen_quantity_bands
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS status;

ALTER TABLE cloud_kitchen_package_components
    DROP CONSTRAINT IF EXISTS cloud_kitchen_package_components_identity_uq,
    ADD CONSTRAINT cloud_kitchen_package_components_package_id_item_ref_variant_ref_key
        UNIQUE (package_id,item_ref,variant_ref);

ALTER TABLE pastry_change_requests
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS version;

ALTER TABLE pastry_options
    DROP CONSTRAINT IF EXISTS pastry_options_capacity_class_check;

ALTER TABLE production_jobs
    DROP COLUMN IF EXISTS source_version;

ALTER TABLE cloud_kitchen_occurrences
    DROP COLUMN IF EXISTS version;

ALTER TABLE pastry_quotes
    DROP CONSTRAINT IF EXISTS pastry_quotes_expiry_after_creation_check;

ALTER TABLE pastry_custom_requests
    DROP CONSTRAINT IF EXISTS pastry_custom_requests_branch_scope_fk,
    DROP CONSTRAINT IF EXISTS pastry_custom_requests_id_business_uq;

ALTER TABLE pastry_products
    DROP CONSTRAINT IF EXISTS pastry_products_capacity_pool_scope_fk,
    DROP CONSTRAINT IF EXISTS pastry_products_branch_scope_fk,
    DROP CONSTRAINT IF EXISTS pastry_products_id_business_uq;

ALTER TABLE cloud_kitchen_corporate_plans
    DROP CONSTRAINT IF EXISTS cloud_kitchen_corporate_plans_package_scope_fk,
    DROP CONSTRAINT IF EXISTS cloud_kitchen_corporate_plans_branch_scope_fk,
    DROP CONSTRAINT IF EXISTS cloud_kitchen_corporate_plans_id_business_uq;

ALTER TABLE cloud_kitchen_settings
    DROP CONSTRAINT IF EXISTS cloud_kitchen_settings_capacity_pool_scope_fk,
    DROP CONSTRAINT IF EXISTS cloud_kitchen_settings_branch_scope_fk;

ALTER TABLE cloud_kitchen_packages
    DROP CONSTRAINT IF EXISTS cloud_kitchen_packages_capacity_pool_scope_fk,
    DROP CONSTRAINT IF EXISTS cloud_kitchen_packages_branch_scope_fk,
    DROP CONSTRAINT IF EXISTS cloud_kitchen_packages_id_business_uq;
