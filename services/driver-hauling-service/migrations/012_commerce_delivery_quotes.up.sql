-- Authoritative commerce delivery quotes.
--
-- Vendor Service (Section 17C fulfilment planning) asks this service whether it
-- can serve a route with a given load, and what that would cost. The answer is
-- STORED HERE BEFORE IT IS RETURNED, which is what makes the returned quote
-- reference evidence rather than a number in a response body: a later section
-- presenting the reference can have the fee verified against the row that
-- produced it.
--
-- Pricing is this service's own fare model. Vendor Service never computes a
-- hauling fare, and nothing in the request may influence the fee beyond the
-- physical inputs the formula already takes.
--
-- This service owns BOTH van and truck. A quote for either class is answered
-- here; there is no separate van service anywhere in the platform.

CREATE TABLE commerce_delivery_quotes (
    id text PRIMARY KEY,

    -- The signed identity of the caller. Part of the idempotency key so two
    -- callers can never collide on a reference, and so a quote can never be
    -- read back by a service that did not ask for it.
    caller_service text NOT NULL,

    -- The caller-owned idempotency key.
    quote_reference text NOT NULL,

    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),

    -- Hash of the request that produced this answer. It distinguishes "the
    -- caller retried" (same answer) from "the caller changed the load" (new
    -- revision).
    request_fingerprint text NOT NULL,

    vehicle_class text NOT NULL CHECK (vehicle_class IN ('VAN', 'TRUCK')),

    -- The truck type this service would actually dispatch. Recorded because it
    -- is what the matcher later filters on, so an operator can see why a quote
    -- and a match disagreed.
    truck_type text NOT NULL DEFAULT '',

    pickup_latitude   double precision NOT NULL
        CHECK (pickup_latitude BETWEEN -90 AND 90),
    pickup_longitude  double precision NOT NULL
        CHECK (pickup_longitude BETWEEN -180 AND 180),
    dropoff_latitude  double precision NOT NULL
        CHECK (dropoff_latitude BETWEEN -90 AND 90),
    dropoff_longitude double precision NOT NULL
        CHECK (dropoff_longitude BETWEEN -180 AND 180),

    package_count      bigint NOT NULL CHECK (package_count > 0),
    total_weight_grams bigint NOT NULL CHECK (total_weight_grams >= 0),

    serviceable boolean NOT NULL,
    reason_code text NOT NULL CHECK (length(btrim(reason_code)) > 0),

    distance_km       double precision NOT NULL CHECK (distance_km >= 0),
    estimated_minutes integer NOT NULL CHECK (estimated_minutes >= 0),

    fee_minor bigint NOT NULL DEFAULT 0 CHECK (fee_minor >= 0),
    currency  text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),

    base_minor      bigint NOT NULL DEFAULT 0 CHECK (base_minor >= 0),
    distance_minor  bigint NOT NULL DEFAULT 0 CHECK (distance_minor >= 0),
    surcharge_minor bigint NOT NULL DEFAULT 0 CHECK (surcharge_minor >= 0),

    -- The serviceability evidence: how many eligible providers this service
    -- could actually see for the route when it answered.
    available_providers integer NOT NULL DEFAULT 0
        CHECK (available_providers >= 0),

    expires_at timestamptz NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- THE INVARIANT, enforced by the database rather than by the handler that
    -- happens to write it: an answer that cannot be served may not carry a
    -- fee, and a servable one must name a real currency and a real provider.
    CONSTRAINT commerce_delivery_quotes_refusal_chk
        CHECK (
            serviceable
            OR (fee_minor = 0 AND currency = '' AND reason_code <> 'SERVICEABLE')
        ),

    CONSTRAINT commerce_delivery_quotes_serviceable_chk
        CHECK (
            NOT serviceable
            OR (
                reason_code = 'SERVICEABLE'
                AND currency <> ''
                AND available_providers > 0
                AND truck_type <> ''
            )
        ),

    UNIQUE (caller_service, quote_reference)
);

CREATE INDEX commerce_delivery_quotes_expiry_idx
    ON commerce_delivery_quotes (expires_at, id);

COMMENT ON TABLE commerce_delivery_quotes IS
'Authoritative hauling (van and truck) delivery quotes issued to internal commerce callers. Persisted before being returned so the quote reference is verifiable evidence, not just a response field.';
