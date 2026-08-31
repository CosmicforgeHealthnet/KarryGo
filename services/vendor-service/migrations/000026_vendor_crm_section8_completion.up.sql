-- Section 8 completion: the CRM-owned half of every original Section 8
-- requirement whose producer does not exist yet.
--
-- The governing decision in this migration is that a projection is built from a
-- FACT SET and recomputed, never incremented. Section 8A already established
-- that shape for order facts, and it is what makes replay, refund, correction,
-- out-of-order delivery and identity merge converge instead of drifting. Every
-- new projection here follows it: an authoritative fact table, a derived
-- projection, and a recomputation that can be run again at any time.
--
-- The second decision is that ABSENCE IS A STATE. Every projection carries an
-- explicit availability state, and its numeric columns are NULL when that state
-- is not CURRENT. A vendor whose Order authority does not exist yet must read
-- "we have never observed this" and never "zero orders, zero spend" — the two
-- are different facts and only one of them is true.

-- ---------------------------------------------------------------------------
-- Order facts: the fields the original specification needs but 8A did not carry
-- ---------------------------------------------------------------------------
-- 8A projects completed commercial totals. The original Section 8 also asks for
-- an order COUNT distinct from the completed count, and for relationship shape
-- (recurring, bulk, corporate) that only the order authority can state. These
-- are additive: every existing fact row keeps its meaning, and the defaults are
-- chosen so an 8A-era row reads exactly as it did before.
ALTER TABLE crm_customer_order_facts
    ADD COLUMN order_type text NOT NULL DEFAULT 'STANDARD'
        CHECK (order_type IN ('STANDARD', 'BULK', 'RECURRING', 'CORPORATE'));
ALTER TABLE crm_customer_order_facts
    ADD COLUMN corporate_customer_id text;
ALTER TABLE crm_customer_order_facts
    ADD CONSTRAINT crm_order_fact_corporate_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id);

-- Section 8A never needed to reference an order fact from another table, so it
-- carries only a single-column key. The completion does need that reference —
-- an order's lines belong to it — and a tenant-composite foreign key is the
-- mechanism every other CRM table uses to make cross-tenant rows unstorable
-- rather than merely unwritten. Additive: no existing row or query changes.
ALTER TABLE crm_customer_order_facts
    ADD CONSTRAINT crm_customer_order_facts_id_business_uq UNIQUE (id, business_id);

CREATE INDEX crm_order_facts_type_idx
    ON crm_customer_order_facts (business_id, profile_id, order_type, completed_at DESC);
CREATE INDEX crm_order_facts_corporate_idx
    ON crm_customer_order_facts (business_id, corporate_customer_id, completed_at DESC)
    WHERE corporate_customer_id IS NOT NULL;

-- observed_order_count is every order the authority has told us about, in any
-- state. completed_order_count, which 8A already maintains, counts only the
-- ones that completed. Keeping both is the point: "ordered four times, kept
-- one" and "ordered once" describe very different relationships.
ALTER TABLE vendor_customer_profiles
    ADD COLUMN observed_order_count integer NOT NULL DEFAULT 0
        CHECK (observed_order_count >= 0);

