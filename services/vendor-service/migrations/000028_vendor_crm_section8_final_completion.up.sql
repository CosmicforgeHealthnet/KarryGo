-- Section 8 FINAL completion: the schema the activation of migration 000026
-- turned out to need.
--
-- Migration 000026 laid down the fact tables and projections for the remaining
-- original Section 8 requirements. Building the runtime on top of it exposed
-- five gaps, and this migration closes exactly those and nothing else:
--
--   1. MONEY ON THE DERIVED PROJECTIONS. Affinity and corporate commercial were
--      given counts but no spend, so "what does this customer buy" could be
--      answered by volume and never by value. Both are wanted, and only the
--      fact set can supply either.
--   2. BRANCH ATTRIBUTION ON THE NEW FACTS. A support case and an RFQ can each
--      belong to a branch. Without that column a branch-scoped actor could
--      only be shown everything or nothing, and "everything" is a cross-branch
--      leak.
--   3. CONFIGURATION. Customer value bands, relationship-signal thresholds and
--      follow-up triggers are all business policy. 000026 modelled their
--      OUTPUT and not the vendor's choice of INPUT, which would have meant
--      hard-coding a threshold and calling it a product decision.
--   4. SYSTEM-AUTHORED TASKS. crm_tasks required a staff creator. An automatic
--      follow-up has none, and inventing a service "staff" row to satisfy a
--      foreign key would have put a fictional person in the audit trail.
--   5. BOUNDS ON THE RELATED REFERENCES. 000026 added related_order_ref and its
--      siblings as unbounded text. They are opaque references from other
--      services and must be length-bounded like every other external reference
--      CRM stores.
--
-- The governing rules of 000026 are unchanged and are what everything below
-- obeys: a projection is RECOMPUTED from a fact set and never incremented, and
-- ABSENCE IS A STATE whose numeric columns are NULL rather than zero.

-- ---------------------------------------------------------------------------
-- 1. Money and shape on the fact/affinity tables
-- ---------------------------------------------------------------------------
-- A line's net contribution, taken from the authoritative order snapshot. It is
-- the order's own arithmetic, never recomputed here from a catalogue price:
-- CRM has no pricing authority and a historical total must not move when a
-- price does.
ALTER TABLE crm_customer_order_fact_lines
    ADD COLUMN net_line_amount_minor bigint NOT NULL DEFAULT 0
        CHECK (net_line_amount_minor >= 0);

ALTER TABLE crm_customer_category_affinities
    ADD COLUMN net_spend_minor bigint NOT NULL DEFAULT 0 CHECK (net_spend_minor >= 0);
ALTER TABLE crm_customer_item_affinities
    ADD COLUMN net_spend_minor bigint NOT NULL DEFAULT 0 CHECK (net_spend_minor >= 0);

-- Every order the authority has told us about at this branch, in any state,
-- alongside the completed count 8A already keeps. A branch-scoped actor needs
-- their own observed count for the same reason the business needs one: "tried
-- us four times, kept one" is a different relationship from "ordered once", and
-- it must be answerable without reading branches the actor cannot see.
ALTER TABLE crm_customer_branch_relations
    ADD COLUMN observed_order_count integer NOT NULL DEFAULT 0
        CHECK (observed_order_count >= 0);

-- ---------------------------------------------------------------------------
-- 2. Branch attribution and refund identity on the new facts
-- ---------------------------------------------------------------------------
ALTER TABLE crm_customer_support_facts
    ADD COLUMN branch_id text;
ALTER TABLE crm_customer_support_facts
    ADD CONSTRAINT crm_support_fact_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id);
ALTER TABLE crm_customer_support_facts
    ADD COLUMN resolved_at timestamptz;
