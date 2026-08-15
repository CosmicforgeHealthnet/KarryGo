-- Section 8B — Tags & Advanced Segmentation Engine.
--
-- This migration adds the vendor-defined classification and segmentation
-- surface on top of the Section 8A CRM foundation: tags, tag memberships,
-- saved segment definitions, immutable segment versions, durable evaluations,
-- materialized segment membership and manual exclusions.
--
-- Boundaries preserved from Section 8A:
--   - customer-service remains the identity authority. No table here stores
--     canonical email, phone, address, DOB or credentials, and no segment
--     rule can reference them (the field registry lives in Go, not SQL).
--   - Every table is tenant-scoped by business_id and reaches its parent
--     through a composite (id, business_id) foreign key, so tenancy is a
--     PostgreSQL invariant, not only an HTTP check.
--   - Vendor users never author raw SQL: dynamic segment rules are stored as
--     validated JSON and compiled by trusted registry code at evaluation time.

-- ---------------------------------------------------------------------------
-- crm_tags — vendor-defined customer tags
-- ---------------------------------------------------------------------------
CREATE TABLE crm_tags (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    -- BUSINESS tags are visible business-wide (subject to permission); BRANCH
    -- tags belong to exactly one branch and are managed by actors authorised
    -- for that branch.
    scope_type text NOT NULL CHECK (scope_type IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    -- Case- and space-insensitive comparison key. VIP / vip / Vip collapse to
    -- one tag; the display name keeps its original casing.
    normalized_name text NOT NULL CHECK (char_length(normalized_name) BETWEEN 1 AND 80),
    description text CHECK (description IS NULL OR char_length(description) <= 500),

    -- ARCHIVED tags cannot receive new memberships and are hidden from ordinary
    -- selectors, but their historical membership and audit trail survive.
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    created_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),

    -- A BRANCH tag names its branch; a BUSINESS tag never does.
    CONSTRAINT crm_tag_scope_branch_ck CHECK (
        (scope_type = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope_type = 'BUSINESS' AND branch_id IS NULL)
    ),
    CONSTRAINT crm_tag_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

-- Active-name uniqueness is scope-aware and partial, so archiving a tag frees
-- its name for later reuse without destroying the archived row's history.
CREATE UNIQUE INDEX crm_tags_business_name_uq
    ON crm_tags (business_id, normalized_name)
    WHERE scope_type = 'BUSINESS' AND status = 'ACTIVE';
CREATE UNIQUE INDEX crm_tags_branch_name_uq
    ON crm_tags (business_id, branch_id, normalized_name)
    WHERE scope_type = 'BRANCH' AND status = 'ACTIVE';

CREATE INDEX crm_tags_business_status_idx ON crm_tags (business_id, status, scope_type);
CREATE INDEX crm_tags_branch_idx ON crm_tags (business_id, branch_id) WHERE branch_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- crm_customer_tag_memberships — tag ↔ profile assignments
-- ---------------------------------------------------------------------------
CREATE TABLE crm_customer_tag_memberships (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    tag_id text NOT NULL,
    -- Mirrors the tag's branch for BRANCH tags; NULL for BUSINESS tags. It lets
    -- branch-scoped reads filter membership without joining the tag table.
    branch_id text,

    -- MANUAL is the only source a vendor request may create. SYSTEM is reserved
    -- for future platform-managed tags and is never client-settable.
    source_type text NOT NULL DEFAULT 'MANUAL' CHECK (source_type IN ('MANUAL', 'SYSTEM')),

    assigned_by_staff_ref text,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    -- One membership per (tag, profile). Re-assigning is idempotent.
    UNIQUE (business_id, tag_id, profile_id),
    CONSTRAINT crm_tag_membership_tag_fk
        FOREIGN KEY (tag_id, business_id) REFERENCES crm_tags (id, business_id),
    CONSTRAINT crm_tag_membership_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_tag_membership_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_tag_memberships_profile_idx ON crm_customer_tag_memberships (business_id, profile_id);
CREATE INDEX crm_tag_memberships_tag_idx ON crm_customer_tag_memberships (business_id, tag_id);

-- ---------------------------------------------------------------------------
-- crm_segments — saved audience definitions
-- ---------------------------------------------------------------------------
CREATE TABLE crm_segments (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    normalized_name text NOT NULL CHECK (char_length(normalized_name) BETWEEN 1 AND 120),
    description text CHECK (description IS NULL OR char_length(description) <= 1000),

    segment_type text NOT NULL CHECK (segment_type IN ('STATIC', 'DYNAMIC')),
    scope_type text NOT NULL CHECK (scope_type IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),

    -- The immutable version the segment currently points at. Version 1 is
    -- written at creation for both types (a STATIC version records type/scope
    -- with a NULL rule).
    current_definition_version bigint NOT NULL DEFAULT 1 CHECK (current_definition_version > 0),

    evaluation_status text NOT NULL DEFAULT 'NOT_EVALUATED'
        CHECK (evaluation_status IN ('NOT_EVALUATED', 'QUEUED', 'RUNNING', 'CURRENT', 'STALE', 'FAILED')),
    current_evaluation_id text,
    current_member_count integer NOT NULL DEFAULT 0 CHECK (current_member_count >= 0),
    definition_hash text,
    last_evaluated_at timestamptz,
    stale_since timestamptz,
    last_evaluation_error_code text,

    created_by_staff_ref text,
    updated_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_segment_scope_branch_ck CHECK (
        (scope_type = 'BRANCH' AND branch_id IS NOT NULL)
     OR (scope_type = 'BUSINESS' AND branch_id IS NULL)
    ),
    CONSTRAINT crm_segment_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

CREATE UNIQUE INDEX crm_segments_business_name_uq
    ON crm_segments (business_id, normalized_name)
    WHERE status <> 'ARCHIVED';
CREATE INDEX crm_segments_business_idx ON crm_segments (business_id, status, segment_type);
CREATE INDEX crm_segments_branch_idx ON crm_segments (business_id, branch_id) WHERE branch_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- crm_segment_versions — immutable definition history
-- ---------------------------------------------------------------------------
-- A new version is minted on every rule change. Old versions are never
-- mutated, so a materialization can always be tied to the exact definition it
-- ran against, and a future campaign snapshot can pin a version.
CREATE TABLE crm_segment_versions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    segment_id text NOT NULL,
    definition_version bigint NOT NULL CHECK (definition_version > 0),

    segment_type text NOT NULL CHECK (segment_type IN ('STATIC', 'DYNAMIC')),
    scope_type text NOT NULL CHECK (scope_type IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    rule_schema_version integer CHECK (rule_schema_version IS NULL OR rule_schema_version > 0),
    -- Validated DSL only; NULL for STATIC segments. Never raw SQL.
    rule_json jsonb,
    exclusion_rule_json jsonb,
    canonical_rule_hash text,
    -- Registry field keys the rule depends on, used to decide which segments a
    -- given change invalidates.
    dependency_set text[] NOT NULL DEFAULT '{}',

    created_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    change_reason text CHECK (change_reason IS NULL OR char_length(change_reason) <= 500),

    UNIQUE (business_id, segment_id, definition_version),
    CONSTRAINT crm_segment_version_segment_fk
        FOREIGN KEY (segment_id, business_id) REFERENCES crm_segments (id, business_id)
);

-- ---------------------------------------------------------------------------
-- crm_segment_evaluations — durable materialization runs
-- ---------------------------------------------------------------------------
-- Each run owns a generation. Only a SUCCESS run becomes the segment's current
-- membership; a FAILED run never deletes the previous successful generation.
CREATE TABLE crm_segment_evaluations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    segment_id text NOT NULL,
    segment_definition_version bigint NOT NULL CHECK (segment_definition_version > 0),
    definition_hash text,
    generation bigint NOT NULL CHECK (generation > 0),

    status text NOT NULL DEFAULT 'QUEUED'
        CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCESS', 'FAILED', 'DEAD_LETTER', 'SUPERSEDED')),
    evaluation_as_of timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    candidate_count integer CHECK (candidate_count IS NULL OR candidate_count >= 0),
    member_count integer CHECK (member_count IS NULL OR member_count >= 0),
    excluded_count integer CHECK (excluded_count IS NULL OR excluded_count >= 0),
    error_code text,
    error_detail_safe text,

    -- Worker durability: lease + backoff + attempt count, matching the Section 7
    -- verification-reconciliation worker's shape.
    worker_attempt_count integer NOT NULL DEFAULT 0 CHECK (worker_attempt_count >= 0),
    lease_owner text,
    lease_expires_at timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (business_id, segment_id, generation),
    CONSTRAINT crm_segment_evaluation_segment_fk
        FOREIGN KEY (segment_id, business_id) REFERENCES crm_segments (id, business_id)
);

-- Worker claim index: due, unleased, non-terminal runs.
CREATE INDEX crm_segment_evaluations_due_idx
    ON crm_segment_evaluations (next_attempt_at)
    WHERE status IN ('QUEUED', 'RUNNING', 'FAILED');
CREATE INDEX crm_segment_evaluations_segment_idx
    ON crm_segment_evaluations (business_id, segment_id, generation DESC);

-- ---------------------------------------------------------------------------
-- crm_segment_memberships — materialized + static membership
-- ---------------------------------------------------------------------------
-- Holds both STATIC membership (generation 0, maintained transactionally) and
-- DYNAMIC membership (generation N from a specific successful evaluation). The
-- previous dynamic generation stays intact until a new one succeeds, so a
-- caller never sees an empty audience during rematerialization.
CREATE TABLE crm_segment_memberships (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    segment_id text NOT NULL,
    profile_id text NOT NULL,

    membership_source text NOT NULL CHECK (membership_source IN ('STATIC', 'DYNAMIC')),
    evaluation_id text,
    -- STATIC membership uses generation 0; DYNAMIC uses the evaluation's
    -- generation. The unique key includes it so multiple dynamic generations
    -- can briefly coexist.
    generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
    segment_definition_version bigint,

    matched_at timestamptz NOT NULL DEFAULT now(),
    added_by_staff_ref text,
    explanation_digest text,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, segment_id, profile_id, generation),
    CONSTRAINT crm_segment_membership_source_ck CHECK (
        (membership_source = 'STATIC' AND generation = 0)
     OR (membership_source = 'DYNAMIC' AND generation > 0)
    ),
    CONSTRAINT crm_segment_membership_segment_fk
        FOREIGN KEY (segment_id, business_id) REFERENCES crm_segments (id, business_id),
    CONSTRAINT crm_segment_membership_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_segment_memberships_current_idx
    ON crm_segment_memberships (business_id, segment_id, generation);
CREATE INDEX crm_segment_memberships_profile_idx
    ON crm_segment_memberships (business_id, profile_id);

-- ---------------------------------------------------------------------------
-- crm_segment_exclusions — manual exclusions from a dynamic segment
-- ---------------------------------------------------------------------------
CREATE TABLE crm_segment_exclusions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    segment_id text NOT NULL,
    profile_id text NOT NULL,
    reason_code text CHECK (reason_code IS NULL OR char_length(reason_code) <= 120),

    created_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (business_id, segment_id, profile_id),
    CONSTRAINT crm_segment_exclusion_segment_fk
        FOREIGN KEY (segment_id, business_id) REFERENCES crm_segments (id, business_id),
    CONSTRAINT crm_segment_exclusion_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_segment_exclusions_segment_idx ON crm_segment_exclusions (business_id, segment_id);
CREATE INDEX crm_segment_exclusions_profile_idx ON crm_segment_exclusions (business_id, profile_id);