-- ---------------------------------------------------------------------------
-- Order fact lines — the evidence affinities are recomputed from
-- ---------------------------------------------------------------------------
-- Category and item affinity must be derived, not accumulated, so the lines of
-- each authoritative order are stored and the affinity tables are rebuilt from
-- them. A corrected or refunded order therefore corrects the affinity, and a
-- replayed event changes nothing.
--
-- Only references and quantities are kept. There is deliberately no item name,
-- description or price: catalogue naming belongs to the catalogue domain, and a
-- copy here would be stale the moment it changed.
CREATE TABLE crm_customer_order_fact_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    order_fact_id text NOT NULL,
    profile_id text NOT NULL,
    branch_id text,

    category_ref text CHECK (category_ref IS NULL OR char_length(category_ref) BETWEEN 1 AND 200),
    item_ref text CHECK (item_ref IS NULL OR char_length(item_ref) BETWEEN 1 AND 200),
    quantity integer NOT NULL DEFAULT 0 CHECK (quantity >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    -- A line that names neither a category nor an item carries no information
    -- and would only dilute the affinity ranking.
    CONSTRAINT crm_order_fact_line_ref_ck CHECK (category_ref IS NOT NULL OR item_ref IS NOT NULL),
    CONSTRAINT crm_order_fact_line_fact_fk FOREIGN KEY (order_fact_id, business_id)
        REFERENCES crm_customer_order_facts (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_order_fact_line_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_order_fact_line_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_order_fact_lines_profile_idx
    ON crm_customer_order_fact_lines (business_id, profile_id);
CREATE INDEX crm_order_fact_lines_fact_idx
    ON crm_customer_order_fact_lines (business_id, order_fact_id);

-- ---------------------------------------------------------------------------
-- Category and item affinities
-- ---------------------------------------------------------------------------
CREATE TABLE crm_customer_category_affinities (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    scope text NOT NULL DEFAULT 'BUSINESS' CHECK (scope IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    category_ref text NOT NULL CHECK (char_length(category_ref) BETWEEN 1 AND 200),
    completed_order_count integer NOT NULL DEFAULT 0 CHECK (completed_order_count >= 0),
    quantity_count integer NOT NULL DEFAULT 0 CHECK (quantity_count >= 0),
    last_ordered_at timestamptz,
    rank integer NOT NULL DEFAULT 0 CHECK (rank >= 0),

    source_as_of timestamptz NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    CONSTRAINT crm_category_affinity_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    CONSTRAINT crm_category_affinity_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_category_affinity_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE UNIQUE INDEX crm_category_affinity_uq
    ON crm_customer_category_affinities
    (business_id, profile_id, scope, COALESCE(branch_id, ''), category_ref);
CREATE INDEX crm_category_affinity_rank_idx
    ON crm_customer_category_affinities (business_id, profile_id, scope, rank);

CREATE TABLE crm_customer_item_affinities (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    scope text NOT NULL DEFAULT 'BUSINESS' CHECK (scope IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    item_ref text NOT NULL CHECK (char_length(item_ref) BETWEEN 1 AND 200),
    completed_order_count integer NOT NULL DEFAULT 0 CHECK (completed_order_count >= 0),
    quantity_count integer NOT NULL DEFAULT 0 CHECK (quantity_count >= 0),
    last_ordered_at timestamptz,
    rank integer NOT NULL DEFAULT 0 CHECK (rank >= 0),

    source_as_of timestamptz NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    CONSTRAINT crm_item_affinity_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    CONSTRAINT crm_item_affinity_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_item_affinity_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE UNIQUE INDEX crm_item_affinity_uq
    ON crm_customer_item_affinities
    (business_id, profile_id, scope, COALESCE(branch_id, ''), item_ref);
CREATE INDEX crm_item_affinity_rank_idx
    ON crm_customer_item_affinities (business_id, profile_id, scope, rank);

-- ---------------------------------------------------------------------------
-- Support facts and the service-recovery projection
-- ---------------------------------------------------------------------------
-- support-dispute-service owns complaints and their adjudication. What CRM
-- needs is the shape of the relationship's service history, and nothing about
-- what was said: there is no narrative, no evidence and no attachment column
-- here, only references, classifications and counts.
CREATE TABLE crm_customer_support_facts (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,

    source_service text NOT NULL,
    source_case_ref text NOT NULL,
    source_event_id text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),

    case_state text NOT NULL
        CHECK (case_state IN ('OPEN', 'IN_REVIEW', 'RESOLVED', 'REJECTED', 'WITHDRAWN', 'CANCELLED')),
    outcome text CHECK (outcome IS NULL OR outcome IN
        ('NONE', 'RESOLVED_IN_FAVOUR', 'RESOLVED_AGAINST', 'PARTIAL', 'REFUND_FULL',
         'REFUND_PARTIAL', 'NO_ACTION')),
    related_order_ref text,
    occurred_at timestamptz NOT NULL,

    last_observed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    UNIQUE (business_id, source_service, source_case_ref),
    CONSTRAINT crm_support_fact_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE
);

CREATE INDEX crm_support_facts_profile_idx
    ON crm_customer_support_facts (business_id, profile_id, occurred_at DESC);
CREATE INDEX crm_support_facts_state_idx
    ON crm_customer_support_facts (business_id, case_state)
    WHERE case_state IN ('OPEN', 'IN_REVIEW');

CREATE TABLE crm_customer_service_recovery_projections (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,

    state text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE')),
    complaint_count integer CHECK (complaint_count IS NULL OR complaint_count >= 0),
    unresolved_complaint_count integer CHECK (unresolved_complaint_count IS NULL OR unresolved_complaint_count >= 0),
    resolved_complaint_count integer CHECK (resolved_complaint_count IS NULL OR resolved_complaint_count >= 0),
    refund_count integer CHECK (refund_count IS NULL OR refund_count >= 0),
    partial_refund_count integer CHECK (partial_refund_count IS NULL OR partial_refund_count >= 0),

    latest_case_ref text,
    latest_case_outcome text,
    latest_case_at timestamptz,
    last_service_recovery_at timestamptz,

    source_as_of timestamptz,
    computed_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, profile_id),
    -- Unavailable means unavailable: no count may be present to be mistaken
    -- for an observed zero.
    CONSTRAINT crm_service_recovery_availability_ck CHECK (
        (state = 'NOT_AVAILABLE' AND complaint_count IS NULL AND refund_count IS NULL)
     OR (state <> 'NOT_AVAILABLE' AND complaint_count IS NOT NULL AND refund_count IS NOT NULL)),
    CONSTRAINT crm_service_recovery_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE
);

CREATE INDEX crm_service_recovery_state_idx
    ON crm_customer_service_recovery_projections (business_id, state);
CREATE INDEX crm_service_recovery_unresolved_idx
    ON crm_customer_service_recovery_projections (business_id, unresolved_complaint_count)
    WHERE unresolved_complaint_count > 0;

-- ---------------------------------------------------------------------------
-- Corporate commercial and RFQ projections
-- ---------------------------------------------------------------------------
CREATE TABLE crm_corporate_commercial_projections (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,

    state text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE')),
    order_count integer CHECK (order_count IS NULL OR order_count >= 0),
    completed_order_count integer CHECK (completed_order_count IS NULL OR completed_order_count >= 0),
    recurring_order_count integer CHECK (recurring_order_count IS NULL OR recurring_order_count >= 0),
    bulk_order_count integer CHECK (bulk_order_count IS NULL OR bulk_order_count >= 0),
    first_order_at timestamptz,
    last_order_at timestamptz,
    last_bulk_order_at timestamptz,
    last_recurring_order_at timestamptz,
    latest_order_ref text,

    source_as_of timestamptz,
    computed_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, corporate_customer_id),
    CONSTRAINT crm_corporate_commercial_availability_ck CHECK (
        (state = 'NOT_AVAILABLE' AND order_count IS NULL)
     OR (state <> 'NOT_AVAILABLE' AND order_count IS NOT NULL)),
    CONSTRAINT crm_corporate_commercial_account_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id) ON DELETE CASCADE
);

-- RFQ facts are owned by a future RFQ authority. CRM keeps the outcome shape so
-- an account manager can see how the commercial conversation has gone, and
-- nothing about what was quoted: no line, no price, no margin.
CREATE TABLE crm_corporate_rfq_facts (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,
    profile_id text,

    source_service text NOT NULL,
    source_rfq_ref text NOT NULL,
    source_quote_ref text,
    source_event_id text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),

    rfq_state text NOT NULL
        CHECK (rfq_state IN ('SUBMITTED', 'QUOTED', 'ACCEPTED', 'REJECTED', 'EXPIRED', 'CANCELLED', 'CONVERTED')),
    quote_outcome text CHECK (quote_outcome IS NULL OR quote_outcome IN
        ('NONE', 'ACCEPTED', 'REJECTED', 'EXPIRED', 'WITHDRAWN')),
    converted_order_ref text,
    occurred_at timestamptz NOT NULL,

    last_observed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    UNIQUE (business_id, source_service, source_rfq_ref),
    CONSTRAINT crm_rfq_fact_account_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id) ON DELETE CASCADE
);

