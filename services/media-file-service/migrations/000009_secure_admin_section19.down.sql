-- Redemption and legacy tombstone evidence must be archived under an approved
-- retention procedure; rollback refuses populated state.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_admin_case_redemptions) OR EXISTS(SELECT 1 FROM media_legacy_asset_events) THEN
  RAISE EXCEPTION 'Cannot roll back populated Section 19 evidence';
 END IF;
 IF EXISTS(SELECT 1 FROM media_assets WHERE legal_hold OR retention_until IS NOT NULL OR tombstoned_at IS NOT NULL) THEN
  RAISE EXCEPTION 'Cannot roll back while legacy assets carry a hold, retention or tombstone';
 END IF;
END $$;
DROP TABLE media_legacy_asset_events;
DROP FUNCTION reject_media_legacy_asset_event_mutation();
ALTER TABLE media_assets DROP CONSTRAINT chk_media_assets_hold_reason;
ALTER TABLE media_assets DROP COLUMN tombstoned_at;
ALTER TABLE media_assets DROP COLUMN retention_until;
ALTER TABLE media_assets DROP COLUMN legal_hold_reason;
ALTER TABLE media_assets DROP COLUMN legal_hold;
DROP TABLE media_admin_case_redemptions;
