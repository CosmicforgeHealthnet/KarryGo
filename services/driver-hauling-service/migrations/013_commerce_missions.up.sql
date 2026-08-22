-- Section 17F: commerce mission execution.
--
-- The execution counterpart to 012's commerce delivery quotes. That table
-- records what hauling SAID a haul would cost; this one records hauling
-- agreeing to actually run it, for a fee the customer has ALREADY been charged
-- under an approved commerce quote.
--
-- The fee is not recomputed here. agreed_fee_minor is copied verbatim onto the
-- booking's fare, because the customer's charge is fixed by an approved quote
-- and any difference from hauling's own current price is an internal platform
-- cost. A schema that re-derived the fare would silently break the commerce
-- total-price guarantee.

CREATE TABLE IF NOT EXISTS commerce_missions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Vendor Service's own mission identity and the idempotency key. A retried
    -- request returns the same mission rather than dispatching a second truck
    -- for the same goods.
    mission_ref TEXT NOT NULL UNIQUE,

    parent_order_ref TEXT NOT NULL,
    vendor_order_refs TEXT[] NOT NULL,

    customer_id UUID NOT NULL,

    -- The booking this mission executes as. Hauling's own matching machinery
    -- then runs unchanged.
    booking_id UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE RESTRICT,

    agreed_fee_minor BIGINT NOT NULL CHECK (agreed_fee_minor >= 0),
    currency TEXT NOT NULL DEFAULT 'NGN',

    quote_id TEXT NOT NULL DEFAULT '',
    quote_reference TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL CHECK (status IN ('accepted', 'refused')),
    refusal_code TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_hauling_commerce_mission_refusal
        CHECK (status <> 'refused' OR length(btrim(refusal_code)) > 0),

    CONSTRAINT chk_hauling_commerce_mission_allocations
        CHECK (array_length(vendor_order_refs, 1) >= 1),

    CONSTRAINT uq_hauling_commerce_mission_booking UNIQUE (booking_id)
);

CREATE INDEX IF NOT EXISTS idx_hauling_commerce_missions_parent_order
    ON commerce_missions (parent_order_ref, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_hauling_commerce_missions_customer
    ON commerce_missions (customer_id, created_at DESC);

COMMENT ON TABLE commerce_missions IS
'Section 17F commerce mission execution. Hauling executes for the fee the customer already paid and never re-prices; mission_ref is the idempotency key.';
