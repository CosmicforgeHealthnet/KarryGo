-- Reverse of 022.

DROP TRIGGER IF EXISTS trg_dispatch_policy_snapshots_immutable ON dispatch_policy_snapshots;
DROP FUNCTION IF EXISTS dispatch_policy_snapshots_immutable();

ALTER TABLE dispatch_policy_snapshots
    DROP COLUMN IF EXISTS source_version,
    DROP COLUMN IF EXISTS event_id;

DROP TABLE IF EXISTS dispatch_policy_publications;
