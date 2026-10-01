-- Versioned pricing policy, approvals, activations, market freezes and rollouts.
--
-- The central rule: a policy version that has left DRAFT can never be edited.
-- A commercial change creates a NEW version; a rollback ACTIVATES an older
-- approved version. Nothing rewrites history, because a quote issued last month
-- must still reconcile against the exact body that priced it.
--
-- Bodies are JSONB with a (kind, schema_version) pair, decoded by a typed Go
-- struct that rejects unknown fields. They are not a general rule language:
-- typed bodies keep approvals understandable and tests deterministic, and an
-- administrator can see what a change will do.

-- ---------------------------------------------------------------------------
-- Policy lineage and versions
-- ---------------------------------------------------------------------------

-- A lineage is "this kind of rule book, for this place". Its versions are the
-- successive editions.
CREATE TABLE IF NOT EXISTS pricing_policies (
    id              text PRIMARY KEY,
    kind            text NOT NULL CHECK (kind <> ''),
    geography_level text NOT NULL CHECK (geography_level IN ('COUNTRY', 'STATE', 'MARKET', 'ZONE')),
    -- The country code, state id, market id or zone id, depending on the level.
    geography_ref   text NOT NULL CHECK (geography_ref <> ''),
    created_by      text NOT NULL CHECK (created_by <> ''),
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- One lineage per kind per place. Two lineages for the same kind and place
    -- would make resolution ambiguous, and ambiguity must be impossible rather
    -- than resolved by some tie-break nobody remembers.
    UNIQUE (kind, geography_level, geography_ref)
);

CREATE TABLE IF NOT EXISTS pricing_policy_versions (
    id                text PRIMARY KEY,
    policy_id         text NOT NULL REFERENCES pricing_policies (id),
    version_no        int NOT NULL CHECK (version_no > 0),
    kind              text NOT NULL CHECK (kind <> ''),
    schema_version    int NOT NULL CHECK (schema_version > 0),
    status            text NOT NULL DEFAULT 'DRAFT'
                          CHECK (status IN ('DRAFT', 'SUBMITTED', 'APPROVED', 'REJECTED', 'SUPERSEDED')),
    body              jsonb NOT NULL,
    -- Hash of the canonical encoding of body. An approval binds to this value,
    -- so editing a draft after approval invalidates that approval.
    body_hash         text NOT NULL CHECK (body_hash ~ '^[0-9a-f]{64}$'),
    cloned_from       text REFERENCES pricing_policy_versions (id),
    created_by        text NOT NULL CHECK (created_by <> ''),
    change_reason     text NOT NULL CHECK (change_reason <> ''),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (policy_id, version_no)
);

CREATE INDEX IF NOT EXISTS pricing_policy_versions_policy_idx
    ON pricing_policy_versions (policy_id, version_no DESC);
CREATE INDEX IF NOT EXISTS pricing_policy_versions_status_idx
    ON pricing_policy_versions (status);

-- pricing_guard_policy_version_mutation permits editing only while a version is
-- still a DRAFT, and permits only legal status transitions.
--
-- This is the single most important trigger in the service. Once a version has
-- been submitted for approval, its numbers are fixed; once it has priced a
-- quote, changing them would silently restate history.
CREATE OR REPLACE FUNCTION pricing_guard_policy_version_mutation() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status = 'DRAFT' THEN
            RETURN OLD; -- an unsubmitted draft may be discarded
        END IF;
        RAISE EXCEPTION
            'DELETE on pricing_policy_versions is refused: version % is %, not a draft',
            OLD.id, OLD.status
            USING ERRCODE = 'restrict_violation';
    END IF;

    -- The commercial content is frozen the moment the version leaves DRAFT.
    IF OLD.status <> 'DRAFT' THEN
        IF NEW.body::text <> OLD.body::text
            OR NEW.body_hash <> OLD.body_hash
            OR NEW.schema_version <> OLD.schema_version
            OR NEW.kind <> OLD.kind
            OR NEW.policy_id <> OLD.policy_id
            OR NEW.version_no <> OLD.version_no THEN
            RAISE EXCEPTION
                'the content of pricing policy version % cannot change once it is % (create a new version instead)',
                OLD.id, OLD.status
                USING ERRCODE = 'restrict_violation';
        END IF;
    END IF;

    -- Legal status transitions only.
    IF NEW.status <> OLD.status THEN
        IF NOT (
            (OLD.status = 'DRAFT'     AND NEW.status IN ('SUBMITTED', 'REJECTED')) OR
            (OLD.status = 'SUBMITTED' AND NEW.status IN ('APPROVED', 'REJECTED', 'DRAFT')) OR
            (OLD.status = 'APPROVED'  AND NEW.status = 'SUPERSEDED')
        ) THEN
            RAISE EXCEPTION 'illegal pricing policy version transition % -> %', OLD.status, NEW.status
                USING ERRCODE = 'restrict_violation';
        END IF;
    END IF;

    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION pricing_guard_policy_version_mutation() IS
    'Freezes a policy version''s commercial content once it leaves DRAFT and '
    'permits only legal status transitions. See migration 000003.';

