-- Section 17A: Customer Cart Foundation
--
-- Scope:
--   * customer-owned FOOD and SHOP carts
--   * hard FOOD/SHOP separation
--   * one FOOD vendor business + branch per cart
--   * multi-vendor / multi-branch SHOP carts
--   * catalogue and price snapshots that MUST be revalidated at checkout
--   * delivery-address snapshot
--   * optional third-party receiver
--   * variable-measure preparation metadata
--   * optimistic cart versioning
--   * append-only cart mutation history
--
-- Explicitly NOT implemented here:
--   * checkout / quote lock
--   * Inventory reservation
--   * logistics quoting / pricing / mission creation
--   * payment / Wallet
--   * ParentOrder / VendorOrder
--   * fulfilment
--
-- customer_id is an opaque Customer Service identity. There is deliberately
-- no cross-service database foreign key to Customer Service.
--
-- business_id / branch_id stored on cart lines are server-resolved catalogue
-- evidence. They are never accepted as customer identity/authority.

CREATE TABLE commerce_carts (
    id                  TEXT PRIMARY KEY,
    customer_id         TEXT NOT NULL,
    domain              TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'ACTIVE',

    -- FOOD only. The first Food line binds the cart to exactly one business
    -- and one branch. SHOP carts leave both columns NULL.
    food_business_id    TEXT,
    food_branch_id      TEXT,

    version             BIGINT NOT NULL DEFAULT 1,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_carts_domain_chk
        CHECK (domain IN ('FOOD', 'SHOP')),

    CONSTRAINT commerce_carts_status_chk
        CHECK (
            status IN (
                'ACTIVE',
                'CHECKOUT_IN_PROGRESS',
                'CONVERTED',
                'ABANDONED',
                'EXPIRED'
            )
        ),

    CONSTRAINT commerce_carts_version_chk
        CHECK (version > 0),

    CONSTRAINT commerce_carts_food_binding_pair_chk
        CHECK (
            (food_business_id IS NULL AND food_branch_id IS NULL)
            OR
            (food_business_id IS NOT NULL AND food_branch_id IS NOT NULL)
        ),

    CONSTRAINT commerce_carts_shop_has_no_food_binding_chk
        CHECK (
            domain <> 'SHOP'
            OR
            (food_business_id IS NULL AND food_branch_id IS NULL)
        )
);

-- A customer has at most one live cart in each hard commerce domain.
-- CHECKOUT_IN_PROGRESS remains live so a second device cannot create a
-- competing cart while later checkout work owns the cart.
CREATE UNIQUE INDEX commerce_carts_one_live_domain_idx
    ON commerce_carts(customer_id, domain)
    WHERE status IN ('ACTIVE', 'CHECKOUT_IN_PROGRESS');

CREATE INDEX commerce_carts_customer_history_idx
    ON commerce_carts(customer_id, created_at DESC, id);

CREATE INDEX commerce_carts_expiry_idx
    ON commerce_carts(expires_at, id)
    WHERE status = 'ACTIVE';


CREATE TABLE commerce_cart_addresses (
    cart_id             TEXT PRIMARY KEY
                        REFERENCES commerce_carts(id)
                        ON DELETE CASCADE,

    -- Opaque Customer Service saved-location reference.
    source_address_ref  TEXT NOT NULL,

    -- Immutable cart-time snapshot. Later Customer Service edits do not
    -- rewrite this evidence.
    title_snapshot      TEXT NOT NULL DEFAULT '',
    address_snapshot    TEXT NOT NULL,
    latitude_snapshot   DOUBLE PRECISION NOT NULL,
    longitude_snapshot  DOUBLE PRECISION NOT NULL,
    description_snapshot TEXT NOT NULL DEFAULT '',

    snapshotted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_cart_address_required_chk
        CHECK (length(btrim(address_snapshot)) > 0),

    CONSTRAINT commerce_cart_address_lat_chk
        CHECK (latitude_snapshot BETWEEN -90 AND 90),

    CONSTRAINT commerce_cart_address_lon_chk
        CHECK (longitude_snapshot BETWEEN -180 AND 180)
);


