-- Section 14E — Reservations, Concurrency & Authoritative Sellability.
--
-- Reservation authority is separate from Order, Checkout, RFQ and Payment.
--
-- Allocation policy in 14E is deliberately ALL_OR_NOTHING.
-- Generic backorder/preorder authority is not invented here.

CREATE TABLE inventory_reservations (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    source_type text NOT NULL
        CHECK (
            source_type IN (
                'ORDER',
                'CHECKOUT',
                'RFQ',
                'MANUAL',
                'INTERNAL'
            )
        ),

    source_ref text NOT NULL
        CHECK (char_length(btrim(source_ref)) > 0),

    request_fingerprint text NOT NULL
        CHECK (char_length(request_fingerprint) = 64),

    allocation_policy text NOT NULL DEFAULT 'ALL_OR_NOTHING'
        CHECK (allocation_policy = 'ALL_OR_NOTHING'),

    status text NOT NULL DEFAULT 'HELD'
        CHECK (
            status IN (
                'HELD',
                'CONFIRMED',
                'COMMITTED',
                'RELEASED',
                'EXPIRED',
                'CANCELLED'
            )
        ),

    expires_at timestamptz,

    held_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    committed_at timestamptz,
    released_at timestamptz,
    expired_at timestamptz,
    cancelled_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_reservations_branch_fk
        FOREIGN KEY (
            branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    UNIQUE (
        business_id,
        source_type,
        source_ref
    ),

    UNIQUE (
        id,
        business_id,
        branch_id
    )
);

CREATE INDEX inventory_reservations_expiry_idx
ON inventory_reservations (
    expires_at,
    id
)
WHERE
    status IN ('HELD','CONFIRMED')
    AND expires_at IS NOT NULL;

CREATE INDEX inventory_reservations_branch_idx
ON inventory_reservations (
    business_id,
    branch_id,
    status,
    created_at DESC,
    id
);

CREATE TABLE inventory_reservation_lines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    reservation_id text NOT NULL,
    inventory_item_id text NOT NULL,

    requested_quantity bigint NOT NULL
        CHECK (requested_quantity > 0),

    physical_quantity bigint NOT NULL
        CHECK (physical_quantity > 0),

    backorder_quantity bigint NOT NULL DEFAULT 0
        CHECK (backorder_quantity = 0),

    status text NOT NULL DEFAULT 'HELD'
        CHECK (
            status IN (
                'HELD',
                'CONFIRMED',
                'COMMITTED',
                'RELEASED',
                'EXPIRED',
                'CANCELLED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_reservation_lines_reservation_fk
        FOREIGN KEY (
            reservation_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_reservations (
            id,
            business_id,
            branch_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_reservation_lines_item_fk
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

    CONSTRAINT inventory_reservation_lines_all_or_nothing_ck
        CHECK (
            physical_quantity = requested_quantity
            AND backorder_quantity = 0
        ),

    UNIQUE (
        reservation_id,
        inventory_item_id
    ),

    UNIQUE (
        id,
        business_id,
        inventory_item_id
    )
);

CREATE TABLE inventory_reservation_allocations (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    reservation_id text NOT NULL,
    reservation_line_id text NOT NULL,

    inventory_item_id text NOT NULL,
    lot_id text NOT NULL,

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    status text NOT NULL DEFAULT 'HELD'
        CHECK (
            status IN (
                'HELD',
                'COMMITTED',
                'RELEASED',
                'EXPIRED',
                'CANCELLED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_reservation_allocations_reservation_fk
        FOREIGN KEY (
            reservation_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_reservations (
            id,
            business_id,
            branch_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_reservation_allocations_line_fk
        FOREIGN KEY (
            reservation_line_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_reservation_lines (
            id,
            business_id,
            inventory_item_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_reservation_allocations_lot_fk
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

    UNIQUE (
        reservation_line_id,
        lot_id
    )
);

CREATE INDEX inventory_reservation_allocations_item_idx
ON inventory_reservation_allocations (
    business_id,
    inventory_item_id,
    status,
    lot_id
);

-- ============================================================
-- EFFECTIVE AVAILABILITY
-- ============================================================
--
-- A hold whose expires_at has passed is no longer allowed to make discovery
-- availability permanently stale merely because the expiry sweeper has not
-- yet processed it.
--
-- Raw reserved_quantity remains the transactional stock projection until the
-- durable release is posted. This view removes only provably stale allocations
-- from the externally authoritative availability calculation.

-- CREATE OR REPLACE VIEW cannot change an existing column data type.
-- 000062 created available_quantity/reserved_quantity/expired_quantity as
-- numeric (SUM over bigint yields numeric); this definition casts them to
-- bigint, so the view must be dropped and recreated rather than replaced.
-- Nothing depends on this view at this point in the chain.
DROP VIEW IF EXISTS inventory_balance_availability;

CREATE VIEW inventory_balance_availability AS

WITH stale_allocations AS (
    SELECT
        a.business_id,
        a.branch_id,
        a.inventory_item_id,
        a.lot_id,
        SUM(a.quantity)::bigint AS quantity
    FROM inventory_reservation_allocations a
    JOIN inventory_reservations r
      ON r.id = a.reservation_id
     AND r.business_id = a.business_id
     AND r.branch_id = a.branch_id
    WHERE
        a.status = 'HELD'
        AND r.status IN ('HELD','CONFIRMED')
        AND r.expires_at IS NOT NULL
        AND r.expires_at <= now()
    GROUP BY
        a.business_id,
        a.branch_id,
        a.inventory_item_id,
        a.lot_id
),

stale_by_item AS (
    SELECT
        business_id,
        branch_id,
        inventory_item_id,
        SUM(quantity)::bigint AS quantity
    FROM stale_allocations
    GROUP BY
        business_id,
        branch_id,
        inventory_item_id
),

effective_lots AS (
    SELECT
        l.*,

        GREATEST(
            l.reserved_quantity
            - COALESCE(sa.quantity,0),
            0
        )::bigint AS effective_reserved_quantity

    FROM inventory_lots l

    LEFT JOIN stale_allocations sa
      ON sa.business_id = l.business_id
     AND sa.branch_id = l.branch_id
     AND sa.inventory_item_id = l.inventory_item_id
     AND sa.lot_id = l.id
)

SELECT
    b.id,
    b.business_id,
    b.branch_id,
    b.inventory_item_id,

    b.on_hand_quantity,

    GREATEST(
        b.reserved_quantity
        - COALESCE(sbi.quantity,0),
        0
    )::bigint AS reserved_quantity,

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
                            - l.effective_reserved_quantity
                            - l.quarantined_quantity,
                            0
                        )
                        ELSE 0
                    END
                ),
                0
            )::bigint

        ELSE
            GREATEST(
                b.on_hand_quantity
                - GREATEST(
                    b.reserved_quantity
                    - COALESCE(sbi.quantity,0),
                    0
                )
                - b.quarantined_quantity,
                0
            )::bigint
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
    )::bigint AS expired_quantity,

    b.revision,
    b.updated_at

FROM inventory_balances b

LEFT JOIN stale_by_item sbi
  ON sbi.business_id = b.business_id
 AND sbi.branch_id = b.branch_id
 AND sbi.inventory_item_id = b.inventory_item_id

LEFT JOIN effective_lots l
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
    sbi.quantity,
    b.revision,
    b.updated_at;