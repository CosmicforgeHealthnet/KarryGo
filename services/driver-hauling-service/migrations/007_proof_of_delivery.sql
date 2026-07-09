-- Proof of delivery: public URLs of the photo and receiver signature captured
-- by the provider at drop-off (uploaded to media-file-service by the app).
ALTER TABLE haulage_bookings
    ADD COLUMN IF NOT EXISTS proof_photo_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS signature_url TEXT NOT NULL DEFAULT '';
