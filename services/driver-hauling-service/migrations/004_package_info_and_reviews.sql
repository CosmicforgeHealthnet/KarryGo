-- Migration 004: add package/receiver info to bookings and booking reviews table.

ALTER TABLE haulage_bookings
  ADD COLUMN IF NOT EXISTS weight_category  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receiver_name    TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receiver_phone   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS package_content  TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS package_size     TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS is_fragile       BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS booking_reviews (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id        UUID NOT NULL UNIQUE REFERENCES haulage_bookings(id) ON DELETE CASCADE,
    customer_id       UUID NOT NULL,
    provider_id       UUID NOT NULL,
    rating            INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    review_text       TEXT NOT NULL DEFAULT '',
    recommends_driver BOOLEAN,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_booking_reviews_provider ON booking_reviews(provider_id);
CREATE INDEX IF NOT EXISTS idx_booking_reviews_booking  ON booking_reviews(booking_id);
