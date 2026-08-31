-- Section 8C: vendor-local CRM interactions, immutable interaction revisions,
-- staff tasks and immutable task activity. These tables contain relationship
-- workflow data only: no canonical customer PII, notification delivery state,
-- support-case authority or payment data.

CREATE TABLE crm_interactions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    customer_profile_id text NOT NULL,
    original_profile_id text NOT NULL,
    branch_id text,
    interaction_type text NOT NULL CHECK (interaction_type IN
        ('NOTE','PHONE_CALL','IN_PERSON','EMAIL','SMS','IN_APP','SOCIAL',
         'SALES_FOLLOW_UP','SERVICE_FOLLOW_UP','FEEDBACK','ACCOUNT_REVIEW','OTHER')),
    direction text NOT NULL CHECK (direction IN ('INBOUND','OUTBOUND','INTERNAL','NOT_APPLICABLE')),
    outcome text NOT NULL DEFAULT 'NONE' CHECK (outcome IN
        ('NONE','SUCCESSFUL','NO_RESPONSE','FOLLOW_UP_REQUIRED','RESOLVED','UNRESOLVED',
         'CUSTOMER_DECLINED','VENDOR_DECLINED','ESCALATED','CANCELLED')),
    subject text NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 200),
    note_body text NOT NULL DEFAULT '' CHECK (char_length(note_body) <= 5000),
    visibility_scope text NOT NULL CHECK (visibility_scope IN ('BUSINESS','BRANCH')),
    occurred_at timestamptz NOT NULL,
    recorded_by_staff_id text NOT NULL,
    source text NOT NULL DEFAULT 'MANUAL' CHECK (source IN ('MANUAL')),
    display_suppressed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    UNIQUE (id, business_id, customer_profile_id),
    CONSTRAINT crm_interaction_scope_ck CHECK (
        (visibility_scope='BUSINESS' AND branch_id IS NULL)
        OR (visibility_scope='BRANCH' AND branch_id IS NOT NULL)
    ),
    CONSTRAINT crm_interaction_profile_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_interaction_original_profile_fk FOREIGN KEY (original_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_interaction_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id),
    CONSTRAINT crm_interaction_recorder_fk FOREIGN KEY (recorded_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX crm_interactions_customer_timeline_idx
    ON crm_interactions (business_id, customer_profile_id, occurred_at DESC, id DESC)
    WHERE display_suppressed=false;
CREATE INDEX crm_interactions_branch_timeline_idx
    ON crm_interactions (business_id, branch_id, occurred_at DESC, id DESC)
    WHERE display_suppressed=false;
CREATE INDEX crm_interactions_type_timeline_idx
    ON crm_interactions (business_id, interaction_type, occurred_at DESC, id DESC)
    WHERE display_suppressed=false;
CREATE INDEX crm_interactions_recorder_idx
    ON crm_interactions (business_id, recorded_by_staff_id, occurred_at DESC)
    WHERE display_suppressed=false;

-- Full snapshots make the CRM-visible history reconstructable. The initial
-- CREATE snapshot is revision 1; every correction appends the next version.
CREATE TABLE crm_interaction_revisions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    interaction_id text NOT NULL,
    revision_version bigint NOT NULL CHECK (revision_version > 0),
    subject text NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 200),
    note_body text NOT NULL DEFAULT '' CHECK (char_length(note_body) <= 5000),
    outcome text NOT NULL CHECK (outcome IN
        ('NONE','SUCCESSFUL','NO_RESPONSE','FOLLOW_UP_REQUIRED','RESOLVED','UNRESOLVED',
         'CUSTOMER_DECLINED','VENDOR_DECLINED','ESCALATED','CANCELLED')),
    occurred_at timestamptz NOT NULL,
    correction_reason text NOT NULL DEFAULT '' CHECK (char_length(correction_reason) <= 500),
    changed_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, interaction_id, revision_version),
    CONSTRAINT crm_interaction_revision_parent_fk FOREIGN KEY (interaction_id, business_id)
        REFERENCES crm_interactions (id, business_id),
    CONSTRAINT crm_interaction_revision_actor_fk FOREIGN KEY (changed_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX crm_interaction_revisions_history_idx
    ON crm_interaction_revisions (business_id, interaction_id, revision_version DESC);

CREATE TABLE crm_tasks (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    customer_profile_id text NOT NULL,
    original_profile_id text NOT NULL,
    branch_id text,
    visibility_scope text NOT NULL CHECK (visibility_scope IN ('BUSINESS','BRANCH')),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','COMPLETED','CANCELLED')),
    priority text NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('LOW','NORMAL','HIGH','URGENT')),
    assigned_staff_id text,
    created_by_staff_id text NOT NULL,
    due_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,
    source_interaction_id text,
    display_suppressed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT crm_task_scope_ck CHECK (
        (visibility_scope='BUSINESS' AND branch_id IS NULL)
        OR (visibility_scope='BRANCH' AND branch_id IS NOT NULL)
    ),
    CONSTRAINT crm_task_terminal_time_ck CHECK (
        (status='COMPLETED' AND completed_at IS NOT NULL AND cancelled_at IS NULL)
        OR (status='CANCELLED' AND cancelled_at IS NOT NULL AND completed_at IS NULL)
        OR (status IN ('OPEN','IN_PROGRESS') AND completed_at IS NULL AND cancelled_at IS NULL)
    ),
    CONSTRAINT crm_task_profile_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_task_original_profile_fk FOREIGN KEY (original_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT crm_task_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id),
    CONSTRAINT crm_task_assignee_fk FOREIGN KEY (assigned_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id),
    CONSTRAINT crm_task_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id),
    CONSTRAINT crm_task_source_interaction_fk
        FOREIGN KEY (source_interaction_id, business_id, customer_profile_id)
        REFERENCES crm_interactions (id, business_id, customer_profile_id)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX crm_tasks_customer_status_idx
    ON crm_tasks (business_id, customer_profile_id, status, due_at, id)
    WHERE display_suppressed=false;
