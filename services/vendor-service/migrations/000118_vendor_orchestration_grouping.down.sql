-- Reverses 000118.
--
-- The event vocabulary is narrowed first, and the grouping evidence is removed
-- before the old CHECK is restored. vendor_orchestration_events is append-only
-- by trigger, so restoring the narrower constraint with GROUP_PLANNED rows
-- still present would fail at exactly the moment an operator needed the
-- rollback to work -- and the trigger would then refuse the delete that would
-- have fixed it.
ALTER TABLE vendor_orchestration_events
    DROP CONSTRAINT IF EXISTS vendor_orchestration_events_event_type_check;

ALTER TABLE vendor_orchestration_events DISABLE TRIGGER USER;
DELETE FROM vendor_orchestration_events WHERE event_type IN ('GROUP_PLANNED', 'GROUP_SPLIT');
ALTER TABLE vendor_orchestration_events ENABLE TRIGGER USER;

ALTER TABLE vendor_orchestration_events
    ADD CONSTRAINT vendor_orchestration_events_event_type_check
    CHECK (event_type IN (
        'REQUIREMENT_BUILT', 'SERVICEABILITY_CHECKED', 'SUPPLY_PREVIEWED',
        'AUTHORITY_SELECTED', 'COMMIT_SCHEDULED', 'COMMIT_ATTEMPTED',
        'MISSION_COMMITTED', 'REPLANNED', 'RECOVERY_OBSERVED', 'OVERRIDDEN'));

-- The planner's per-pickup snapshot and output are derived, so dropping them
-- loses no fact that cannot be recomputed from the vendor orders themselves.
DROP INDEX IF EXISTS vendor_orchestration_pickups_group_idx;

ALTER TABLE vendor_orchestration_pickups
    DROP CONSTRAINT IF EXISTS chk_orchestration_pickup_group;
ALTER TABLE vendor_orchestration_pickups
    DROP CONSTRAINT IF EXISTS chk_orchestration_pickup_load;

ALTER TABLE vendor_orchestration_pickups
    DROP COLUMN IF EXISTS split_reason,
    DROP COLUMN IF EXISTS group_index,
    DROP COLUMN IF EXISTS secure_pickup,
    DROP COLUMN IF EXISTS temperature_band,
    DROP COLUMN IF EXISTS handling,
    DROP COLUMN IF EXISTS package_count,
    DROP COLUMN IF EXISTS volume_ml,
    DROP COLUMN IF EXISTS weight_grams,
    DROP COLUMN IF EXISTS pickup_longitude,
    DROP COLUMN IF EXISTS pickup_latitude;
