DROP TRIGGER IF EXISTS dispatch_booking_receiver_channel_guard ON dispatch_bookings;
DROP FUNCTION IF EXISTS dispatch_invalidate_handover_on_booking_receiver_change();
ALTER TABLE dispatch_bookings DROP COLUMN IF EXISTS receiver_customer_id, DROP COLUMN IF EXISTS receiver_email;
