-- Keep the append-only fulfilment planning evidence vocabulary aligned with
-- the controlled events emitted by the optimizer. This is additive migration
-- history because 000078 may already be deployed.
ALTER TABLE commerce_fulfilment_plan_events
    DROP CONSTRAINT commerce_fulfilment_plan_events_action_check;

ALTER TABLE commerce_fulfilment_plan_events
    ADD CONSTRAINT commerce_fulfilment_plan_events_action_check
    CHECK (action IN (
        'commerce.fulfilment.planning_started',
        'commerce.fulfilment.group_requirement_resolved',
        'commerce.fulfilment.vendor_own_selected',
        'commerce.fulfilment.quote_requested',
        'commerce.fulfilment.quote_received',
        'commerce.fulfilment.quote_refused',
        'commerce.fulfilment.quote_unavailable',
        'commerce.fulfilment.group_unserviceable',
        'commerce.fulfilment.consolidated',
        'commerce.fulfilment.candidate_selected',
        'commerce.fulfilment.planned',
        'commerce.fulfilment.failed',
        'commerce.fulfilment.expired',
        'commerce.fulfilment.superseded'
    ));
