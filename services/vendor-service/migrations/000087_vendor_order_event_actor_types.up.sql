-- Corrective Vendor Order Lifecycle evidence. Historical migrations remain immutable.
-- The lifecycle implementation records authenticated authority and vendor actors
-- explicitly; fresh databases must accept those actor types as well as the
-- original Section 17 event actors.

ALTER TABLE commerce_order_events
    DROP CONSTRAINT IF EXISTS commerce_order_events_actor_type_check;

ALTER TABLE commerce_order_events
    ADD CONSTRAINT commerce_order_events_actor_type_check CHECK (
        actor_type IN (
            'CUSTOMER',
            'VENDOR_STAFF',
            'PLATFORM',
            'SYSTEM',
            'AUTHORITY',
            'VENDOR'
        )
    );