-- The refund's own reference, so a CRM user can quote it to the customer
-- without CRM holding a single figure of the money. payment-wallet-service
-- remains the only place an amount exists.
ALTER TABLE crm_customer_support_facts
    ADD COLUMN source_refund_ref text
        CHECK (source_refund_ref IS NULL OR char_length(source_refund_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_customer_support_facts
    ADD CONSTRAINT crm_support_fact_case_ref_ck
        CHECK (char_length(source_case_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_customer_support_facts
    ADD CONSTRAINT crm_support_fact_order_ref_ck
        CHECK (related_order_ref IS NULL OR char_length(related_order_ref) BETWEEN 1 AND 200);

CREATE INDEX crm_support_facts_branch_idx
    ON crm_customer_support_facts (business_id, branch_id, occurred_at DESC)
    WHERE branch_id IS NOT NULL;

ALTER TABLE crm_customer_service_recovery_projections
    ADD COLUMN latest_refund_ref text;
ALTER TABLE crm_customer_service_recovery_projections
    ADD COLUMN latest_refund_at timestamptz;

ALTER TABLE crm_corporate_rfq_facts
    ADD COLUMN branch_id text;
ALTER TABLE crm_corporate_rfq_facts
    ADD CONSTRAINT crm_rfq_fact_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id);
ALTER TABLE crm_corporate_rfq_facts
    ADD COLUMN resolved_at timestamptz;
ALTER TABLE crm_corporate_rfq_facts
    ADD CONSTRAINT crm_rfq_fact_ref_ck
        CHECK (char_length(source_rfq_ref) BETWEEN 1 AND 200);

ALTER TABLE crm_corporate_commercial_projections
    ADD COLUMN net_spend_minor bigint CHECK (net_spend_minor IS NULL OR net_spend_minor >= 0);
ALTER TABLE crm_corporate_commercial_projections
    ADD COLUMN currency char(3) CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$');
ALTER TABLE crm_corporate_commercial_projections
    ADD COLUMN last_branch_ref text;
-- Money is present exactly when the projection is current, for the same reason
-- the counts are: an unavailable spend must not be readable as zero spend.
ALTER TABLE crm_corporate_commercial_projections
    ADD CONSTRAINT crm_corporate_commercial_money_ck CHECK (
        (state = 'NOT_AVAILABLE' AND net_spend_minor IS NULL)
     OR (state <> 'NOT_AVAILABLE' AND net_spend_minor IS NOT NULL));

-- Resolved RFQ count is the quote-conversion denominator, kept explicitly so
-- the ratio can be audited from the stored row rather than re-derived from a
-- rule the reader has to trust.
ALTER TABLE crm_corporate_rfq_projections
    ADD COLUMN resolved_rfq_count integer CHECK (resolved_rfq_count IS NULL OR resolved_rfq_count >= 0);
ALTER TABLE crm_corporate_rfq_projections
    ADD COLUMN latest_rfq_outcome text;

-- ---------------------------------------------------------------------------
-- 3. Configuration: customer value bands
-- ---------------------------------------------------------------------------
-- The band KEYS are the fixed four that 000026 already constrains the
-- projection to. What a business configures is the entry threshold of each rung
-- and its own label for it — the same division loyalty tiers use, and the one
-- that keeps the segment engine's band field a safe closed ENUM. A vendor who
-- could mint band names would also be minting segment-field values.
--
-- A business may configure any subset: two rungs is a perfectly good ladder,
-- and a rung a vendor has not defined simply does not exist for them.
CREATE TABLE crm_customer_value_band_definitions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    band_key text NOT NULL CHECK (band_key IN ('STANDARD', 'ESTABLISHED', 'PRIORITY', 'STRATEGIC')),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),

    -- Both thresholds must be met. Two measures rather than one because spend
    -- alone rewards a single large order and count alone rewards a frequent
    -- trivial one; a vendor who cares about only one sets the other to zero.
    minimum_net_spend_minor bigint NOT NULL CHECK (minimum_net_spend_minor >= 0),
    minimum_completed_orders integer NOT NULL CHECK (minimum_completed_orders >= 0),

    rank integer NOT NULL CHECK (rank BETWEEN 1 AND 4),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_value_band_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
     OR (status = 'ACTIVE' AND archived_at IS NULL))
);

ALTER TABLE crm_customer_value_projections
    DROP CONSTRAINT IF EXISTS crm_customer_value_band_ck;
ALTER TABLE crm_customer_value_projections
    DROP CONSTRAINT IF EXISTS crm_customer_value_projections_state_check;
ALTER TABLE crm_customer_value_projections
    DROP CONSTRAINT IF EXISTS crm_customer_value_state_ck;
ALTER TABLE crm_customer_value_projections
    ADD CONSTRAINT crm_customer_value_state_ck
    CHECK (state IN ('NOT_AVAILABLE', 'NO_BAND', 'CURRENT', 'STALE', 'FAILED'));
