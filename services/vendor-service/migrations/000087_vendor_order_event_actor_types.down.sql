ALTER TABLE commerce_order_events
    DROP CONSTRAINT IF EXISTS commerce_order_events_actor_type_check;

ALTER TABLE commerce_order_events
    ADD CONSTRAINT commerce_order_events_actor_type_check CHECK (
        actor_type IN ('CUSTOMER', 'VENDOR_STAFF', 'PLATFORM', 'SYSTEM')
    );
