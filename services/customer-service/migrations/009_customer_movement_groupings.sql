-- Section 6: commerce grouping supplied by Vendor Service.
--
-- Vendor owns the commercial grouping of a customer's order: which movements
-- belong to one purchase, and in what order they were planned. It owns none of
-- their execution. This table therefore decorates the movement projection and
-- can never create a movement, change its state, or name its provider.
-- Re-runnable.
CREATE TABLE IF NOT EXISTS customer_movement_groupings (
    customer_id          UUID NOT NULL,
    fulfilment_group_ref TEXT NOT NULL CHECK (length(fulfilment_group_ref) BETWEEN 1 AND 128),
    parent_context_type  TEXT NOT NULL DEFAULT 'COMMERCE_ORDER',
    parent_context_ref   TEXT NOT NULL CHECK (length(parent_context_ref) BETWEEN 1 AND 128),
    fulfilment_plan_ref  TEXT NOT NULL DEFAULT '',
    mission_sequence     INT CHECK (mission_sequence IS NULL OR mission_sequence >= 1),
    mission_count        INT CHECK (mission_count IS NULL OR mission_count >= 1),
    source_version       BIGINT NOT NULL CHECK (source_version >= 1),
    last_event_id        TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (fulfilment_group_ref)
);

CREATE INDEX IF NOT EXISTS customer_movement_groupings_parent_idx
    ON customer_movement_groupings(customer_id, parent_context_ref);
