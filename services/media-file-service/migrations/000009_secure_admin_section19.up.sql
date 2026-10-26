-- Section 19: media, documents and evidence hardening for Secure Admin.
--
-- 1. One-time administrative case grants. A grant is redeemed at most once;
--    the redemption is immutable evidence written in the same transaction
--    that releases the bytes, and its primary key makes a concurrent second
--    redemption fail rather than race.
CREATE TABLE IF NOT EXISTS media_admin_case_redemptions (
    grant_id    uuid PRIMARY KEY REFERENCES media_admin_case_grants (id),
    staff_ref   text NOT NULL,
    redeemed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER media_admin_case_redemption_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON media_admin_case_redemptions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_media_admin_case_mutation();

-- 2. Legacy media_assets privacy controls. Legacy rows carry no recorded
--    legal hold or retention, so the columns default to "no hold" and NULL;
--    no retention period is invented for existing rows.
ALTER TABLE media_assets ADD COLUMN IF NOT EXISTS legal_hold boolean NOT NULL DEFAULT false;
ALTER TABLE media_assets ADD COLUMN IF NOT EXISTS legal_hold_reason text;
ALTER TABLE media_assets ADD COLUMN IF NOT EXISTS retention_until timestamptz;
ALTER TABLE media_assets ADD COLUMN IF NOT EXISTS tombstoned_at timestamptz;
ALTER TABLE media_assets ADD CONSTRAINT chk_media_assets_hold_reason
    CHECK (legal_hold = false OR (legal_hold_reason IS NOT NULL AND legal_hold_reason <> ''));

-- 3. Immutable legacy asset event trail: tombstones and refused deletions.
--    It records codes and actors only; never a filename or caller metadata.
CREATE TABLE IF NOT EXISTS media_legacy_asset_events (
    id            bigserial PRIMARY KEY,
    asset_id      uuid NOT NULL REFERENCES media_assets (id),
    event_type    text NOT NULL CHECK (event_type IN ('TOMBSTONED', 'DELETION_BLOCKED')),
    actor_service text NOT NULL,
    reason_code   text NOT NULL,
    recorded_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_media_legacy_asset_events_asset ON media_legacy_asset_events (asset_id);
CREATE OR REPLACE FUNCTION reject_media_legacy_asset_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Media legacy asset events are immutable'; END $$;
CREATE TRIGGER media_legacy_asset_event_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON media_legacy_asset_events
    FOR EACH STATEMENT EXECUTE FUNCTION reject_media_legacy_asset_event_mutation();
