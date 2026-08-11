-- Section 6: Vendor Business, Brands, Capabilities and Branches.
-- Section 5 intentionally used vendor_id/branch_id as opaque scope keys. The
-- business primary key below is that same authenticated vendor scope, making
-- the Section 5 -> Section 6 relationship additive and backward compatible.

CREATE UNIQUE INDEX IF NOT EXISTS vendor_staff_id_vendor_unique
    ON vendor_staff (id, vendor_id);

CREATE TABLE vendor_businesses (
    id text PRIMARY KEY,
    legal_name text NOT NULL CHECK (char_length(legal_name) BETWEEN 2 AND 200),
    trading_name text CHECK (trading_name IS NULL OR char_length(trading_name) BETWEEN 2 AND 200),
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','PENDING_VERIFICATION','PENDING_REVIEW','ACTIVE','PAUSED','SUSPENDED','CLOSED')),
    owner_ref text NOT NULL,
    support_email text,
    support_phone text,
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 3 AND 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_business_owner_scope_fk
        FOREIGN KEY (owner_ref, id) REFERENCES vendor_staff (id, vendor_id)
);

CREATE INDEX vendor_businesses_owner_idx ON vendor_businesses (owner_ref);
CREATE INDEX vendor_businesses_status_idx ON vendor_businesses (status, created_at DESC);

CREATE TABLE vendor_capabilities (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    capability_type text NOT NULL
        CHECK (capability_type IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY','SHOP_GROCERY','MARKET','WHOLESALE')),
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','PENDING_REVIEW','ACTIVE','PAUSED','SUSPENDED','REJECTED')),
    status_reason text,
    requested_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (business_id, capability_type),
    UNIQUE (id, business_id, capability_type)
);

CREATE INDEX vendor_capabilities_business_status_idx
    ON vendor_capabilities (business_id, status, capability_type);

CREATE TABLE vendor_brands (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    media_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(media_refs) = 'array'),
    verification_state text NOT NULL DEFAULT 'NOT_STARTED'
        CHECK (verification_state IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (id, business_id)
);

CREATE INDEX vendor_brands_business_name_idx ON vendor_brands (business_id, display_name);
CREATE INDEX vendor_brands_verification_idx ON vendor_brands (business_id, verification_state);

CREATE TABLE vendor_branches (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    brand_id text,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 2 AND 160),
    address_line1 text NOT NULL CHECK (char_length(address_line1) BETWEEN 3 AND 240),
    address_line2 text,
    city text NOT NULL CHECK (char_length(city) BETWEEN 2 AND 120),
    region text NOT NULL CHECK (char_length(region) BETWEEN 2 AND 120),
    postal_code text,
    country_code char(2) NOT NULL CHECK (country_code ~ '^[A-Z]{2}$'),
    latitude numeric(9,6) NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude numeric(9,6) NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 3 AND 100),
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','ACTIVE','PAUSED','SUSPENDED','CLOSED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (id, business_id),
    CONSTRAINT vendor_branch_brand_scope_fk
        FOREIGN KEY (brand_id, business_id) REFERENCES vendor_brands (id, business_id)
);

CREATE INDEX vendor_branches_business_status_idx ON vendor_branches (business_id, status, created_at DESC);
CREATE INDEX vendor_branches_brand_idx ON vendor_branches (business_id, brand_id) WHERE brand_id IS NOT NULL;

CREATE TABLE vendor_branch_hours (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    is_closed boolean NOT NULL DEFAULT false,
    opens_at time,
    closes_at time,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_branch_hours_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id) ON DELETE CASCADE,
    CONSTRAINT vendor_branch_hours_interval_check CHECK (
        (is_closed AND opens_at IS NULL AND closes_at IS NULL)
        OR (NOT is_closed AND opens_at IS NOT NULL AND closes_at IS NOT NULL AND opens_at < closes_at)
    ),
    UNIQUE (branch_id, weekday, opens_at)
);

CREATE INDEX vendor_branch_hours_lookup_idx ON vendor_branch_hours (business_id, branch_id, weekday, opens_at);

CREATE TABLE vendor_branch_service_areas (
    branch_id text PRIMARY KEY,
    business_id text NOT NULL,
    mode text NOT NULL DEFAULT 'RADIUS' CHECK (mode = 'RADIUS'),
    center_latitude numeric(9,6) NOT NULL CHECK (center_latitude BETWEEN -90 AND 90),
    center_longitude numeric(9,6) NOT NULL CHECK (center_longitude BETWEEN -180 AND 180),
    radius_meters integer NOT NULL CHECK (radius_meters BETWEEN 1 AND 200000),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_branch_service_area_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id) ON DELETE CASCADE
);

CREATE TABLE vendor_branch_settings (
    branch_id text PRIMARY KEY,
    business_id text NOT NULL,
    settings_schema_version integer NOT NULL DEFAULT 1 CHECK (settings_schema_version = 1),
    accepting_new_orders boolean NOT NULL DEFAULT false,
    pickup_enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_branch_settings_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id) ON DELETE CASCADE
);

CREATE TABLE vendor_branch_capabilities (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    vendor_capability_id text NOT NULL,
    capability_type text NOT NULL,
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','ACTIVE','PAUSED','SUSPENDED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT vendor_branch_capability_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id) ON DELETE CASCADE,
    CONSTRAINT vendor_branch_capability_business_scope_fk
        FOREIGN KEY (vendor_capability_id, business_id, capability_type)
        REFERENCES vendor_capabilities (id, business_id, capability_type),
    UNIQUE (branch_id, capability_type),
    UNIQUE (id, business_id)
);

CREATE INDEX vendor_branch_capabilities_lookup_idx
    ON vendor_branch_capabilities (business_id, branch_id, status, capability_type);
