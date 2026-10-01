-- Section 11: generalized fulfilment readiness and just-in-time orchestration.
--
-- Two problems, one cause. Vendor could record that a production job promised a
-- window (commerce_vendor_order_readiness, migration 000091) but could not say
-- what state the order was actually IN, how confident that estimate was, or who
-- last said so. And fulfilment planning was synchronous: a mission was created
-- when the order was placed, which is how a rider ends up waiting twenty
-- minutes outside a kitchen that had not started cooking.
--
-- The readiness table is EXTENDED rather than replaced. A second readiness
-- table would fork the truth the production timing projection already writes
-- into this one, and the two would disagree the first time one of them was
-- updated alone.

-- ---------------------------------------------------------------------------
-- Readiness state
-- ---------------------------------------------------------------------------
ALTER TABLE commerce_vendor_order_readiness
    -- The vendor category this order is being prepared under. A branch may hold
    -- several capabilities, so the type is recorded per order rather than
    -- looked up: a grocery order and a bakery order from one business move
    -- through different substates.
    ADD COLUMN IF NOT EXISTS vendor_type TEXT NOT NULL DEFAULT '',

    ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'NOT_STARTED',
    ADD COLUMN IF NOT EXISTS substate TEXT NOT NULL DEFAULT '',

    -- readiness_version is the optimistic-concurrency guard AND the ordering a
    -- consumer uses. It is separate from source_version, which tracks the
    -- production job's own revisions.
    ADD COLUMN IF NOT EXISTS readiness_version BIGINT NOT NULL DEFAULT 1,

    ADD COLUMN IF NOT EXISTS prep_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS predicted_ready_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS predicted_ready_end TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS vendor_estimated_ready_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS actual_ready_at TIMESTAMPTZ,

    -- first_actual_ready_at is immutable once set, enforced by a trigger below.
    -- A vendor who declares READY and is then corrected has still declared it,
    -- and a dispute about a rider who arrived to nothing needs that record to
    -- survive the correction.
    ADD COLUMN IF NOT EXISTS first_actual_ready_at TIMESTAMPTZ,

    ADD COLUMN IF NOT EXISTS confidence_score NUMERIC(4,3) NOT NULL DEFAULT 0.500,
    ADD COLUMN IF NOT EXISTS confidence_band TEXT NOT NULL DEFAULT 'LOW',

    -- Who last moved this. SYSTEM means an adapter applied a Vendor-owned
    -- production or picking event; it is never a customer and never a provider.
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'SYSTEM',
    ADD COLUMN IF NOT EXISTS block_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delay_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_signal_at TIMESTAMPTZ;

ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_state_check;
ALTER TABLE commerce_vendor_order_readiness
    ADD CONSTRAINT commerce_vendor_order_readiness_state_check CHECK (state IN (
        'NOT_STARTED', 'ACCEPTED', 'PREPARING', 'READY_SOON', 'READY',
        'BLOCKED', 'DELAYED', 'CANCELLED'));

ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_confidence_check;
ALTER TABLE commerce_vendor_order_readiness
    ADD CONSTRAINT commerce_vendor_order_readiness_confidence_check
        CHECK (confidence_score >= 0 AND confidence_score <= 1
               AND confidence_band IN ('LOW', 'MEDIUM', 'HIGH'));

ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_source_check;
ALTER TABLE commerce_vendor_order_readiness
    ADD CONSTRAINT commerce_vendor_order_readiness_source_check
        CHECK (source IN ('VENDOR_STAFF', 'SYSTEM', 'ADMIN'));

ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_version_check;
ALTER TABLE commerce_vendor_order_readiness
    ADD CONSTRAINT commerce_vendor_order_readiness_version_check
        CHECK (readiness_version > 0);

-- A predicted window must be a window.
ALTER TABLE commerce_vendor_order_readiness
    DROP CONSTRAINT IF EXISTS commerce_vendor_order_readiness_window_check;
