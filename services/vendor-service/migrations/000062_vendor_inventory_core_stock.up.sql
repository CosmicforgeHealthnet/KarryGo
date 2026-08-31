-- Section 14D — Core Stock Operations, Lots, Movements & Availability.
--
-- Generic Inventory becomes independently capable of holding authoritative
-- physical stock state.
--
-- `available` is deliberately NOT persisted. It is derived from stock truth.
--
-- Section 14E adds first-class reservation entities/concurrency semantics.
-- Section 14F adds transfers, stocktakes, quarantine cases, reorder and alerts.

CREATE UNIQUE INDEX inventory_items_scope_identity_uq
ON inventory_items (
    id,
    business_id,
    branch_id
);

-- ============================================================
-- BALANCES
-- ============================================================

CREATE TABLE inventory_balances (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    on_hand_quantity bigint NOT NULL DEFAULT 0
        CHECK (on_hand_quantity >= 0),

    reserved_quantity bigint NOT NULL DEFAULT 0
        CHECK (reserved_quantity >= 0),

    quarantined_quantity bigint NOT NULL DEFAULT 0
        CHECK (quarantined_quantity >= 0),

    in_transit_quantity bigint NOT NULL DEFAULT 0
        CHECK (in_transit_quantity >= 0),

    incoming_quantity bigint NOT NULL DEFAULT 0
        CHECK (incoming_quantity >= 0),

    backorder_quantity bigint NOT NULL DEFAULT 0
        CHECK (backorder_quantity >= 0),

    revision bigint NOT NULL DEFAULT 0
        CHECK (revision >= 0),

    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_balances_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_balances_physical_ck
        CHECK (
            reserved_quantity + quarantined_quantity
            <= on_hand_quantity
        ),

    UNIQUE (
        inventory_item_id,
        business_id
    ),

    UNIQUE (
        business_id,
        branch_id,
        inventory_item_id
    )
);

CREATE INDEX inventory_balances_branch_idx
ON inventory_balances (
    business_id,
    branch_id,
    inventory_item_id
);

-- ============================================================
-- LOTS / BATCHES
-- ============================================================

CREATE TABLE inventory_lots (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    lot_number text,
    batch_number text,

    source_type text NOT NULL,
    source_ref text NOT NULL,

    received_at timestamptz NOT NULL,

    manufactured_at timestamptz,
    best_before_at timestamptz,
    expires_at timestamptz,

    on_hand_quantity bigint NOT NULL
        CHECK (on_hand_quantity >= 0),

    reserved_quantity bigint NOT NULL DEFAULT 0
        CHECK (reserved_quantity >= 0),

    quarantined_quantity bigint NOT NULL DEFAULT 0
        CHECK (quarantined_quantity >= 0),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'DEPLETED',
                'QUARANTINED',
                'EXPIRED',
                'ARCHIVED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_lots_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_lots_quantity_ck
        CHECK (
            reserved_quantity + quarantined_quantity
            <= on_hand_quantity
        ),

    CONSTRAINT inventory_lots_dates_ck
        CHECK (
            (
                manufactured_at IS NULL
                OR manufactured_at <= received_at
            )
            AND (
                best_before_at IS NULL
                OR manufactured_at IS NULL
                OR best_before_at >= manufactured_at
            )
            AND (
                expires_at IS NULL
                OR expires_at >= received_at
            )
        ),

    UNIQUE (
        id,
        business_id,
        inventory_item_id
    )
);

CREATE INDEX inventory_lots_allocation_idx
ON inventory_lots (
    business_id,
    inventory_item_id,
    status,
    expires_at,
    received_at,
    id
)
WHERE on_hand_quantity > 0;

CREATE INDEX inventory_lots_expiry_idx
ON inventory_lots (
    business_id,
    branch_id,
    expires_at
)
WHERE expires_at IS NOT NULL;

-- ============================================================
-- IMMUTABLE MOVEMENT LEDGER
-- ============================================================

CREATE TABLE inventory_movements (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    lot_id text,

    movement_type text NOT NULL
        CHECK (
            movement_type IN (
                'RECEIVE',
                'RESERVE',
                'RELEASE',
                'COMMIT',
                'ADJUSTMENT_IN',
                'ADJUSTMENT_OUT',
                'TRANSFER_OUT',
                'TRANSFER_IN',
                'RETURN',
                'DAMAGE',
                'WASTE',
                'EXPIRY',
                'QUARANTINE_IN',
                'QUARANTINE_RELEASE',
                'STOCKTAKE_CORRECTION'
            )
        ),

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    on_hand_delta bigint NOT NULL DEFAULT 0,
    reserved_delta bigint NOT NULL DEFAULT 0,
    quarantined_delta bigint NOT NULL DEFAULT 0,
    in_transit_delta bigint NOT NULL DEFAULT 0,
    incoming_delta bigint NOT NULL DEFAULT 0,
    backorder_delta bigint NOT NULL DEFAULT 0,

    source_type text NOT NULL,
    source_ref text NOT NULL,

    reason_code text,
    actor_ref text,

    occurred_at timestamptz NOT NULL DEFAULT now(),

    inventory_revision bigint NOT NULL
        CHECK (inventory_revision > 0),

    balance_revision bigint NOT NULL
        CHECK (balance_revision > 0),

    CONSTRAINT inventory_movements_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_movements_lot_fk
        FOREIGN KEY (
            lot_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_lots (
            id,
            business_id,
            inventory_item_id
        ),

    CONSTRAINT inventory_movements_effect_ck
        CHECK (
            on_hand_delta <> 0
            OR reserved_delta <> 0
            OR quarantined_delta <> 0
            OR in_transit_delta <> 0
            OR incoming_delta <> 0
            OR backorder_delta <> 0
        ),

    UNIQUE (
        id,
        business_id,
        inventory_item_id
    )
);

CREATE INDEX inventory_movements_history_idx
ON inventory_movements (
    business_id,
    branch_id,
    occurred_at DESC,
    id
);

CREATE INDEX inventory_movements_item_idx
ON inventory_movements (
    business_id,
    inventory_item_id,
    occurred_at DESC,
    id
);

CREATE INDEX inventory_movements_source_idx
ON inventory_movements (
    business_id,
    source_type,
    source_ref
);

-- ============================================================
-- RECEIPTS
-- ============================================================

CREATE TABLE inventory_receipts (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    source_ref text,
    supplier_ref text,

    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 2000),

    received_at timestamptz NOT NULL,

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'POSTED',
                'CANCELLED'
            )
        ),

    posted_at timestamptz,
    cancelled_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_receipts_branch_fk
        FOREIGN KEY (
            branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    UNIQUE (
        id,
        business_id,
        branch_id
    )
);

