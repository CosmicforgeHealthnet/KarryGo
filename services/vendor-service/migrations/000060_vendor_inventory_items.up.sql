-- Section 14B — Generic Inventory Item / Stock Basis Model.
--
-- This migration is additive.
--
-- Existing shop_inventory_* tables remain untouched and continue to serve the
-- certified Section 12 implementation until the Section 14C compatibility
-- bridge is introduced.
--
-- Generic Inventory owns identity and branch revision here. It does NOT yet
-- own generic balances, lots, movements, reservations, stocktakes, transfers,
-- reorder rules or alerts.

CREATE TABLE inventory_items (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses (id),

    branch_id text NOT NULL,

    -- Typed source identity. Future types are represented structurally now,
    -- but Section 14B application logic activates SHOP_VARIANT only.
    item_type text NOT NULL
        CHECK (
            item_type IN (
                'SHOP_VARIANT',
                'INGREDIENT',
                'PACKAGING',
                'PRODUCTION_COMPONENT'
            )
        ),

    source_ref text NOT NULL
        CHECK (
            char_length(source_ref) BETWEEN 1 AND 160
        ),

    -- Measurement truth remains owned by the source catalogue. Generic
    -- Inventory stores the resolved reference and ownership evidence only.
    stock_measure_ref text NOT NULL
        CHECK (
            char_length(stock_measure_ref) BETWEEN 1 AND 160
        ),

    stock_measure_owner_key text NOT NULL
        CHECK (
            char_length(stock_measure_owner_key) BETWEEN 1 AND 160
        ),

    -- Coarse normalized basis used by generic Inventory without redefining the
    -- Shop measurement vocabulary.
    stock_unit_kind text NOT NULL
        CHECK (
            stock_unit_kind IN (
                'COUNT',
                'MASS',
                'VOLUME',
                'CONFIGURED_MEASURE'
            )
        ),

    -- Preserve the already-certified Section 12 semantics.
    stock_basis_mode text NOT NULL
        CHECK (
            stock_basis_mode IN (
                'SAME_UNIT',
                'EXACT_CONVERSION',
                'VARIABLE_FULFILMENT'
            )
        ),

    tracking_mode text NOT NULL
        CHECK (
            tracking_mode IN (
                'NONE',
                'SIMPLE',
                'LOT',
                'EXPIRY',
                'VARIABLE_MEASURE'
            )
        ),

    lot_policy text NOT NULL DEFAULT 'FIFO'
        CHECK (
            lot_policy IN (
                'FIFO',
                'FEFO'
            )
        ),

    lot_tracking_enabled boolean NOT NULL DEFAULT false,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_items_branch_fk
        FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id),

    CONSTRAINT inventory_items_tracking_consistency_ck
        CHECK (
            (tracking_mode IN ('LOT','EXPIRY') AND lot_tracking_enabled)
            OR tracking_mode NOT IN ('LOT','EXPIRY')
        ),

    CONSTRAINT inventory_items_variable_measure_ck
        CHECK (
            stock_basis_mode <> 'VARIABLE_FULFILMENT'
            OR tracking_mode = 'VARIABLE_MEASURE'
        )
);

-- One logical stock identity per business/branch/source/measure.
--
-- Changing a stock policy must evolve the existing identity rather than
-- silently minting a second stock identity for the same physical source.
CREATE UNIQUE INDEX inventory_items_logical_uq
ON inventory_items (
    business_id,
    branch_id,
    item_type,
    source_ref,
    stock_measure_ref
);

CREATE INDEX inventory_items_business_branch_idx
ON inventory_items (
    business_id,
    branch_id,
    status,
    item_type
);

CREATE INDEX inventory_items_source_idx
ON inventory_items (
    business_id,
    item_type,
    source_ref
);

-- Generic Inventory revision. Future domains must not depend on
-- shop_inventory_revisions.
CREATE TABLE inventory_revisions (
    business_id text NOT NULL
        REFERENCES vendor_businesses (id),

    branch_id text NOT NULL,

    revision bigint NOT NULL DEFAULT 0
        CHECK (revision >= 0),

    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (
        business_id,
        branch_id
    ),

    CONSTRAINT inventory_revisions_branch_fk
        FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

-- Database-side protection for the currently supported source type.
--
-- Application adapters perform the richer exact/sell-unit checks. This
-- trigger exists so direct persistence cannot link another business's variant,
-- an inactive source, a foreign branch, or an invalid measurement.
CREATE OR REPLACE FUNCTION inventory_items_validate_shop_variant()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    resolved_measure_owner text;
    resolved_measure_base bigint;
BEGIN
    IF NEW.item_type <> 'SHOP_VARIANT' THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM shop_variants v
         WHERE v.id = NEW.source_ref
           AND v.business_id = NEW.business_id
           AND v.status = 'ACTIVE'
    ) THEN
        RAISE EXCEPTION
            'SHOP_VARIANT source is missing, foreign or inactive'
            USING ERRCODE = '23514';
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM shop_branch_listings l
         WHERE l.business_id = NEW.business_id
           AND l.branch_id = NEW.branch_id
           AND l.variant_id = NEW.source_ref
           AND l.status = 'ACTIVE'
    ) THEN
        RAISE EXCEPTION
            'SHOP_VARIANT is not active in this branch'
            USING ERRCODE = '23514';
    END IF;

    SELECT
        m.owner_key,
        m.base_quantity
      INTO
        resolved_measure_owner,
        resolved_measure_base
      FROM shop_measure_definitions m
     WHERE m.id = NEW.stock_measure_ref
       AND m.status = 'ACTIVE'
       AND m.owner_key IN ('PLATFORM', NEW.business_id)
     LIMIT 1;

    IF NOT FOUND THEN
        RAISE EXCEPTION
            'stock measure is missing, foreign or inactive'
            USING ERRCODE = '23514';
    END IF;

    IF resolved_measure_owner <> NEW.stock_measure_owner_key THEN
        RAISE EXCEPTION
            'stock measure ownership evidence does not match'
            USING ERRCODE = '23514';
    END IF;

    -- Exact arithmetic is never fabricated. Local measures whose canonical
    -- base quantity is unknown cannot be registered as EXACT_CONVERSION.
    IF NEW.stock_basis_mode = 'EXACT_CONVERSION'
       AND resolved_measure_base IS NULL THEN
        RAISE EXCEPTION
            'stock measure has no exact canonical conversion'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER inventory_items_validate_shop_variant_trigger
BEFORE INSERT OR UPDATE OF
    business_id,
    branch_id,
    item_type,
    source_ref,
    stock_measure_ref,
    stock_measure_owner_key,
    stock_basis_mode,
    status
ON inventory_items
FOR EACH ROW
WHEN (NEW.status = 'ACTIVE')
EXECUTE FUNCTION inventory_items_validate_shop_variant();