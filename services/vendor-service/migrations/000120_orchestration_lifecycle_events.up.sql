-- Two more event types for the production order binding.
--
-- 000118 widened this vocabulary for the planner; these two are for the
-- lifecycle either side of it. ORDER_BOUND records that a real paid order
-- handed this orchestration a durable mission slot -- the fact that makes the
-- plan attributable to a customer order rather than to a worker. CANCELLED
-- records a withdrawal before any authority was asked.
--
-- The column stays a closed vocabulary rather than becoming free text. An
-- event type nobody defined is a typo, and a typo in an append-only audit
-- trail is a record that can never be corrected.
ALTER TABLE vendor_orchestration_events
    DROP CONSTRAINT IF EXISTS vendor_orchestration_events_event_type_check;
ALTER TABLE vendor_orchestration_events
    ADD CONSTRAINT vendor_orchestration_events_event_type_check
    CHECK (event_type IN (
        'REQUIREMENT_BUILT', 'SERVICEABILITY_CHECKED', 'SUPPLY_PREVIEWED',
        'AUTHORITY_SELECTED', 'GROUP_PLANNED', 'GROUP_SPLIT',
        'ORDER_BOUND', 'CANCELLED',
        'COMMIT_SCHEDULED', 'COMMIT_ATTEMPTED',
        'MISSION_COMMITTED', 'REPLANNED', 'RECOVERY_OBSERVED', 'OVERRIDDEN'));
