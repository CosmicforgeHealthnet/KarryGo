-- Section 14G — Food / Ingredient Inventory Seam & Availability Integration.
--
-- This migration does NOT introduce:
--   recipes
--   ingredients
--   recipe composition
--   yields
--   portions
--   substitutions
--   production batches
--   packaging requirements
--
-- Those authorities remain Section 16 / Section 20.
--
-- This table exists only to remember the last emitted inventory availability
-- state so projection invalidation events are bounded and transitions are not
-- emitted repeatedly without a state change.

CREATE TABLE inventory_availability_signal_state (
    business_id text NOT NULL,
    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    is_available boolean NOT NULL,

    available_quantity bigint NOT NULL
        CHECK (available_quantity >= 0),

    inventory_revision bigint NOT NULL
        CHECK (inventory_revision >= 0),

    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (
        business_id,
        branch_id,
        inventory_item_id
    ),

    CONSTRAINT inventory_availability_signal_state_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        )
        ON DELETE CASCADE
);

CREATE INDEX inventory_availability_signal_state_branch_idx
ON inventory_availability_signal_state (
    business_id,
    branch_id,
    updated_at DESC
);