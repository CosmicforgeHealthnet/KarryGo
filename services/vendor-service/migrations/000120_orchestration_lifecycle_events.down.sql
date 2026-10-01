-- Reverses 000120.
--
-- The evidence is removed before the narrower CHECK is restored, and the
-- append-only trigger is suspended to do it: restoring the constraint with
-- ORDER_BOUND rows still present would fail, and the trigger would then refuse
-- the delete that would have fixed it.
ALTER TABLE vendor_orchestration_events
    DROP CONSTRAINT IF EXISTS vendor_orchestration_events_event_type_check;

ALTER TABLE vendor_orchestration_events DISABLE TRIGGER USER;
DELETE FROM vendor_orchestration_events WHERE event_type IN ('ORDER_BOUND', 'CANCELLED');
ALTER TABLE vendor_orchestration_events ENABLE TRIGGER USER;

ALTER TABLE vendor_orchestration_events
    ADD CONSTRAINT vendor_orchestration_events_event_type_check
    CHECK (event_type IN (
        'REQUIREMENT_BUILT', 'SERVICEABILITY_CHECKED', 'SUPPLY_PREVIEWED',
        'AUTHORITY_SELECTED', 'GROUP_PLANNED', 'GROUP_SPLIT',
        'COMMIT_SCHEDULED', 'COMMIT_ATTEMPTED',
        'MISSION_COMMITTED', 'REPLANNED', 'RECOVERY_OBSERVED', 'OVERRIDDEN'));
