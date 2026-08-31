-- Section 10: Cloud Kitchen commercial and recurring-production truth.
CREATE TABLE cloud_kitchen_settings (
    business_id text NOT NULL, branch_id text NOT NULL, bulk_order_threshold integer NOT NULL DEFAULT 20 CHECK (bulk_order_threshold > 0),
    minimum_lead_time_minutes integer NOT NULL DEFAULT 60 CHECK (minimum_lead_time_minutes >= 0), default_capacity_pool_ref text,
    reservation_hold_duration_minutes integer NOT NULL DEFAULT 30 CHECK (reservation_hold_duration_minutes > 0), corporate_reservation_horizon_days integer NOT NULL DEFAULT 90 CHECK (corporate_reservation_horizon_days > 0),
    version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (business_id, branch_id)
);
CREATE TABLE cloud_kitchen_packages (
    id text PRIMARY KEY, business_id text NOT NULL, branch_id text NOT NULL, name text NOT NULL, description text NOT NULL DEFAULT '', capability text NOT NULL DEFAULT 'CLOUD_KITCHEN' CHECK (capability='CLOUD_KITCHEN'),
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','ARCHIVED')), minimum_quantity integer NOT NULL CHECK (minimum_quantity>0), maximum_quantity integer CHECK (maximum_quantity IS NULL OR maximum_quantity>=minimum_quantity),
    capacity_pool_id text, production_class text, lead_time_minutes integer CHECK (lead_time_minutes IS NULL OR lead_time_minutes>=0), logistics jsonb NOT NULL DEFAULT '{}'::jsonb, version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), archived_at timestamptz
);
CREATE INDEX cloud_kitchen_packages_branch_idx ON cloud_kitchen_packages(business_id,branch_id,status);
CREATE TABLE cloud_kitchen_package_components (
    id text PRIMARY KEY, package_id text NOT NULL REFERENCES cloud_kitchen_packages(id) ON DELETE CASCADE, item_ref text NOT NULL, default_quantity integer NOT NULL CHECK(default_quantity>0), variant_ref text, position integer NOT NULL DEFAULT 0, UNIQUE(package_id,item_ref,variant_ref)
);
CREATE TABLE cloud_kitchen_quantity_bands (
    id text PRIMARY KEY, package_id text NOT NULL REFERENCES cloud_kitchen_packages(id) ON DELETE CASCADE, min_quantity integer NOT NULL CHECK(min_quantity>0), max_quantity integer CHECK(max_quantity IS NULL OR max_quantity>=min_quantity), unit_price_minor bigint NOT NULL CHECK(unit_price_minor>=0), package_price_minor bigint CHECK(package_price_minor IS NULL OR package_price_minor>=0), lead_time_minutes integer CHECK(lead_time_minutes IS NULL OR lead_time_minutes>=0), version bigint NOT NULL DEFAULT 1, UNIQUE(package_id,min_quantity)
);
CREATE TABLE cloud_kitchen_price_history (
    id text PRIMARY KEY, package_id text NOT NULL REFERENCES cloud_kitchen_packages(id), band_id text NOT NULL REFERENCES cloud_kitchen_quantity_bands(id), unit_price_minor bigint NOT NULL, package_price_minor bigint, version bigint NOT NULL, recorded_at timestamptz NOT NULL DEFAULT now(), UNIQUE(band_id,version)
);
CREATE TABLE cloud_kitchen_corporate_plans (
    id text PRIMARY KEY, business_id text NOT NULL, branch_id text NOT NULL, corporate_profile_ref text NOT NULL, name text NOT NULL, package_id text NOT NULL REFERENCES cloud_kitchen_packages(id), quantity integer NOT NULL CHECK(quantity>0), recurrence_type text NOT NULL CHECK(recurrence_type IN ('DAILY','WEEKLY','SELECTED_WEEKDAYS')), weekdays smallint[] NOT NULL DEFAULT '{}', timezone text NOT NULL, local_time time NOT NULL, effective_from date NOT NULL, effective_until date, production_minutes integer NOT NULL DEFAULT 60 CHECK(production_minutes>0), lead_time_minutes integer, status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','ACTIVE','SUSPENDED','ARCHIVED')), version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(business_id,branch_id,name)
);
CREATE TABLE cloud_kitchen_plan_exceptions (
    id text PRIMARY KEY, plan_id text NOT NULL REFERENCES cloud_kitchen_corporate_plans(id) ON DELETE CASCADE, occurrence_date date NOT NULL, action text NOT NULL CHECK(action IN ('SKIP','RESCHEDULE','QUANTITY_OVERRIDE','PACKAGE_OVERRIDE')), replacement_date date, quantity_override integer CHECK(quantity_override IS NULL OR quantity_override>0), package_override text REFERENCES cloud_kitchen_packages(id), UNIQUE(plan_id,occurrence_date)
);
CREATE TABLE cloud_kitchen_occurrences (
    id text PRIMARY KEY, plan_id text NOT NULL REFERENCES cloud_kitchen_corporate_plans(id), occurrence_key text NOT NULL, occurrence_date date NOT NULL, starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL, quantity integer NOT NULL CHECK(quantity>0), package_id text NOT NULL REFERENCES cloud_kitchen_packages(id), state text NOT NULL DEFAULT 'PLANNED' CHECK(state IN ('PLANNED','RESERVED','CAPACITY_CONFLICT','SKIPPED','CANCELLED','COMPLETED')), reservation_id text, conflict_reason text, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(plan_id,occurrence_key)
);
CREATE INDEX cloud_kitchen_occurrences_due_idx ON cloud_kitchen_occurrences(state,starts_at);
