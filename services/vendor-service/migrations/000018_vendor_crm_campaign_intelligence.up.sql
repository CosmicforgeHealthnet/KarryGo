-- Section 8E: campaign intelligence and the audience engine.
--
-- These tables answer WHO a campaign should target. They deliberately cannot
-- answer HOW or WHEN a message is delivered: there is no scheduled_at, no
-- template, no provider reference, no destination address, no delivery state
-- and no send attempt anywhere in this migration. Delivery is Section 8F and
-- belongs to notification-service; a column here that looked like a send would
-- make that boundary advisory rather than real.
--
-- They also hold no canonical customer PII. A customer is referenced only by
-- the vendor-local CRM profile id, exactly as Sections 8B, 8C and 8D do. There
-- is no email, phone, address or push-token column, so an audience snapshot
-- cannot become a contact-list export.

-- ---------------------------------------------------------------------------
-- crm_campaigns — the campaign aggregate
-- ---------------------------------------------------------------------------
-- The mutable head of a campaign. Everything that defines what the campaign
-- actually targets lives in the immutable version rows below; this table only
-- carries identity, lifecycle and the two pointers (current definition, current
-- snapshot) that say which of those immutable rows are live right now.
CREATE TABLE crm_campaigns (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 120),
    normalized_name text NOT NULL CHECK (char_length(normalized_name) BETWEEN 1 AND 120),

    -- Marketing objectives only. TRANSACTIONAL_PAYMENT, ORDER_STATUS and
    -- SECURITY_OTP are deliberately absent: those are operational messages that
    -- do not belong to a CRM marketing campaign, and modelling them here would
    -- invite a vendor to route a transactional message through a surface whose
    -- entire consent model is marketing consent.
    objective text NOT NULL DEFAULT 'GENERAL_MARKETING' CHECK (objective IN
        ('GENERAL_MARKETING', 'RETENTION', 'REACTIVATION', 'LOYALTY', 'ANNOUNCEMENT')),

    scope_type text NOT NULL CHECK (scope_type IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    -- The Section 8E lifecycle stops before scheduling. SCHEDULED, SENDING,
    -- SENT and DELIVERED are absent by design: a campaign this service can
    -- describe as "sent" would be a campaign this service claimed to deliver.
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN
        ('DRAFT', 'PREPARING', 'PREPARED', 'PREPARATION_FAILED', 'ARCHIVED')),

    current_definition_version bigint NOT NULL DEFAULT 1 CHECK (current_definition_version > 0),
    -- The last snapshot that completed successfully. It is NOT cleared when a
    -- later preparation fails: the last good audience stays usable, exactly as
    -- a segment keeps its last successful membership.
    current_snapshot_id text,

    created_by_staff_ref text,
    updated_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    UNIQUE (business_id, normalized_name),

    -- A BRANCH campaign names exactly one branch; a BUSINESS campaign names
    -- none. Without this a "business" campaign could carry a branch id that
    -- nothing enforced, and the scope check would become advisory.
    CONSTRAINT crm_campaign_scope_ck CHECK (
        (scope_type = 'BUSINESS' AND branch_id IS NULL)
        OR (scope_type = 'BRANCH' AND branch_id IS NOT NULL)
    ),
    CONSTRAINT crm_campaign_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT crm_campaign_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_campaigns_status_idx ON crm_campaigns (business_id, status, updated_at DESC, id);
CREATE INDEX crm_campaigns_branch_idx ON crm_campaigns (business_id, branch_id, status, updated_at DESC)
    WHERE branch_id IS NOT NULL;
CREATE INDEX crm_campaigns_objective_idx ON crm_campaigns (business_id, objective, updated_at DESC);

