ALTER TABLE dispatch_bookings
    ADD COLUMN receiver_email text NOT NULL DEFAULT '',
    ADD COLUMN receiver_customer_id uuid;

-- Receiver contact changes invalidate issued credentials even when the Trip's
-- legacy phone snapshot is unchanged. A new challenge must bind the new
-- frozen contact and version.
CREATE OR REPLACE FUNCTION dispatch_invalidate_handover_on_booking_receiver_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE dispatch_handover_challenges h
       SET status='INVALIDATED', invalidated_at=now(), invalidated_reason='RECEIVER_CHANGED',
           locked_at=NULL, updated_at=now(), optimistic_version=optimistic_version+1
      FROM trips t
     WHERE t.booking_id=NEW.id AND h.trip_id=t.id AND h.status='ACTIVE';
    RETURN NEW;
END;
$$;
CREATE TRIGGER dispatch_booking_receiver_channel_guard
    AFTER UPDATE OF receiver_phone, receiver_email, receiver_customer_id ON dispatch_bookings
    FOR EACH ROW WHEN (NEW.receiver_phone IS DISTINCT FROM OLD.receiver_phone OR
                       NEW.receiver_email IS DISTINCT FROM OLD.receiver_email OR
                       NEW.receiver_customer_id IS DISTINCT FROM OLD.receiver_customer_id)
    EXECUTE FUNCTION dispatch_invalidate_handover_on_booking_receiver_change();
