-- Reverse of 000061.
--
-- Dropping the capability table loses every survey a rider entered. That is the
-- honest consequence of rolling back a schema that introduced them: there is
-- nowhere else for those facts to live. `bikes` itself is untouched, so the
-- legacy bike_type-based candidate SQL still has everything it ever used.

ALTER TABLE commerce_missions
    DROP COLUMN IF EXISTS service_product_code,
    DROP COLUMN IF EXISTS service_product_version,
    DROP COLUMN IF EXISTS requirement_snapshot,
    DROP COLUMN IF EXISTS capability_policy_version,
    DROP COLUMN IF EXISTS guarantee_profile_version,
    DROP COLUMN IF EXISTS commitment_reference;

DROP TABLE IF EXISTS dispatch_match_decisions;
DROP TABLE IF EXISTS dispatch_policy_snapshots;
DROP TABLE IF EXISTS dispatch_vehicle_product_eligibility;
DROP TABLE IF EXISTS dispatch_bike_capabilities;
