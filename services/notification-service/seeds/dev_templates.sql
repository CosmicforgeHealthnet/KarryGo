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
    -- ── Hauling booking lifecycle ──────────────────────────────────────────
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
     'Your driver had to cancel this booking. We are finding you another option.',
     '["push", "websocket", "in_app"]'),

    -- ── Payments & wallet ──────────────────────────────────────────────────
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
     '["push", "websocket", "in_app"]')
ON CONFLICT (key, locale) DO UPDATE SET
    title = EXCLUDED.title,
    body = EXCLUDED.body,
    default_channels = EXCLUDED.default_channels,
    active = true,
    updated_at = now();
