-- Section 8G: vendor-owned corporate customer CRM.
--
-- A corporate customer is a Vendor's commercial relationship with an
-- organization: a company, school, hotel, hospital, NGO. It is emphatically NOT
-- a second KYB service, a billing account or an order surface. There is no
-- credit limit, no budget, no wallet reference, no invoice, no payment term and
-- no price agreement anywhere in this migration; a column here that looked like
-- one would make vendor-service a financial authority it is not.
--
-- These tables also hold no canonical individual PII. A corporate contact is a
-- reference to an existing vendor-local CRM profile plus a relationship role —
-- there is no email, phone, home address or date-of-birth column to copy one
-- into, which is what makes "we do not duplicate customer PII" a property of
-- the schema rather than a rule the application has to remember.

-- ---------------------------------------------------------------------------
-- corporate_customer_code_counters — per-tenant human reference allocation
-- ---------------------------------------------------------------------------
-- A per-business counter rather than a global sequence. A global sequence would
-- make CORP-000001 leak how many corporate accounts every OTHER tenant has
-- created, which is a cross-tenant disclosure hiding inside a convenience
-- feature.
CREATE TABLE corporate_customer_code_counters (
    business_id text PRIMARY KEY REFERENCES vendor_businesses (id),
    next_value bigint NOT NULL DEFAULT 1 CHECK (next_value > 0)
);

-- ---------------------------------------------------------------------------
-- corporate_customer_profiles — the organization relationship
-- ---------------------------------------------------------------------------
CREATE TABLE corporate_customer_profiles (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),

    -- Immutable, vendor-local, human-readable. Never reassigned.
    corporate_customer_code text NOT NULL CHECK (corporate_customer_code ~ '^CORP-[0-9]{6,12}$'),

    organization_name text NOT NULL CHECK (char_length(organization_name) BETWEEN 2 AND 250),
    -- Search key only. Deliberately NOT unique: two genuinely different
    -- organizations may legitimately share a name, and enforcing uniqueness
    -- would both block real relationships and turn a duplicate-name error into
    -- an oracle for accounts the caller cannot see.
    normalized_name text NOT NULL CHECK (char_length(normalized_name) BETWEEN 1 AND 250),
    display_name text CHECK (display_name IS NULL OR char_length(display_name) BETWEEN 1 AND 250),

    -- CRM classification only. This field says nothing about legal entity
    -- status, registration or verification; verification-compliance-service
    -- remains the authority for anything of that kind.
    organization_type text NOT NULL DEFAULT 'COMPANY' CHECK (organization_type IN
        ('COMPANY', 'INSTITUTION', 'SCHOOL', 'HOSPITAL', 'HOTEL', 'RESTAURANT',
         'RETAILER', 'NGO', 'ASSOCIATION', 'GOVERNMENT', 'OTHER')),

    status text NOT NULL DEFAULT 'PROSPECT' CHECK (status IN
        ('PROSPECT', 'ACTIVE', 'ON_HOLD', 'INACTIVE', 'ARCHIVED')),

    -- BUSINESS: the whole vendor business may work this account.
    -- BRANCH_SET: only the explicitly listed branches may. The two are
    -- different from the corporate customer's own physical locations, which
    -- live in corporate_customer_locations and mean something else entirely.
    scope_mode text NOT NULL DEFAULT 'BUSINESS' CHECK (scope_mode IN ('BUSINESS', 'BRANCH_SET')),

    -- The Vendor-side owner of the relationship. Not a contact at the customer.
    account_manager_staff_id text,

    relationship_started_at timestamptz,
    external_organization_ref text CHECK (external_organization_ref IS NULL OR char_length(external_organization_ref) BETWEEN 1 AND 200),

    created_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    UNIQUE (business_id, corporate_customer_code),
    CONSTRAINT corporate_profile_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT corporate_profile_manager_fk FOREIGN KEY (account_manager_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id),
    CONSTRAINT corporate_profile_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

