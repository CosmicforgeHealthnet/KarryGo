-- FLEET-10.2: record the owning service's association so the provisioning
-- saga can read the provider's own accept/decline back.
--
-- The provider accepts in their own app on Dispatch, Taxi or Hauling; Fleet
-- never sees that request. The reconciler reads the association by this id
-- and only then moves the membership from PENDING_PROVIDER_ACCEPTANCE to
-- PENDING_SETUP (or DECLINED). The id is opaque and owned by the provider
-- service: no foreign key.
ALTER TABLE fleet_partner_provider_provisioning_sagas
    ADD COLUMN IF NOT EXISTS association_id text,
    ADD COLUMN IF NOT EXISTS association_source_version bigint NOT NULL DEFAULT 0;

-- Sagas still waiting on a provider, for the reconciler.
CREATE INDEX IF NOT EXISTS idx_fleet_partner_provisioning_association
    ON fleet_partner_provider_provisioning_sagas (association_id)
    WHERE association_id IS NOT NULL;