ALTER TABLE crm_customer_value_projections
    ADD CONSTRAINT crm_customer_value_band_ck CHECK (
        (state = 'CURRENT' AND band IS NOT NULL)
     OR (state IN ('NOT_AVAILABLE', 'NO_BAND', 'STALE', 'FAILED') AND band IS NULL));

-- One key and one rank per business among live bands. A repeated rank or a
-- repeated key would make "which band is this customer in" ambiguous, and an
-- ambiguous commercial classification is worse than no classification.
CREATE UNIQUE INDEX crm_value_band_key_uq
    ON crm_customer_value_band_definitions (business_id, band_key) WHERE status = 'ACTIVE';
CREATE UNIQUE INDEX crm_value_band_rank_uq
    ON crm_customer_value_band_definitions (business_id, rank) WHERE status = 'ACTIVE';

-- ---------------------------------------------------------------------------
-- 4. Configuration: relationship signal thresholds
-- ---------------------------------------------------------------------------
-- One row per business holding every signal threshold, because they are read
-- together on every evaluation and a table per threshold would be four joins
-- to answer one question. A business with no row uses the documented defaults;
-- that is a real configuration state, not a missing one.
CREATE TABLE crm_relationship_signal_configs (
    business_id text PRIMARY KEY REFERENCES vendor_businesses (id),

    -- REPEAT: completed orders at or above this count.
    repeat_order_threshold integer NOT NULL DEFAULT 2 CHECK (repeat_order_threshold BETWEEN 1 AND 100),
    -- LAPSED: whole days since the last completed order.
    lapsed_after_days integer NOT NULL DEFAULT 90 CHECK (lapsed_after_days BETWEEN 1 AND 1825),
    -- AT_RISK: the CHURN_V1 heuristic at or above this score. The model is
    -- unchanged; this is the policy applied to it, which is why it lives here
    -- and not in the 8H model definition.
    at_risk_churn_threshold integer NOT NULL DEFAULT 70 CHECK (at_risk_churn_threshold BETWEEN 0 AND 100),
    -- HIGH_VALUE: the lowest value-band rank that counts as high value.
    high_value_minimum_rank integer NOT NULL DEFAULT 3 CHECK (high_value_minimum_rank BETWEEN 1 AND 4),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);

-- The lapsed sweeper's work queue is the signal set itself: a row whose stored
-- LAPSED no longer matches the elapsed time IS the outstanding job. This index
-- is what lets the sweeper find those rows without a table scan, and what makes
-- a separate queue table unnecessary.
-- The evaluation inputs are kept on the signal row so the sweeper can decide
-- whether a recomputation is even needed without joining back to the profile,
-- and so an operator can see WHY a signal says what it says.
ALTER TABLE crm_relationship_signals
    ADD COLUMN last_completed_order_at timestamptz;
ALTER TABLE crm_relationship_signals
    ADD COLUMN completed_order_count integer CHECK (completed_order_count IS NULL OR completed_order_count >= 0);
ALTER TABLE crm_relationship_signals
    ADD COLUMN value_band_rank integer CHECK (value_band_rank IS NULL OR value_band_rank BETWEEN 1 AND 4);
ALTER TABLE crm_relationship_signals
    ADD COLUMN churn_risk_score integer CHECK (churn_risk_score IS NULL OR churn_risk_score BETWEEN 0 AND 100);

CREATE INDEX crm_relationship_signals_lapsed_idx
    ON crm_relationship_signals (business_id, lapsed_signal, last_completed_order_at)
    WHERE state = 'CURRENT';

-- ---------------------------------------------------------------------------
-- 5. Configuration: automatic follow-up triggers
-- ---------------------------------------------------------------------------
-- Deliberately not a rules engine. A row is an on/off switch plus a due-date
-- offset and a priority for exactly one named, hand-written rule. There is no
-- expression column, because a vendor-authored expression that could create
-- tasks would be a second segmentation DSL with none of Section 8B's guards.
CREATE TABLE crm_task_trigger_configs (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    trigger_key text NOT NULL CHECK (trigger_key IN
        ('UNRESOLVED_COMPLAINT', 'REJECTED_RFQ', 'HIGH_VALUE_RECOVERY',
         'LAPSED_CUSTOMER', 'CORPORATE_LEAD')),

    enabled boolean NOT NULL DEFAULT false,
    due_offset_hours integer NOT NULL DEFAULT 48 CHECK (due_offset_hours BETWEEN 0 AND 8760),
    priority text NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
    -- UNRESOLVED_COMPLAINT only: how long a case may stay open before it is
    -- worth a human's attention. Zero means "the moment it is observed".
    unresolved_after_hours integer NOT NULL DEFAULT 24 CHECK (unresolved_after_hours BETWEEN 0 AND 8760),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, trigger_key)
);