-- A trusted external organization reference identifies one organization within
-- one vendor business. Partial, because most accounts will not carry one.
CREATE UNIQUE INDEX corporate_profiles_external_ref_uq
    ON corporate_customer_profiles (business_id, external_organization_ref)
    WHERE external_organization_ref IS NOT NULL;

CREATE INDEX corporate_profiles_status_idx
    ON corporate_customer_profiles (business_id, status, updated_at DESC, id);
CREATE INDEX corporate_profiles_name_idx
    ON corporate_customer_profiles (business_id, normalized_name, id);
CREATE INDEX corporate_profiles_manager_idx
    ON corporate_customer_profiles (business_id, account_manager_staff_id, updated_at DESC)
    WHERE account_manager_staff_id IS NOT NULL;
CREATE INDEX corporate_profiles_type_idx
    ON corporate_customer_profiles (business_id, organization_type, updated_at DESC);

-- ---------------------------------------------------------------------------
-- corporate_customer_branch_scopes — which Vendor branches may work an account
-- ---------------------------------------------------------------------------
-- Rows exist only for BRANCH_SET accounts. A BUSINESS-scope account carries no
-- rows at all, so "which branches" and "everybody" are structurally different
-- states rather than one being a special case of the other.
CREATE TABLE corporate_customer_branch_scopes (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,
    branch_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, corporate_customer_id, branch_id),
    CONSTRAINT corporate_branch_scope_profile_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id) ON DELETE CASCADE,
    CONSTRAINT corporate_branch_scope_branch_fk FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id)
);

CREATE INDEX corporate_branch_scopes_branch_idx
    ON corporate_customer_branch_scopes (business_id, branch_id, corporate_customer_id);

-- ---------------------------------------------------------------------------
-- corporate_customer_locations — the organization's own sites
-- ---------------------------------------------------------------------------
-- These are organizational relationship data: a head office, a campus, a
-- warehouse. They are NOT customer home addresses, NOT checkout delivery
-- addresses and NOT logistics mission addresses. A future order domain may
-- reference one or snapshot it at order time under its own authority; that
-- integration deliberately does not exist here.
--
-- Note the deliberate naming: location_type BRANCH means a branch OF THE
-- CUSTOMER'S organization and has nothing to do with a vendor_branches row.
CREATE TABLE corporate_customer_locations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,

    location_code text NOT NULL CHECK (char_length(location_code) BETWEEN 1 AND 100),
    normalized_location_code text NOT NULL CHECK (char_length(normalized_location_code) BETWEEN 1 AND 100),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    location_type text NOT NULL DEFAULT 'SITE' CHECK (location_type IN
        ('HEADQUARTERS', 'OFFICE', 'BRANCH', 'STORE', 'WAREHOUSE', 'SITE', 'CAMPUS', 'OTHER')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),

    -- Bounded discrete components, never a free-form JSON blob: an unbounded
    -- address field is where PII and unvalidated text arrive together.
    address_line1 text CHECK (address_line1 IS NULL OR char_length(address_line1) BETWEEN 1 AND 200),
    address_line2 text CHECK (address_line2 IS NULL OR char_length(address_line2) BETWEEN 1 AND 200),
    city text CHECK (city IS NULL OR char_length(city) BETWEEN 1 AND 120),
    state_region text CHECK (state_region IS NULL OR char_length(state_region) BETWEEN 1 AND 120),
    postal_code text CHECK (postal_code IS NULL OR char_length(postal_code) BETWEEN 1 AND 40),
    country_code text CHECK (country_code IS NULL OR country_code ~ '^[A-Z]{2}$'),
    timezone text CHECK (timezone IS NULL OR char_length(timezone) BETWEEN 1 AND 64),
    external_location_ref text CHECK (external_location_ref IS NULL OR char_length(external_location_ref) BETWEEN 1 AND 200),

    created_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    -- The three-column key is what lets every child reach this row while
    -- proving it belongs to the same corporate account, so cross-account
    -- linking is refused by PostgreSQL rather than by a lookup.
    UNIQUE (id, business_id, corporate_customer_id),
    UNIQUE (business_id, corporate_customer_id, normalized_location_code),
    CONSTRAINT corporate_location_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT corporate_location_profile_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id),
    CONSTRAINT corporate_location_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX corporate_locations_status_idx
    ON corporate_customer_locations (business_id, corporate_customer_id, status, name, id);

