-- Section 6: unified, read-only customer movement history projection.
-- This is never canonical execution state. Taxi, Dispatch and Hauling own
-- their bookings/missions and publish summary snapshots. Re-runnable.

CREATE TABLE IF NOT EXISTS customer_movement_history_projection (
    customer_id                UUID NOT NULL,
    authority                  TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
    source_ref                 TEXT NOT NULL CHECK (length(source_ref) BETWEEN 1 AND 128),
    source_version             BIGINT NOT NULL CHECK (source_version >= 1),
    product_code               TEXT,
    movement_type              TEXT NOT NULL CHECK (movement_type IN ('PASSENGER','GOODS','DISPATCH','HAULAGE')),
    state                      TEXT NOT NULL,
    origin_summary_json        JSONB,
    destination_summary_json   JSONB,
    scheduled_at               TIMESTAMPTZ,
    started_at                 TIMESTAMPTZ,
    completed_at               TIMESTAMPTZ,
    provider_replacement_count INT NOT NULL DEFAULT 0 CHECK (provider_replacement_count >= 0),
    recovery_state             TEXT,
    parent_context_type        TEXT,
    parent_context_ref         TEXT,
    fulfilment_plan_ref        TEXT,
    fulfilment_group_ref       TEXT,
    mission_sequence           INT CHECK (mission_sequence IS NULL OR mission_sequence >= 1),
    mission_count              INT CHECK (mission_count IS NULL OR mission_count >= 1),
    last_event_id              TEXT NOT NULL,
    last_event_at              TIMESTAMPTZ NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (authority, source_ref)
);

CREATE INDEX IF NOT EXISTS customer_movement_history_customer_idx
    ON customer_movement_history_projection(customer_id, last_event_at DESC, authority, source_ref);
CREATE INDEX IF NOT EXISTS customer_movement_history_parent_idx
    ON customer_movement_history_projection(customer_id, parent_context_ref) WHERE parent_context_ref IS NOT NULL;

-- Inbox: every accepted event id is recorded exactly once, with the outcome,
-- so replays are acknowledged without reapplying.
CREATE TABLE IF NOT EXISTS customer_movement_event_inbox (
    event_id        TEXT PRIMARY KEY,
    source_service  TEXT NOT NULL,
    event_name      TEXT NOT NULL,
    authority       TEXT NOT NULL,
    source_ref      TEXT NOT NULL,
    source_version  BIGINT NOT NULL,
    outcome         TEXT NOT NULL CHECK (outcome IN ('APPLIED','STALE','TERMINAL_PROTECTED')),
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Well-formed but semantically rejected events are retained for operator
-- reconciliation against the canonical authority instead of being dropped.
CREATE TABLE IF NOT EXISTS customer_movement_event_rejections (
    event_id        TEXT PRIMARY KEY,
    source_service  TEXT NOT NULL,
    event_name      TEXT NOT NULL,
    reason_code     TEXT NOT NULL,
    envelope        JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