ALTER TABLE commerce_vendor_order_readiness
    ADD CONSTRAINT commerce_vendor_order_readiness_window_check
        CHECK (predicted_ready_end IS NULL OR predicted_ready_start IS NULL
               OR predicted_ready_end >= predicted_ready_start);

CREATE INDEX IF NOT EXISTS commerce_vendor_order_readiness_state_idx
    ON commerce_vendor_order_readiness (state, predicted_ready_start)
    WHERE state IN ('ACCEPTED', 'PREPARING', 'READY_SOON', 'DELAYED');

-- Stale-signal detection reads this: orders that are moving but have not been
-- heard from. Partial so it stays small as completed orders accumulate.
CREATE INDEX IF NOT EXISTS commerce_vendor_order_readiness_signal_idx
    ON commerce_vendor_order_readiness (last_signal_at)
    WHERE state IN ('ACCEPTED', 'PREPARING', 'READY_SOON');

-- first_actual_ready_at is write-once. Enforced here rather than in code
-- because the value's whole purpose is to survive a later correction, and a
-- correction is exactly the code path most likely to overwrite it by accident.
CREATE OR REPLACE FUNCTION protect_first_actual_ready_at() RETURNS trigger
LANGUAGE plpgsql AS $guard$
BEGIN
    IF OLD.first_actual_ready_at IS NOT NULL
       AND NEW.first_actual_ready_at IS DISTINCT FROM OLD.first_actual_ready_at THEN
        RAISE EXCEPTION 'first_actual_ready_at is immutable once recorded';
    END IF;
    RETURN NEW;
END $guard$;
DROP TRIGGER IF EXISTS commerce_vendor_order_readiness_first_ready_guard
    ON commerce_vendor_order_readiness;
CREATE TRIGGER commerce_vendor_order_readiness_first_ready_guard
    BEFORE UPDATE ON commerce_vendor_order_readiness
    FOR EACH ROW EXECUTE FUNCTION protect_first_actual_ready_at();

-- ---------------------------------------------------------------------------
-- Readiness history (append-only)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS vendor_readiness_history (
    id                TEXT PRIMARY KEY,
    vendor_order_id   TEXT NOT NULL
                        REFERENCES commerce_vendor_order_readiness(vendor_order_id) ON DELETE CASCADE,
    from_state        TEXT NOT NULL,
    to_state          TEXT NOT NULL,
    from_substate     TEXT NOT NULL DEFAULT '',
    to_substate       TEXT NOT NULL DEFAULT '',
    source            TEXT NOT NULL CHECK (source IN ('VENDOR_STAFF', 'SYSTEM', 'ADMIN')),
    estimate_before   TIMESTAMPTZ,
    estimate_after    TIMESTAMPTZ,
    actor_ref         TEXT NOT NULL DEFAULT '',
    -- origin_event_ref is the Vendor-owned production or picking event a SYSTEM
    -- transition came from. Without it a system transition is unattributable,
    -- which is the same as unauditable.
    origin_event_ref  TEXT NOT NULL DEFAULT '',
    reason_code       TEXT NOT NULL DEFAULT '',
    readiness_version BIGINT NOT NULL CHECK (readiness_version > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vendor_order_id, readiness_version)
);
CREATE INDEX IF NOT EXISTS vendor_readiness_history_order_idx
    ON vendor_readiness_history (vendor_order_id, created_at DESC);

CREATE OR REPLACE FUNCTION reject_readiness_history_mutation() RETURNS trigger
LANGUAGE plpgsql AS $guard$
BEGIN
    RAISE EXCEPTION 'readiness history is append-only';
END $guard$;
DROP TRIGGER IF EXISTS vendor_readiness_history_append_only ON vendor_readiness_history;
CREATE TRIGGER vendor_readiness_history_append_only
    BEFORE UPDATE OR DELETE ON vendor_readiness_history
    FOR EACH ROW EXECUTE FUNCTION reject_readiness_history_mutation();