CREATE INDEX crm_rfq_facts_account_idx
    ON crm_corporate_rfq_facts (business_id, corporate_customer_id, occurred_at DESC);

CREATE TABLE crm_corporate_rfq_projections (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,

    state text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE')),
    rfq_count integer CHECK (rfq_count IS NULL OR rfq_count >= 0),
    quoted_rfq_count integer CHECK (quoted_rfq_count IS NULL OR quoted_rfq_count >= 0),
    accepted_quote_count integer CHECK (accepted_quote_count IS NULL OR accepted_quote_count >= 0),
    rejected_quote_count integer CHECK (rejected_quote_count IS NULL OR rejected_quote_count >= 0),
    converted_rfq_count integer CHECK (converted_rfq_count IS NULL OR converted_rfq_count >= 0),
    expired_rfq_count integer CHECK (expired_rfq_count IS NULL OR expired_rfq_count >= 0),
    -- Integer basis points, so conversion never carries a float that invites
    -- being read as a probability. NULL when the denominator is zero: "no
    -- quoted RFQs" is not "0% conversion".
    quote_conversion_bps integer CHECK (quote_conversion_bps IS NULL OR quote_conversion_bps BETWEEN 0 AND 10000),

    latest_rfq_ref text,
    latest_quote_ref text,
    latest_rfq_at timestamptz,

    source_as_of timestamptz,
    computed_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, corporate_customer_id),
    CONSTRAINT crm_corporate_rfq_availability_ck CHECK (
        (state = 'NOT_AVAILABLE' AND rfq_count IS NULL)
     OR (state <> 'NOT_AVAILABLE' AND rfq_count IS NOT NULL)),
    CONSTRAINT crm_corporate_rfq_account_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- Loyalty tiers