-- ---------------------------------------------------------------------------
-- corporate_customer_cost_centers — organizational grouping labels
-- ---------------------------------------------------------------------------
-- Labels, and only labels. There is deliberately no budget, credit limit,
-- spending authorization, invoice limit, wallet balance or currency column: a
-- cost centre that carried a number would be a finance feature wearing a CRM
-- name, and payment-wallet-service remains the sole monetary authority.
CREATE TABLE corporate_customer_cost_centers (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,

    code text NOT NULL CHECK (char_length(code) BETWEEN 1 AND 100),
    normalized_code text NOT NULL CHECK (char_length(normalized_code) BETWEEN 1 AND 100),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    corporate_location_id text,
    external_reference text CHECK (external_reference IS NULL OR char_length(external_reference) BETWEEN 1 AND 200),

    created_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    UNIQUE (id, business_id, corporate_customer_id),
    UNIQUE (business_id, corporate_customer_id, normalized_code),
    CONSTRAINT corporate_cost_center_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT corporate_cost_center_profile_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id),
    -- Carrying corporate_customer_id into the key makes a cost centre pointing
    -- at another account's location structurally impossible.
    CONSTRAINT corporate_cost_center_location_fk
        FOREIGN KEY (corporate_location_id, business_id, corporate_customer_id)
        REFERENCES corporate_customer_locations (id, business_id, corporate_customer_id),
    CONSTRAINT corporate_cost_center_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX corporate_cost_centers_status_idx
    ON corporate_customer_cost_centers (business_id, corporate_customer_id, status, normalized_code, id);
CREATE INDEX corporate_cost_centers_location_idx
    ON corporate_customer_cost_centers (business_id, corporate_location_id)
    WHERE corporate_location_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- corporate_customer_contacts — an individual's role at the organization
-- ---------------------------------------------------------------------------
-- A contact is a REFERENCE to an existing vendor-local CRM profile plus a
-- relationship role. There is no name, email, phone, address or identity column
-- here, so Corporate CRM cannot become a second copy of customer identity even
-- by accident. customer-service remains the identity authority.
--
-- relationship_role describes the person's role AT THE CUSTOMER organization.
-- It is not a Vendor IAM role and is never consulted for authorization.
CREATE TABLE corporate_customer_contacts (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,

    customer_profile_id text NOT NULL,
    -- The profile this contact was originally created against, retained across
    -- an identity merge so the historical relationship stays explicable.
    original_profile_id text NOT NULL,

    relationship_role text NOT NULL DEFAULT 'OTHER' CHECK (relationship_role IN
        ('PRIMARY_CONTACT', 'PROCUREMENT', 'OPERATIONS', 'FINANCE',
         'MANAGEMENT', 'ADMINISTRATION', 'TECHNICAL', 'OTHER')),
    is_primary_contact boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),

    -- Set when the underlying individual is redacted. The organizational
    -- relationship survives; the person stops being actionable. Deleting the
    -- row instead would destroy corporate history that has nothing to do with
    -- the individual's erasure request.
    privacy_suppressed boolean NOT NULL DEFAULT false,

    created_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    UNIQUE (id, business_id, corporate_customer_id),
    CONSTRAINT corporate_contact_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT corporate_contact_profile_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id),
    CONSTRAINT corporate_contact_customer_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT corporate_contact_original_customer_fk FOREIGN KEY (original_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id),
    CONSTRAINT corporate_contact_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

-- One live relationship per person per account. This is also what makes an
-- identity merge safe: if both the source and the survivor are contacts of the
-- same account, the merge cannot produce two live rows for one human.
CREATE UNIQUE INDEX corporate_contacts_live_person_uq
    ON corporate_customer_contacts (business_id, corporate_customer_id, customer_profile_id)
    WHERE status <> 'ARCHIVED';

