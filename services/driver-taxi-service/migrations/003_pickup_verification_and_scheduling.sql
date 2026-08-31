-- Pickup verification detail (addendum §9) and the scheduled-dispatch index.
-- Reservation/backup-dispatch columns already live on taxi_bookings (see 001).

CREATE TABLE IF NOT EXISTS taxi_pickup_verifications (
    booking_id       UUID PRIMARY KEY REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    pin_hash         TEXT NOT NULL DEFAULT '',
    qr_token_hash    TEXT NOT NULL DEFAULT '',
    plaintext_pin    TEXT NOT NULL DEFAULT '',   -- redisplay only; see handover trade-off note
    expires_at       TIMESTAMPTZ,
    attempts         INT NOT NULL DEFAULT 0,
    resend_count     INT NOT NULL DEFAULT 0,
    last_sent_at     TIMESTAMPTZ,
    verified_at      TIMESTAMPTZ,
    override_by      TEXT,             -- support/safety actor id, if manually overridden
    override_reason  TEXT
);

CREATE INDEX IF NOT EXISTS idx_taxi_bookings_scheduled
  ON taxi_bookings(dispatch_start_at)
  WHERE status = 'scheduled';
