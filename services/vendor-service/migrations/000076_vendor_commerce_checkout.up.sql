-- Section 17B: Checkout Validation & Inventory Reservation.
--
-- Scope:
--   * turn an ACTIVE Section 17A cart into a validated checkout candidate
--   * authoritative Food revalidation (business/branch/menu/item/variant,
--     service window, sellability, production-or-inventory availability,
--     current price)
--   * authoritative Shop revalidation (listing/variant/sell unit/measurement,
--     current price, current Section 14 Inventory availability)
--   * Shop physical stock reservation through Section 14 authority
--   * a configurable checkout reservation TTL
--   * deterministic multi-vendor reservation coordination and compensation
--   * variable-weight preparation evidence
--
-- Explicitly NOT implemented here:
--   * the final immutable quote and its approval/lock (17D)
--   * logistics pricing, dispatch/hauling/taxi mission creation
--   * payment, Wallet, escrow or any money movement
--   * ParentOrder / VendorOrder
--   * fulfilment
--
-- A checkout session is validation evidence, not authorization. Nothing in
-- this migration stores an authorized amount that may be captured; the two
-- money columns are inputs a later section needs in order to build a quote,
-- and both are recomputed rather than trusted whenever they are used.
--
-- Every row is customer-scoped. customer_id is an opaque Customer Service
-- identity and there is deliberately no cross-service foreign key to it.
-- business_id / branch_id are server-resolved catalogue evidence carried
-- forward from the cart line, never customer-supplied authority.

-- Section 17A wrote a closed action vocabulary. Checkout adds exactly two
-- cart-level transitions and no other action.
ALTER TABLE commerce_cart_history
    DROP CONSTRAINT commerce_cart_history_action_chk;

ALTER TABLE commerce_cart_history
    ADD CONSTRAINT commerce_cart_history_action_chk
    CHECK (
        action IN (
            'commerce.cart.created',
            'commerce.cart.item_added',
            'commerce.cart.item_updated',
            'commerce.cart.item_removed',
            'commerce.cart.cleared',
            'commerce.cart.address_changed',
            'commerce.cart.address_removed',
            'commerce.cart.receiver_changed',
            'commerce.cart.receiver_removed',
            'commerce.cart.abandoned',
            'commerce.cart.expired',
            'commerce.cart.checkout_started',
            'commerce.cart.checkout_released'
        )
    );


