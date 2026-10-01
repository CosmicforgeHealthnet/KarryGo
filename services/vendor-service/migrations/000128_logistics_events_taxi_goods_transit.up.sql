-- Preserve the Taxi Goods authority's actual transit fact. Application
-- projection maps it to Vendor DELIVERING after checking the current executor.
ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_event_type_check;

ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_event_type_check CHECK (
        event_type IN (
            'PICKED_UP','DELIVERING','DELIVERED','DELIVERY_FAILED',
            'TIMING_UPDATED','PICKUP_TIMING_UPDATED','ETA_UPDATED','TIMING_APPLIED',
            'TIMING_NO_CHANGE','TIMING_TOO_LATE','TIMING_TERMINAL','TIMING_UNKNOWN'
        ) OR (source = 'TAXI' AND event_type = 'IN_TRANSIT')
    );