-- ---------------------------------------------------------------------------
-- 6. System-authored tasks
-- ---------------------------------------------------------------------------
-- An automatic follow-up has no human author, and the honest representation of
-- that is NULL rather than a service account impersonating a colleague in the
-- audit trail. The replacement constraint keeps the invariant that mattered:
-- every task says where it came from, either a staff member or a named rule.
ALTER TABLE crm_tasks ALTER COLUMN created_by_staff_id DROP NOT NULL;
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_origin_ck CHECK (
    created_by_staff_id IS NOT NULL OR trigger_key IS NOT NULL);

-- ---------------------------------------------------------------------------
-- 7. Bounds on the opaque related references
-- ---------------------------------------------------------------------------
-- These are references produced by other services. Bounding them is the same
-- discipline every other external reference in this schema already carries:
-- an unbounded text column reachable from a vendor request is a place to put
-- something that is not a reference.
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_related_order_ck
    CHECK (related_order_ref IS NULL OR char_length(related_order_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_related_rfq_ck
    CHECK (related_rfq_ref IS NULL OR char_length(related_rfq_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_related_case_ck
    CHECK (related_support_case_ref IS NULL OR char_length(related_support_case_ref) BETWEEN 1 AND 200);

ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_related_order_ck
    CHECK (related_order_ref IS NULL OR char_length(related_order_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_related_rfq_ck
    CHECK (related_rfq_ref IS NULL OR char_length(related_rfq_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_related_case_ck
    CHECK (related_support_case_ref IS NULL OR char_length(related_support_case_ref) BETWEEN 1 AND 200);

-- Interaction revisions record the contextual references alongside the text, so
-- a correction that re-points an interaction at a different order leaves the
-- previous claim visible instead of overwriting the evidence.
ALTER TABLE crm_interaction_revisions ADD COLUMN related_order_ref text
    CHECK (related_order_ref IS NULL OR char_length(related_order_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_interaction_revisions ADD COLUMN related_rfq_ref text
    CHECK (related_rfq_ref IS NULL OR char_length(related_rfq_ref) BETWEEN 1 AND 200);
ALTER TABLE crm_interaction_revisions ADD COLUMN related_support_case_ref text
    CHECK (related_support_case_ref IS NULL OR char_length(related_support_case_ref) BETWEEN 1 AND 200);

-- Task history records them too, for the same reason and with the addition that
-- a generated task's history must be able to explain the rule that made it
-- after the trigger configuration has moved on.
ALTER TABLE crm_task_history ADD COLUMN related_order_ref text;
ALTER TABLE crm_task_history ADD COLUMN related_rfq_ref text;
ALTER TABLE crm_task_history ADD COLUMN related_support_case_ref text;
ALTER TABLE crm_task_history ADD COLUMN trigger_key text;

-- ---------------------------------------------------------------------------
-- 8. Trigger evaluation lifecycle
-- ---------------------------------------------------------------------------
-- A firing that no longer qualifies is RESOLVED, not deleted: the task it
-- created stays as evidence, and the record of why it was created has to stay
-- with it. RESOLVED is also the re-arm point — a later qualification produces a
-- new dedupe identity and therefore a new firing.
ALTER TABLE crm_task_trigger_evaluations DROP CONSTRAINT crm_task_trigger_evaluations_status_check;
ALTER TABLE crm_task_trigger_evaluations ADD CONSTRAINT crm_task_trigger_status_ck
    CHECK (status IN ('FIRED', 'SUPERSEDED', 'CANCELLED', 'RESOLVED'));
ALTER TABLE crm_task_trigger_evaluations ADD COLUMN resolved_at timestamptz;
ALTER TABLE crm_task_trigger_evaluations ADD COLUMN last_evaluated_at timestamptz;
