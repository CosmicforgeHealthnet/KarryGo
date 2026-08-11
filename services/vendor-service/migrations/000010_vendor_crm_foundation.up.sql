-- Section 8A — CRM Foundation, Customer 360, Consent & Privacy.
--
-- Vendor Service CRM stores the *commercial relationship* between one vendor
-- business and a customer subject. It is deliberately NOT a customer master:
-- customer-service remains the authority for canonical identity, credentials,
-- email, phone, addresses, consent and privacy decisions. Nothing in this
-- migration stores canonical customer PII, and no hash of a customer
-- reference is used as a substitute for erasure.
--
-- Every table is tenant-scoped: it either carries business_id directly or is
-- reached through a composite (business_id, id) foreign key onto a
-- business-owned parent, so tenancy is enforced by PostgreSQL and not only by
-- HTTP middleware.

-- ---------------------------------------------------------------------------
-- Inbox fingerprint (additive to the Section 0 shared inbox)
-- ---------------------------------------------------------------------------
-- The existing inbox dedupes by (source_service, event_id) only. Section 8A
-- additionally requires that the same event id replayed with a *conflicting*
-- payload is rejected as an integrity conflict rather than silently accepted
-- as a duplicate, so the claim records a payload fingerprint. Nullable, so
-- every pre-existing row and every non-CRM caller stays valid.
ALTER TABLE vendor_inbox_processed_events
    ADD COLUMN IF NOT EXISTS payload_fingerprint text;

