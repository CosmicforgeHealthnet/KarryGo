ALTER TABLE haulage_bookings
    DROP CONSTRAINT IF EXISTS haulage_bookings_no_plaintext_handover;

DROP TABLE IF EXISTS hauling_handover_challenges;

