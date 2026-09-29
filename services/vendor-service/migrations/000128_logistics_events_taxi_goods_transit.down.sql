ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_event_type_check;

-- Retain already recorded authority history while restoring the old ingress
-- rule for new events, as with the Taxi Goods source migration.
ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_event_type_check CHECK (event_type IN (
        'PICKED_UP','DELIVERING','DELIVERED','DELIVERY_FAILED',
        'TIMING_UPDATED','PICKUP_TIMING_UPDATED','ETA_UPDATED','TIMING_APPLIED',
        'TIMING_NO_CHANGE','TIMING_TOO_LATE','TIMING_TERMINAL','TIMING_UNKNOWN'
    )) NOT VALID;
