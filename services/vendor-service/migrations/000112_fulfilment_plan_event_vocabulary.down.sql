-- Refuse an unsafe downgrade rather than silently deleting or rewriting
-- append-only evidence that the older vocabulary cannot represent.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM commerce_fulfilment_plan_events
         WHERE action = 'commerce.fulfilment.candidate_selected'
    ) THEN
        RAISE EXCEPTION
            'cannot downgrade fulfilment event vocabulary while candidate_selected evidence exists';
    END IF;
END
$$;

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
        'commerce.fulfilment.planned',
        'commerce.fulfilment.failed',
        'commerce.fulfilment.expired',
        'commerce.fulfilment.superseded'
    ));