-- ---------------------------------------------------------------------------
-- crm_campaign_versions — the immutable definition
-- ---------------------------------------------------------------------------
-- One row per meaningful definition change. Rows are never updated: a snapshot
-- points at the exact version it was built from, so editing a prepared campaign
-- creates version N+1 and leaves N — and everything prepared against N —
-- untouched. That is what makes "no definition mixing" a property of the schema
-- rather than a rule the application has to remember.
CREATE TABLE crm_campaign_versions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    campaign_id text NOT NULL,
    definition_version bigint NOT NULL CHECK (definition_version > 0),

    name text NOT NULL,
    objective text NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('BUSINESS', 'BRANCH')),
    branch_id text,

    -- The channels this campaign intends to use. These are the Section 8A
    -- consent channels and nothing else; a channel outside that set could never
    -- have a consent decision projected for it, so a campaign could never
    -- lawfully target it.
    channels text[] NOT NULL CHECK (
        array_length(channels, 1) BETWEEN 1 AND 4
        AND channels <@ ARRAY['EMAIL', 'PUSH', 'IN_APP', 'SMS']::text[]
    ),

    -- Audience comes from the Section 8B segment engine and only from there.
    -- There is no rule column, no SQL column and no customer-list column: a
    -- second audience language would be a second place for the branch,
    -- privacy and consent walls to be got wrong.
    source_segment_id text NOT NULL,
    exclusion_segment_ids text[] NOT NULL DEFAULT '{}'::text[]
        CHECK (array_length(exclusion_segment_ids, 1) IS NULL OR array_length(exclusion_segment_ids, 1) <= 10),

    -- Frequency policy. Stored and immutable here; never enforced here, because
    -- enforcing it needs real delivery facts that only Section 8F will own.
    frequency_cap_count integer CHECK (frequency_cap_count IS NULL OR frequency_cap_count BETWEEN 1 AND 1000),
    frequency_cap_window_minutes integer
        CHECK (frequency_cap_window_minutes IS NULL OR frequency_cap_window_minutes BETWEEN 1 AND 525600),

    -- Quiet hours as minute-of-day plus a mandatory IANA zone. Minutes rather
    -- than a bare `time` because a wall-clock time with no zone is exactly the
    -- ambiguity this policy exists to remove, and because a cross-midnight
    -- window (22:00 -> 07:00) is then a plain integer comparison.
    quiet_hours_enabled boolean NOT NULL DEFAULT false,
    quiet_hours_start_minute smallint CHECK (quiet_hours_start_minute IS NULL OR quiet_hours_start_minute BETWEEN 0 AND 1439),
    quiet_hours_end_minute smallint CHECK (quiet_hours_end_minute IS NULL OR quiet_hours_end_minute BETWEEN 0 AND 1439),
    quiet_hours_timezone text CHECK (quiet_hours_timezone IS NULL OR char_length(quiet_hours_timezone) BETWEEN 1 AND 64),

    service_recovery_suppression_enabled boolean NOT NULL DEFAULT true,

    experiment_enabled boolean NOT NULL DEFAULT false,
    -- The allocation total is stored so the "exactly 10,000 basis points"
    -- invariant is asserted by the database on the same row that declares the
    -- experiment, not only by the code that wrote the variants.
    experiment_total_bps integer NOT NULL DEFAULT 0 CHECK (experiment_total_bps BETWEEN 0 AND 10000),

    definition_hash text NOT NULL,
    created_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    change_reason text CHECK (change_reason IS NULL OR char_length(change_reason) <= 500),

    UNIQUE (id, business_id),
    UNIQUE (business_id, campaign_id, definition_version),

    CONSTRAINT crm_campaign_version_scope_ck CHECK (
        (scope_type = 'BUSINESS' AND branch_id IS NULL)
        OR (scope_type = 'BRANCH' AND branch_id IS NOT NULL)
    ),
    -- An enabled quiet-hours policy without a zone is unevaluable, and a
    -- disabled one carrying a window would silently look enforced.
    CONSTRAINT crm_campaign_version_quiet_hours_ck CHECK (
        (quiet_hours_enabled = false
            AND quiet_hours_start_minute IS NULL AND quiet_hours_end_minute IS NULL AND quiet_hours_timezone IS NULL)
        OR (quiet_hours_enabled = true
            AND quiet_hours_start_minute IS NOT NULL AND quiet_hours_end_minute IS NOT NULL
            AND quiet_hours_timezone IS NOT NULL
            AND quiet_hours_start_minute <> quiet_hours_end_minute)
    ),
    -- A cap without a window (or a window without a cap) is half a policy.
    CONSTRAINT crm_campaign_version_frequency_ck CHECK (
        (frequency_cap_count IS NULL AND frequency_cap_window_minutes IS NULL)
        OR (frequency_cap_count IS NOT NULL AND frequency_cap_window_minutes IS NOT NULL)
    ),
    CONSTRAINT crm_campaign_version_experiment_ck CHECK (
        (experiment_enabled = false AND experiment_total_bps = 0)
        OR (experiment_enabled = true AND experiment_total_bps = 10000)
    ),
    CONSTRAINT crm_campaign_version_campaign_fk
        FOREIGN KEY (campaign_id, business_id) REFERENCES crm_campaigns (id, business_id),
    -- Tenant-safe segment binding: a campaign can only ever name a segment of
    -- its own business, enforced by the key rather than by a lookup.
    CONSTRAINT crm_campaign_version_segment_fk
        FOREIGN KEY (source_segment_id, business_id) REFERENCES crm_segments (id, business_id),
    CONSTRAINT crm_campaign_version_branch_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX crm_campaign_versions_campaign_idx
    ON crm_campaign_versions (business_id, campaign_id, definition_version DESC);