CREATE TABLE commerce_checkout_sessions (
    id                      TEXT PRIMARY KEY,

    customer_id             TEXT NOT NULL,

    cart_id                 TEXT NOT NULL
                            REFERENCES commerce_carts(id)
                            ON DELETE CASCADE,

    -- The exact cart version this validation was computed against. A cart
    -- mutation bumps the version, so a session can never be silently applied
    -- to different cart content.
    cart_version            BIGINT NOT NULL,

    domain                  TEXT NOT NULL,

    status                  TEXT NOT NULL DEFAULT 'VALIDATING',

    -- Monotonic per-cart validation attempt counter.
    validation_revision     BIGINT NOT NULL,

    -- Section 17A delivery address snapshot evidence. The snapshot itself
    -- stays on the cart; checkout records which one it validated against.
    address_ref             TEXT NOT NULL DEFAULT '',
    address_snapshotted_at  TIMESTAMPTZ,

    currency                TEXT NOT NULL DEFAULT '',

    -- Preparation for the Section 17D quote. NOT a quote, NOT an
    -- authorization, and never a settlement figure.
    quoted_items_total_minor            BIGINT NOT NULL DEFAULT 0,
    max_authorized_items_total_minor    BIGINT NOT NULL DEFAULT 0,

    -- True when any line's authoritative catalogue/price differs materially
    -- from the cart snapshot. 17B reports the change; it does not resolve it.
    price_changed           BOOLEAN NOT NULL DEFAULT false,
    catalogue_changed       BOOLEAN NOT NULL DEFAULT false,

    failure_code            TEXT NOT NULL DEFAULT '',

    reservation_ttl_seconds INTEGER NOT NULL,

    expires_at              TIMESTAMPTZ NOT NULL,

    -- Durable multi-vendor coordination state. This is what makes a crash
    -- mid-reservation recoverable: it names, before any Inventory call is
    -- made, that reservations may exist which nothing has yet acknowledged.
    reservation_coordination_state TEXT NOT NULL DEFAULT 'NONE',

    version                 BIGINT NOT NULL DEFAULT 1,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    validated_at            TIMESTAMPTZ,
    settled_at              TIMESTAMPTZ,

    CONSTRAINT commerce_checkout_sessions_domain_chk
        CHECK (domain IN ('FOOD', 'SHOP')),

    CONSTRAINT commerce_checkout_sessions_status_chk
        CHECK (
            status IN (
                'VALIDATING',
                'VALIDATED',
                'FAILED',
                'EXPIRED',
                'CANCELLED'
            )
        ),

    CONSTRAINT commerce_checkout_sessions_coordination_chk
        CHECK (
            reservation_coordination_state IN (
                'NONE',
                'IN_PROGRESS',
                'COMPLETE',
                'COMPENSATING',
                'COMPENSATED',
                'ORPHANED'
            )
        ),

    CONSTRAINT commerce_checkout_sessions_revision_chk
        CHECK (validation_revision > 0),

    CONSTRAINT commerce_checkout_sessions_version_chk
        CHECK (version > 0),

    CONSTRAINT commerce_checkout_sessions_cart_version_chk
        CHECK (cart_version > 0),

    CONSTRAINT commerce_checkout_sessions_ttl_chk
        CHECK (reservation_ttl_seconds BETWEEN 1 AND 86400),

    CONSTRAINT commerce_checkout_sessions_currency_chk
        CHECK (
            currency = ''
            OR currency ~ '^[A-Z]{3}$'
        ),

    CONSTRAINT commerce_checkout_sessions_money_chk
        CHECK (
            quoted_items_total_minor >= 0
            AND max_authorized_items_total_minor >= quoted_items_total_minor
        ),

    -- A validated session must name the money basis a quote will be built
    -- from. A failed one must name why it failed.
    CONSTRAINT commerce_checkout_sessions_validated_evidence_chk
        CHECK (
            status <> 'VALIDATED'
            OR (
                validated_at IS NOT NULL
                AND currency <> ''
                AND failure_code = ''
            )
        ),

    CONSTRAINT commerce_checkout_sessions_failed_evidence_chk
        CHECK (
            status <> 'FAILED'
            OR failure_code <> ''
        ),

    -- Customer isolation is enforced by the row shape, not only by queries.
    UNIQUE (id, customer_id)
);

-- One live checkout session per cart. A second device cannot open a competing
-- validation while real stock is held for the first.
CREATE UNIQUE INDEX commerce_checkout_sessions_one_live_idx
    ON commerce_checkout_sessions(cart_id)
    WHERE status IN ('VALIDATING', 'VALIDATED');

CREATE UNIQUE INDEX commerce_checkout_sessions_cart_revision_idx
    ON commerce_checkout_sessions(cart_id, validation_revision);

CREATE INDEX commerce_checkout_sessions_customer_idx
    ON commerce_checkout_sessions(customer_id, created_at DESC, id);

-- The sweep index. Both the TTL sweep and the stranded-coordination sweep
-- read live sessions only.
CREATE INDEX commerce_checkout_sessions_sweep_idx
    ON commerce_checkout_sessions(expires_at, id)
    WHERE status IN ('VALIDATING', 'VALIDATED');

CREATE INDEX commerce_checkout_sessions_coordination_idx
    ON commerce_checkout_sessions(updated_at, id)
    WHERE reservation_coordination_state IN ('IN_PROGRESS', 'COMPENSATING');


