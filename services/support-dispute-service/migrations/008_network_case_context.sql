-- Section 15: structured operational context, point-in-time snapshots, and
-- action requests routed to the owning service.
--
-- NOTE ON RE-RUNNING: this service's runner applies every file in migrations/
-- on every start, with no tracker. Every statement below is therefore written
-- to be a no-op the second time -- IF NOT EXISTS, DO-block guards, and
-- CREATE OR REPLACE for functions. The matching down script lives in
-- migrations/down/, a directory the runner deliberately skips; placing it here
-- would drop the schema on every boot.
--
-- What is NOT here matters as much as what is. None of these columns holds a
-- trip, mission or booking STATE. Support records references to things other
-- services own, and snapshots of what it saw; it never keeps a second copy of
-- operational truth that could disagree with the first.

-- ---------------------------------------------------------------------------
-- Context references on complaints
-- ---------------------------------------------------------------------------
-- All nullable, all additive. A complaint raised before this existed carries
-- none of them and means exactly what it meant before.
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS context_type text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS order_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS fulfilment_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS mission_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS provider_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS asset_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS guarantee_breach_ref text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS recovery_case_ref text;

DO $$ BEGIN
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaints_context_type_chk') THEN
    ALTER TABLE complaints ADD CONSTRAINT complaints_context_type_chk CHECK (
        context_type IS NULL OR context_type IN (
            'TAXI_BOOKING', 'TAXI_GOODS_MISSION', 'DISPATCH_MISSION', 'HAULAGE_BOOKING',
            'VENDOR_ORDER', 'FLEET_PRIVATE_ORDER'));
END IF;
END $$;

-- Looking up every case about one mission is the question an agent asks when
-- a second complaint arrives about the same job.
CREATE INDEX IF NOT EXISTS idx_complaints_mission_ref
    ON complaints (mission_ref) WHERE mission_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_complaints_recovery_case_ref
    ON complaints (recovery_case_ref) WHERE recovery_case_ref IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Context snapshots: what Support observed, and when
-- ---------------------------------------------------------------------------
-- Append-only. A snapshot is evidence of what an agent was shown at a moment,
-- so it must never be edited to match what is true later -- that would erase
-- the very thing a dispute about "what did support know?" turns on.
CREATE TABLE IF NOT EXISTS complaint_context_snapshots (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    complaint_id    uuid NOT NULL REFERENCES complaints(id) ON DELETE RESTRICT,
    source_service  text NOT NULL CHECK (source_service <> ''),
    source_ref      text NOT NULL CHECK (source_ref <> ''),
    source_version  bigint NOT NULL CHECK (source_version >= 0),
    minimized_json  jsonb NOT NULL CHECK (jsonb_typeof(minimized_json) = 'object'),
    observed_at     timestamptz NOT NULL DEFAULT now(),
    correlation_id  text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_complaint_context_snapshots_complaint
    ON complaint_context_snapshots (complaint_id, observed_at DESC);

CREATE OR REPLACE FUNCTION complaint_context_snapshots_append_only() RETURNS trigger
LANGUAGE plpgsql AS $append_only$
BEGIN
    RAISE EXCEPTION 'complaint context snapshots are append-only'
        USING ERRCODE = 'P0001';
END $append_only$;

DROP TRIGGER IF EXISTS complaint_context_snapshots_no_mutation ON complaint_context_snapshots;
CREATE TRIGGER complaint_context_snapshots_no_mutation
    BEFORE UPDATE OR DELETE ON complaint_context_snapshots
    FOR EACH ROW EXECUTE FUNCTION complaint_context_snapshots_append_only();

-- ---------------------------------------------------------------------------
-- Action requests: Support ASKS, the owner DECIDES
-- ---------------------------------------------------------------------------
-- An action request is a durable record that Support asked a canonical owner
-- to act. It is never the action itself. The status tracks the conversation
-- with the owner, not the state of the thing being acted on.
CREATE TABLE IF NOT EXISTS support_action_requests (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    complaint_id         uuid NOT NULL REFERENCES complaints(id) ON DELETE RESTRICT,
    target_service       text NOT NULL CHECK (target_service <> ''),
    action_code          text NOT NULL CHECK (action_code <> ''),
    target_ref           text NOT NULL DEFAULT '',
    -- The owner-facing inputs, e.g. a recovery trigger code. Persisted because
    -- a delivery retried after an ambiguous outcome must send exactly what the
    -- first attempt sent, and they are covered by request_fingerprint.
    parameters           jsonb NOT NULL DEFAULT '{}'::jsonb
                         CHECK (jsonb_typeof(parameters) = 'object'),
    request_version      bigint NOT NULL DEFAULT 1 CHECK (request_version > 0),
    status               text NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN ('PENDING', 'DELIVERED', 'ACCEPTED', 'REFUSED',
                                           'UNKNOWN', 'DEAD_LETTER')),
    idempotency_key      text NOT NULL CHECK (idempotency_key <> ''),
    request_fingerprint  text NOT NULL CHECK (length(request_fingerprint) = 64),
    result_ref           text NOT NULL DEFAULT '',
    result_code          text NOT NULL DEFAULT '',
    requested_by         text NOT NULL CHECK (requested_by <> ''),
    attempts             integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at      timestamptz NOT NULL DEFAULT now(),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz
);

-- The idempotency key is scoped to the complaint. Two agents on two different
-- cases may legitimately reuse a key; one agent retrying on one case may not
-- create two requests.
CREATE UNIQUE INDEX IF NOT EXISTS uq_support_action_requests_key
    ON support_action_requests (complaint_id, idempotency_key);

-- An owner's answer is final. A request that was ACCEPTED or REFUSED records
-- when; one still in flight does not claim to be finished.
DO $$ BEGIN
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_action_requests_completion_chk') THEN
    ALTER TABLE support_action_requests ADD CONSTRAINT support_action_requests_completion_chk CHECK (
        (status IN ('ACCEPTED', 'REFUSED')) = (completed_at IS NOT NULL));
END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_support_action_requests_pending
    ON support_action_requests (next_attempt_at)
    WHERE status IN ('PENDING', 'UNKNOWN');