-- ---------------------------------------------------------------------------
-- crm_campaign_experiment_variants — control and treatment cohorts
-- ---------------------------------------------------------------------------
-- Allocation is in integer basis points. There is no percentage column and no
-- floating-point anywhere: 33.33% three ways does not sum to a whole, and an
-- allocation that does not sum exactly is an experiment whose control group
-- silently changes size between runs.
CREATE TABLE crm_campaign_experiment_variants (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    campaign_version_id text NOT NULL,

    variant_key text NOT NULL CHECK (variant_key ~ '^[A-Z0-9_]{1,32}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),
    allocation_bps integer NOT NULL CHECK (allocation_bps BETWEEN 0 AND 10000),
    is_control boolean NOT NULL DEFAULT false,
    -- Control is bounded at 50%: a "control" larger than the treatment stops
    -- being a holdout and becomes a way to quietly not run the campaign.
    CONSTRAINT crm_campaign_variant_control_share_ck CHECK (
        is_control = false OR allocation_bps <= 5000
    ),
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, campaign_version_id, variant_key),
    CONSTRAINT crm_campaign_variant_version_fk
        FOREIGN KEY (campaign_version_id, business_id) REFERENCES crm_campaign_versions (id, business_id) ON DELETE CASCADE
);

-- At most one control cohort per version.
CREATE UNIQUE INDEX crm_campaign_variant_single_control_idx
    ON crm_campaign_experiment_variants (business_id, campaign_version_id)
    WHERE is_control;