CREATE TABLE commerce_cart_receivers (
    cart_id             TEXT PRIMARY KEY
                        REFERENCES commerce_carts(id)
                        ON DELETE CASCADE,

    -- A receiver is delivery contact evidence only.
    -- It never becomes a Customer account.
    receiver_name       TEXT NOT NULL,
    receiver_phone      TEXT NOT NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_cart_receiver_name_chk
        CHECK (
            length(btrim(receiver_name)) BETWEEN 1 AND 160
        ),

    CONSTRAINT commerce_cart_receiver_phone_chk
        CHECK (
            length(btrim(receiver_phone)) BETWEEN 3 AND 32
        )
);


CREATE TABLE commerce_cart_items (
    id                          TEXT PRIMARY KEY,
    cart_id                     TEXT NOT NULL
                                REFERENCES commerce_carts(id)
                                ON DELETE CASCADE,

    domain                      TEXT NOT NULL,

    -- SERVER-RESOLVED authoritative grouping evidence.
    business_id                 TEXT NOT NULL,
    branch_id                   TEXT NOT NULL,

    -- Authoritative catalogue references.
    item_ref                    TEXT NOT NULL,
    variant_ref                 TEXT,
    menu_ref                    TEXT,
    listing_ref                 TEXT,
    sell_unit_ref               TEXT,
    size_ref                    TEXT,

    quantity                    BIGINT NOT NULL,

    -- Current commercial snapshot only.
    -- Checkout MUST re-resolve/revalidate before quoting or ordering.
    unit_price_minor_snapshot   BIGINT NOT NULL,
    currency_snapshot           TEXT NOT NULL,
    price_reference             TEXT NOT NULL,
    catalogue_revision          BIGINT NOT NULL,
    inventory_revision          BIGINT,

    -- Bounded customer UX display evidence.
    display_name_snapshot       TEXT NOT NULL,
    variant_name_snapshot       TEXT NOT NULL DEFAULT '',
    sell_unit_name_snapshot     TEXT NOT NULL DEFAULT '',

    -- Variable-weight / variable-measure preparation.
    requested_measure_ref       TEXT,
    expected_measure_quantity   BIGINT,
    pricing_basis               TEXT NOT NULL DEFAULT 'FIXED_UNIT',

    -- Safe logistics characteristics only. These are NOT logistics prices.
    logistics_profile_ref       TEXT,
    estimated_weight_grams      BIGINT,
    temperature_zone            TEXT,

    version                     BIGINT NOT NULL DEFAULT 1,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_cart_items_domain_chk
        CHECK (domain IN ('FOOD', 'SHOP')),

    CONSTRAINT commerce_cart_items_quantity_chk
        CHECK (quantity BETWEEN 1 AND 100000),

    CONSTRAINT commerce_cart_items_price_chk
        CHECK (unit_price_minor_snapshot >= 0),

    CONSTRAINT commerce_cart_items_currency_chk
        CHECK (currency_snapshot ~ '^[A-Z]{3}$'),

    CONSTRAINT commerce_cart_items_catalogue_revision_chk
        CHECK (catalogue_revision >= 0),

    CONSTRAINT commerce_cart_items_inventory_revision_chk
        CHECK (
            inventory_revision IS NULL
            OR inventory_revision >= 0
        ),

    CONSTRAINT commerce_cart_items_pricing_basis_chk
        CHECK (
            pricing_basis IN (
                'FIXED_UNIT',
                'FIXED_MEASURE',
                'VARIABLE_MEASURE'
            )
        ),

    CONSTRAINT commerce_cart_items_variable_measure_chk
        CHECK (
            pricing_basis <> 'VARIABLE_MEASURE'
            OR (
                requested_measure_ref IS NOT NULL
                AND length(btrim(requested_measure_ref)) > 0
                AND expected_measure_quantity IS NOT NULL
                AND expected_measure_quantity > 0
            )
        ),

    CONSTRAINT commerce_cart_items_weight_chk
        CHECK (
            estimated_weight_grams IS NULL
            OR estimated_weight_grams >= 0
        ),

    -- Food uses menu_ref; Shop uses listing_ref.
    CONSTRAINT commerce_cart_items_domain_reference_chk
        CHECK (
            (
                domain = 'FOOD'
                AND menu_ref IS NOT NULL
                AND listing_ref IS NULL
                AND sell_unit_ref IS NULL
            )
            OR
            (
                domain = 'SHOP'
                AND listing_ref IS NOT NULL
                AND menu_ref IS NULL
                AND sell_unit_ref IS NOT NULL
            )
        )
);

