-- Reverse of 022.

DROP TRIGGER IF EXISTS trg_hauling_policy_snapshots_immutable ON hauling_policy_snapshots;
DROP FUNCTION IF EXISTS hauling_policy_snapshots_immutable();

ALTER TABLE hauling_policy_snapshots
    DROP COLUMN IF EXISTS source_version,
    DROP COLUMN IF EXISTS event_id;

DROP TABLE IF EXISTS hauling_policy_publications;
