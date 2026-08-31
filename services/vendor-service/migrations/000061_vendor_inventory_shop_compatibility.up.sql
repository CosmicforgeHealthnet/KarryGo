-- Section 14C — Shop Inventory Compatibility & Migration Bridge.
--
-- Existing shop_inventory_* tables remain operational.
--
-- This migration does NOT reconstruct:
--   balances
--   lots
--   movements
--   reservations
--   receipts
--   transfers
--   stocktakes
--   reorder rules
--   alerts
--
-- Generic inventory_items receives identity/policy linkage only.
--
-- Legacy Shop stock-basis IDs are preserved as generic inventory item IDs.

-- Shop has one authoritative stock basis per business/branch/variant.
--
-- Section 14B's general logical identity included the stock measure so future
-- source types could support richer patterns. Shop compatibility is stricter:
-- one Shop variant is one generic Inventory identity in a branch.
CREATE UNIQUE INDEX inventory_items_shop_variant_identity_uq
ON inventory_items (
    business_id,
    branch_id,
    source_ref
)
WHERE item_type = 'SHOP_VARIANT';

-- Durable, restartable state for the legacy Shop stock-basis identity backfill.
--
-- No large data rewrite runs inside this migration.
CREATE TABLE inventory_shop_compat_backfill_jobs (
    job_key text PRIMARY KEY,

    status text NOT NULL DEFAULT 'PENDING'
        CHECK (
            status IN (
                'PENDING',
                'RUNNING',
                'COMPLETED',
                'FAILED'
            )
        ),

    cursor_stock_basis_id text NOT NULL DEFAULT '',

    processed_count bigint NOT NULL DEFAULT 0
        CHECK (processed_count >= 0),

    batch_count bigint NOT NULL DEFAULT 0
        CHECK (batch_count >= 0),

    attempt_count bigint NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0),

    last_error text,

    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Auditable successful batch history.
CREATE TABLE inventory_shop_compat_backfill_batches (
    id text PRIMARY KEY,

    job_key text NOT NULL
        REFERENCES inventory_shop_compat_backfill_jobs (job_key),

    from_cursor text NOT NULL DEFAULT '',
    to_cursor text NOT NULL DEFAULT '',

    processed_count integer NOT NULL
        CHECK (processed_count >= 0),

    changed_count integer NOT NULL
        CHECK (changed_count >= 0),

    status text NOT NULL
        CHECK (
            status IN (
                'COMPLETED',
                'FAILED'
            )
        ),

    error_message text,

    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX inventory_shop_compat_batches_job_idx
ON inventory_shop_compat_backfill_batches (
    job_key,
    created_at DESC
);