-- Section 14F — Stocktakes, Transfers, Quarantine, Reorder & Alerts.
--
-- Supplier authority is NOT introduced here.
-- supplier_ref is an opaque future integration seam only.

CREATE UNIQUE INDEX IF NOT EXISTS inventory_items_id_business_uq
ON inventory_items (
    id,
    business_id
);

-- ============================================================
-- QUARANTINE CASES
-- ============================================================

CREATE TABLE inventory_quarantines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    reason_code text NOT NULL
        CHECK (char_length(btrim(reason_code)) > 0),

    note text NOT NULL DEFAULT ''
        CHECK (char_length(note) <= 1000),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'RELEASED',
                'DISPOSED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_quarantines_item_fk
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

    UNIQUE (
        id,
        business_id,
        inventory_item_id
    )
);

CREATE INDEX inventory_quarantines_active_idx
ON inventory_quarantines (
    business_id,
    branch_id,
    status,
    created_at DESC
);

CREATE TABLE inventory_quarantine_allocations (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    quarantine_id text NOT NULL,
    inventory_item_id text NOT NULL,
    lot_id text NOT NULL,

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'RELEASED',
                'DISPOSED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_quarantine_allocations_quarantine_fk
        FOREIGN KEY (
            quarantine_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_quarantines (
            id,
            business_id,
            inventory_item_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_quarantine_allocations_lot_fk
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
        quarantine_id,
        lot_id
    )
);

-- ============================================================
-- STOCKTAKES
-- ============================================================

CREATE TABLE inventory_stocktakes (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    stocktake_type text NOT NULL
        CHECK (
            stocktake_type IN (
                'FULL_STOCKTAKE',
                'CYCLE_COUNT'
            )
        ),

    snapshot_revision bigint NOT NULL
        CHECK (snapshot_revision >= 0),

    status text NOT NULL DEFAULT 'COUNTING'
        CHECK (
            status IN (
                'COUNTING',
                'POSTED',
                'STALE',
                'CANCELLED'
            )
        ),

    started_at timestamptz NOT NULL DEFAULT now(),
    posted_at timestamptz,
    cancelled_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_stocktakes_branch_fk
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
        business_id
    )
);

CREATE UNIQUE INDEX inventory_stocktakes_active_uq
ON inventory_stocktakes (
    business_id,
    branch_id,
    stocktake_type
)
WHERE status = 'COUNTING';

CREATE TABLE inventory_stocktake_lines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    stocktake_id text NOT NULL,
    inventory_item_id text NOT NULL,

    expected_quantity bigint NOT NULL
        CHECK (expected_quantity >= 0),

    counted_quantity bigint
        CHECK (
            counted_quantity IS NULL
            OR counted_quantity >= 0
        ),

    variance_quantity bigint,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_stocktake_lines_stocktake_fk
        FOREIGN KEY (
            stocktake_id,
            business_id
        )
        REFERENCES inventory_stocktakes (
            id,
            business_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_stocktake_lines_item_fk
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

    UNIQUE (
        stocktake_id,
        inventory_item_id
    )
);

-- ============================================================
-- INTERNAL BRANCH TRANSFERS
-- ============================================================