DROP TRIGGER IF EXISTS pricing_policy_versions_guard ON pricing_policy_versions;
CREATE TRIGGER pricing_policy_versions_guard
    BEFORE UPDATE OR DELETE ON pricing_policy_versions
    FOR EACH ROW EXECUTE FUNCTION pricing_guard_policy_version_mutation();

-- The lifecycle history of every version: who submitted, who approved, when it
-- activated. Append-only, and separate from the audit log because this is the
-- version's own story rather than a record of an operator's action.
CREATE TABLE IF NOT EXISTS pricing_policy_version_events (
    id           bigserial PRIMARY KEY,
    version_id   text NOT NULL REFERENCES pricing_policy_versions (id),
    event        text NOT NULL CHECK (event <> ''),
    from_status  text,
    to_status    text,
    performed_by text NOT NULL CHECK (performed_by <> ''),
    reason       text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pricing_policy_version_events_version_idx
    ON pricing_policy_version_events (version_id, created_at);

DROP TRIGGER IF EXISTS pricing_policy_version_events_append_only ON pricing_policy_version_events;
CREATE TRIGGER pricing_policy_version_events_append_only
    BEFORE UPDATE OR DELETE ON pricing_policy_version_events
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- ---------------------------------------------------------------------------
-- Four-eye approval
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS pricing_approval_requests (
    id           text PRIMARY KEY,
    subject_type text NOT NULL CHECK (subject_type IN
                     ('POLICY_VERSION', 'ROLLBACK', 'ROLLOUT', 'MARKET_UNFREEZE', 'INDEX_BATCH', 'INDEX_OVERRIDE')),
    subject_id   text NOT NULL CHECK (subject_id <> ''),
    -- The content the approver is agreeing to. If the subject changes, the
    -- approval no longer applies: an approval is for a specific body, not for a
    -- name.
    subject_hash text NOT NULL CHECK (subject_hash ~ '^[0-9a-f]{64}$'),
    requested_by text NOT NULL CHECK (requested_by <> ''),
    reason       text NOT NULL CHECK (reason <> ''),
    status       text NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    decided_at   timestamptz,
    CONSTRAINT pricing_approval_decided_has_time
        CHECK ((status IN ('APPROVED', 'REJECTED')) = (decided_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS pricing_approval_requests_pending_idx
    ON pricing_approval_requests (status, created_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS pricing_approval_requests_subject_idx
    ON pricing_approval_requests (subject_type, subject_id);

CREATE TABLE IF NOT EXISTS pricing_approval_decisions (
    id         text PRIMARY KEY,
    -- One decision per request. A second approver cannot overwrite the first.
    request_id text NOT NULL UNIQUE REFERENCES pricing_approval_requests (id),
    decided_by text NOT NULL CHECK (decided_by <> ''),
    decision   text NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    reason     text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Separation of duties, enforced by the database.
--
-- The usecase layer checks this too, with a friendlier message. It is enforced
-- here as well because this is the control that stops one person changing prices
-- for an entire market, and a control that lives only in application code is one
-- refactor away from being absent.
CREATE OR REPLACE FUNCTION pricing_reject_self_approval() RETURNS trigger AS $$
DECLARE
    requester text;
BEGIN
    SELECT requested_by INTO requester
    FROM pricing_approval_requests
    WHERE id = NEW.request_id;

    IF requester IS NULL THEN
        RAISE EXCEPTION 'approval request % does not exist', NEW.request_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF requester = NEW.decided_by THEN
        RAISE EXCEPTION
            'self-approval is refused: % requested this change and cannot also decide it',
            NEW.decided_by
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION pricing_reject_self_approval() IS
    'Four-eye guarantee: the requester of a change may not decide it. See migration 000003.';

DROP TRIGGER IF EXISTS pricing_approval_decisions_no_self_approval ON pricing_approval_decisions;
CREATE TRIGGER pricing_approval_decisions_no_self_approval
    BEFORE INSERT ON pricing_approval_decisions
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_self_approval();

DROP TRIGGER IF EXISTS pricing_approval_decisions_append_only ON pricing_approval_decisions;
CREATE TRIGGER pricing_approval_decisions_append_only
    BEFORE UPDATE OR DELETE ON pricing_approval_decisions
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- ---------------------------------------------------------------------------
-- Activations
-- ---------------------------------------------------------------------------

-- Which version is live, from when. Append-only: a rollback is a NEW activation
-- pointing at an older approved version, so the timeline always reads forward
-- and "what was active last Tuesday" is answerable exactly.
CREATE TABLE IF NOT EXISTS pricing_policy_activations (
    id                     text PRIMARY KEY,
    policy_id              text NOT NULL REFERENCES pricing_policies (id),
    version_id             text NOT NULL REFERENCES pricing_policy_versions (id),
    effective_from         timestamptz NOT NULL,
    action                 text NOT NULL DEFAULT 'ACTIVATE' CHECK (action IN ('ACTIVATE', 'ROLLBACK')),
    activated_by           text NOT NULL CHECK (activated_by <> ''),
    approval_request_id    text REFERENCES pricing_approval_requests (id),
    supersedes_activation  text REFERENCES pricing_policy_activations (id),
    reason                 text NOT NULL CHECK (reason <> ''),
    created_at             timestamptz NOT NULL DEFAULT now()
);

-- Resolution reads "the latest activation for this policy whose effective_from
-- is at or before the quote's as-of time", so this index serves the hot path.
CREATE INDEX IF NOT EXISTS pricing_policy_activations_resolve_idx
    ON pricing_policy_activations (policy_id, effective_from DESC);

DROP TRIGGER IF EXISTS pricing_policy_activations_append_only ON pricing_policy_activations;
CREATE TRIGGER pricing_policy_activations_append_only
    BEFORE UPDATE OR DELETE ON pricing_policy_activations
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- ---------------------------------------------------------------------------
-- Market freezes and rollouts
-- ---------------------------------------------------------------------------

-- Freezing is protective and needs one operator; UNfreezing needs two, because
-- it is the act that lets prices move again.
CREATE TABLE IF NOT EXISTS pricing_market_freezes (
    id                  text PRIMARY KEY,
    market_code         text NOT NULL CHECK (market_code <> ''),
    action              text NOT NULL CHECK (action IN ('FREEZE', 'UNFREEZE')),
    performed_by        text NOT NULL CHECK (performed_by <> ''),
    reason              text NOT NULL CHECK (reason <> ''),
    approval_request_id text REFERENCES pricing_approval_requests (id),
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pricing_market_freezes_market_idx
    ON pricing_market_freezes (market_code, created_at DESC);

DROP TRIGGER IF EXISTS pricing_market_freezes_append_only ON pricing_market_freezes;
CREATE TRIGGER pricing_market_freezes_append_only
    BEFORE UPDATE OR DELETE ON pricing_market_freezes
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- The pricing-side half of the two-key switch. A consuming service also has its
-- own flag; pricing refuses to issue a bindable quote for a market that is not
-- CENTRAL here. That way one mistaken deployment setting cannot quietly make
-- this service authoritative for real customer prices.
CREATE TABLE IF NOT EXISTS pricing_rollouts (
    id                  text PRIMARY KEY,
    scope               text NOT NULL CHECK (scope IN ('TAXI', 'DISPATCH', 'HAULAGE')),
    capability          text NOT NULL CHECK (capability IN ('QUOTE', 'BIND', 'RECONCILE')),
    market_code         text NOT NULL CHECK (market_code <> ''),
    mode                text NOT NULL CHECK (mode IN ('LEGACY', 'SHADOW', 'CENTRAL')),
    performed_by        text NOT NULL CHECK (performed_by <> ''),
    reason              text NOT NULL CHECK (reason <> ''),
    approval_request_id text REFERENCES pricing_approval_requests (id),
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pricing_rollouts_lookup_idx
    ON pricing_rollouts (scope, capability, market_code, created_at DESC);

DROP TRIGGER IF EXISTS pricing_rollouts_append_only ON pricing_rollouts;
CREATE TRIGGER pricing_rollouts_append_only
    BEFORE UPDATE OR DELETE ON pricing_rollouts
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();