-- One Vendor business/branch group of the checkout candidate.
--
-- Shop checkouts may have many; Food checkouts always have exactly one,
-- because Section 17A binds a Food cart to a single branch.
CREATE TABLE commerce_checkout_groups (
    id                      TEXT PRIMARY KEY,

    session_id              TEXT NOT NULL
                            REFERENCES commerce_checkout_sessions(id)
                            ON DELETE CASCADE,

    customer_id             TEXT NOT NULL,

    domain                  TEXT NOT NULL,

    business_id             TEXT NOT NULL,
    branch_id               TEXT NOT NULL,

    -- Food never reserves raw Shop stock. Its production/inventory
    -- availability is resolved through the Food authority's own seam.
    requires_stock_reservation BOOLEAN NOT NULL DEFAULT false,

    reservation_status      TEXT NOT NULL DEFAULT 'NOT_REQUIRED',

    -- The Section 14 reservation source reference. It is generated and
    -- committed BEFORE the Inventory call, so a crash always leaves a durable
    -- key that can find, and release, a reservation nothing acknowledged.
    reservation_source_ref  TEXT NOT NULL DEFAULT '',

    -- The Section 14 reservation identity, once Inventory has answered.
    inventory_reservation_ref TEXT NOT NULL DEFAULT '',

    reservation_expires_at  TIMESTAMPTZ,

    reservation_failure_code TEXT NOT NULL DEFAULT '',

    catalogue_revision      BIGINT NOT NULL DEFAULT 0,
    inventory_revision      BIGINT,

    currency                TEXT NOT NULL DEFAULT '',

    quoted_total_minor              BIGINT NOT NULL DEFAULT 0,
    max_authorized_total_minor      BIGINT NOT NULL DEFAULT 0,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_checkout_groups_domain_chk
        CHECK (domain IN ('FOOD', 'SHOP')),

    CONSTRAINT commerce_checkout_groups_reservation_status_chk
        CHECK (
            reservation_status IN (
                'NOT_REQUIRED',
                'PENDING',
                'RESERVED',
                'FAILED',
                'RELEASED'
            )
        ),

    -- Food may never carry a stock reservation in 17B.
    CONSTRAINT commerce_checkout_groups_food_no_reservation_chk
        CHECK (
            domain <> 'FOOD'
            OR (
                requires_stock_reservation = false
                AND reservation_status = 'NOT_REQUIRED'
                AND inventory_reservation_ref = ''
            )
        ),

    -- A group that needs stock must carry the idempotency key it used.
    CONSTRAINT commerce_checkout_groups_source_ref_chk
        CHECK (
            requires_stock_reservation = false
            OR length(btrim(reservation_source_ref)) > 0
        ),

    -- RESERVED is only sayable with real Inventory evidence.
    CONSTRAINT commerce_checkout_groups_reserved_evidence_chk
        CHECK (
            reservation_status <> 'RESERVED'
            OR (
                requires_stock_reservation = true
                AND length(btrim(inventory_reservation_ref)) > 0
            )
        ),

    CONSTRAINT commerce_checkout_groups_money_chk
        CHECK (
            quoted_total_minor >= 0
            AND max_authorized_total_minor >= quoted_total_minor
        ),

    CONSTRAINT commerce_checkout_groups_currency_chk
        CHECK (
            currency = ''
            OR currency ~ '^[A-Z]{3}$'
        ),

    CONSTRAINT commerce_checkout_groups_inventory_revision_chk
        CHECK (
            inventory_revision IS NULL
            OR inventory_revision >= 0
        ),

    UNIQUE (session_id, business_id, branch_id),
    UNIQUE (id, session_id)
);

CREATE INDEX commerce_checkout_groups_session_idx
    ON commerce_checkout_groups(session_id, business_id, branch_id, id);

-- The recovery lookup: a durable key that may point at a reservation nothing
-- has acknowledged yet.
CREATE INDEX commerce_checkout_groups_recovery_idx
    ON commerce_checkout_groups(reservation_status, id)
    WHERE reservation_status IN ('PENDING', 'RESERVED');