CREATE INDEX commerce_cart_items_cart_idx
    ON commerce_cart_items(cart_id, created_at, id);

CREATE INDEX commerce_cart_items_group_idx
    ON commerce_cart_items(cart_id, business_id, branch_id, id);

CREATE INDEX commerce_cart_items_catalogue_idx
    ON commerce_cart_items(domain, item_ref, variant_ref);


CREATE TABLE commerce_cart_item_options (
    id                          TEXT PRIMARY KEY,
    cart_id                     TEXT NOT NULL
                                REFERENCES commerce_carts(id)
                                ON DELETE CASCADE,
    cart_item_id                TEXT NOT NULL
                                REFERENCES commerce_cart_items(id)
                                ON DELETE CASCADE,

    group_ref                   TEXT NOT NULL,
    option_ref                  TEXT NOT NULL,
    quantity                    BIGINT NOT NULL DEFAULT 1,

    group_name_snapshot         TEXT NOT NULL DEFAULT '',
    option_name_snapshot        TEXT NOT NULL,

    price_delta_minor_snapshot  BIGINT NOT NULL DEFAULT 0,
    price_reference             TEXT NOT NULL,
    catalogue_revision          BIGINT NOT NULL,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_cart_item_options_quantity_chk
        CHECK (quantity BETWEEN 1 AND 100),

    CONSTRAINT commerce_cart_item_options_catalogue_revision_chk
        CHECK (catalogue_revision >= 0),

    CONSTRAINT commerce_cart_item_options_unique_selection
        UNIQUE (cart_item_id, group_ref, option_ref)
);

CREATE INDEX commerce_cart_item_options_cart_idx
    ON commerce_cart_item_options(cart_id, cart_item_id, id);


-- Append-only minimal mutation evidence.
-- This intentionally avoids copying Customer Service profile PII.
CREATE TABLE commerce_cart_history (
    id                  TEXT PRIMARY KEY,
    cart_id             TEXT NOT NULL
                        REFERENCES commerce_carts(id)
                        ON DELETE CASCADE,
    customer_id         TEXT NOT NULL,
    action              TEXT NOT NULL,
    item_id             TEXT,
    address_ref         TEXT,
    cart_version        BIGINT NOT NULL,
    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_cart_history_version_chk
        CHECK (cart_version > 0),

    CONSTRAINT commerce_cart_history_action_chk
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
                'commerce.cart.expired'
            )
        )
);

CREATE INDEX commerce_cart_history_cart_idx
    ON commerce_cart_history(cart_id, occurred_at, id);

CREATE INDEX commerce_cart_history_customer_idx
    ON commerce_cart_history(customer_id, occurred_at DESC, id);

COMMENT ON TABLE commerce_carts IS
'Section 17A customer cart aggregate. Customer identity comes from Customer Service bearer authentication; no Customer account is created here.';

COMMENT ON TABLE commerce_cart_items IS
'Section 17A cart catalogue snapshots. Snapshot price/availability evidence is not checkout authorization and must be revalidated later.';

COMMENT ON COLUMN commerce_cart_items.unit_price_minor_snapshot IS
'Observed catalogue selling price at cart mutation time; never final checkout or settlement authority.';

COMMENT ON COLUMN commerce_cart_items.logistics_profile_ref IS
'Safe catalogue/order logistics characteristic only. Vendor Service does not calculate platform logistics pricing from this field.';