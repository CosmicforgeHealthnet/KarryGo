-- Section 7: onboarding, authoritative verification projections and activation.

CREATE TABLE vendor_onboarding_profiles (
    business_id text PRIMARY KEY REFERENCES vendor_businesses(id),
    status text NOT NULL DEFAULT 'NOT_STARTED' CHECK (status IN
        ('NOT_STARTED','IN_PROGRESS','PENDING_VERIFICATION','PENDING_REVIEW','ACTION_REQUIRED','READY','ACTIVATED')),
    submitted_at timestamptz,
    activated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE TABLE vendor_authorized_representatives (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    staff_ref text NOT NULL,
    capacity text NOT NULL CHECK (capacity IN ('OWNER','DIRECTOR','AUTHORIZED_SIGNATORY')),
    first_name text NOT NULL CHECK (char_length(first_name) BETWEEN 1 AND 100),
    last_name text NOT NULL CHECK (char_length(last_name) BETWEEN 1 AND 100),
    email text NOT NULL CHECK (char_length(email) <= 254),
    phone text NOT NULL CHECK (char_length(phone) BETWEEN 7 AND 32),
    ownership_percent numeric(5,2) CHECK (ownership_percent IS NULL OR ownership_percent BETWEEN 0 AND 100),
    authorized boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (business_id),
    UNIQUE (id,business_id)
);

CREATE TABLE vendor_onboarding_documents (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    representative_id text,
    branch_id text,
    brand_id text,
    dimension text NOT NULL CHECK (dimension IN ('IDENTITY','BUSINESS','BANK','LOCATION','BRAND','FOOD_KITCHEN')),
    media_ref text NOT NULL CHECK (char_length(media_ref) BETWEEN 8 AND 512),
    document_type text NOT NULL CHECK (char_length(document_type) BETWEEN 2 AND 80),
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SUBMITTED','SUPERSEDED')),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_onboarding_document_representative_scope_fk FOREIGN KEY (representative_id,business_id) REFERENCES vendor_authorized_representatives(id,business_id),
    CONSTRAINT vendor_onboarding_document_branch_scope_fk FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    CONSTRAINT vendor_onboarding_document_brand_scope_fk FOREIGN KEY (brand_id,business_id) REFERENCES vendor_brands(id,business_id)
);
CREATE INDEX vendor_onboarding_documents_scope_idx ON vendor_onboarding_documents(business_id,dimension,status);

CREATE TABLE vendor_verification_projections (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    dimension text NOT NULL CHECK (dimension IN ('IDENTITY','BUSINESS','BANK','LOCATION','BRAND','FOOD_KITCHEN')),
    subject_type text NOT NULL CHECK (subject_type IN ('BUSINESS','REPRESENTATIVE','BRANCH','BRAND','CAPABILITY','BRANCH_CAPABILITY')),
    subject_ref text NOT NULL,
    source_service text NOT NULL CHECK (source_service IN ('verification-compliance-service','payment-wallet-service')),
    source_ref text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),
    status text NOT NULL CHECK (status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
    observed_at timestamptz NOT NULL,
    expires_at timestamptz,
    reason_code text,
    remediation text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (source_service,source_ref),
    UNIQUE (business_id,dimension,subject_type,subject_ref)
);
CREATE INDEX vendor_verification_projection_status_idx ON vendor_verification_projections(business_id,status,dimension);

CREATE TABLE vendor_verification_history (
    id text PRIMARY KEY,
    projection_id text NOT NULL REFERENCES vendor_verification_projections(id),
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    source_service text NOT NULL,
    source_ref text NOT NULL,
    source_version bigint NOT NULL,
    status text NOT NULL CHECK (status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
    observed_at timestamptz NOT NULL,
    expires_at timestamptz,
    reason_code text,
    remediation text,
    recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX vendor_verification_history_source_idx ON vendor_verification_history(source_service,source_ref,source_version,recorded_at);

CREATE TABLE vendor_compliance_requirements (
    id text PRIMARY KEY,
    requirement_code text NOT NULL UNIQUE,
    scope text NOT NULL CHECK (scope IN ('BUSINESS','CAPABILITY','BRANCH','BRANCH_CAPABILITY','BRAND')),
    capability_type text,
    dimension text,
    local_check text,
    mandatory boolean NOT NULL DEFAULT true,
    active boolean NOT NULL DEFAULT true,
    CHECK ((dimension IS NOT NULL) <> (local_check IS NOT NULL)),
    CHECK (dimension IS NULL OR dimension IN ('IDENTITY','BUSINESS','BANK','LOCATION','BRAND','FOOD_KITCHEN')),
    CHECK (capability_type IS NULL OR capability_type IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY','SHOP_GROCERY','MARKET','WHOLESALE'))
);

INSERT INTO vendor_compliance_requirements(id,requirement_code,scope,dimension,local_check) VALUES
 ('vreq_business_basics','BUSINESS_BASICS','BUSINESS',NULL,'BUSINESS_BASICS'),
 ('vreq_representative','AUTHORIZED_REPRESENTATIVE','BUSINESS',NULL,'AUTHORIZED_REPRESENTATIVE'),
 ('vreq_first_branch','FIRST_BRANCH','BUSINESS',NULL,'FIRST_BRANCH'),
 ('vreq_identity','IDENTITY_VERIFIED','BUSINESS','IDENTITY',NULL),
 ('vreq_business','BUSINESS_VERIFIED','BUSINESS','BUSINESS',NULL),
 ('vreq_bank','BANK_VERIFIED','BUSINESS','BANK',NULL),
 ('vreq_branch_location','BRANCH_LOCATION_VERIFIED','BRANCH','LOCATION',NULL);

INSERT INTO vendor_compliance_requirements(id,requirement_code,scope,capability_type,dimension) VALUES
 ('vreq_food_restaurant','FOOD_KITCHEN_RESTAURANT','BRANCH_CAPABILITY','RESTAURANT','FOOD_KITCHEN'),
 ('vreq_food_fast_food','FOOD_KITCHEN_FAST_FOOD','BRANCH_CAPABILITY','FAST_FOOD','FOOD_KITCHEN'),
 ('vreq_food_cloud_kitchen','FOOD_KITCHEN_CLOUD_KITCHEN','BRANCH_CAPABILITY','CLOUD_KITCHEN','FOOD_KITCHEN');

CREATE TABLE vendor_bank_profiles (
    business_id text PRIMARY KEY REFERENCES vendor_businesses(id),
    external_ref text NOT NULL,
    bank_code text NOT NULL,
    masked_account text NOT NULL,
    resolved_account_name text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),
    status text NOT NULL CHECK (status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE TABLE vendor_verification_operations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    dimension text NOT NULL CHECK (dimension IN ('IDENTITY','BUSINESS','BANK','LOCATION','BRAND','FOOD_KITCHEN')),
    subject_type text NOT NULL,
    subject_ref text NOT NULL,
    client_reference text NOT NULL UNIQUE,
    request_fingerprint text NOT NULL,
    document_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','UNKNOWN','COMPLETED','FAILED','DEAD_LETTER')),
    source_ref text,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);
CREATE INDEX vendor_verification_operations_work_idx ON vendor_verification_operations(status,next_attempt_at,lease_expires_at);

CREATE TABLE vendor_service_nonces (
    caller text NOT NULL,
    nonce text NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY(caller,nonce)
);
CREATE INDEX vendor_service_nonces_expiry_idx ON vendor_service_nonces(expires_at);