-- At most one primary contact per account, enforced by the database rather
-- than by a read-then-write check that two concurrent writers could both pass.
CREATE UNIQUE INDEX corporate_contacts_primary_uq
    ON corporate_customer_contacts (business_id, corporate_customer_id)
    WHERE is_primary_contact AND status = 'ACTIVE';

CREATE INDEX corporate_contacts_status_idx
    ON corporate_customer_contacts (business_id, corporate_customer_id, status, created_at DESC, id);
CREATE INDEX corporate_contacts_profile_idx
    ON corporate_customer_contacts (business_id, customer_profile_id, corporate_customer_id);

-- ---------------------------------------------------------------------------
-- corporate_customer_memberships — a contact's place inside the organization
-- ---------------------------------------------------------------------------
-- Relationship metadata only. APPROVER here records that somebody approves
-- things at their own company; it creates no approval workflow, no purchase
-- order gate and no Vendor authorization of any kind.
CREATE TABLE corporate_customer_memberships (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    corporate_customer_id text NOT NULL,
    corporate_contact_id text NOT NULL,
    corporate_location_id text,
    cost_center_id text,

    membership_role text NOT NULL DEFAULT 'MEMBER' CHECK (membership_role IN
        ('MEMBER', 'MANAGER', 'APPROVER', 'REQUESTER', 'PROCUREMENT', 'FINANCE', 'OPERATIONS', 'OTHER')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),

    created_by_staff_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    CONSTRAINT corporate_membership_archived_ck CHECK (
        (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        OR (status <> 'ARCHIVED' AND archived_at IS NULL)
    ),
    CONSTRAINT corporate_membership_profile_fk FOREIGN KEY (corporate_customer_id, business_id)
        REFERENCES corporate_customer_profiles (id, business_id),
    -- Every child key carries corporate_customer_id, so a membership binding a
    -- contact from account A to a location from account B cannot be written.
    CONSTRAINT corporate_membership_contact_fk
        FOREIGN KEY (corporate_contact_id, business_id, corporate_customer_id)
        REFERENCES corporate_customer_contacts (id, business_id, corporate_customer_id),
    CONSTRAINT corporate_membership_location_fk
        FOREIGN KEY (corporate_location_id, business_id, corporate_customer_id)
        REFERENCES corporate_customer_locations (id, business_id, corporate_customer_id),
    CONSTRAINT corporate_membership_cost_center_fk
        FOREIGN KEY (cost_center_id, business_id, corporate_customer_id)
        REFERENCES corporate_customer_cost_centers (id, business_id, corporate_customer_id),
    CONSTRAINT corporate_membership_creator_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id)
);

-- One live membership per (contact, location, cost centre, role). The COALESCE
-- keys are what make a duplicate detectable when the optional sides are null,
-- which is the common case.
CREATE UNIQUE INDEX corporate_memberships_live_uq
    ON corporate_customer_memberships (
        business_id, corporate_contact_id,
        COALESCE(corporate_location_id, ''), COALESCE(cost_center_id, ''), membership_role)
    WHERE status <> 'ARCHIVED';

CREATE INDEX corporate_memberships_contact_idx
    ON corporate_customer_memberships (business_id, corporate_customer_id, corporate_contact_id, status, id);
CREATE INDEX corporate_memberships_location_idx
    ON corporate_customer_memberships (business_id, corporate_location_id)
    WHERE corporate_location_id IS NOT NULL;
CREATE INDEX corporate_memberships_cost_center_idx
    ON corporate_customer_memberships (business_id, cost_center_id)
    WHERE cost_center_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Section 8C interactions and tasks — additive corporate subject support
