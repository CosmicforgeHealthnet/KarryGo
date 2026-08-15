-- Reverse of Section 12A's Shop catalogue foundation.

DROP TRIGGER IF EXISTS shop_price_versions_no_update_or_delete ON shop_price_versions;
DROP FUNCTION IF EXISTS shop_price_versions_immutable();
DROP TRIGGER IF EXISTS shop_quantity_breaks_no_overlap_trigger ON shop_quantity_breaks;
DROP FUNCTION IF EXISTS shop_quantity_breaks_no_overlap();
DROP TRIGGER IF EXISTS shop_package_relations_validate_tree_trigger ON shop_package_relations;
DROP FUNCTION IF EXISTS shop_package_relations_validate_tree();
DROP TRIGGER IF EXISTS shop_categories_validate_tree_trigger ON shop_categories;
DROP FUNCTION IF EXISTS shop_categories_validate_tree();

DROP TABLE IF EXISTS shop_catalogue_revisions;
DROP TABLE IF EXISTS shop_price_versions;
DROP TABLE IF EXISTS shop_fresh_attribute_options;
DROP TABLE IF EXISTS shop_fresh_attributes;
DROP TABLE IF EXISTS shop_quantity_breaks;
DROP TABLE IF EXISTS shop_listing_sell_units;
DROP TABLE IF EXISTS shop_branch_listings;
DROP TABLE IF EXISTS shop_sell_units;
DROP TABLE IF EXISTS shop_measure_calibrations;
DROP TABLE IF EXISTS shop_package_relations;
DROP TABLE IF EXISTS shop_product_identifiers;
DROP TABLE IF EXISTS shop_variant_handling_flags;
DROP TABLE IF EXISTS shop_variants;
DROP TABLE IF EXISTS shop_product_operating_models;
DROP TABLE IF EXISTS shop_product_media;
DROP TABLE IF EXISTS shop_products;
DROP TABLE IF EXISTS shop_categories;
DROP TABLE IF EXISTS shop_branch_operating_model_modes;
DROP TABLE IF EXISTS shop_branch_operating_models;
DROP TABLE IF EXISTS shop_measure_aliases;
DROP TABLE IF EXISTS shop_measure_definitions;