-- ---------------------------------------------------------------------------
-- Branch readiness profiles
-- ---------------------------------------------------------------------------
-- What this branch has historically taken. Analytics may supply it; Vendor
-- remains the readiness decision authority, and an absent or stale profile only
-- costs prediction accuracy rather than blocking readiness entirely.
CREATE TABLE IF NOT EXISTS vendor_branch_readiness_profiles (
    business_id          TEXT NOT NULL,
    branch_id            TEXT NOT NULL,
    vendor_type          TEXT NOT NULL,
    profile_version      BIGINT NOT NULL DEFAULT 1 CHECK (profile_version > 0),
    baseline_p50_seconds INTEGER NOT NULL CHECK (baseline_p50_seconds >= 0),
    baseline_p80_seconds INTEGER NOT NULL CHECK (baseline_p80_seconds >= 0),
    baseline_p90_seconds INTEGER NOT NULL CHECK (baseline_p90_seconds >= 0),
    handover_p50_seconds INTEGER NOT NULL DEFAULT 0 CHECK (handover_p50_seconds >= 0),
    handover_p90_seconds INTEGER NOT NULL DEFAULT 0 CHECK (handover_p90_seconds >= 0),
    confidence           NUMERIC(4,3) NOT NULL DEFAULT 0.500
                           CHECK (confidence >= 0 AND confidence <= 1),
    sample_count         INTEGER NOT NULL DEFAULT 0 CHECK (sample_count >= 0),
    data_as_of           TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, vendor_type),
    -- Percentiles that are not ordered are not percentiles.
    CHECK (baseline_p80_seconds >= baseline_p50_seconds),
    CHECK (baseline_p90_seconds >= baseline_p80_seconds),
    CHECK (handover_p90_seconds >= handover_p50_seconds)
);

-- ---------------------------------------------------------------------------
-- Readiness rules
-- ---------------------------------------------------------------------------
-- Operational policy, versioned. Scoped from most general (vendor type) to most
-- specific (one branch), so a branch can be tuned without forking the type's
-- defaults. There is deliberately no price, fee or currency column: readiness
-- is about time.
CREATE TABLE IF NOT EXISTS vendor_readiness_rules (
    id                          TEXT PRIMARY KEY,
    vendor_type                 TEXT NOT NULL,
    business_id                 TEXT NOT NULL DEFAULT '',
    branch_id                   TEXT NOT NULL DEFAULT '',
    default_prep_seconds        INTEGER NOT NULL CHECK (default_prep_seconds > 0),
    ready_soon_threshold_seconds INTEGER NOT NULL CHECK (ready_soon_threshold_seconds >= 0),
    stale_signal_seconds        INTEGER NOT NULL CHECK (stale_signal_seconds > 0),
    -- Bounds on what a vendor may claim. A vendor estimate of four seconds or
    -- four days is not an estimate, and accepting either would let one order
    -- drag the whole branch's planning with it.
    min_vendor_estimate_seconds INTEGER NOT NULL CHECK (min_vendor_estimate_seconds >= 0),
    max_vendor_estimate_seconds INTEGER NOT NULL CHECK (max_vendor_estimate_seconds > 0),
    version                     BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    effective_from              TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_to                TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (max_vendor_estimate_seconds >= min_vendor_estimate_seconds),
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    UNIQUE (vendor_type, business_id, branch_id, version)
);
CREATE INDEX IF NOT EXISTS vendor_readiness_rules_scope_idx
    ON vendor_readiness_rules (vendor_type, business_id, branch_id, effective_from DESC);