CREATE TABLE inventory_transfers (
    id text PRIMARY KEY,

    business_id text NOT NULL,

    source_branch_id text NOT NULL,
    destination_branch_id text NOT NULL,

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'APPROVED',
                'IN_TRANSIT',
                'PARTIALLY_RECEIVED',
                'RECEIVED',
                'CANCELLED'
            )
        ),

    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 2000),

    approved_at timestamptz,
    dispatched_at timestamptz,
    received_at timestamptz,
    cancelled_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_transfers_source_fk
        FOREIGN KEY (
            source_branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    CONSTRAINT inventory_transfers_destination_fk
        FOREIGN KEY (
            destination_branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    CONSTRAINT inventory_transfers_distinct_branch_ck
        CHECK (
            source_branch_id <> destination_branch_id
        ),

    UNIQUE (
        id,
        business_id
    )
);

CREATE INDEX inventory_transfers_branch_idx
ON inventory_transfers (
    business_id,
    source_branch_id,
    destination_branch_id,
    status,
    created_at DESC
);

CREATE TABLE inventory_transfer_lines (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    transfer_id text NOT NULL,

    source_branch_id text NOT NULL,
    destination_branch_id text NOT NULL,

    source_inventory_item_id text NOT NULL,
    destination_inventory_item_id text NOT NULL,

    sent_quantity bigint NOT NULL
        CHECK (sent_quantity > 0),

    received_quantity bigint NOT NULL DEFAULT 0
        CHECK (received_quantity >= 0),

    damaged_quantity bigint NOT NULL DEFAULT 0
        CHECK (damaged_quantity >= 0),

    missing_quantity bigint NOT NULL DEFAULT 0
        CHECK (missing_quantity >= 0),

    status text NOT NULL DEFAULT 'PENDING'
        CHECK (
            status IN (
                'PENDING',
                'IN_TRANSIT',
                'PARTIALLY_RECEIVED',
                'RECEIVED',
                'CANCELLED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_transfer_lines_transfer_fk
        FOREIGN KEY (
            transfer_id,
            business_id
        )
        REFERENCES inventory_transfers (
            id,
            business_id
        )
        ON DELETE CASCADE,

    CONSTRAINT inventory_transfer_lines_source_item_fk
        FOREIGN KEY (
            source_inventory_item_id,
            business_id,
            source_branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_transfer_lines_destination_item_fk
        FOREIGN KEY (
            destination_inventory_item_id,
            business_id,
            destination_branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT inventory_transfer_lines_resolved_ck
        CHECK (
            received_quantity
            + damaged_quantity
            + missing_quantity
            <= sent_quantity
        ),

    UNIQUE (
        transfer_id,
        source_inventory_item_id
    )
);

-- Idempotency boundary for receive commands.
CREATE TABLE inventory_transfer_receipts (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    transfer_id text NOT NULL,

    source_ref text NOT NULL
        CHECK (char_length(btrim(source_ref)) > 0),

    request_fingerprint text NOT NULL
        CHECK (char_length(request_fingerprint) = 64),

    request_payload jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT inventory_transfer_receipts_transfer_fk
        FOREIGN KEY (
            transfer_id,
            business_id
        )
        REFERENCES inventory_transfers (
            id,
            business_id
        ),

    UNIQUE (
        business_id,
        transfer_id,
        source_ref
    )
);

-- ============================================================
-- REORDER RULES
-- ============================================================

CREATE TABLE inventory_reorder_rules (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    minimum_quantity bigint NOT NULL
        CHECK (minimum_quantity >= 0),

    target_quantity bigint NOT NULL
        CHECK (target_quantity > minimum_quantity),

    expiring_soon_hours integer
        CHECK (
            expiring_soon_hours IS NULL
            OR expiring_soon_hours BETWEEN 1 AND 8760
        ),

    -- Future Section 15 seam only.
    -- No supplier FK and no supplier CRUD is owned here.
    supplier_ref text,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'INACTIVE',
                'ARCHIVED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    CONSTRAINT inventory_reorder_rules_item_fk
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

    UNIQUE (
        business_id,
        branch_id,
        inventory_item_id
    ),

    UNIQUE (
        id,
        business_id
    )
);

CREATE INDEX inventory_reorder_rules_active_idx
ON inventory_reorder_rules (
    business_id,
    branch_id,
    status
);

-- ============================================================
-- OPERATIONAL ALERTS
-- ============================================================

CREATE TABLE inventory_alerts (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    inventory_item_id text,

    alert_type text NOT NULL
        CHECK (
            alert_type IN (
                'LOW_STOCK',
                'OUT_OF_STOCK',
                'EXPIRING_SOON',
                'EXPIRED',
                'BACKORDER',
                'BALANCE_MISMATCH',
                'TRANSFER_EXCEPTION'
            )
        ),

    severity text NOT NULL DEFAULT 'WARNING'
        CHECK (
            severity IN (
                'INFO',
                'WARNING',
                'CRITICAL'
            )
        ),

    dedupe_key text NOT NULL
        CHECK (char_length(btrim(dedupe_key)) > 0),

    details jsonb NOT NULL DEFAULT '{}'::jsonb,

    status text NOT NULL DEFAULT 'OPEN'
        CHECK (
            status IN (
                'OPEN',
                'ACKNOWLEDGED',
                'RESOLVED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    resolved_at timestamptz,

    CONSTRAINT inventory_alerts_branch_fk
        FOREIGN KEY (
            branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    CONSTRAINT inventory_alerts_item_fk
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

    -- One durable record per logical condition. A resolved condition may
    -- reopen later without creating duplicate active alerts.
    UNIQUE (
        business_id,
        dedupe_key
    )
);

CREATE INDEX inventory_alerts_open_idx
ON inventory_alerts (
    business_id,
    branch_id,
    status,
    created_at DESC
);