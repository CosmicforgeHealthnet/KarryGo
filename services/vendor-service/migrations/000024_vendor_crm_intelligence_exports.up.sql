-- Section 8H — CRM intelligence, analytics and secure exports.
--
-- Two domains share this migration because they share one property: both
-- derive from CRM facts this service already owns, and neither may invent a
-- fact it was not given.
--
-- Intelligence is deterministic, versioned and explainable. There is no model
-- artefact, no training data, no probability and no free-text explanation
-- anywhere in this schema, because Section 8H computes heuristics in Go from
-- observed order facts and nothing else. A score column is therefore always an
-- integer with a checked range, never a float that could be mistaken for a
-- likelihood.
--
-- The other property both halves share is that MISSING IS NOT ZERO. A
-- relationship with no observed commercial facts gets a NOT_AVAILABLE score
-- row whose every numeric column is NULL, enforced by CHECK — a row asserting
-- "unavailable, composite 3" is unstorable rather than merely unwritten. That
-- is what stops "we have never seen this customer order" from being read as
-- "this customer is worthless".

-- ---------------------------------------------------------------------------
-- Intelligence evaluations
-- ---------------------------------------------------------------------------
-- One durable job per (scope, model) rebuild. It is the unit of work the
-- materializer claims, the unit a generation belongs to, and the unit whose
-- counts make availability honest at the scope level.
CREATE TABLE crm_intelligence_evaluations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    scope text NOT NULL CHECK (scope IN ('BUSINESS', 'BRANCH')),

    model_key text NOT NULL CHECK (model_key IN ('RFM_V1', 'CHURN_V1')),
    model_version integer NOT NULL CHECK (model_version > 0),
    -- The hash of the code-defined model descriptor that produced this run.
    -- Two runs with the same hash used the same rules; a changed hash is a
    -- changed model, which is why scores carry it too.
    definition_hash text NOT NULL CHECK (char_length(definition_hash) BETWEEN 16 AND 128),
    generation bigint NOT NULL CHECK (generation > 0),

    status text NOT NULL DEFAULT 'QUEUED'
        CHECK (status IN ('QUEUED', 'RUNNING', 'COMPLETED', 'FAILED', 'DEAD_LETTER', 'SUPERSEDED')),

    -- The instant the run judges every profile against, so one generation is
    -- internally consistent even if it takes minutes to build.
    source_as_of timestamptz NOT NULL,

    profile_count integer NOT NULL DEFAULT 0 CHECK (profile_count >= 0),
    available_count integer NOT NULL DEFAULT 0 CHECK (available_count >= 0),
    unavailable_count integer NOT NULL DEFAULT 0 CHECK (unavailable_count >= 0),
    failure_count integer NOT NULL DEFAULT 0 CHECK (failure_count >= 0),

    error_code text CHECK (error_code IS NULL OR char_length(error_code) BETWEEN 1 AND 64),
    worker_attempt_count integer NOT NULL DEFAULT 0 CHECK (worker_attempt_count >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,

    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    -- BRANCH scope means an actual branch. A branch-less branch score would be
    -- a business score wearing a branch label, which is exactly the leak the
    -- scope distinction exists to prevent.
    CONSTRAINT crm_intelligence_evaluation_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    CONSTRAINT crm_intelligence_evaluation_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

-- One generation number is used once per scope and model.
CREATE UNIQUE INDEX crm_intelligence_evaluation_generation_uq
    ON crm_intelligence_evaluations
    (business_id, model_key, scope, COALESCE(branch_id, ''), generation);

CREATE INDEX crm_intelligence_evaluations_claim_idx
    ON crm_intelligence_evaluations (status, next_attempt_at, created_at)
    WHERE status IN ('QUEUED', 'RUNNING', 'FAILED');
CREATE INDEX crm_intelligence_evaluations_scope_idx
    ON crm_intelligence_evaluations (business_id, model_key, scope, status, created_at DESC);

-- ---------------------------------------------------------------------------
-- Intelligence scope state
-- ---------------------------------------------------------------------------
-- The current-generation pointer, and the honest answer to "does this scope
-- have usable intelligence at all". It exists so a caller can be told
-- NOT_AVAILABLE or STALE without scanning score rows, and so a failed rebuild
-- can report failure while the previous generation stays live.
CREATE TABLE crm_intelligence_scope_states (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    scope text NOT NULL CHECK (scope IN ('BUSINESS', 'BRANCH')),
    model_key text NOT NULL CHECK (model_key IN ('RFM_V1', 'CHURN_V1')),

    status text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (status IN ('NOT_AVAILABLE', 'QUEUED', 'RUNNING', 'CURRENT', 'STALE', 'FAILED')),
    current_generation bigint CHECK (current_generation IS NULL OR current_generation > 0),
    current_evaluation_id text,
    model_version integer CHECK (model_version IS NULL OR model_version > 0),
    definition_hash text,
    source_as_of timestamptz,
    computed_at timestamptz,
    stale_since timestamptz,
    error_code text CHECK (error_code IS NULL OR char_length(error_code) BETWEEN 1 AND 64),

    profile_count integer NOT NULL DEFAULT 0 CHECK (profile_count >= 0),
    available_count integer NOT NULL DEFAULT 0 CHECK (available_count >= 0),
    unavailable_count integer NOT NULL DEFAULT 0 CHECK (unavailable_count >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_intelligence_scope_state_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),
    -- A scope claiming CURRENT must name the generation it is current at.
    -- Without this a failed swap could leave a pointer-less "current".
    CONSTRAINT crm_intelligence_scope_state_current_ck CHECK (
        status <> 'CURRENT'
     OR (current_generation IS NOT NULL AND current_evaluation_id IS NOT NULL
         AND model_version IS NOT NULL AND computed_at IS NOT NULL)),
    CONSTRAINT crm_intelligence_scope_state_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id),
    CONSTRAINT crm_intelligence_scope_state_evaluation_fk FOREIGN KEY (current_evaluation_id, business_id)
        REFERENCES crm_intelligence_evaluations (id, business_id)
);

