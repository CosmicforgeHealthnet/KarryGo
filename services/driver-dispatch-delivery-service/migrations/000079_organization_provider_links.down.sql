DROP TABLE IF EXISTS dispatch_organization_provider_operations;
DROP TABLE IF EXISTS dispatch_organization_provider_link_history;
DROP TABLE IF EXISTS dispatch_organization_provider_links;
ALTER TABLE providers DROP CONSTRAINT IF EXISTS chk_providers_onboarding_mode;
ALTER TABLE providers DROP COLUMN IF EXISTS onboarding_mode;
ALTER TABLE providers DROP COLUMN IF EXISTS managed_by_organization_id;
