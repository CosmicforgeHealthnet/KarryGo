-- Sections 10/11 closure durability: worker leases, occurrence linkage and
-- immutable change-request decisions.  These are additive so existing dirty
-- environments can replay the complete migration chain safely.
ALTER TABLE cloud_kitchen_occurrences
    ADD COLUMN IF NOT EXISTS lease_owner text,
    ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    ADD COLUMN IF NOT EXISTS replay_count integer NOT NULL DEFAULT 0 CHECK (replay_count >= 0),
    ADD COLUMN IF NOT EXISTS dead_lettered_at timestamptz,
    ADD COLUMN IF NOT EXISTS last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 280),
    ADD COLUMN IF NOT EXISTS production_job_id text;
CREATE INDEX IF NOT EXISTS cloud_kitchen_occurrences_lease_idx
    ON cloud_kitchen_occurrences (state, lease_expires_at);

ALTER TABLE pastry_change_requests
    ADD COLUMN IF NOT EXISTS decided_at timestamptz,
    ADD COLUMN IF NOT EXISTS decided_by text,
    ADD COLUMN IF NOT EXISTS decision_note text NOT NULL DEFAULT '' CHECK (char_length(decision_note) <= 280);
CREATE TABLE IF NOT EXISTS pastry_change_request_history (
    id text PRIMARY KEY,
    change_request_id text NOT NULL REFERENCES pastry_change_requests(id) ON DELETE CASCADE,
    from_status text,
    to_status text NOT NULL,
    actor_id text,
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280),
    occurred_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE production_jobs
    ADD COLUMN IF NOT EXISTS risk_flags jsonb NOT NULL DEFAULT '[]'::jsonb;
