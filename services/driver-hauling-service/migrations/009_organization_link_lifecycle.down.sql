DROP TABLE IF EXISTS hauling_organization_link_history;

DROP INDEX IF EXISTS idx_hauling_truck_links_org_state;
DROP INDEX IF EXISTS uq_hauling_truck_link_idempotency;
DROP INDEX IF EXISTS uq_hauling_truck_link_id;
ALTER TABLE hauling_organization_truck_links
  DROP CONSTRAINT IF EXISTS hauling_organization_truck_links_state_check,
  DROP COLUMN IF EXISTS request_fingerprint,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS membership_id,
  DROP COLUMN IF EXISTS branch_id,
  DROP COLUMN IF EXISTS link_id;
ALTER TABLE hauling_organization_truck_links
  ADD CONSTRAINT hauling_organization_truck_links_state_check CHECK (state IN ('ACTIVE','REVOKED'));

DROP INDEX IF EXISTS idx_hauling_provider_links_org_state;
DROP INDEX IF EXISTS uq_hauling_provider_active_relationship;
DROP INDEX IF EXISTS uq_hauling_provider_link_idempotency;
DROP INDEX IF EXISTS uq_hauling_provider_link_id;
ALTER TABLE hauling_organization_provider_links
  DROP CONSTRAINT IF EXISTS hauling_organization_provider_links_state_check,
  DROP COLUMN IF EXISTS request_fingerprint,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS branch_id,
  DROP COLUMN IF EXISTS link_id;
ALTER TABLE hauling_organization_provider_links
  ADD CONSTRAINT hauling_organization_provider_links_state_check CHECK (state IN ('ACTIVE','REVOKED'));

ALTER TABLE truck_providers
  DROP COLUMN IF EXISTS source_version,
  DROP COLUMN IF EXISTS managed_by_organization_id;
