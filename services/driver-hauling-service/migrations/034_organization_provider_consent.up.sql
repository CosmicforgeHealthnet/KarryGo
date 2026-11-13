-- Fleet FLEET-10.2: organization-sponsored truck providers and the
-- provider's own consent to an organization association.
--
-- 1. managed_by_organization_id predates Fleet's opaque identifier contract
--    (009 made it UUID); Fleet organization ids are org_<hex> TEXT, so the
--    sponsored-provider write could never succeed. It has no local foreign
--    key, so the conversion is lossless. 010 already did the same for the
--    link tables.
-- 2. The provider accepts or declines an association from their own app;
--    decided_at records when, and the history rows say WHO decided, so a
--    provider's consent can never be confused with an organization change.
ALTER TABLE truck_providers
  ALTER COLUMN managed_by_organization_id TYPE TEXT USING managed_by_organization_id::text;

ALTER TABLE hauling_organization_provider_links
  ADD COLUMN IF NOT EXISTS decided_at TIMESTAMPTZ;

ALTER TABLE hauling_organization_link_history
  ADD COLUMN IF NOT EXISTS actor_type TEXT NOT NULL DEFAULT 'SERVICE',
  ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT '';
ALTER TABLE hauling_organization_link_history
  DROP CONSTRAINT IF EXISTS chk_hauling_link_history_actor_type;
ALTER TABLE hauling_organization_link_history
  ADD CONSTRAINT chk_hauling_link_history_actor_type CHECK (actor_type IN ('SERVICE', 'PROVIDER'));
