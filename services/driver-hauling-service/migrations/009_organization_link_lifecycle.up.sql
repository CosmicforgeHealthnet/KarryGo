-- Complete the existing Fleet/Hauling link records without replacing the
-- applied 008 authority migration.
ALTER TABLE truck_providers
  ADD COLUMN IF NOT EXISTS managed_by_organization_id UUID,
  ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0);

ALTER TABLE hauling_organization_provider_links
  DROP CONSTRAINT IF EXISTS hauling_organization_provider_links_state_check,
  ADD COLUMN IF NOT EXISTS link_id UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN IF NOT EXISTS branch_id UUID,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;
ALTER TABLE hauling_organization_provider_links
  ADD CONSTRAINT hauling_organization_provider_links_state_check
  CHECK (state IN ('REQUESTED','ACTIVE','SUSPENDED','REVOKED','REJECTED'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_provider_link_id
  ON hauling_organization_provider_links(link_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_provider_link_idempotency
  ON hauling_organization_provider_links(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hauling_provider_links_org_state
  ON hauling_organization_provider_links(organization_id,state,updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_provider_active_relationship
  ON hauling_organization_provider_links(provider_id)
  WHERE state IN ('REQUESTED','ACTIVE','SUSPENDED');

ALTER TABLE hauling_organization_truck_links
  DROP CONSTRAINT IF EXISTS hauling_organization_truck_links_state_check,
  ADD COLUMN IF NOT EXISTS link_id UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN IF NOT EXISTS branch_id UUID,
  ADD COLUMN IF NOT EXISTS membership_id UUID,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;
ALTER TABLE hauling_organization_truck_links
  ADD CONSTRAINT hauling_organization_truck_links_state_check
  CHECK (state IN ('REQUESTED','ACTIVE','SUSPENDED','REVOKED','REJECTED'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_truck_link_id
  ON hauling_organization_truck_links(link_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_truck_link_idempotency
  ON hauling_organization_truck_links(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hauling_truck_links_org_state
  ON hauling_organization_truck_links(organization_id,state,updated_at DESC);

CREATE TABLE IF NOT EXISTS hauling_organization_link_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  link_type TEXT NOT NULL CHECK (link_type IN ('PROVIDER','TRUCK')),
  link_id UUID NOT NULL,
  organization_id UUID NOT NULL,
  branch_id UUID,
  prior_state TEXT,
  new_state TEXT NOT NULL,
  source_version BIGINT NOT NULL CHECK (source_version > 0),
  actor_service TEXT NOT NULL,
  reason_code TEXT NOT NULL DEFAULT '',
  correlation_id TEXT NOT NULL DEFAULT '',
  causation_id TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (link_type,link_id,source_version)
);
CREATE INDEX IF NOT EXISTS idx_hauling_link_history_link
  ON hauling_organization_link_history(link_type,link_id,source_version);