-- ---------------------------------------------------------------------------
-- Fulfilment orchestration
-- ---------------------------------------------------------------------------
-- The Plan -> Warm Match -> Commit aggregate. One row per fulfilment group,
-- carrying the decision, its timing and the mission it eventually produced.
CREATE TABLE IF NOT EXISTS vendor_fulfilment_orchestrations (
    id                       TEXT PRIMARY KEY,
    fulfilment_group_ref     TEXT NOT NULL UNIQUE,
    parent_order_id          TEXT NOT NULL,
    plan_version             BIGINT NOT NULL DEFAULT 1 CHECK (plan_version > 0),

    state                    TEXT NOT NULL DEFAULT 'PENDING_REQUIREMENTS'
                               CHECK (state IN (
                                 'PENDING_REQUIREMENTS', 'PLANNED', 'WAITING_READINESS',
                                 'WARM_MATCH', 'COMMIT_DUE', 'COMMITTING', 'COMMITTED',
                                 'IN_EXECUTION', 'COMPLETED',
                                 'RECOVERING', 'OPS_REVIEW', 'CANCELLED', 'FAILED')),

    -- The normalized transport requirement, frozen once the orchestration
    -- commits. A requirement that can change after commitment is a requirement
    -- the executing authority was never asked about.
    requirements_snapshot    JSONB NOT NULL DEFAULT '{}'::jsonb,
    requirements_frozen_at   TIMESTAMPTZ,

    selected_authority       TEXT NOT NULL DEFAULT '',
    selected_product_code    TEXT NOT NULL DEFAULT '',
    capability_policy_version TEXT NOT NULL DEFAULT '',
    selection_source         TEXT NOT NULL DEFAULT 'AUTOMATIC'
                               CHECK (selection_source IN ('AUTOMATIC', 'ADMIN_OVERRIDE', 'ASSISTED_OVERRIDE')),

    desired_pickup_start     TIMESTAMPTZ,
    desired_pickup_end       TIMESTAMPTZ,
    -- commit_not_before is recalculated continuously, never armed once. An
    -- order that becomes ready early must be collected early, and an order that
    -- slips must not have a rider already sent.
    commit_not_before        TIMESTAMPTZ,
    next_evaluation_at       TIMESTAMPTZ,

    current_mission_ref      TEXT NOT NULL DEFAULT '',
    recovery_state           TEXT NOT NULL DEFAULT 'NONE'
                               CHECK (recovery_state IN ('NONE', 'AUTHORITY_RECOVERING', 'REPLANNING', 'RECONCILING')),
    last_reason_code         TEXT NOT NULL DEFAULT '',

    version                  BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A committed orchestration must name what it committed to. Without this a
    -- crash mid-commit could leave a row that claims success and points at
    -- nothing, which no reconciler could resolve.
    CONSTRAINT vendor_orchestration_committed_has_mission
        CHECK (state NOT IN ('COMMITTED', 'IN_EXECUTION', 'COMPLETED')
               OR length(btrim(current_mission_ref)) > 0),
    CONSTRAINT vendor_orchestration_committed_has_authority
        CHECK (state NOT IN ('COMMITTING', 'COMMITTED', 'IN_EXECUTION', 'COMPLETED')
               OR length(btrim(selected_authority)) > 0),
    CONSTRAINT vendor_orchestration_frozen_when_committed
        CHECK (state NOT IN ('COMMITTED', 'IN_EXECUTION', 'COMPLETED')
               OR requirements_frozen_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS vendor_fulfilment_orchestrations_due_idx
    ON vendor_fulfilment_orchestrations (next_evaluation_at)
    WHERE state IN ('PLANNED', 'WAITING_READINESS', 'WARM_MATCH', 'COMMIT_DUE');
CREATE INDEX IF NOT EXISTS vendor_fulfilment_orchestrations_order_idx
    ON vendor_fulfilment_orchestrations (parent_order_id);
CREATE INDEX IF NOT EXISTS vendor_fulfilment_orchestrations_recovery_idx
    ON vendor_fulfilment_orchestrations (recovery_state)
    WHERE recovery_state <> 'NONE';

-- The frozen requirement really is frozen.
CREATE OR REPLACE FUNCTION protect_frozen_requirements() RETURNS trigger
LANGUAGE plpgsql AS $guard$
BEGIN
    IF OLD.requirements_frozen_at IS NOT NULL
       AND NEW.requirements_snapshot IS DISTINCT FROM OLD.requirements_snapshot THEN
        RAISE EXCEPTION 'the requirement snapshot is immutable once the orchestration has committed';
    END IF;
    RETURN NEW;
END $guard$;
DROP TRIGGER IF EXISTS vendor_orchestration_frozen_guard ON vendor_fulfilment_orchestrations;
CREATE TRIGGER vendor_orchestration_frozen_guard
    BEFORE UPDATE ON vendor_fulfilment_orchestrations
    FOR EACH ROW EXECUTE FUNCTION protect_frozen_requirements();

-- ---------------------------------------------------------------------------
-- Orchestration decision evidence (append-only)
-- ---------------------------------------------------------------------------
-- Why an authority was chosen, and why the others were not. This is what makes
-- "why did this go by truck?" answerable months later, and what distinguishes
-- an authority that refused from one that could not be asked.
--
-- It carries no provider identity. An orchestration decision names an authority
-- and a product; the driver, rider or vehicle is the executing service's to
-- choose, after Vendor's work is done.
CREATE TABLE IF NOT EXISTS vendor_orchestration_events (
    id                      TEXT PRIMARY KEY,
    orchestration_id        TEXT NOT NULL
                              REFERENCES vendor_fulfilment_orchestrations(id) ON DELETE CASCADE,
    plan_version            BIGINT NOT NULL CHECK (plan_version > 0),
    event_type              TEXT NOT NULL CHECK (event_type IN (
                              'REQUIREMENT_BUILT', 'SERVICEABILITY_CHECKED', 'SUPPLY_PREVIEWED',
                              'AUTHORITY_SELECTED', 'COMMIT_SCHEDULED', 'COMMIT_ATTEMPTED',
                              'MISSION_COMMITTED', 'REPLANNED', 'RECOVERY_OBSERVED', 'OVERRIDDEN')),
    requirement_snapshot    JSONB NOT NULL DEFAULT '{}'::jsonb,
    readiness_version       BIGINT NOT NULL DEFAULT 0 CHECK (readiness_version >= 0),
    candidate_authority     TEXT NOT NULL DEFAULT '',
    candidate_product_code  TEXT NOT NULL DEFAULT '',
    serviceable             BOOLEAN,
    -- outcome_class separates a physical refusal from a dependency that never
    -- answered. Collapsing them is how one outage becomes "this authority does
    -- not serve this city" for every later order.
    outcome_class           TEXT NOT NULL DEFAULT ''
                              CHECK (outcome_class IN ('', 'SERVICEABLE', 'CAPABILITY_MISMATCH',
                                                       'NOT_SERVICEABLE', 'SUPPLY_UNAVAILABLE',
                                                       'DEPENDENCY_UNAVAILABLE', 'DISABLED', 'CREDENTIAL')),
    reason_code             TEXT NOT NULL DEFAULT '',
    supply_status           TEXT NOT NULL DEFAULT ''
                              CHECK (supply_status IN ('', 'AVAILABLE', 'LIMITED', 'UNKNOWN')),
    estimated_pickup_eta_seconds BIGINT NOT NULL DEFAULT 0,
    policy_version          TEXT NOT NULL DEFAULT '',
    selected                BOOLEAN NOT NULL DEFAULT FALSE,
    actor_ref               TEXT NOT NULL DEFAULT '',
    override_reason         TEXT NOT NULL DEFAULT '',
    correlation_id          TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS vendor_orchestration_events_idx
    ON vendor_orchestration_events (orchestration_id, created_at DESC);

CREATE OR REPLACE FUNCTION reject_orchestration_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $guard$
BEGIN
    RAISE EXCEPTION 'orchestration decision evidence is append-only';
END $guard$;
DROP TRIGGER IF EXISTS vendor_orchestration_events_append_only ON vendor_orchestration_events;
CREATE TRIGGER vendor_orchestration_events_append_only
    BEFORE UPDATE OR DELETE ON vendor_orchestration_events
    FOR EACH ROW EXECUTE FUNCTION reject_orchestration_event_mutation();

-- ---------------------------------------------------------------------------
-- Multi-vendor grouping
-- ---------------------------------------------------------------------------
-- Which vendor orders one orchestration collects from. Persisted as logical
-- pickup stops even when two vendors share an address, because custody,
-- manifests and handover are per vendor and collapsing them would make a
-- partial collection unrepresentable.
CREATE TABLE IF NOT EXISTS vendor_orchestration_pickups (
    orchestration_id  TEXT NOT NULL
                        REFERENCES vendor_fulfilment_orchestrations(id) ON DELETE CASCADE,
    vendor_order_id   TEXT NOT NULL,
    sequence          INTEGER NOT NULL CHECK (sequence > 0),
    business_id       TEXT NOT NULL,
    branch_id         TEXT NOT NULL,
    -- live mirrors whether the owning orchestration is still in play. It is a
    -- denormalization, maintained by the trigger below, and it exists for one
    -- reason: PostgreSQL will not accept a subquery in an index predicate, and
    -- this invariant has to be an INDEX rather than a check. Two concurrent
    -- planners each verifying "is this order already taken?" with a SELECT
    -- would both see no, and both would send someone.
    live              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (orchestration_id, vendor_order_id),
    UNIQUE (orchestration_id, sequence)
);

-- One vendor order belongs to at most one live orchestration.
CREATE UNIQUE INDEX IF NOT EXISTS vendor_orchestration_pickups_live_idx
    ON vendor_orchestration_pickups (vendor_order_id) WHERE live;

-- Retiring an orchestration releases its vendor orders for replanning. Doing it
-- in a trigger rather than in application code means a cancellation that
-- crashes halfway cannot strand goods nobody may collect.
CREATE OR REPLACE FUNCTION release_orchestration_pickups() RETURNS trigger
LANGUAGE plpgsql AS $release$
BEGIN
    IF NEW.state IN ('CANCELLED', 'FAILED', 'COMPLETED')
       AND OLD.state IS DISTINCT FROM NEW.state THEN
        UPDATE vendor_orchestration_pickups
           SET live = FALSE
         WHERE orchestration_id = NEW.id AND live;
    END IF;
    RETURN NEW;
END $release$;
DROP TRIGGER IF EXISTS vendor_orchestration_release_pickups ON vendor_fulfilment_orchestrations;
CREATE TRIGGER vendor_orchestration_release_pickups
    AFTER UPDATE ON vendor_fulfilment_orchestrations
    FOR EACH ROW EXECUTE FUNCTION release_orchestration_pickups();

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------
-- Overriding automatic authority selection is high risk: it replaces a decision
-- made from the authorities' own answers with one made by a person, and the
-- wrong override sends goods on a vehicle that cannot legally carry them.
INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
    ('perm_vendor_readiness_read', 'vendor.readiness.read',
     'View order readiness state and predictions.', 'fulfilment', false),
    ('perm_vendor_readiness_manage', 'vendor.readiness.manage',
     'Advance order readiness for permitted branches.', 'fulfilment', false),
    ('perm_vendor_orchestration_read', 'vendor.orchestration.read',
     'View fulfilment orchestration and its decision evidence.', 'fulfilment', false),
    ('perm_vendor_orchestration_override', 'vendor.orchestration.override',
     'Override the automatically selected execution authority.', 'fulfilment', true)
ON CONFLICT (code) DO UPDATE
    SET description = EXCLUDED.description,
        category = EXCLUDED.category,
        is_high_risk = EXCLUDED.is_high_risk;