-- ---------------------------------------------------------------------------
-- vendor_customer_profiles — the Vendor-local relationship projection
-- ---------------------------------------------------------------------------
CREATE TABLE vendor_customer_profiles (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    -- Subject identity is an opaque, authority-owned reference. Exactly one
    -- primary reference applies per subject_type (see the invariant below).
    subject_type text NOT NULL CHECK (subject_type IN ('CUSTOMER', 'GUEST', 'EXTERNAL')),
    customer_ref text,
    guest_ref text,
    external_ref text,

    -- Optional, only ever populated from an approved PII-free presentation
    -- contract. No such contract exists today, so this stays NULL: it must
    -- never be back-filled from an email address or phone number.
    safe_display_label text CHECK (safe_display_label IS NULL OR char_length(safe_display_label) BETWEEN 1 AND 160),

    -- Vendor CRM lifecycle. ARCHIVED is an operator convenience and is NOT a
    -- privacy outcome; it can never stand in for REDACTED or MERGED.
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    -- Authoritative privacy outcome projected from the privacy authority.
    privacy_state text NOT NULL DEFAULT 'ACTIVE'
        CHECK (privacy_state IN ('ACTIVE', 'RESTRICTED', 'REDACTED', 'MERGED')),

    merge_state text NOT NULL DEFAULT 'NONE'
        CHECK (merge_state IN ('NONE', 'LINK_PENDING', 'MERGED_SOURCE', 'MERGED_TARGET')),

    acquisition_source_type text,
    acquisition_source_ref text,

    -- Commercial summary, projected from crm_customer_order_facts. Money is
    -- always an integer count of minor units; there is no float anywhere.
    first_completed_order_at timestamptz,
    last_completed_order_at timestamptz,
    completed_order_count integer NOT NULL DEFAULT 0 CHECK (completed_order_count >= 0),
    gross_spend_minor bigint NOT NULL DEFAULT 0 CHECK (gross_spend_minor >= 0),
    net_spend_minor bigint NOT NULL DEFAULT 0 CHECK (net_spend_minor >= 0),
    average_order_value_minor bigint NOT NULL DEFAULT 0 CHECK (average_order_value_minor >= 0),
    -- NULL until authoritative commercial data has actually been observed.
    -- A currency is never guessed, and totals of different currencies are
    -- never silently combined.
    commercial_currency char(3) CHECK (commercial_currency IS NULL OR commercial_currency ~ '^[A-Z]{3}$'),

    last_order_ref text,
    last_branch_ref text,
    preferred_branch_ref text,
    last_capability_type text CHECK (last_capability_type IS NULL OR last_capability_type IN
        ('RESTAURANT', 'FAST_FOOD', 'CLOUD_KITCHEN', 'PASTRY_BAKERY', 'SHOP_GROCERY', 'MARKET', 'WHOLESALE')),

    -- NOT_AVAILABLE is the honest default: no order authority is integrated
    -- yet, so "we have never received commercial data" is representable and
    -- is never rendered as a spend of zero.
    commercial_projection_state text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (commercial_projection_state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'REBUILDING', 'ERROR')),
    commercial_projection_version bigint CHECK (commercial_projection_version IS NULL OR commercial_projection_version > 0),
    last_commercial_source_observed_at timestamptz,

    -- Reserved for Section 8C (interactions/tasks). No 8C table exists, so
    -- these stay NULL/0 and Customer 360 reports the module as unavailable
    -- rather than reporting an empty-but-operational feature.
    last_interaction_at timestamptz,
    open_task_count integer NOT NULL DEFAULT 0 CHECK (open_task_count >= 0),

    consent_projection_version bigint CHECK (consent_projection_version IS NULL OR consent_projection_version > 0),
    consent_observed_at timestamptz,

    redacted_at timestamptz,
    merged_into_profile_id text,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    -- Composite key every child table points at, so a child row can never be
    -- attached to a profile belonging to a different business.
    UNIQUE (id, business_id),

    -- Exactly one primary subject reference, matching subject_type. A GUEST
    -- may additionally carry customer_ref once an authoritative identity link
    -- has been applied; a CUSTOMER never carries a guest/external ref.
    CONSTRAINT vendor_customer_profile_subject_reference_ck CHECK (
        (subject_type = 'CUSTOMER' AND customer_ref IS NOT NULL AND guest_ref IS NULL AND external_ref IS NULL)
     OR (subject_type = 'GUEST' AND guest_ref IS NOT NULL AND external_ref IS NULL)
     OR (subject_type = 'EXTERNAL' AND external_ref IS NOT NULL AND guest_ref IS NULL)
    ),

    -- A merged profile must name its survivor; a non-merged profile must not.
    CONSTRAINT vendor_customer_profile_merge_target_ck CHECK (
        (merge_state = 'MERGED_SOURCE' AND merged_into_profile_id IS NOT NULL)
     OR (merge_state <> 'MERGED_SOURCE' AND merged_into_profile_id IS NULL)
    ),
    -- MERGED_SOURCE and privacy_state MERGED are the same fact seen twice;
    -- they may not disagree.
    CONSTRAINT vendor_customer_profile_merge_state_ck CHECK (
        (merge_state = 'MERGED_SOURCE') = (privacy_state = 'MERGED')
    ),
    -- A profile is redacted if and only if it carries a redaction timestamp.
    CONSTRAINT vendor_customer_profile_redacted_at_ck CHECK (
        (privacy_state = 'REDACTED') = (redacted_at IS NOT NULL)
    ),
    -- A redacted profile carries no presentation data.
    CONSTRAINT vendor_customer_profile_redacted_label_ck CHECK (
        privacy_state <> 'REDACTED' OR safe_display_label IS NULL
    ),
    -- Any observed commercial data implies a known currency, and vice versa.
    CONSTRAINT vendor_customer_profile_currency_ck CHECK (
        (commercial_projection_state = 'NOT_AVAILABLE' AND commercial_currency IS NULL)
     OR (commercial_projection_state <> 'NOT_AVAILABLE' AND commercial_currency IS NOT NULL)
    ),
    CONSTRAINT vendor_customer_profile_merge_self_ck CHECK (merged_into_profile_id IS NULL OR merged_into_profile_id <> id),
    CONSTRAINT vendor_customer_profile_merge_scope_fk
        FOREIGN KEY (merged_into_profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT vendor_customer_profile_last_branch_fk
        FOREIGN KEY (last_branch_ref, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT vendor_customer_profile_preferred_branch_fk
        FOREIGN KEY (preferred_branch_ref, business_id) REFERENCES vendor_branches (id, business_id)
);

-- One usable relationship profile per (business, subject reference). Merged
-- sources are excluded so a survivor can hold the reference while the merged
-- history is preserved.
CREATE UNIQUE INDEX vendor_customer_profiles_customer_ref_uq
    ON vendor_customer_profiles (business_id, customer_ref)
    WHERE customer_ref IS NOT NULL AND merge_state <> 'MERGED_SOURCE';
CREATE UNIQUE INDEX vendor_customer_profiles_guest_ref_uq
    ON vendor_customer_profiles (business_id, guest_ref)
    WHERE guest_ref IS NOT NULL AND merge_state <> 'MERGED_SOURCE';
CREATE UNIQUE INDEX vendor_customer_profiles_external_ref_uq
    ON vendor_customer_profiles (business_id, external_ref)
    WHERE external_ref IS NOT NULL AND merge_state <> 'MERGED_SOURCE';

CREATE INDEX vendor_customer_profiles_business_idx
    ON vendor_customer_profiles (business_id, last_completed_order_at DESC NULLS LAST, id);
CREATE INDEX vendor_customer_profiles_privacy_idx
    ON vendor_customer_profiles (business_id, privacy_state);
CREATE INDEX vendor_customer_profiles_merged_into_idx
    ON vendor_customer_profiles (merged_into_profile_id)
    WHERE merged_into_profile_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- crm_customer_identity_links — authoritative alias/merge history
-- ---------------------------------------------------------------------------
-- Links are only ever created from authoritative evidence supplied by an
-- identity authority. Vendor staff assertion is not evidence and there is no
-- public route that writes this table.
CREATE TABLE crm_customer_identity_links (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    source_profile_id text NOT NULL,
    target_profile_id text NOT NULL,
    link_type text NOT NULL CHECK (link_type IN ('GUEST_TO_CUSTOMER', 'CUSTOMER_MERGE', 'EXTERNAL_TO_CUSTOMER')),
    authority_service text NOT NULL,
    authority_reference text NOT NULL,
    authority_version bigint NOT NULL CHECK (authority_version > 0),
    evidence_event_id text NOT NULL,
    status text NOT NULL DEFAULT 'APPLIED' CHECK (status IN ('PENDING', 'APPLIED', 'REJECTED', 'SUPERSEDED')),
    safe_reason_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    CONSTRAINT crm_identity_link_no_self_ck CHECK (source_profile_id <> target_profile_id),
    CONSTRAINT crm_identity_link_source_fk
        FOREIGN KEY (source_profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_identity_link_target_fk
        FOREIGN KEY (target_profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

-- The same authoritative evidence replayed is one link, not two.
CREATE UNIQUE INDEX crm_identity_links_evidence_uq
    ON crm_customer_identity_links (business_id, authority_service, evidence_event_id);
CREATE INDEX crm_identity_links_source_idx ON crm_customer_identity_links (business_id, source_profile_id);
CREATE INDEX crm_identity_links_target_idx ON crm_customer_identity_links (business_id, target_profile_id);

-- ---------------------------------------------------------------------------
-- crm_customer_branch_relations — branch-scoped relationship projection
-- ---------------------------------------------------------------------------
-- This is what makes branch-scoped visibility safe: a branch-scoped actor is
-- filtered through this table by SQL join, never by loading a business-wide
-- profile and filtering it in memory.
CREATE TABLE crm_customer_branch_relations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,
    branch_id text NOT NULL,

    first_relationship_at timestamptz,
    last_relationship_at timestamptz,
    completed_order_count integer NOT NULL DEFAULT 0 CHECK (completed_order_count >= 0),
    gross_spend_minor bigint NOT NULL DEFAULT 0 CHECK (gross_spend_minor >= 0),
    net_spend_minor bigint NOT NULL DEFAULT 0 CHECK (net_spend_minor >= 0),
    currency char(3) CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    last_order_ref text,
    last_capability_type text CHECK (last_capability_type IS NULL OR last_capability_type IN
        ('RESTAURANT', 'FAST_FOOD', 'CLOUD_KITCHEN', 'PASTRY_BAKERY', 'SHOP_GROCERY', 'MARKET', 'WHOLESALE')),
    projection_state text NOT NULL DEFAULT 'NOT_AVAILABLE'
        CHECK (projection_state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'REBUILDING', 'ERROR')),
    source_version bigint CHECK (source_version IS NULL OR source_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (business_id, profile_id, branch_id),
    CONSTRAINT crm_branch_relation_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_branch_relation_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_branch_relations_branch_idx ON crm_customer_branch_relations (business_id, branch_id, last_relationship_at DESC NULLS LAST);

-- ---------------------------------------------------------------------------
-- crm_customer_order_facts — CRM fact projection (NOT the order domain)
-- ---------------------------------------------------------------------------
-- One row per authoritative order reference. Aggregates are always derivable
-- from these rows, which is what makes refunds non-double-counting, merges
-- deduplicating, and rebuild possible. Deliberately minimal: no addresses, no
-- line items, no receiver contact, no payment data.
CREATE TABLE crm_customer_order_facts (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,

    source_service text NOT NULL,
    source_order_ref text NOT NULL,
    source_event_id text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),

    order_state text NOT NULL CHECK (order_state IN ('COMPLETED', 'CANCELLED', 'REFUNDED', 'UPDATED')),
    completed_at timestamptz,
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    gross_amount_minor bigint NOT NULL DEFAULT 0 CHECK (gross_amount_minor >= 0),
    net_amount_minor bigint NOT NULL DEFAULT 0 CHECK (net_amount_minor >= 0),
    refunded_amount_minor bigint NOT NULL DEFAULT 0 CHECK (refunded_amount_minor >= 0),

    branch_id text,
    capability_type text CHECK (capability_type IS NULL OR capability_type IN
        ('RESTAURANT', 'FAST_FOOD', 'CLOUD_KITCHEN', 'PASTRY_BAKERY', 'SHOP_GROCERY', 'MARKET', 'WHOLESALE')),

    last_observed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- A completed order has a completion time; a non-completed one does not
    -- silently claim one.
    CONSTRAINT crm_order_fact_completed_at_ck CHECK (
        (order_state = 'COMPLETED') = (completed_at IS NOT NULL)
    ),
    CONSTRAINT crm_order_fact_net_ck CHECK (net_amount_minor <= gross_amount_minor),
    CONSTRAINT crm_order_fact_refund_ck CHECK (refunded_amount_minor <= gross_amount_minor),
    CONSTRAINT crm_order_fact_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_order_fact_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

-- One authoritative order reference is exactly one fact row, whatever the
-- delivery count of the events describing it.
CREATE UNIQUE INDEX crm_order_facts_source_uq
    ON crm_customer_order_facts (business_id, source_service, source_order_ref);
CREATE INDEX crm_order_facts_profile_idx ON crm_customer_order_facts (business_id, profile_id, completed_at DESC NULLS LAST);

-- ---------------------------------------------------------------------------
-- crm_consent_projections — local projection of an external consent decision
-- ---------------------------------------------------------------------------
-- Vendor Service never authors consent. UNKNOWN is a first-class decision and
-- is not equivalent to ALLOW: absence of a row is itself UNKNOWN, which fails
-- marketing eligibility closed.
CREATE TABLE crm_consent_projections (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text NOT NULL,

    purpose text NOT NULL CHECK (purpose IN ('MARKETING')),
    channel text NOT NULL CHECK (channel IN ('EMAIL', 'PUSH', 'IN_APP', 'SMS')),
    decision text NOT NULL CHECK (decision IN ('ALLOW', 'DENY', 'UNKNOWN')),

    source_service text NOT NULL,
    source_ref text,
    source_version bigint NOT NULL CHECK (source_version > 0),
    observed_at timestamptz NOT NULL,
    effective_at timestamptz,
    expires_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (business_id, profile_id, purpose, channel),
    CONSTRAINT crm_consent_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

-- ---------------------------------------------------------------------------
-- crm_privacy_projections — authoritative privacy instructions
-- ---------------------------------------------------------------------------
-- profile_id is nullable so an instruction that arrives before (or without) a
-- matching relationship is still recorded and auditable rather than dropped.
CREATE TABLE crm_privacy_projections (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    profile_id text,
    privacy_subject_ref text NOT NULL,

    action text NOT NULL CHECK (action IN ('RESTRICT', 'REDACT', 'MARKETING_SUPPRESS', 'IDENTITY_MERGE')),
    source_service text NOT NULL,
    source_ref text,
    source_version bigint NOT NULL CHECK (source_version > 0),
    observed_at timestamptz NOT NULL,
    processed_at timestamptz,
    processing_status text NOT NULL DEFAULT 'PENDING'
        CHECK (processing_status IN ('PENDING', 'PROCESSING', 'APPLIED', 'FAILED', 'DEAD_LETTER')),
    retention_policy_ref text,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    safe_reason_code text,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    -- The current instruction per (business, subject, action); history lives
    -- in crm_privacy_processing_history.
    UNIQUE (business_id, privacy_subject_ref, action),
    CONSTRAINT crm_privacy_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_privacy_projections_pending_idx
    ON crm_privacy_projections (processing_status, observed_at)
    WHERE processing_status IN ('PENDING', 'PROCESSING');
CREATE INDEX crm_privacy_projections_subject_idx
    ON crm_privacy_projections (business_id, privacy_subject_ref);

-- ---------------------------------------------------------------------------
-- crm_privacy_processing_history — immutable processing record
-- ---------------------------------------------------------------------------
-- Reason codes only; never raw PII and never the source payload.
CREATE TABLE crm_privacy_processing_history (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    privacy_projection_id text NOT NULL REFERENCES crm_privacy_projections (id),
    profile_id text,
    action text NOT NULL CHECK (action IN ('RESTRICT', 'REDACT', 'MARKETING_SUPPRESS', 'IDENTITY_MERGE')),
    previous_privacy_state text NOT NULL
        CHECK (previous_privacy_state IN ('ACTIVE', 'RESTRICTED', 'REDACTED', 'MERGED', 'NONE')),
    new_privacy_state text NOT NULL
        CHECK (new_privacy_state IN ('ACTIVE', 'RESTRICTED', 'REDACTED', 'MERGED', 'NONE')),
    processing_result text NOT NULL
        CHECK (processing_result IN ('APPLIED', 'NO_OP', 'STALE_IGNORED', 'BLOCKED', 'FAILED')),
    safe_reason_code text,
    source_version bigint NOT NULL CHECK (source_version > 0),
    processed_at timestamptz NOT NULL DEFAULT now(),
    correlation_id text,

    CONSTRAINT crm_privacy_history_profile_fk
        FOREIGN KEY (profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_privacy_history_projection_idx
    ON crm_privacy_processing_history (privacy_projection_id, processed_at DESC);