-- ---------------------------------------------------------------------------
-- Corporate CRM reuses the accepted Section 8C interaction and task engine
-- rather than duplicating it. A second task state machine would be two places
-- for OPEN/IN_PROGRESS/COMPLETED/CANCELLED to disagree, and a second revision
-- model would be two definitions of what "immutable history" means.
--
-- The change is additive: a row now names EITHER an individual customer profile
-- OR a corporate customer, never both and never neither. Every existing Section
-- 8C row keeps its individual subject untouched, and the CHECK below is what
-- makes a subject-less or double-subject row unstorable.
ALTER TABLE crm_interactions ALTER COLUMN customer_profile_id DROP NOT NULL;
ALTER TABLE crm_interactions ALTER COLUMN original_profile_id DROP NOT NULL;
ALTER TABLE crm_interactions ADD COLUMN corporate_customer_id text;
ALTER TABLE crm_interactions ADD COLUMN corporate_contact_id text;
ALTER TABLE crm_interactions ADD COLUMN corporate_location_id text;
ALTER TABLE crm_interactions ADD COLUMN corporate_cost_center_id text;

ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_subject_ck CHECK (
    (customer_profile_id IS NOT NULL AND original_profile_id IS NOT NULL
        AND corporate_customer_id IS NULL AND corporate_contact_id IS NULL
        AND corporate_location_id IS NULL AND corporate_cost_center_id IS NULL)
    OR (customer_profile_id IS NULL AND original_profile_id IS NULL
        AND corporate_customer_id IS NOT NULL)
);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_corporate_fk
    FOREIGN KEY (corporate_customer_id, business_id)
    REFERENCES corporate_customer_profiles (id, business_id);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_corporate_contact_fk
    FOREIGN KEY (corporate_contact_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_contacts (id, business_id, corporate_customer_id);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_corporate_location_fk
    FOREIGN KEY (corporate_location_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_locations (id, business_id, corporate_customer_id);
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interaction_corporate_cost_center_fk
    FOREIGN KEY (corporate_cost_center_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_cost_centers (id, business_id, corporate_customer_id);
-- Mirrors the existing (id, business_id, customer_profile_id) key so a
-- corporate task's source interaction can be proven to belong to the same
-- corporate account.
ALTER TABLE crm_interactions ADD CONSTRAINT crm_interactions_corporate_subject_uq
    UNIQUE (id, business_id, corporate_customer_id);

CREATE INDEX crm_interactions_corporate_timeline_idx
    ON crm_interactions (business_id, corporate_customer_id, occurred_at DESC, id DESC)
    WHERE display_suppressed = false AND corporate_customer_id IS NOT NULL;

ALTER TABLE crm_tasks ALTER COLUMN customer_profile_id DROP NOT NULL;
ALTER TABLE crm_tasks ALTER COLUMN original_profile_id DROP NOT NULL;
ALTER TABLE crm_tasks ADD COLUMN corporate_customer_id text;
ALTER TABLE crm_tasks ADD COLUMN corporate_contact_id text;
ALTER TABLE crm_tasks ADD COLUMN corporate_location_id text;
ALTER TABLE crm_tasks ADD COLUMN corporate_cost_center_id text;

ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_subject_ck CHECK (
    (customer_profile_id IS NOT NULL AND original_profile_id IS NOT NULL
        AND corporate_customer_id IS NULL AND corporate_contact_id IS NULL
        AND corporate_location_id IS NULL AND corporate_cost_center_id IS NULL)
    OR (customer_profile_id IS NULL AND original_profile_id IS NULL
        AND corporate_customer_id IS NOT NULL)
);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_corporate_fk
    FOREIGN KEY (corporate_customer_id, business_id)
    REFERENCES corporate_customer_profiles (id, business_id);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_corporate_contact_fk
    FOREIGN KEY (corporate_contact_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_contacts (id, business_id, corporate_customer_id);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_corporate_location_fk
    FOREIGN KEY (corporate_location_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_locations (id, business_id, corporate_customer_id);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_corporate_cost_center_fk
    FOREIGN KEY (corporate_cost_center_id, business_id, corporate_customer_id)
    REFERENCES corporate_customer_cost_centers (id, business_id, corporate_customer_id);
ALTER TABLE crm_tasks ADD CONSTRAINT crm_task_corporate_source_interaction_fk
    FOREIGN KEY (source_interaction_id, business_id, corporate_customer_id)
    REFERENCES crm_interactions (id, business_id, corporate_customer_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX crm_tasks_corporate_status_idx
    ON crm_tasks (business_id, corporate_customer_id, status, due_at, id)
    WHERE display_suppressed = false AND corporate_customer_id IS NOT NULL;
