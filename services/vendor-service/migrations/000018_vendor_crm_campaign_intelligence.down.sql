-- The pointer constraint is dropped first: crm_campaigns references the
-- snapshot table, and the snapshot table references crm_campaigns, so neither
-- can be dropped while that cycle stands.
ALTER TABLE IF EXISTS crm_campaigns DROP CONSTRAINT IF EXISTS crm_campaign_current_snapshot_fk;

DROP TABLE IF EXISTS crm_campaign_target_channels;
DROP TABLE IF EXISTS crm_campaign_targets;
DROP TABLE IF EXISTS crm_campaign_audience_snapshots;
DROP TABLE IF EXISTS crm_campaign_experiment_variants;
DROP TABLE IF EXISTS crm_campaign_versions;
DROP TABLE IF EXISTS crm_campaigns;
