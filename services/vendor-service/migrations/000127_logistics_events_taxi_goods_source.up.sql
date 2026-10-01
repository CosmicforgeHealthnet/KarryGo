-- Taxi Goods (Section 29) is a commerce executor and delivers its own pickup,
-- transit and delivery facts through the logistics-event ingress. The event
-- log only admitted DISPATCH and HAULING, so every Taxi Goods callback was
-- refused and a car-executed order could never be delivered.
ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_source_check;

ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_source_check
    CHECK (source IN ('DISPATCH','HAULING','TAXI'));