-- One revalidated cart line.
--
-- Every commercial value here is the CURRENT authoritative answer. The
-- snapshot_* columns are the Section 17A cart evidence the customer last saw,
-- kept only so the difference can be reported honestly.
CREATE TABLE commerce_checkout_lines (
    id                      TEXT PRIMARY KEY,

    session_id              TEXT NOT NULL
                            REFERENCES commerce_checkout_sessions(id)
                            ON DELETE CASCADE,

    group_id                TEXT
                            REFERENCES commerce_checkout_groups(id)
                            ON DELETE CASCADE,

    customer_id             TEXT NOT NULL,

    cart_item_id            TEXT NOT NULL,

    domain                  TEXT NOT NULL,

    business_id             TEXT NOT NULL,
    branch_id               TEXT NOT NULL,

    item_ref                TEXT NOT NULL,
    variant_ref             TEXT NOT NULL DEFAULT '',
    menu_ref                TEXT NOT NULL DEFAULT '',
    listing_ref             TEXT NOT NULL DEFAULT '',
    sell_unit_ref           TEXT NOT NULL DEFAULT '',

    quantity                BIGINT NOT NULL,

    display_name            TEXT NOT NULL DEFAULT '',

    -- Current authoritative commercial answer.
    unit_price_minor        BIGINT NOT NULL DEFAULT 0,
    currency                TEXT NOT NULL DEFAULT '',
    price_reference         TEXT NOT NULL DEFAULT '',
    catalogue_revision      BIGINT NOT NULL DEFAULT 0,
    inventory_revision      BIGINT,

    -- What the cart snapshot said, for change reporting only.
    snapshot_unit_price_minor   BIGINT NOT NULL DEFAULT 0,
    snapshot_price_reference    TEXT NOT NULL DEFAULT '',
    snapshot_catalogue_revision BIGINT NOT NULL DEFAULT 0,

    price_changed           BOOLEAN NOT NULL DEFAULT false,
    catalogue_changed       BOOLEAN NOT NULL DEFAULT false,

    -- Variable-measure preparation. Section 17D needs all four to build a
    -- quote whose maximum authorized amount is defensible.
    pricing_basis           TEXT NOT NULL DEFAULT 'FIXED_UNIT',
    requested_measure_ref   TEXT NOT NULL DEFAULT '',
    expected_measure_quantity BIGINT NOT NULL DEFAULT 0,
    measure_underfill_bps   INTEGER NOT NULL DEFAULT 0,
    measure_overfill_bps    INTEGER NOT NULL DEFAULT 0,

    quoted_total_minor          BIGINT NOT NULL DEFAULT 0,
    max_authorized_total_minor  BIGINT NOT NULL DEFAULT 0,

    -- The Section 14 reservation basis this line contributed.
    inventory_item_ref      TEXT NOT NULL DEFAULT '',
    reservation_quantity    BIGINT NOT NULL DEFAULT 0,
    reservation_basis       TEXT NOT NULL DEFAULT 'NONE',

    validation_status       TEXT NOT NULL,
    reason_code             TEXT NOT NULL DEFAULT '',
    reason_detail           TEXT NOT NULL DEFAULT '',

    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_checkout_lines_domain_chk
        CHECK (domain IN ('FOOD', 'SHOP')),

    CONSTRAINT commerce_checkout_lines_quantity_chk
        CHECK (quantity BETWEEN 1 AND 100000),

    CONSTRAINT commerce_checkout_lines_validation_chk
        CHECK (
            validation_status IN (
                'VALID',
                'CHANGED',
                'UNAVAILABLE',
                'INVALID'
            )
        ),

    CONSTRAINT commerce_checkout_lines_pricing_basis_chk
        CHECK (
            pricing_basis IN (
                'FIXED_UNIT',
                'FIXED_MEASURE',
                'VARIABLE_MEASURE'
            )
        ),

    CONSTRAINT commerce_checkout_lines_reservation_basis_chk
        CHECK (
            reservation_basis IN (
                'NONE',
                'SELL_UNIT',
                'STOCK_MEASURE'
            )
        ),

    -- A variable-measure line must carry the measure it was quoted on.
    CONSTRAINT commerce_checkout_lines_variable_measure_chk
        CHECK (
            pricing_basis <> 'VARIABLE_MEASURE'
            OR validation_status IN ('UNAVAILABLE', 'INVALID')
            OR (
                length(btrim(requested_measure_ref)) > 0
                AND expected_measure_quantity > 0
            )
        ),

    CONSTRAINT commerce_checkout_lines_tolerance_chk
        CHECK (
            measure_underfill_bps BETWEEN 0 AND 10000
            AND measure_overfill_bps BETWEEN 0 AND 10000
        ),

    CONSTRAINT commerce_checkout_lines_money_chk
        CHECK (
            unit_price_minor >= 0
            AND quoted_total_minor >= 0
            AND max_authorized_total_minor >= quoted_total_minor
        ),

    CONSTRAINT commerce_checkout_lines_reservation_quantity_chk
        CHECK (reservation_quantity >= 0),

    -- A line that reserves stock must name the Inventory identity it
    -- reserved against and how much, in what basis.
    CONSTRAINT commerce_checkout_lines_reservation_evidence_chk
        CHECK (
            reservation_basis = 'NONE'
            OR (
                length(btrim(inventory_item_ref)) > 0
                AND reservation_quantity > 0
            )
        ),

    -- A denied line must say why.
    CONSTRAINT commerce_checkout_lines_reason_chk
        CHECK (
            validation_status IN ('VALID', 'CHANGED')
            OR length(btrim(reason_code)) > 0
        ),

    UNIQUE (session_id, cart_item_id)
);

