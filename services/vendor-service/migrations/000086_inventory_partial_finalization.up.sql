-- Corrective Inventory authority capability for Shop partial picks.
-- 000063 remains immutable and continues to create all-or-nothing holds;
-- these evidence columns let finalisation split a held quantity safely.

ALTER TABLE inventory_reservation_allocations
    ADD COLUMN IF NOT EXISTS committed_quantity BIGINT NOT NULL DEFAULT 0
        CHECK (committed_quantity >= 0),
    ADD COLUMN IF NOT EXISTS released_quantity BIGINT NOT NULL DEFAULT 0
        CHECK (released_quantity >= 0);

ALTER TABLE inventory_reservation_allocations
    DROP CONSTRAINT IF EXISTS inventory_reservation_allocations_quantity_reconcile_ck;
ALTER TABLE inventory_reservation_allocations
    ADD CONSTRAINT inventory_reservation_allocations_quantity_reconcile_ck
        CHECK (committed_quantity + released_quantity <= quantity);

CREATE INDEX IF NOT EXISTS inventory_reservation_allocations_partial_idx
    ON inventory_reservation_allocations(reservation_id, inventory_item_id, status, id)
    WHERE committed_quantity + released_quantity < quantity;

COMMENT ON COLUMN inventory_reservation_allocations.committed_quantity IS
    'Authoritative quantity consumed by a partial inventory finalisation.';
COMMENT ON COLUMN inventory_reservation_allocations.released_quantity IS
    'Authoritative quantity returned to available stock by a partial finalisation.';