CREATE INDEX inventory_receipts_branch_idx
ON inventory_receipts (
    business_id,
    branch_id,
    status,
    received_at DESC
);

CREATE TABLE inventory_receipt_lines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    receipt_id text NOT NULL,
    inventory_item_id text NOT NULL,

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    lot_number text,
    batch_number text,

    manufactured_at timestamptz,
    best_before_at timestamptz,
    expires_at timestamptz,

    unit_cost_minor bigint
        CHECK (
            unit_cost_minor IS NULL
            OR unit_cost_minor >= 0
        ),

    lot_id text,
    posted_movement_id text,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_receipt_lines_receipt_fk
        FOREIGN KEY (
            receipt_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_receipts (
            id,
            business_id,
            branch_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_receipt_lines_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_receipt_lines_lot_fk
        FOREIGN KEY (
            lot_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_lots (
            id,
            business_id,
            inventory_item_id
        ),

    CONSTRAINT inventory_receipt_lines_movement_fk
        FOREIGN KEY (
            posted_movement_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_movements (
            id,
            business_id,
            inventory_item_id
        )
);

-- ============================================================
-- DERIVED AVAILABILITY VIEW
-- ============================================================

CREATE VIEW inventory_balance_availability AS
SELECT
    b.id,
    b.business_id,
    b.branch_id,
    b.inventory_item_id,

    b.on_hand_quantity,
    b.reserved_quantity,
    b.quarantined_quantity,
    b.in_transit_quantity,
    b.incoming_quantity,
    b.backorder_quantity,

    CASE
        WHEN COUNT(l.id) > 0 THEN
            COALESCE(
                SUM(
                    CASE
                        WHEN l.status = 'ACTIVE'
                         AND (
                             l.expires_at IS NULL
                             OR l.expires_at > now()
                         )
                        THEN GREATEST(
                            l.on_hand_quantity
                            - l.reserved_quantity
                            - l.quarantined_quantity,
                            0
                        )
                        ELSE 0
                    END
                ),
                0
            )
        ELSE
            GREATEST(
                b.on_hand_quantity
                - b.reserved_quantity
                - b.quarantined_quantity,
                0
            )
    END AS available_quantity,

    COALESCE(
        SUM(
            CASE
                WHEN l.expires_at IS NOT NULL
                 AND l.expires_at <= now()
                 AND l.on_hand_quantity > 0
                THEN l.on_hand_quantity
                ELSE 0
            END
        ),
        0
    ) AS expired_quantity,

    b.revision,
    b.updated_at

FROM inventory_balances b

LEFT JOIN inventory_lots l
  ON l.business_id = b.business_id
 AND l.branch_id = b.branch_id
 AND l.inventory_item_id = b.inventory_item_id

GROUP BY
    b.id,
    b.business_id,
    b.branch_id,
    b.inventory_item_id,
    b.on_hand_quantity,
    b.reserved_quantity,
    b.quarantined_quantity,
    b.in_transit_quantity,
    b.incoming_quantity,
    b.backorder_quantity,
    b.revision,
    b.updated_at;

-- ============================================================
-- DATABASE MUTATION GUARDS
-- ============================================================

CREATE OR REPLACE FUNCTION inventory_authorized_mutation_required()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF COALESCE(
        current_setting(
            'karrygo.inventory_mutation',
            true
        ),
        ''
    ) <> '1' THEN
        RAISE EXCEPTION
            'inventory physical stock may only change through an authorized inventory mutation'
            USING ERRCODE = '55000';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER inventory_balances_mutation_guard
BEFORE INSERT OR UPDATE OR DELETE
ON inventory_balances
FOR EACH ROW
EXECUTE FUNCTION inventory_authorized_mutation_required();

CREATE TRIGGER inventory_lots_mutation_guard
BEFORE INSERT OR UPDATE OR DELETE
ON inventory_lots
FOR EACH ROW
EXECUTE FUNCTION inventory_authorized_mutation_required();

CREATE TRIGGER inventory_movements_insert_guard
BEFORE INSERT
ON inventory_movements
FOR EACH ROW
EXECUTE FUNCTION inventory_authorized_mutation_required();

CREATE OR REPLACE FUNCTION inventory_movements_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'inventory movement ledger is append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER inventory_movements_immutable_trigger
BEFORE UPDATE OR DELETE
ON inventory_movements
FOR EACH ROW
EXECUTE FUNCTION inventory_movements_immutable();