-- ---------------------------------------------------------------------------
-- Tiers are a vendor-configured, ordered set of thresholds over the loyalty
-- ledger. They confer recognition, not benefits: there is no discount, no
-- entitlement and no monetary column here, because a benefits engine would be
-- a commerce feature and points are not money.
CREATE TABLE crm_loyalty_tier_definitions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    tier_key text NOT NULL CHECK (tier_key ~ '^[A-Z][A-Z0-9_]{1,39}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),
    minimum_lifetime_points bigint NOT NULL CHECK (minimum_lifetime_points >= 0),
    rank integer NOT NULL CHECK (rank BETWEEN 1 AND 8),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id)
);

-- One key, one rank and one threshold per business among live tiers. Two tiers
-- sharing a threshold would make membership ambiguous, and ambiguity in a
-- customer-facing status is worse than having no tiers at all.
CREATE UNIQUE INDEX crm_loyalty_tier_key_uq
    ON crm_loyalty_tier_definitions (business_id, tier_key) WHERE status = 'ACTIVE';
CREATE UNIQUE INDEX crm_loyalty_tier_rank_uq
    ON crm_loyalty_tier_definitions (business_id, rank) WHERE status = 'ACTIVE';
CREATE UNIQUE INDEX crm_loyalty_tier_threshold_uq
    ON crm_loyalty_tier_definitions (business_id, minimum_lifetime_points) WHERE status = 'ACTIVE';

-- ---------------------------------------------------------------------------
-- Customer value and relationship signals
-- ---------------------------------------------------------------------------
-- Both are derived, versioned and hashed exactly like the Section 8H models,
-- for the same reason: a score whose rules cannot be identified afterwards is a
-- number nobody can defend.
CREATE TABLE crm_customer_value_projections (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    scope text NOT NULL DEFAULT 'BUSINESS' CHECK (scope IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    model_key text NOT NULL DEFAULT 'CUSTOMER_VALUE_V1' CHECK (model_key = 'CUSTOMER_VALUE_V1'),
    model_version integer NOT NULL CHECK (model_version > 0),
    definition_hash text NOT NULL,

    state text NOT NULL CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'FAILED')),
    band text CHECK (band IS NULL OR band IN ('STANDARD', 'ESTABLISHED', 'PRIORITY', 'STRATEGIC')),
    reason_codes text[] NOT NULL DEFAULT '{}',

    source_as_of timestamptz NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, profile_id, scope, model_key),
    -- A band exists exactly when the projection is current, so an unavailable
    -- value can never be read as the lowest band.
    CONSTRAINT crm_customer_value_band_ck CHECK ((state = 'CURRENT') = (band IS NOT NULL)),
    CONSTRAINT crm_customer_value_reason_ck CHECK (
        state = 'CURRENT' OR array_length(reason_codes, 1) >= 1),
    CONSTRAINT crm_customer_value_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    CONSTRAINT crm_customer_value_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_customer_value_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_customer_value_band_idx
    ON crm_customer_value_projections (business_id, scope, band)
    WHERE state = 'CURRENT';

