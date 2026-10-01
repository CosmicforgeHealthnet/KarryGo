ALTER TABLE haulage_bookings
    ADD COLUMN receiver_email text NOT NULL DEFAULT '',
    ADD COLUMN receiver_customer_id uuid;

CREATE OR REPLACE FUNCTION hauling_invalidate_handover_on_receiver_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE hauling_handover_challenges
       SET status='INVALIDATED', updated_at=now()
     WHERE booking_id=NEW.id AND stage='delivery' AND status='ACTIVE';
    RETURN NEW;
END;
$$;
CREATE TRIGGER hauling_receiver_channel_guard
    AFTER UPDATE OF receiver_phone, receiver_email, receiver_customer_id ON haulage_bookings
    FOR EACH ROW WHEN (NEW.receiver_phone IS DISTINCT FROM OLD.receiver_phone OR
                       NEW.receiver_email IS DISTINCT FROM OLD.receiver_email OR
                       NEW.receiver_customer_id IS DISTINCT FROM OLD.receiver_customer_id)
    EXECUTE FUNCTION hauling_invalidate_handover_on_receiver_change();
