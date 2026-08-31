-- Section 19 blocker closure: idempotent, auditable replacement allocation.
-- All stock arithmetic remains inside the generic Inventory authority.

CREATE TABLE inventory_reservation_replacements (
    id TEXT PRIMARY KEY,
    reservation_id TEXT NOT NULL,
    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL CHECK (char_length(request_fingerprint) = 64),
    original_item_id TEXT NOT NULL,
    replacement_item_id TEXT NOT NULL,
    original_found_quantity BIGINT NOT NULL CHECK (original_found_quantity >= 0),
    superseded_quantity BIGINT NOT NULL CHECK (superseded_quantity > 0),
    replacement_quantity BIGINT NOT NULL CHECK (replacement_quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (reservation_id, idempotency_key),
    CONSTRAINT inventory_replacement_reservation_fk
        FOREIGN KEY (reservation_id, business_id, branch_id)
        REFERENCES inventory_reservations(id, business_id, branch_id)
        ON DELETE RESTRICT,
    CONSTRAINT inventory_replacement_items_differ_ck
        CHECK (original_item_id <> replacement_item_id),
    CONSTRAINT inventory_replacement_quantity_match_ck
        CHECK (superseded_quantity = replacement_quantity)
);

CREATE INDEX inventory_reservation_replacements_lookup_idx
    ON inventory_reservation_replacements(business_id, branch_id, reservation_id, created_at, id);
