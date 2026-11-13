ALTER TABLE hauling_organization_link_history
  DROP CONSTRAINT IF EXISTS chk_hauling_link_history_actor_type;
ALTER TABLE hauling_organization_link_history
  DROP COLUMN IF EXISTS actor_id,
  DROP COLUMN IF EXISTS actor_type;

ALTER TABLE hauling_organization_provider_links
  DROP COLUMN IF EXISTS decided_at;

-- Fleet's opaque org_<hex> ids cannot be represented as UUID. Rolling back
-- keeps UUID-shaped values and clears the rest, exactly the set 009 could
-- hold.
ALTER TABLE truck_providers
  ALTER COLUMN managed_by_organization_id TYPE UUID
  USING CASE
    WHEN managed_by_organization_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    THEN managed_by_organization_id::uuid
  END;
