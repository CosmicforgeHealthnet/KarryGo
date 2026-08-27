-- Fulfilment plan candidates: the alternatives the optimizer considered.
--
-- Until now a plan stored the grouping it executed and nothing about the
-- groupings it did not. That is enough to charge a customer and not enough to
-- explain the charge, and every question Support actually receives is about
-- the alternative that lost:
--
--   "Why were these two vendors put on one delivery?"
--   "Why was my order split into two?"
--   "Why did the faster option cost more?"
--   "Why could the cheaper option not combine these pickups?"
--
-- None of them is answerable from a row that records only the winner. So each
-- alternative gets a row, with its status, its reason, the legs it would have
-- run, and the policy version it was judged under.
--
-- Two invariants are enforced here rather than in Go, because they are the
-- ones that would be most damaging to lose and least likely to be noticed:
--
--   1. A HARD refusal may only ever appear on an INELIGIBLE row.
--
--      Recording "capacity exceeded" as a REJECTED candidate would say the
--      optimizer weighed a safety limit against a price and chose the price.
--      It never does, and the constraint makes that unrepresentable rather
--      than merely untrue.
--
--   2. At most one SELECTED candidate per plan.
--
--      Two selected alternatives is two different plans claiming to be the
--      one that ran.

CREATE TABLE commerce_fulfilment_plan_candidates (
    id text PRIMARY KEY,

    plan_id text NOT NULL,

    -- Carried explicitly, and joined back to the plan by a COMPOSITE key.
    -- Customer isolation is then a property of the row shape rather than of
    -- every query remembering a predicate.
    customer_id text NOT NULL,

    kind text NOT NULL
        CHECK (kind IN ('CONSOLIDATED', 'SPLIT_PARALLEL')),

    -- The preference this candidate was evaluated FOR. A candidate judged
    -- under SAVER is not evidence about what FASTEST would have done, and
    -- storing the mode is what stops it being read that way later.
    mode text NOT NULL
        CHECK (mode IN ('SINGLE', 'FASTEST', 'BALANCED', 'SAVER')),

    status text NOT NULL
        CHECK (status IN ('SELECTED', 'REJECTED', 'INELIGIBLE')),

    reason_code text NOT NULL CHECK (reason_code <> ''),

    -- The threshold envelope in force at decision time. Thresholds move; a
    -- stored explanation that silently re-reads today's numbers has stopped
    -- explaining what happened.
    policy_version bigint NOT NULL CHECK (policy_version > 0),

    mission_count integer NOT NULL CHECK (mission_count >= 0),
    vendor_group_count integer NOT NULL CHECK (vendor_group_count >= 0),

    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),

    -- Gross is what the delivery actually costs; customer is the part the
    -- customer is asked for. They differ whenever a vendor or the platform
    -- funds delivery, and comparing the wrong one is how a subsidised plan
    -- gets recorded as "cheaper for the customer" when it was not.
    delivery_gross_minor bigint NOT NULL DEFAULT 0
        CHECK (delivery_gross_minor >= 0),
    delivery_customer_minor bigint NOT NULL DEFAULT 0
        CHECK (delivery_customer_minor >= 0),

    -- Time to the LAST delivery, in minutes. Parallel legs overlap, so this
    -- is not the sum of the legs.
    completion_minutes integer NOT NULL DEFAULT 0
        CHECK (completion_minutes >= 0),

    -- How much longer this candidate's slowest leg is than the slowest leg of
    -- the fully split alternative. Zero on the split alternative, which is
    -- the baseline.
    detour_minutes integer NOT NULL DEFAULT 0
        CHECK (detour_minutes >= 0),

    multi_stop_required boolean NOT NULL DEFAULT false,

    -- Which authorities could drive a multi-stop collection at the moment of
    -- the decision. Without it, "MULTI_STOP_UNSUPPORTED" read back in a year
    -- would be an assertion with no evidence behind it.
    service_capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,

    -- The legs this candidate would have run: groups, service, class, stop
    -- count, quoted distance and transit, and the funding split.
    legs jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Semantic identity: the shape of the plan, with no timestamps, ids or
    -- quote references in it. Two candidates with the same fingerprint are
    -- the same plan, which is what makes deduplication possible.
    fingerprint text NOT NULL CHECK (fingerprint <> ''),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_fulfilment_plan_candidates_plan_fk
        FOREIGN KEY (plan_id, customer_id)
        REFERENCES commerce_fulfilment_plans (id, customer_id)
        ON DELETE CASCADE,

    -- Invariant 1: a hard constraint refusal can only be INELIGIBLE.
    CONSTRAINT commerce_fulfilment_plan_candidates_hard_reason_chk
        CHECK (
            reason_code NOT IN (
                'CAPACITY_EXCEEDED',
                'TEMPERATURE_CONFLICT',
                'FRAGILE_UNSAFE',
                'SEPARATION_CONFLICT',
                'READY_WINDOW_CONFLICT',
                'ETA_DETOUR_EXCEEDED',
                'MULTI_STOP_UNSUPPORTED',
                'SERVICE_OR_CLASS_MISMATCH',
                'URGENCY_CONFLICT',
                'CANDIDATE_NOT_QUOTABLE',
                'PICKUP_PROXIMITY_EXCEEDED'
            )
            OR status = 'INELIGIBLE'
        ),

    -- And the converse: an INELIGIBLE row must carry a constraint reason. A
    -- candidate refused "because it was slower" is a rejection, not an
    -- impossibility, and blurring the two is what the whole table exists to
    -- prevent.
    CONSTRAINT commerce_fulfilment_plan_candidates_ineligible_reason_chk
        CHECK (
            status <> 'INELIGIBLE'
            OR reason_code IN (
                'CAPACITY_EXCEEDED',
                'TEMPERATURE_CONFLICT',
                'FRAGILE_UNSAFE',
                'SEPARATION_CONFLICT',
                'READY_WINDOW_CONFLICT',
                'ETA_DETOUR_EXCEEDED',
                'MULTI_STOP_UNSUPPORTED',
                'SERVICE_OR_CLASS_MISMATCH',
                'URGENCY_CONFLICT',
                'CANDIDATE_NOT_QUOTABLE',
                'PICKUP_PROXIMITY_EXCEEDED'
            )
        ),

    -- A multi-stop refusal must actually be about a multi-stop plan.
    CONSTRAINT commerce_fulfilment_plan_candidates_multi_stop_chk
        CHECK (
            reason_code <> 'MULTI_STOP_UNSUPPORTED'
            OR multi_stop_required
        ),

    -- One row per distinct plan shape. Offering the same plan twice under two
    -- labels is the "lie told twice" the section forbids, and this makes it
    -- impossible to store rather than merely discouraged.
    UNIQUE (plan_id, fingerprint)
);

-- Invariant 2: at most one selected alternative per plan.
CREATE UNIQUE INDEX commerce_fulfilment_plan_candidates_one_selected_idx
    ON commerce_fulfilment_plan_candidates (plan_id)
    WHERE status = 'SELECTED';

-- Reading a plan's alternatives back, in the order they are presented.
CREATE INDEX commerce_fulfilment_plan_candidates_plan_idx
    ON commerce_fulfilment_plan_candidates (plan_id, status, kind, id);

-- Operational counting: how often is a given constraint the reason orders get
-- split. This is the query that turns "multi-stop is unsupported" from an
-- architectural note into a number someone can prioritise against.
CREATE INDEX commerce_fulfilment_plan_candidates_reason_idx
    ON commerce_fulfilment_plan_candidates (reason_code, created_at DESC)
    WHERE status <> 'SELECTED';
