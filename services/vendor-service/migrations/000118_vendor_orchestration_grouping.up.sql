-- Multi-vendor grouping: the per-pickup facts the planner needs, and the group
-- it was assigned to.
--
-- 000117 stored only the identity of each pickup (which vendor order, at which
-- branch, in what sequence). That is enough to commit one mission for the whole
-- orchestration, and it is exactly what makes the grouping decision
-- impossible: whether two vendors can share a trip depends on where they are,
-- how much they are handing over, how it must be carried and when each will be
-- ready. Those facts are per pickup, so they have to live beside the pickup.
--
-- They are a SNAPSHOT taken when the orchestration is opened, not a live
-- reference to the vendor order. A grouping decision has to be explicable
-- afterwards from what was known at the time; re-reading today's numbers would
-- make last week's split look arbitrary.

ALTER TABLE vendor_orchestration_pickups
    ADD COLUMN IF NOT EXISTS pickup_latitude   DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS pickup_longitude  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS weight_grams      BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS volume_ml         BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS package_count     BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS handling          TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS temperature_band  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS secure_pickup     BOOLEAN NOT NULL DEFAULT FALSE,
    -- group_index is the planner's output: which mission this pickup belongs
    -- to. NULL means the group has not been planned yet, which is different
    -- from group 0 -- an unplanned pickup must never be read as "planned into
    -- the first mission".
    ADD COLUMN IF NOT EXISTS group_index       INTEGER,
    -- The reason this pickup did NOT join the preceding group, when it was
    -- split out. It is kept so an operator asking "why two riders?" gets the
    -- specific incompatibility rather than a shrug.
    ADD COLUMN IF NOT EXISTS split_reason      TEXT NOT NULL DEFAULT '';

ALTER TABLE vendor_orchestration_pickups
    DROP CONSTRAINT IF EXISTS chk_orchestration_pickup_load;
ALTER TABLE vendor_orchestration_pickups
    ADD CONSTRAINT chk_orchestration_pickup_load
    CHECK (weight_grams >= 0 AND volume_ml >= 0 AND package_count >= 0);

ALTER TABLE vendor_orchestration_pickups
    DROP CONSTRAINT IF EXISTS chk_orchestration_pickup_group;
ALTER TABLE vendor_orchestration_pickups
    ADD CONSTRAINT chk_orchestration_pickup_group
    CHECK (group_index IS NULL OR group_index >= 0);

-- Reading one planned mission's pickups is the hot read once grouping exists.
CREATE INDEX IF NOT EXISTS vendor_orchestration_pickups_group_idx
    ON vendor_orchestration_pickups (orchestration_id, group_index)
    WHERE group_index IS NOT NULL;

-- The planner's own evidence needs its own event types. Widening the CHECK
-- rather than dropping it keeps the column a closed vocabulary: an event type
-- nobody defined is a typo, and a typo in an append-only audit trail is a
-- record that can never be corrected.
ALTER TABLE vendor_orchestration_events
    DROP CONSTRAINT IF EXISTS vendor_orchestration_events_event_type_check;
ALTER TABLE vendor_orchestration_events
    ADD CONSTRAINT vendor_orchestration_events_event_type_check
    CHECK (event_type IN (
        'REQUIREMENT_BUILT', 'SERVICEABILITY_CHECKED', 'SUPPLY_PREVIEWED',
        'AUTHORITY_SELECTED', 'GROUP_PLANNED', 'GROUP_SPLIT',
        'COMMIT_SCHEDULED', 'COMMIT_ATTEMPTED',
        'MISSION_COMMITTED', 'REPLANNED', 'RECOVERY_OBSERVED', 'OVERRIDDEN'));