-- ---------------------------------------------------------------------------
-- crm_campaign_audience_snapshots — the immutable frozen audience
-- ---------------------------------------------------------------------------
-- This row is both the snapshot and its own durable preparation job, exactly as
-- crm_segment_evaluations is for Section 8B. One table rather than two means
-- the lease, the retry state and the audience it is building can never
-- disagree about which run they belong to.
CREATE TABLE crm_campaign_audience_snapshots (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    campaign_id text NOT NULL,
    -- The exact definition this audience was built from. A campaign edited
    -- mid-preparation moves to a new version; this pointer does not move with
    -- it, which is what stops two definitions being mixed into one audience.
    campaign_version_id text NOT NULL,
    campaign_definition_version bigint NOT NULL CHECK (campaign_definition_version > 0),

    source_segment_id text NOT NULL,
    source_segment_definition_version bigint NOT NULL CHECK (source_segment_definition_version > 0),
    -- NULL for a STATIC segment (membership is generation 0 and has no
    -- evaluation run). For a DYNAMIC segment this is the exact materialized
    -- generation, and the composite foreign key below pins it so Section 8B's
    -- generation cleanup cannot delete the rows out from under a live snapshot.
    source_segment_generation bigint CHECK (source_segment_generation IS NULL OR source_segment_generation > 0),
    source_segment_evaluation_id text,

    definition_hash text NOT NULL,
    -- Order-independent digest of the target set, so two snapshots of the same
    -- audience are recognisably the same audience without listing its members.
    audience_hash text,
    evaluation_as_of timestamptz NOT NULL,

    status text NOT NULL DEFAULT 'BUILDING'
        CHECK (status IN ('BUILDING', 'CURRENT', 'FAILED', 'SUPERSEDED')),

    -- Reconciled counts. Every one is non-negative and the funnel adds up:
    -- source - segment_exclusion - privacy - service_recovery = target_count,
    -- and control + treatment = target_count. Per-channel consent figures are
    -- derived from crm_campaign_target_channels rather than duplicated here, so
    -- they cannot drift from the rows they describe.
    source_count integer NOT NULL DEFAULT 0 CHECK (source_count >= 0),
    segment_exclusion_count integer NOT NULL DEFAULT 0 CHECK (segment_exclusion_count >= 0),
    privacy_suppressed_count integer NOT NULL DEFAULT 0 CHECK (privacy_suppressed_count >= 0),
    service_recovery_suppressed_count integer NOT NULL DEFAULT 0 CHECK (service_recovery_suppressed_count >= 0),
    -- Targets for whom no campaign channel was eligible at snapshot time.
    consent_suppressed_count integer NOT NULL DEFAULT 0 CHECK (consent_suppressed_count >= 0),
    target_count integer NOT NULL DEFAULT 0 CHECK (target_count >= 0),
    control_count integer NOT NULL DEFAULT 0 CHECK (control_count >= 0),
    treatment_count integer NOT NULL DEFAULT 0 CHECK (treatment_count >= 0),

    -- Durable preparation worker state, matching the Section 8B materializer.
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    lease_owner text,
    lease_expires_at timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    -- true once the run has exhausted its attempts. Kept separate from status
    -- so the four-value status vocabulary stays exactly what the audience is,
    -- never a mixture of what it is and how its worker is doing.
    dead_lettered boolean NOT NULL DEFAULT false,
    error_code text CHECK (error_code IS NULL OR char_length(error_code) <= 64),
    error_detail_safe text CHECK (error_detail_safe IS NULL OR char_length(error_detail_safe) <= 500),

    requested_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_campaign_snapshot_cohorts_ck CHECK (control_count + treatment_count = target_count),
    CONSTRAINT crm_campaign_snapshot_funnel_ck CHECK (
        source_count - segment_exclusion_count - privacy_suppressed_count
            - service_recovery_suppressed_count = target_count
    ),
    CONSTRAINT crm_campaign_snapshot_dead_letter_ck CHECK (dead_lettered = false OR status = 'FAILED'),
    CONSTRAINT crm_campaign_snapshot_campaign_fk
        FOREIGN KEY (campaign_id, business_id) REFERENCES crm_campaigns (id, business_id),
    CONSTRAINT crm_campaign_snapshot_version_fk
        FOREIGN KEY (campaign_version_id, business_id) REFERENCES crm_campaign_versions (id, business_id),
    CONSTRAINT crm_campaign_snapshot_segment_fk
        FOREIGN KEY (source_segment_id, business_id) REFERENCES crm_segments (id, business_id),
    -- Pins the exact source generation. With MATCH SIMPLE a NULL generation
    -- (a STATIC segment) skips the check, which is correct: there is no
    -- evaluation row to point at.
    CONSTRAINT crm_campaign_snapshot_generation_fk
        FOREIGN KEY (business_id, source_segment_id, source_segment_generation)
        REFERENCES crm_segment_evaluations (business_id, segment_id, generation)
);

