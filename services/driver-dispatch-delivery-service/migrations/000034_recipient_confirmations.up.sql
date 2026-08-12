-- WP6.3: recipient confirmation as authoritative dispatch lifecycle data.
--
-- Before this, the only record that a delivery reached the right person was
-- delivery_proofs.receiver_name / receiver_phone, captured by the provider.
-- That is evidence the PROVIDER produced; it is not a confirmation the
-- RECIPIENT gave, and a managing organization reading a trip could not tell
-- the two apart.
--
-- This adds the confirmation as its own authoritative record. Deliberately NOT
-- a new trip status: the trip state machine already models the delivery
-- lifecycle, and adding a parallel status would give two places to answer
-- "where is this delivery?". Confirmation is an attribute of the delivery that
-- advances the trip's source_version, so consumers order it correctly.
--
-- No recipient PII lives here. The confirming party is identified by an opaque
-- surrogate generated per confirmation; whether they matched the expected
-- receiver is recorded as a boolean, computed server-side and never echoed
-- back with the value it was computed from.

CREATE TABLE IF NOT EXISTS recipient_confirmations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    -- How the recipient's identity was established.
    verification_method TEXT NOT NULL
        CHECK (verification_method IN ('SIGNATURE', 'PHOTO', 'OTP', 'ID_CHECK', 'VERBAL')),
    -- Opaque per-confirmation surrogate for the confirming party. It reveals
    -- nothing on its own and is safe to place on an event stream.
    recipient_reference TEXT NOT NULL,
    -- Whether the confirming party matched the receiver the booking expected.
    -- The comparison inputs (name, phone) stay in delivery_proofs and on the
    -- trip; only this verdict travels.
    recipient_matched_expected BOOLEAN NOT NULL DEFAULT false,
    -- The proof this confirmation accompanies, when one exists.
    proof_id UUID NULL REFERENCES delivery_proofs(id) ON DELETE SET NULL,
    from_status TEXT NOT NULL,
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_by_actor_type TEXT NOT NULL
        CHECK (confirmed_by_actor_type IN ('provider', 'recipient', 'system')),
    confirmed_by_actor_id TEXT NOT NULL,
    -- Command envelope. Every field here exists so a retry is safe and a
    -- replay is distinguishable from a second confirmation.
    expected_source_version BIGINT NOT NULL DEFAULT 0,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly one confirmation per trip: a second one is a replay, never a second
-- event.
CREATE UNIQUE INDEX IF NOT EXISTS uq_recipient_confirmation_trip
    ON recipient_confirmations (trip_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_recipient_confirmation_idempotency
    ON recipient_confirmations (idempotency_key);

CREATE INDEX IF NOT EXISTS idx_recipient_confirmation_confirmed_at
    ON recipient_confirmations (confirmed_at DESC);

-- Denormalised onto the trip so the common read ("was this delivery
-- confirmed?") does not need a join, and so the trip row carries the fact a
-- projection consumer needs.
ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS recipient_confirmed_at TIMESTAMPTZ NULL;

ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS recipient_confirmation_method TEXT NULL;
