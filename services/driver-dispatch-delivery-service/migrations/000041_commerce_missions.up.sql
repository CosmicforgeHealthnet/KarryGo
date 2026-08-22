-- Section 17F: commerce mission execution.
--
-- The execution counterpart to 000040's commerce delivery quotes. That table
-- records what dispatch SAID a delivery would cost; this one records dispatch
-- agreeing to actually run it, for a fee the customer has ALREADY been charged
-- under an approved commerce quote.
--
-- The fee is not recomputed here. agreed_fee_minor is copied verbatim from the
-- request onto the booking's fare, because the customer's charge is fixed by
-- an approved quote and any difference from dispatch's own current price is an
-- internal platform cost. A schema that re-derived the fare would silently
-- break the commerce total-price guarantee.

CREATE TABLE IF NOT EXISTS commerce_missions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Vendor Service's own mission identity. UNIQUE is what makes creation
    -- idempotent: a retried request returns the same mission rather than
    -- dispatching a second rider for the same goods.
    mission_ref TEXT NOT NULL UNIQUE,

    parent_order_ref TEXT NOT NULL,
    vendor_order_refs TEXT[] NOT NULL,

    customer_id UUID NOT NULL,

    -- The booking this mission is executed as. Dispatch's own machinery
    -- (broadcast, rider acceptance, trip) then runs unchanged.
    booking_id UUID NOT NULL REFERENCES dispatch_bookings(id) ON DELETE RESTRICT,

    -- Echoed back to the caller so it can assert dispatch did not re-price.
    agreed_fee_minor BIGINT NOT NULL CHECK (agreed_fee_minor >= 0),
    currency TEXT NOT NULL DEFAULT 'NGN',

    -- The authority's own earlier quote, so execution ties back to what was
    -- priced. Optional: a mission may be created without one.
    quote_id TEXT NOT NULL DEFAULT '',
    quote_reference TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL CHECK (status IN ('accepted', 'refused')),
    refusal_code TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A refusal must name its reason so commerce can act on it rather than
    -- retrying blindly.
    CONSTRAINT chk_commerce_mission_refusal
        CHECK (status <> 'refused' OR length(btrim(refusal_code)) > 0),

    CONSTRAINT chk_commerce_mission_allocations
        CHECK (array_length(vendor_order_refs, 1) >= 1),

    -- One mission per booking, so a booking can never be claimed by two
    -- commerce missions.
    CONSTRAINT uq_commerce_mission_booking UNIQUE (booking_id)
);

CREATE INDEX IF NOT EXISTS idx_commerce_missions_parent_order
    ON commerce_missions (parent_order_ref, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commerce_missions_customer
    ON commerce_missions (customer_id, created_at DESC);

COMMENT ON TABLE commerce_missions IS
'Section 17F commerce mission execution. Dispatch executes for the fee the customer already paid and never re-prices; mission_ref is the idempotency key.';
COMMENT ON COLUMN commerce_missions.agreed_fee_minor IS
'The fee the customer was already charged under an approved commerce quote. Copied onto the booking fare verbatim. Any difference from dispatch own current price is an internal platform cost, never the customer''s.';