CREATE UNIQUE INDEX crm_intelligence_scope_states_uq
    ON crm_intelligence_scope_states (business_id, model_key, scope, COALESCE(branch_id, ''));
CREATE INDEX crm_intelligence_scope_states_status_idx
    ON crm_intelligence_scope_states (business_id, status, model_key);

-- ---------------------------------------------------------------------------
-- Customer intelligence scores
-- ---------------------------------------------------------------------------
-- One row per (profile, model, scope, generation). No personal data: a score
-- names a relationship by its opaque CRM reference and says nothing about the
-- human behind it.
CREATE TABLE crm_customer_intelligence_scores (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    scope text NOT NULL CHECK (scope IN ('BUSINESS', 'BRANCH')),

    evaluation_id text NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    customer_profile_id text NOT NULL,

    model_key text NOT NULL CHECK (model_key IN ('RFM_V1', 'CHURN_V1')),
    model_version integer NOT NULL CHECK (model_version > 0),
    definition_hash text NOT NULL,

    state text NOT NULL CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'FAILED')),
    -- is_current is the swap pointer. Only the generation the scope state
    -- points at carries it, which is what makes a half-built generation
    -- invisible rather than merely unreferenced.
    is_current boolean NOT NULL DEFAULT false,

    -- RFM_V1 output. 1 is the weakest quintile of this scope's scored cohort
    -- and 5 the strongest, so the numbers are comparable within a scope and
    -- deliberately not across businesses.
    recency_score smallint CHECK (recency_score IS NULL OR recency_score BETWEEN 1 AND 5),
    frequency_score smallint CHECK (frequency_score IS NULL OR frequency_score BETWEEN 1 AND 5),
    monetary_score smallint CHECK (monetary_score IS NULL OR monetary_score BETWEEN 1 AND 5),
    composite_score smallint CHECK (composite_score IS NULL OR composite_score BETWEEN 3 AND 15),

    -- CHURN_V1 output. An integer risk heuristic, never a probability.
    risk_score smallint CHECK (risk_score IS NULL OR risk_score BETWEEN 0 AND 100),
    risk_band text CHECK (risk_band IS NULL OR risk_band IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),

    reason_codes text[] NOT NULL DEFAULT '{}',

    source_as_of timestamptz NOT NULL,
    computed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    CONSTRAINT crm_intelligence_score_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),

    -- A CURRENT RFM row carries exactly the RFM columns, a CURRENT churn row
    -- exactly the churn columns, and anything not CURRENT carries none. This
    -- is the structural half of "unavailable is not zero": there is no way to
    -- store a NOT_AVAILABLE row with a number in it.
    CONSTRAINT crm_intelligence_score_payload_ck CHECK (
        (state = 'CURRENT' AND model_key = 'RFM_V1'
            AND recency_score IS NOT NULL AND frequency_score IS NOT NULL
            AND monetary_score IS NOT NULL AND composite_score IS NOT NULL
            AND composite_score = recency_score + frequency_score + monetary_score
            AND risk_score IS NULL AND risk_band IS NULL)
     OR (state = 'CURRENT' AND model_key = 'CHURN_V1'
            AND risk_score IS NOT NULL AND risk_band IS NOT NULL
            AND recency_score IS NULL AND frequency_score IS NULL
            AND monetary_score IS NULL AND composite_score IS NULL)
     OR (state <> 'CURRENT'
            AND recency_score IS NULL AND frequency_score IS NULL
            AND monetary_score IS NULL AND composite_score IS NULL
            AND risk_score IS NULL AND risk_band IS NULL)),
    -- Every non-current state must say why, so "no score" is never silent.
    CONSTRAINT crm_intelligence_score_reason_ck CHECK (
        state = 'CURRENT' OR array_length(reason_codes, 1) >= 1),

    CONSTRAINT crm_intelligence_score_evaluation_fk FOREIGN KEY (evaluation_id, business_id)
        REFERENCES crm_intelligence_evaluations (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_intelligence_score_profile_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_intelligence_score_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

-- One score per relationship per model per scope per generation, and at most
-- one current score per relationship per model per scope.
CREATE UNIQUE INDEX crm_intelligence_scores_generation_uq
    ON crm_customer_intelligence_scores
    (business_id, model_key, scope, COALESCE(branch_id, ''), generation, customer_profile_id);
CREATE UNIQUE INDEX crm_intelligence_scores_current_uq
    ON crm_customer_intelligence_scores
    (business_id, model_key, scope, COALESCE(branch_id, ''), customer_profile_id)
    WHERE is_current;

-- The segment predicate path: one relationship's current score for one model.
CREATE INDEX crm_intelligence_scores_lookup_idx
    ON crm_customer_intelligence_scores
    (business_id, customer_profile_id, model_key, scope, state)
    WHERE is_current;
-- The analytics path: distribution over a scope's current scores.
CREATE INDEX crm_intelligence_scores_distribution_idx
    ON crm_customer_intelligence_scores
    (business_id, model_key, scope, branch_id, state)
    WHERE is_current;
CREATE INDEX crm_intelligence_scores_evaluation_idx
    ON crm_customer_intelligence_scores (business_id, evaluation_id, customer_profile_id);

-- ---------------------------------------------------------------------------
-- Secure CRM exports
-- ---------------------------------------------------------------------------
-- An export is a durable job, not a download. The row is the authority on
-- whether an artifact may be released at all, which is why status, expiry and
-- artifact identity live here rather than in whatever produced the bytes.
--
-- There is deliberately no URL column. A stored URL would be a credential with
-- no expiry that survives every check this table exists to enforce; access is
-- always mediated live, against this row, at the moment it is requested.
CREATE TABLE crm_exports (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    scope text NOT NULL CHECK (scope IN ('BUSINESS', 'BRANCH')),
    requested_by_staff_id text NOT NULL,

    export_type text NOT NULL CHECK (export_type IN (
        'CUSTOMER_SUMMARY', 'SEGMENT_MEMBERS', 'CORPORATE_CUSTOMERS',
        'LOYALTY_SUMMARY', 'CAMPAIGN_PERFORMANCE_SUMMARY', 'INTELLIGENCE_SCORES')),
    format text NOT NULL CHECK (format IN ('CSV', 'JSONL')),
    status text NOT NULL DEFAULT 'QUEUED'
        CHECK (status IN ('QUEUED', 'RUNNING', 'READY', 'FAILED', 'CANCELLED', 'EXPIRED')),

    -- A closed, validated filter document. No SQL, no column names, no
    -- expression ever reaches this column: it is the parsed and re-serialised
    -- form of a typed request, kept so a completed export can say what it was.
    filter_document jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- The version of the dataset's field allowlist that produced the file, so
    -- a consumer can tell whether two files of the same type share a shape.
    field_set_version integer NOT NULL CHECK (field_set_version > 0),

    row_limit integer NOT NULL CHECK (row_limit BETWEEN 1 AND 250000),
    row_count bigint NOT NULL DEFAULT 0 CHECK (row_count >= 0),

    artifact_ref text,
    artifact_bytes bigint CHECK (artifact_bytes IS NULL OR artifact_bytes >= 0),
    checksum_sha256 text CHECK (checksum_sha256 IS NULL OR checksum_sha256 ~ '^[0-9a-f]{64}$'),

    -- The privacy fence. Every row is selected against relationships that were
    -- already non-redacted at this instant, and the publish step re-certifies
    -- against it, so a relationship redacted mid-run cannot survive into a
    -- readable artifact.
    privacy_fence_at timestamptz NOT NULL,

    requested_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    expires_at timestamptz,
    cancelled_at timestamptz,
    failed_at timestamptz,
    error_code text CHECK (error_code IS NULL OR char_length(error_code) BETWEEN 1 AND 64),

    worker_attempt_count integer NOT NULL DEFAULT 0 CHECK (worker_attempt_count >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_export_scope_ck CHECK (
        (scope = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope = 'BUSINESS' AND branch_id IS NULL)),

    -- READY is the only state that releases bytes, so it is the state with the
    -- strictest precondition: a complete artifact, its checksum, its size, a
    -- completion instant and an expiry. A worker that died halfway cannot
    -- satisfy this, which is the structural reason a partial export can never
    -- become downloadable.
    CONSTRAINT crm_export_ready_ck CHECK (
        status <> 'READY'
     OR (artifact_ref IS NOT NULL AND checksum_sha256 IS NOT NULL
         AND artifact_bytes IS NOT NULL AND completed_at IS NOT NULL
         AND expires_at IS NOT NULL)),
    CONSTRAINT crm_export_expiry_ck CHECK (
        expires_at IS NULL OR completed_at IS NULL OR expires_at > completed_at),
    CONSTRAINT crm_export_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id),
    CONSTRAINT crm_export_staff_fk FOREIGN KEY (requested_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX crm_exports_business_idx ON crm_exports (business_id, requested_at DESC, id DESC);
CREATE INDEX crm_exports_status_idx ON crm_exports (business_id, status, requested_at DESC);
CREATE INDEX crm_exports_requester_idx ON crm_exports (business_id, requested_by_staff_id, requested_at DESC);
CREATE INDEX crm_exports_claim_idx ON crm_exports (status, next_attempt_at, created_at)
    WHERE status IN ('QUEUED', 'RUNNING');
CREATE INDEX crm_exports_expiry_idx ON crm_exports (expires_at)
    WHERE status = 'READY';
