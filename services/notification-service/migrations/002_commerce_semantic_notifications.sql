-- Commerce semantic notification provenance and Notification-owned templates.
-- Business services supply opaque recipient references plus semantic data;
-- transport copy and channel selection remain owned here.
ALTER TABLE notification_messages
    ADD COLUMN IF NOT EXISTS source_aggregate_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_aggregate_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_event_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_version bigint NOT NULL DEFAULT 0 CHECK (source_version >= 0),
    ADD COLUMN IF NOT EXISTS correlation_ref text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_notification_messages_source
    ON notification_messages(source_service, source_aggregate_type, source_aggregate_id, source_version DESC)
    WHERE source_aggregate_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_messages_semantic_source_recipient
    ON notification_messages(source_service, source_event_id, event_type, recipient_type, recipient_id)
    WHERE source_event_id <> '';

INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('commerce.vendor.new_order','en-NG','New order','A new order is ready for review.','["push","websocket","in_app"]',true),
 ('commerce.vendor.substitution_action','en-NG','Substitution updated','A customer decision requires attention on an order.','["push","websocket","in_app"]',true),
 ('commerce.vendor.scheduled_cutoff','en-NG','Scheduled order cutoff','A scheduled order is approaching its preparation cutoff.','["push","websocket","in_app"]',true),
 ('commerce.vendor.provider_assigned','en-NG','Provider assigned','A delivery provider has been assigned.','["push","websocket","in_app"]',true),
 ('commerce.vendor.provider_arrived','en-NG','Provider arrived','The assigned provider has arrived for pickup.','["push","websocket","in_app"]',true),
 ('commerce.vendor.pickup_confirmed','en-NG','Pickup confirmed','Custody transfer for the order was confirmed.','["push","websocket","in_app"]',true),
 ('commerce.vendor.refund_action','en-NG','Refund update','An authoritative refund update is available for an order.','["push","websocket","in_app"]',true),
 ('commerce.vendor.support_action','en-NG','Support action','A support case linked to an order requires attention.','["push","websocket","in_app"]',true),
 ('commerce.vendor.rider_assignment','en-NG','Delivery assignment','A vendor delivery mission assignment was updated.','["push","websocket","in_app"]',true),
 ('commerce.customer.order_accepted','en-NG','Order accepted','A vendor accepted your order.','["push","websocket","in_app"]',true),
 ('commerce.customer.order_rejected','en-NG','Order update','A vendor could not accept an allocation in your order.','["push","websocket","in_app"]',true),
 ('commerce.customer.substitution','en-NG','Substitution needs review','A substitution update is available for your order.','["push","websocket","in_app"]',true),
 ('commerce.customer.ready','en-NG','Order ready','Part of your order is ready for pickup.','["push","websocket","in_app"]',true),
 ('commerce.customer.provider_assigned','en-NG','Provider assigned','A delivery provider has been assigned to your order.','["push","websocket","in_app"]',true),
 ('commerce.customer.multi_delivery','en-NG','Multiple deliveries','Your order will arrive in more than one delivery.','["push","websocket","in_app"]',true),
 ('commerce.customer.delay','en-NG','Delivery update','The expected delivery range has changed meaningfully.','["push","websocket","in_app"]',true),
 ('commerce.customer.delivered','en-NG','Delivered','Your delivery was confirmed by the delivery authority.','["push","websocket","in_app"]',true),
 ('commerce.customer.refunded','en-NG','Refund completed','A refund for your order was completed.','["push","websocket","in_app"]',true),
 ('commerce.customer_eta_changed','en-NG','Delivery estimate updated','The delivery estimate for your order was updated.','["websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET
    title=EXCLUDED.title, body=EXCLUDED.body,
    default_channels=EXCLUDED.default_channels, active=true, updated_at=now();