CREATE INDEX commerce_checkout_lines_session_idx
    ON commerce_checkout_lines(session_id, group_id, id);

CREATE INDEX commerce_checkout_lines_group_idx
    ON commerce_checkout_lines(group_id, id);


-- Append-only checkout evidence. Deliberately minimal: no customer profile
-- data, no money beyond what the session already records.
CREATE TABLE commerce_checkout_events (
    id                  TEXT PRIMARY KEY,

    session_id          TEXT NOT NULL
                        REFERENCES commerce_checkout_sessions(id)
                        ON DELETE CASCADE,

    customer_id         TEXT NOT NULL,

    action              TEXT NOT NULL,

    group_id            TEXT,

    detail              TEXT NOT NULL DEFAULT '',

    session_version     BIGINT NOT NULL,

    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_checkout_events_version_chk
        CHECK (session_version > 0),

    CONSTRAINT commerce_checkout_events_action_chk
        CHECK (
            action IN (
                'commerce.checkout.validation_started',
                'commerce.checkout.validation_failed',
                'commerce.checkout.reservation_requested',
                'commerce.checkout.reservation_held',
                'commerce.checkout.reservation_failed',
                'commerce.checkout.reservation_released',
                'commerce.checkout.compensation_started',
                'commerce.checkout.compensation_completed',
                'commerce.checkout.validated',
                'commerce.checkout.cancelled',
                'commerce.checkout.expired',
                'commerce.checkout.orphaned'
            )
        )
);

CREATE INDEX commerce_checkout_events_session_idx
    ON commerce_checkout_events(session_id, occurred_at, id);


COMMENT ON TABLE commerce_checkout_sessions IS
'Section 17B checkout validation record. It is a validated checkout candidate, not a quote, not an authorization and not an order.';

COMMENT ON COLUMN commerce_checkout_sessions.reservation_coordination_state IS
'Durable multi-vendor coordination progress. IN_PROGRESS means reservations may exist that nothing has acknowledged; recovery must release them.';

COMMENT ON COLUMN commerce_checkout_sessions.max_authorized_items_total_minor IS
'Variable-weight preparation input for the Section 17D quote. Nothing in Section 17B may capture, hold or move money against it.';

COMMENT ON COLUMN commerce_checkout_groups.reservation_source_ref IS
'Section 14 reservation source reference, committed before the Inventory call so a crashed coordination is always recoverable and every retry is idempotent.';

COMMENT ON COLUMN commerce_checkout_lines.reservation_basis IS
'SELL_UNIT reserves in the ordered sell-unit count; STOCK_MEASURE reserves in the stock basis measure for variable fulfilment. Vendor Service never invents a conversion between them.';
