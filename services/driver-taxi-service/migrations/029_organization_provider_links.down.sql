DROP TABLE IF EXISTS taxi_organization_provider_operations;
DROP TABLE IF EXISTS taxi_organization_provider_link_history;
DROP TABLE IF EXISTS taxi_organization_provider_links;
ALTER TABLE taxi_providers DROP CONSTRAINT IF EXISTS chk_taxi_providers_onboarding_mode;
ALTER TABLE taxi_providers DROP COLUMN IF EXISTS onboarding_mode;
ALTER TABLE taxi_providers DROP COLUMN IF EXISTS managed_by_organization_id;
