-- Section 21 Prompt 2: tenant-scoped corporate recurring feeding.
-- Corporate identity remains CRM/Customer-owned; corporate_ref is an opaque
-- relationship reference and never a local identity authority.

CREATE TABLE commerce_recurring_schedules (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    requester_id text NOT NULL,
    corporate_ref text NOT NULL,
    branch_id text NOT NULL,
    weekdays smallint[] NOT NULL,
    meal_time time NOT NULL,
    timezone text NOT NULL,
    headcount bigint NOT NULL CHECK (headcount > 0 AND headcount <= 1000000),
    allowance_minor bigint NOT NULL CHECK (allowance_minor >= 0),
    destination_snapshot jsonb NOT NULL,
    menu_rotation jsonb NOT NULL DEFAULT '{}'::jsonb,
    production_requirement jsonb NOT NULL DEFAULT '{}'::jsonb,
    tentative_vehicle_class text
        CHECK (tentative_vehicle_class IS NULL OR tentative_vehicle_class IN ('MOTORCYCLE','VAN','TRUCK')),
    advance_horizon_days integer NOT NULL DEFAULT 14 CHECK (advance_horizon_days BETWEEN 1 AND 90),
    starts_on date NOT NULL,
    ends_on date,
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','PAUSED','CANCELLED','ENDED')),
    idempotency_key text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_on IS NULL OR ends_on >= starts_on),
    CHECK (cardinality(weekdays) > 0),
    CHECK (weekdays <@ ARRAY[1,2,3,4,5,6,7]::smallint[]),
    UNIQUE (business_id, idempotency_key)
);
CREATE INDEX commerce_recurring_schedules_due_idx
    ON commerce_recurring_schedules (status, starts_on, updated_at);
CREATE INDEX commerce_recurring_schedules_tenant_idx
    ON commerce_recurring_schedules (business_id, corporate_ref, status);

CREATE TABLE commerce_recurring_occurrences (
    id text PRIMARY KEY,
    schedule_id text NOT NULL REFERENCES commerce_recurring_schedules(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    occurrence_key text NOT NULL,
    occurrence_at timestamptz NOT NULL,
    local_date date NOT NULL,
    timezone text NOT NULL,
    headcount bigint NOT NULL CHECK (headcount > 0),
    allowance_minor bigint NOT NULL CHECK (allowance_minor >= 0),
    destination_snapshot jsonb NOT NULL,
    menu_snapshot jsonb NOT NULL,
    production_requirement jsonb NOT NULL DEFAULT '{}'::jsonb,
    capacity_reservation_id text,
    capacity_status text NOT NULL DEFAULT 'NOT_REQUIRED'
        CHECK (capacity_status IN ('NOT_REQUIRED','RESERVED','CONFLICT','RELEASED')),
    tentative_vehicle_class text
        CHECK (tentative_vehicle_class IS NULL OR tentative_vehicle_class IN ('MOTORCYCLE','VAN','TRUCK')),
    status text NOT NULL DEFAULT 'PROPOSED'
        CHECK (status IN ('PROPOSED','RESERVED','CONFIRMED','CANCELLED','EXPIRED')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, occurrence_key),
    CHECK ((capacity_status = 'RESERVED') = (capacity_reservation_id IS NOT NULL))
);
CREATE INDEX commerce_recurring_occurrences_due_idx
    ON commerce_recurring_occurrences (status, occurrence_at, id);
CREATE INDEX commerce_recurring_occurrences_tenant_idx
    ON commerce_recurring_occurrences (business_id, schedule_id, occurrence_at);

CREATE TABLE commerce_recurring_order_proposals (
    id text PRIMARY KEY,
    occurrence_id text NOT NULL UNIQUE REFERENCES commerce_recurring_occurrences(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    proposal_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','CONFIRMED','CANCELLED','EXPIRED')),
    quote_ref text,
    vendor_order_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
