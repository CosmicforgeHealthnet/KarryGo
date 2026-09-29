DROP TRIGGER IF EXISTS hauling_receiver_channel_guard ON haulage_bookings;
DROP FUNCTION IF EXISTS hauling_invalidate_handover_on_receiver_change();
ALTER TABLE haulage_bookings DROP COLUMN IF EXISTS receiver_customer_id, DROP COLUMN IF EXISTS receiver_email;
