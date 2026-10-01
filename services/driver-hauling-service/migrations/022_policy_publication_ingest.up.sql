-- Section 8/9 gap closure: the ingestion path for published network policy.
--
-- Migration 021 gave Hauling somewhere to KEEP a policy snapshot and a reader
-- that matching uses. What it did not give it was a way to RECEIVE one, so the
-- snapshot table has only ever held the built-in default. This adds the inbox
-- that makes a published catalogue land here safely.
--
-- Four columns carry the whole safety argument, and each exists because the
-- obvious implementation gets it wrong:
--
--   event_id       delivery is at-least-once, and applying a publication twice
--                  would bump a version that committed work is pinned to.
--   source_version a delayed publication that overtakes a newer one would
--                  silently roll the network back to superseded rules.
--   checksum       a policy that arrives corrupted is worse than one that does
--                  not arrive: the first is applied and believed.
--   state          a read-back that timed out has NOT failed, and recording it
--                  as a failure would abandon a version the network moved to.

CREATE TABLE IF NOT EXISTS hauling_policy_publications (
    -- The publisher's event id IS the primary key. Deduplication is then a
    -- property of the schema rather than of a query someone has to remember
    -- to write, and two concurrent deliveries of the same publication
    -- resolve to exactly one winner without an advisory lock.
    event_id TEXT PRIMARY KEY,

    policy_kind TEXT NOT NULL,
    policy_version TEXT NOT NULL,

    source_service TEXT NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    schema_version INT NOT NULL CHECK (schema_version > 0),

    checksum TEXT NOT NULL,

    -- Set when the catalogue travelled as a reference rather than inline. It
    -- is only ever fetched over this service's own HMAC client, never
    -- anonymously, and only from an allowlisted host.
    document_ref TEXT NOT NULL DEFAULT '',

    state TEXT NOT NULL
        CHECK (state IN ('APPLIED', 'DUPLICATE', 'STALE', 'REJECTED', 'PENDING_READBACK')),
    reason_code TEXT NOT NULL DEFAULT '',

    correlation_id TEXT NOT NULL DEFAULT '',

    occurred_at TIMESTAMPTZ NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ,

    -- Retry bookkeeping for a read-back whose outcome was unknown.
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',

    -- A publication is applied exactly when it has an applied_at. Letting the
    -- two disagree would make "when did this build adopt that version" a
    -- question with two different answers.
    CONSTRAINT chk_hauling_publication_applied
        CHECK ((state = 'APPLIED') = (applied_at IS NOT NULL))
);

-- The reconcile worker's only query.
CREATE INDEX IF NOT EXISTS idx_hauling_policy_publications_pending
    ON hauling_policy_publications (next_attempt_at)
    WHERE state = 'PENDING_READBACK';

-- The source-version regression guard's only query.
CREATE INDEX IF NOT EXISTS idx_hauling_policy_publications_applied
    ON hauling_policy_publications (policy_kind, source_version DESC)
    WHERE state = 'APPLIED';

COMMENT ON TABLE hauling_policy_publications IS
'Inbox for network.catalog.published. event_id is the primary key so deduplication is a schema property, not a query someone must remember to write.';
COMMENT ON COLUMN hauling_policy_publications.state IS
'PENDING_READBACK is UNKNOWN, not failed: the control plane may have served the document and the response been lost, so it is retried rather than abandoned.';

-- The snapshot table gains the provenance 021 had no way to record, because
-- nothing published into it yet.
ALTER TABLE hauling_policy_snapshots
    ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS event_id TEXT NOT NULL DEFAULT '';

-- Snapshots remain immutable. This trigger is the enforcement: an UPDATE that
-- changes the document, its checksum or its version is refused outright,
-- because committed bookings pin a policy version and rewriting one in place
-- would rewrite the rules a past decision was made under -- invisibly, and
-- after the fact.
CREATE OR REPLACE FUNCTION hauling_policy_snapshots_immutable()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.payload IS DISTINCT FROM OLD.payload
       OR NEW.checksum IS DISTINCT FROM OLD.checksum
       OR NEW.policy_version IS DISTINCT FROM OLD.policy_version
       OR NEW.policy_kind IS DISTINCT FROM OLD.policy_kind
       OR NEW.source_version IS DISTINCT FROM OLD.source_version THEN
        RAISE EXCEPTION 'hauling_policy_snapshots rows are immutable (kind=% version=%)',
            OLD.policy_kind, OLD.policy_version;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_hauling_policy_snapshots_immutable ON hauling_policy_snapshots;
CREATE TRIGGER trg_hauling_policy_snapshots_immutable
    BEFORE UPDATE ON hauling_policy_snapshots
    FOR EACH ROW EXECUTE FUNCTION hauling_policy_snapshots_immutable();
