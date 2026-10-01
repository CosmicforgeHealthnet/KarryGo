-- Notification templates for platform event types.
--
-- Senders reference these by template_key (== event_type) and pass template_data
-- for {{placeholder}} interpolation. If a template is missing, the sender's
-- inline title/body is used instead, so seeding is additive and safe.
--
-- Idempotent: ON CONFLICT (key, locale) updates the title/body/channels so
-- re-running picks up copy changes. Locale is en-NG to match the platform
-- default.

INSERT INTO notification_templates (key, locale, title, body, default_channels) VALUES
    -- Hauling booking lifecycle
    ('booking.matched', 'en-NG',
     'New haulage request',
     'You have a new haulage booking to accept. Open the app to review and respond.',
     '["push", "websocket", "in_app"]'),

    ('booking.accepted', 'en-NG',
     'Driver on the way',
     'A driver has accepted your booking and is preparing for pickup.',
     '["push", "websocket", "in_app"]'),

    ('booking.unmatched', 'en-NG',
     'No drivers available',
     'We could not match a driver for your booking right now. Please try again shortly.',
     '["push", "websocket", "in_app"]'),

    ('booking.en_route_pickup', 'en-NG',
     'Driver on the way to pickup',
     'Your driver is heading to the pickup location now.',
     '["push", "websocket", "in_app"]'),

    ('booking.arrived_at_pickup', 'en-NG',
     'Driver has arrived',
     'Your driver has arrived at the pickup location.',
     '["push", "websocket", "in_app"]'),

    ('booking.pickup_verification_required', 'en-NG',
     'Verify your driver',
     'Your driver has arrived. Open the app to verify their identity before handing over your package.',
     '["push", "websocket", "in_app"]'),

    ('booking.en_route_delivery', 'en-NG',
     'Cargo on the way',
     'Your cargo is on its way to the destination.',
     '["push", "websocket", "in_app"]'),

    ('booking.delivery_code_ready', 'en-NG',
     'Delivery verification code',
     'A delivery credential is ready. Open the secure receiver view to retrieve it.',
     '["push", "websocket", "in_app"]'),

    ('cargo.picked_up', 'en-NG',
     'Cargo picked up',
     'Your cargo has been picked up and is on its way to the destination.',
     '["push", "websocket", "in_app"]'),

    ('cargo.delivered', 'en-NG',
     'Cargo delivered',
     'Your cargo has been delivered. Please confirm and leave a review.',
     '["push", "websocket", "in_app"]'),

    ('booking.completed', 'en-NG',
     'Booking completed',
     'Your haulage booking is now complete. Thank you for using Cosmicforge Logistics.',
     '["push", "websocket", "in_app"]'),

    ('booking.cancelled', 'en-NG',
     'Booking cancelled',
     'The customer cancelled this booking. You are free to take new requests.',
     '["push", "websocket", "in_app"]'),

    ('booking.cancelled_by_provider', 'en-NG',
     'Booking cancelled',
     'Your driver had to cancel this booking. Please book again to find another driver.',
     '["push", "websocket", "in_app"]'),

    -- Dispatch bike-delivery lifecycle
    ('delivery.matched', 'en-NG',
     'New delivery request',
     'You have a new delivery request to accept. Open the app to review and respond.',
     '["push", "websocket", "in_app"]'),

    ('delivery.accepted', 'en-NG',
     'Rider on the way',
     'A rider has accepted your delivery and is preparing for pickup.',
     '["push", "websocket", "in_app"]'),

    ('delivery.unmatched', 'en-NG',
     'No riders available',
     'We could not match a rider for your delivery right now. Please try again shortly.',
     '["push", "websocket", "in_app"]'),

    ('delivery.en_route_pickup', 'en-NG',
     'Rider on the way to pickup',
     'Your rider is heading to the pickup location now.',
     '["push", "websocket", "in_app"]'),

    ('delivery.arrived_at_pickup', 'en-NG',
     'Rider has arrived',
     'Your rider has arrived at the pickup location.',
     '["push", "websocket", "in_app"]'),

    ('delivery.pickup_verification_required', 'en-NG',
     'Verify your rider',
     'Your rider has arrived. Open the app to verify their identity before handing over your package.',
     '["push", "websocket", "in_app"]'),

    ('package.picked_up', 'en-NG',
     'Package picked up',
     'Your package has been picked up and is on its way to the destination.',
     '["push", "websocket", "in_app"]'),

    ('package.delivery_code_ready', 'en-NG',
     'Delivery verification code',
     'A delivery credential is ready. Open the secure receiver view to retrieve it.',
     '["push", "websocket", "in_app"]'),

    ('package.delivered', 'en-NG',
     'Package delivered',
     'Your package has been delivered. Please confirm and leave a review.',
     '["push", "websocket", "in_app"]'),

    ('delivery.completed', 'en-NG',
     'Delivery completed',
     'Your delivery is now complete. Thank you for using Cosmicforge Logistics.',
     '["push", "websocket", "in_app"]'),

    ('delivery.cancelled', 'en-NG',
     'Delivery cancelled',
     'The customer cancelled this delivery. You are free to take new requests.',
     '["push", "websocket", "in_app"]'),

    ('delivery.cancelled_by_provider', 'en-NG',
     'Delivery cancelled',
     'Your rider had to cancel this delivery. Please book again to find another rider.',
     '["push", "websocket", "in_app"]'),

    -- Payments & wallet
    ('payment.topup_success', 'en-NG',
     'Wallet funded',
     'Your wallet top-up was successful.',
     '["push", "websocket", "in_app"]'),

    ('payment.success', 'en-NG',
     'Payment successful',
     'Your payment was completed successfully.',
     '["push", "websocket", "in_app"]'),

    ('payment.failed', 'en-NG',
     'Payment failed',
     'Your payment could not be completed. Please try again.',
     '["push", "websocket", "in_app"]'),

    ('withdrawal.completed', 'en-NG',
     'Withdrawal paid out',
     'Your withdrawal has been paid to your bank account.',
     '["push", "websocket", "in_app"]'),

    ('withdrawal.failed', 'en-NG',
     'Withdrawal failed',
     'Your withdrawal could not be processed. The amount remains in your wallet.',
     '["push", "websocket", "in_app"]'),

    ('withdrawal.reversed', 'en-NG',
     'Withdrawal reversed',
     'Your withdrawal was reversed and the amount returned to your wallet.',
     '["push", "websocket", "in_app"]'),

    -- ── Taxi ride lifecycle ────────────────────────────────────────────────
    -- Its own namespace, so a taxi passenger never receives hauling's "cargo
    -- delivered" copy and a driver never gets "New haulage request" for a car
    -- ride. A ride may carry a package instead of a passenger, which is why a
    -- parcel drop-off key sits among otherwise ride-shaped copy.
    ('ride.matched', 'en-NG',
     'New trip request',
     'You have a new trip to accept. Open the app to review and respond.',
     '["push", "websocket", "in_app"]'),

    ('ride.accepted', 'en-NG',
     'Driver on the way',
     'A driver accepted your trip and is heading to your pickup point.',
     '["push", "websocket", "in_app"]'),

    ('ride.unmatched', 'en-NG',
     'No driver found',
     'We could not find a driver nearby. Nothing was charged -- please try again.',
     '["push", "websocket", "in_app"]'),

    ('ride.en_route_pickup', 'en-NG',
     'Your driver is on the way',
     'Your driver has set off for the pickup point.',
     '["push", "websocket", "in_app"]'),

    -- The pickup credential is generated in the same transition, so arrival
    -- doubles as "your code is ready".
    ('ride.arrived_at_pickup', 'en-NG',
     'Your driver has arrived',
     'Your driver is at the pickup point. Show them your code to start the trip.',
     '["push", "websocket", "in_app"]'),

    ('ride.started', 'en-NG',
     'Trip started',
     'Your trip is under way. Have a safe journey.',
     '["push", "websocket", "in_app"]'),

    ('ride.completed', 'en-NG',
     'Trip complete',
     'You have arrived. Thanks for riding with Karry Go.',
     '["push", "websocket", "in_app"]'),

    -- Sent to the DRIVER when the passenger calls it off; they may already be
    -- en route, which is exactly why this exists.
    ('ride.cancelled', 'en-NG',
     'Trip cancelled',
     'The customer cancelled this trip. You are free to take new requests.',
     '["push", "websocket", "in_app"]'),

    ('ride.cancelled_by_provider', 'en-NG',
     'Trip cancelled',
     'Your driver cancelled this trip. Anything held was refunded -- please book again.',
     '["push", "websocket", "in_app"]'),

    -- Parcel only: the receiver's drop-off code has become available.
    ('ride.delivery_code_ready', 'en-NG',
     'Delivery code ready',
     'Your package is on its way. Share the delivery code with whoever receives it.',
     '["push", "websocket", "in_app"]'),

    -- Corporate auth OTP
    ('corporate.auth.otp', 'en-NG',
     'Your KarryGo verification code',
     'Your verification code is: {{code}}. It expires in {{expires_in_minutes}} minutes.',
     '["email"]')
ON CONFLICT (key, locale) DO UPDATE SET
    title = EXCLUDED.title,
    body = EXCLUDED.body,
    default_channels = EXCLUDED.default_channels,
    active = true,
    updated_at = now();