-- One row per relationship per scope, holding every signal. A row per signal
-- would multiply the privacy and merge surface by seven for no gain.
CREATE TABLE crm_relationship_signals (
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    scope text NOT NULL DEFAULT 'BUSINESS' CHECK (scope IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    model_version integer NOT NULL CHECK (model_version > 0),
    definition_hash text NOT NULL,
    state text NOT NULL CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'FAILED')),

    -- Every signal is three-valued. NULL is "we cannot know", which is a
    -- different statement from false and must never collapse into it.
    repeat_signal boolean,
    lapsed_signal boolean,
    high_value_signal boolean,
    at_risk_signal boolean,
    bulk_capable_signal boolean,
    corporate_signal boolean,
    recurring_signal boolean,

    reason_codes text[] NOT NULL DEFAULT '{}',
    source_as_of timestamptz NOT NULL,
    computed_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    PRIMARY KEY (business_id, profile_id, scope),
    CONSTRAINT crm_relationship_signal_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    CONSTRAINT crm_relationship_signal_profile_fk FOREIGN KEY (profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_relationship_signal_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_relationship_signals_lookup_idx
    ON crm_relationship_signals (business_id, profile_id, scope)
    WHERE state = 'CURRENT';

-- ---------------------------------------------------------------------------
-- Interactions and tasks: related references and trigger provenance
-- ---------------------------------------------------------------------------
-- These are CONTEXT, not subjects. The Section 8C invariant that a row names
-- exactly one CRM subject is untouched; an order or support reference simply
-- says what the conversation was about.
ALTER TABLE crm_interactions ADD COLUMN related_order_ref text;
ALTER TABLE crm_interactions ADD COLUMN related_rfq_ref text;
ALTER TABLE crm_interactions ADD COLUMN related_support_case_ref text;

ALTER TABLE crm_tasks ADD COLUMN related_order_ref text;
ALTER TABLE crm_tasks ADD COLUMN related_rfq_ref text;
ALTER TABLE crm_tasks ADD COLUMN related_support_case_ref text;
ALTER TABLE crm_tasks ADD COLUMN trigger_key text;
ALTER TABLE crm_tasks ADD COLUMN trigger_version integer
    CHECK (trigger_version IS NULL OR trigger_version > 0);
ALTER TABLE crm_tasks ADD COLUMN trigger_source_ref text;
ALTER TABLE crm_tasks ADD COLUMN trigger_evaluated_at timestamptz;
ALTER TABLE crm_tasks ADD COLUMN completion_outcome text
    CHECK (completion_outcome IS NULL OR completion_outcome IN
        ('RESOLVED', 'NO_ACTION_NEEDED', 'CUSTOMER_UNREACHABLE', 'ESCALATED', 'DUPLICATE'));

-- An automatic task always says which rule produced it, so an operator can
-- understand why it is on their queue.
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_trigger_provenance_ck CHECK (
    trigger_key IS NULL OR (trigger_version IS NOT NULL AND trigger_evaluated_at IS NOT NULL));

CREATE INDEX crm_tasks_trigger_idx ON crm_tasks (business_id, trigger_key, status)
    WHERE trigger_key IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Automatic follow-up trigger state
-- ---------------------------------------------------------------------------
-- The dedupe record. A trigger fires at most once per identity, and that
-- identity is stored rather than recomputed at read time, so a worker restart,
-- a replayed source fact and a second worker instance all converge on one task.
CREATE TABLE crm_task_trigger_evaluations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    trigger_key text NOT NULL,
    trigger_version integer NOT NULL CHECK (trigger_version > 0),
    subject_type text NOT NULL CHECK (subject_type IN ('CUSTOMER', 'CORPORATE')),
    customer_profile_id text,
    corporate_customer_id text,

    -- The window or source fact this firing is keyed on. Two evaluations with
    -- the same identity are the same firing, whoever computed them.
    dedupe_identity text NOT NULL CHECK (char_length(dedupe_identity) BETWEEN 1 AND 200),
    source_ref text,

    status text NOT NULL DEFAULT 'FIRED' CHECK (status IN ('FIRED', 'SUPERSEDED', 'CANCELLED')),
    task_id text,
    evaluated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    CONSTRAINT crm_task_trigger_subject_ck CHECK (
        (subject_type = 'CUSTOMER' AND customer_profile_id IS NOT NULL AND corporate_customer_id IS NULL)
     OR (subject_type = 'CORPORATE' AND corporate_customer_id IS NOT NULL AND customer_profile_id IS NULL)),
    CONSTRAINT crm_task_trigger_profile_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_task_trigger_corporate_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_task_trigger_task_fk FOREIGN KEY (task_id, business_id)
        REFERENCES crm_tasks (id, business_id) ON DELETE SET NULL
);

-- The dedupe fence itself.
CREATE UNIQUE INDEX crm_task_trigger_identity_uq
    ON crm_task_trigger_evaluations (business_id, trigger_key, dedupe_identity);
CREATE INDEX crm_task_trigger_subject_idx
    ON crm_task_trigger_evaluations (business_id, subject_type, COALESCE(customer_profile_id, corporate_customer_id));
