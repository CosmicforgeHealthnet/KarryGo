DROP INDEX IF EXISTS idx_fleet_partner_provisioning_association;
ALTER TABLE fleet_partner_provider_provisioning_sagas
    DROP COLUMN IF EXISTS association_source_version,
    DROP COLUMN IF EXISTS association_id;