-- At most one preparation in flight per campaign. This is the structural half
-- of duplicate-prepare suppression: two concurrent prepare requests cannot both
-- create a BUILDING snapshot, so the loser reads back the winner's run instead
-- of starting a second one.
CREATE UNIQUE INDEX crm_campaign_snapshots_single_building_idx
    ON crm_campaign_audience_snapshots (business_id, campaign_id)
    WHERE status = 'BUILDING';

CREATE INDEX crm_campaign_snapshots_campaign_idx
    ON crm_campaign_audience_snapshots (business_id, campaign_id, created_at DESC, id);
CREATE INDEX crm_campaign_snapshots_status_idx
    ON crm_campaign_audience_snapshots (business_id, campaign_id, status);
-- Worker claim index: due, unleased, still-building runs.
CREATE INDEX crm_campaign_snapshots_due_idx
    ON crm_campaign_audience_snapshots (next_attempt_at)
    WHERE status = 'BUILDING';
-- Generation retention lookup: "is any live snapshot still pinned to this
-- segment generation?"
CREATE INDEX crm_campaign_snapshots_retention_idx
    ON crm_campaign_audience_snapshots (business_id, source_segment_id, source_segment_generation)
    WHERE status IN ('BUILDING', 'CURRENT');

-- ---------------------------------------------------------------------------
-- crm_campaign_targets — one selected relationship inside one snapshot
-- ---------------------------------------------------------------------------
-- A target is a privacy-safe reference and an experiment assignment. It holds
-- no contact detail of any kind, because 8E never learns one.
--
-- Privacy- and service-recovery-suppressed relationships get NO row at all;
-- they are counted in the snapshot aggregate and nowhere else. That is a
-- deliberately stronger position than storing them with a suppressed state: a
-- customer who must not be contacted is absent from the only table Section 8F
-- will read, so no future query mistake can select them.
CREATE TABLE crm_campaign_targets (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    snapshot_id text NOT NULL,
    customer_profile_id text NOT NULL,

    experiment_variant_key text CHECK (experiment_variant_key IS NULL OR experiment_variant_key ~ '^[A-Z0-9_]{1,32}$'),
    is_control boolean NOT NULL DEFAULT false,
    -- The deterministic bucket this assignment came from, retained so an
    -- assignment can be re-derived and audited rather than taken on trust.
    experiment_bucket integer CHECK (experiment_bucket IS NULL OR experiment_bucket BETWEEN 0 AND 9999),

    -- Current actionability, which is NOT part of the immutable snapshot.
    -- A redaction or identity merge that lands after the snapshot flips this
    -- flag; the historical evidence above and in the channel rows is never
    -- rewritten. Separating the two is what lets the snapshot stay auditable
    -- while the customer stops being contactable immediately.
    current_actionable boolean NOT NULL DEFAULT true,
    suppression_reason_code text CHECK (suppression_reason_code IS NULL OR char_length(suppression_reason_code) <= 64),
    suppressed_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    -- One row per relationship per snapshot. This is also what makes a
    -- restarted preparation safe: a re-run of the same generation reproduces
    -- the same rows instead of duplicating them.
    UNIQUE (business_id, snapshot_id, customer_profile_id),
    CONSTRAINT crm_campaign_target_suppression_ck CHECK (
        (current_actionable = true AND suppression_reason_code IS NULL AND suppressed_at IS NULL)
        OR (current_actionable = false AND suppression_reason_code IS NOT NULL AND suppressed_at IS NOT NULL)
    ),
    CONSTRAINT crm_campaign_target_snapshot_fk
        FOREIGN KEY (snapshot_id, business_id) REFERENCES crm_campaign_audience_snapshots (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_campaign_target_profile_fk
        FOREIGN KEY (customer_profile_id, business_id) REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_campaign_targets_snapshot_idx
    ON crm_campaign_targets (business_id, snapshot_id, customer_profile_id);
CREATE INDEX crm_campaign_targets_cohort_idx
    ON crm_campaign_targets (business_id, snapshot_id, is_control, experiment_variant_key);
CREATE INDEX crm_campaign_targets_profile_idx
    ON crm_campaign_targets (business_id, customer_profile_id);

-- ---------------------------------------------------------------------------
-- crm_campaign_target_channels — snapshot-time consent evidence, per channel
-- ---------------------------------------------------------------------------
-- What the consent authority's projection said about this relationship and this
-- channel at the moment the audience was frozen. It is HISTORICAL EVIDENCE and
-- nothing more.
--
-- The state vocabulary is two values, not five. SUPPRESSED_PRIVACY and
-- SUPPRESSED_SERVICE_RECOVERY are unrepresentable rather than merely unused,
-- because a suppressed relationship has no target row for a channel row to hang
-- off. RECHECK_REQUIRED is not a state either: it is true for every row at
-- once, so it is the boolean below. And there is deliberately no
-- AUTHORIZED_TO_SEND: Section 8E cannot authorise a future delivery, and a
-- state that said otherwise would be a lie the schema told for us.
CREATE TABLE crm_campaign_target_channels (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    snapshot_id text NOT NULL,
    target_id text NOT NULL,
    channel text NOT NULL CHECK (channel IN ('EMAIL', 'PUSH', 'IN_APP', 'SMS')),

    snapshot_consent_decision text NOT NULL CHECK (snapshot_consent_decision IN ('ALLOW', 'DENY', 'UNKNOWN')),
    snapshot_consent_eligible boolean NOT NULL,
    consent_as_of timestamptz,
    consent_source_version bigint,

    snapshot_eligibility_state text NOT NULL
        CHECK (snapshot_eligibility_state IN ('ELIGIBLE_AT_SNAPSHOT', 'INELIGIBLE_CONSENT')),
    reason_code text NOT NULL CHECK (char_length(reason_code) <= 64),

    -- Always true, and the database will not accept anything else. Preparing an
    -- audience is not permission to contact it; Section 8F must revalidate
    -- current consent immediately before any delivery handoff.
    send_time_recheck_required boolean NOT NULL DEFAULT true
        CHECK (send_time_recheck_required = true),

    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, target_id, channel),
    -- An ALLOW that produced an ineligible channel, or a DENY that produced an
    -- eligible one, would mean the stored evidence and the stored decision
    -- disagree.
    CONSTRAINT crm_campaign_target_channel_state_ck CHECK (
        (snapshot_consent_eligible = true AND snapshot_eligibility_state = 'ELIGIBLE_AT_SNAPSHOT'
            AND snapshot_consent_decision = 'ALLOW')
        OR (snapshot_consent_eligible = false AND snapshot_eligibility_state = 'INELIGIBLE_CONSENT')
    ),
    CONSTRAINT crm_campaign_target_channel_target_fk
        FOREIGN KEY (target_id, business_id) REFERENCES crm_campaign_targets (id, business_id) ON DELETE CASCADE,
    CONSTRAINT crm_campaign_target_channel_snapshot_fk
        FOREIGN KEY (snapshot_id, business_id) REFERENCES crm_campaign_audience_snapshots (id, business_id) ON DELETE CASCADE
);

CREATE INDEX crm_campaign_target_channels_eligibility_idx
    ON crm_campaign_target_channels (business_id, snapshot_id, channel, snapshot_eligibility_state);

-- The current-snapshot pointer is added last, once the snapshot table exists.
ALTER TABLE crm_campaigns
    ADD CONSTRAINT crm_campaign_current_snapshot_fk
    FOREIGN KEY (current_snapshot_id, business_id)
    REFERENCES crm_campaign_audience_snapshots (id, business_id);
