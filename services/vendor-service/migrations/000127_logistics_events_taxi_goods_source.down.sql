ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_source_check;

-- NOT VALID: Taxi Goods rows recorded while the wider rule was live are
-- history and stay; only new rows are held to the narrower rule again.
ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_source_check
    CHECK (source IN ('DISPATCH','HAULING')) NOT VALID;
