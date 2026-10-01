-- Reverse of 021.
--
-- The capability columns are dropped along with the tables. That does lose
-- every survey a provider entered, which is the honest consequence of rolling
-- back a schema that introduced them: there is nowhere else for those facts to
-- live. capacity_kg and has_insurance are untouched, so the legacy
-- weight-and-type matcher still has everything it ever used.

DROP TABLE IF EXISTS hauling_match_decisions;
DROP TABLE IF EXISTS hauling_policy_snapshots;
DROP TABLE IF EXISTS hauling_truck_product_eligibility;

ALTER TABLE trucks
    DROP COLUMN IF EXISTS max_payload_grams,
    DROP COLUMN IF EXISTS internal_length_mm,
    DROP COLUMN IF EXISTS internal_width_mm,
    DROP COLUMN IF EXISTS internal_height_mm,
    DROP COLUMN IF EXISTS usable_volume_ml,
    DROP COLUMN IF EXISTS opening_width_mm,
    DROP COLUMN IF EXISTS opening_height_mm,
    DROP COLUMN IF EXISTS enclosed,
    DROP COLUMN IF EXISTS weather_protected,
    DROP COLUMN IF EXISTS upright_safe,
    DROP COLUMN IF EXISTS fragile_safe,
    DROP COLUMN IF EXISTS refrigerated,
    DROP COLUMN IF EXISTS temperature_min_c,
    DROP COLUMN IF EXISTS temperature_max_c,
    DROP COLUMN IF EXISTS temperature_class,
    DROP COLUMN IF EXISTS tail_lift,
    DROP COLUMN IF EXISTS crane,
    DROP COLUMN IF EXISTS hydraulic,
    DROP COLUMN IF EXISTS loading_ramp,
    DROP COLUMN IF EXISTS axle_count,
    DROP COLUMN IF EXISTS trailer_capable,
    DROP COLUMN IF EXISTS supported_container_sizes,
    DROP COLUMN IF EXISTS supported_goods_classes,
    DROP COLUMN IF EXISTS prohibited_goods_classes,
    DROP COLUMN IF EXISTS insurance_valid,
    DROP COLUMN IF EXISTS inspection_ref,
    DROP COLUMN IF EXISTS certification_ref,
    DROP COLUMN IF EXISTS capability_version,
    DROP COLUMN IF EXISTS capability_source_version,
    DROP COLUMN IF EXISTS capability_observed_at;