CREATE INDEX crm_tasks_branch_status_idx
    ON crm_tasks (business_id, branch_id, status, due_at, id)
    WHERE display_suppressed=false;
CREATE INDEX crm_tasks_assignee_status_idx
    ON crm_tasks (business_id, assigned_staff_id, status, due_at, id)
    WHERE display_suppressed=false;
CREATE INDEX crm_tasks_status_due_idx
    ON crm_tasks (business_id, status, due_at, id)
    WHERE display_suppressed=false AND status IN ('OPEN','IN_PROGRESS');
CREATE INDEX crm_tasks_branch_status_due_idx
    ON crm_tasks (business_id, branch_id, status, due_at, id)
    WHERE display_suppressed=false AND status IN ('OPEN','IN_PROGRESS');

CREATE TABLE crm_task_history (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    task_id text NOT NULL,
    action text NOT NULL CHECK (action IN
        ('CREATED','UPDATED','STARTED','COMPLETED','CANCELLED','REASSIGNED','PRIVACY_SUPPRESSED')),
    status text NOT NULL CHECK (status IN ('OPEN','IN_PROGRESS','COMPLETED','CANCELLED')),
    priority text NOT NULL CHECK (priority IN ('LOW','NORMAL','HIGH','URGENT')),
    assigned_staff_id text,
    due_at timestamptz,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    task_version bigint NOT NULL CHECK (task_version > 0),
    changed_by_staff_id text,
    occurred_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, task_id, task_version),
    CONSTRAINT crm_task_history_parent_fk FOREIGN KEY (task_id, business_id)
        REFERENCES crm_tasks (id, business_id),
    CONSTRAINT crm_task_history_assignee_fk FOREIGN KEY (assigned_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id),
    CONSTRAINT crm_task_history_actor_fk FOREIGN KEY (changed_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX crm_task_history_task_idx
    ON crm_task_history (business_id, task_id, task_version DESC);
