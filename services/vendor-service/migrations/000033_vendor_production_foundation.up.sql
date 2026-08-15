-- Sections 10 + 11: shared production platform.
--
-- ONE production/capacity engine, consumed by two first-class capabilities.
-- CLOUD_KITCHEN and PASTRY_BAKERY each get their own capacity POOLS and their
-- own domain workflows, but they share this reservation engine: two engines
-- would mean two definitions of "is there room", and the one that was wrong
-- would still be issuing confirmations.
--
-- Nothing here is an Order, a payment, a stock level or a delivery. A capacity
-- reservation says a branch has committed production capacity; it does not say
-- anyone bought anything.

-- ---------------------------------------------------------------------------
-- Capacity pools
-- ---------------------------------------------------------------------------
--
-- One pool is one constrained production resource: a hot line, the decoration
-- bench, the chiller, the packing table. Capacity is always an integer count of
-- a declared unit — there is no float quantity anywhere in this platform,
-- because "0.3 of a cake production unit" is not a thing a kitchen can make.
CREATE TABLE production_capacity_pools (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    code text NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,60}$'),
    scope_type text NOT NULL DEFAULT 'CAPABILITY'
        CHECK (scope_type IN ('BRANCH','CAPABILITY','PRODUCTION_LINE','ITEM_FAMILY','CUSTOM_CLASS')),
    scope_ref text CHECK (scope_ref IS NULL OR char_length(scope_ref) BETWEEN 1 AND 200),
    unit_type text NOT NULL
        CHECK (unit_type IN ('MEAL','ITEM','TRAY','PACKAGE','PRODUCTION_UNIT','WORKLOAD_UNIT')),
    -- The pool's baseline capacity per evaluation window. Capacity rules narrow
    -- it further; a slot may override it for one window.
    default_capacity integer NOT NULL CHECK (default_capacity >= 0 AND default_capacity <= 1000000),
    -- Buffers extend the window a reservation actually occupies, so a job that
    -- ends at 11:00 with a 15-minute cleanup keeps the resource until 11:15.
    setup_buffer_minutes integer NOT NULL DEFAULT 0 CHECK (setup_buffer_minutes BETWEEN 0 AND 480),
    cleanup_buffer_minutes integer NOT NULL DEFAULT 0 CHECK (cleanup_buffer_minutes BETWEEN 0 AND 480),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','PAUSED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_capacity_pools_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT production_capacity_pools_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id),
    UNIQUE (id, business_id, branch_id)
);

CREATE UNIQUE INDEX production_capacity_pools_live_code_unique
    ON production_capacity_pools (business_id, branch_id, code)
    WHERE status <> 'ARCHIVED';
CREATE INDEX production_capacity_pools_branch_capability_idx
    ON production_capacity_pools (business_id, branch_id, capability, status);

-- ---------------------------------------------------------------------------
-- Production lines
-- ---------------------------------------------------------------------------
--
-- Configuration and resources, not people. There is deliberately no staff or
-- machine model here: rostering is not this service's authority and modelling
-- it would invite scheduling decisions Vendor Service has no right to make.
CREATE TABLE production_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    code text NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,60}$'),
    production_class text NOT NULL DEFAULT 'GENERAL'
        CHECK (production_class IN ('GENERAL','HOT','COLD','BAKERY','DECORATION','PACKING','CHILLING')),
    capacity_pool_id text,
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','PAUSED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_lines_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT production_lines_pool_scope_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT production_lines_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX production_lines_live_code_unique
    ON production_lines (business_id, branch_id, code) WHERE status <> 'ARCHIVED';
CREATE INDEX production_lines_branch_idx
    ON production_lines (business_id, branch_id, capability, status);

-- ---------------------------------------------------------------------------
-- Capacity rules
-- ---------------------------------------------------------------------------
--
-- Rules NARROW a pool; they never widen it. Overlapping rules all apply, and
-- the binding constraint is the smallest — a branch cap of 1000, a capability
-- cap of 800 and a line cap of 400 mean 400, not "the most specific rule wins".
-- Specificity decides which rule is REPORTED as blocking, never which limit is
-- enforced, because a more specific rule that could raise a ceiling would be a
-- way to quietly overbook the broader resource.
CREATE TABLE production_capacity_rules (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    capacity_pool_id text NOT NULL,
    production_line_id text,
    item_family text CHECK (item_family IS NULL OR char_length(item_family) BETWEEN 1 AND 120),
    max_quantity integer NOT NULL CHECK (max_quantity >= 0 AND max_quantity <= 1000000),
    -- NULL days means every day; NULL times mean the whole day.
    days_of_week smallint[]
        CHECK (days_of_week IS NULL
               OR (array_length(days_of_week, 1) BETWEEN 1 AND 7
                   AND days_of_week <@ ARRAY[1,2,3,4,5,6,7]::smallint[])),
    start_local_time time,
    end_local_time time,
    effective_start_date date,
    effective_end_date date,
    -- Higher specificity is reported first when a rule blocks. It is a
    -- reporting order, not an override order.
    specificity integer NOT NULL DEFAULT 0 CHECK (specificity BETWEEN 0 AND 100),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_capacity_rules_pool_scope_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT production_capacity_rules_line_scope_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines (id, business_id),
    CONSTRAINT production_capacity_rules_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT production_capacity_rules_clock_pair_check
        CHECK ((start_local_time IS NULL) = (end_local_time IS NULL)),
    CONSTRAINT production_capacity_rules_date_range_check
        CHECK (effective_start_date IS NULL OR effective_end_date IS NULL
               OR effective_start_date <= effective_end_date),
    CONSTRAINT production_capacity_rules_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id)
);

CREATE INDEX production_capacity_rules_pool_idx
    ON production_capacity_rules (capacity_pool_id, status);

-- ---------------------------------------------------------------------------
-- Slot templates and slots
-- ---------------------------------------------------------------------------
--
-- Templates exist so an operations team declares "weekday mornings" once
-- instead of creating every window forever. Generated slots are concrete rows
-- with absolute timestamps, because a slot that only existed as a rule could
-- not carry a per-window capacity override or a blackout.
CREATE TABLE production_slot_templates (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    capacity_pool_id text NOT NULL,
    production_line_id text,
    label text NOT NULL CHECK (char_length(label) BETWEEN 2 AND 120),
    days_of_week smallint[] NOT NULL
        CHECK (array_length(days_of_week, 1) BETWEEN 1 AND 7
               AND days_of_week <@ ARRAY[1,2,3,4,5,6,7]::smallint[]),
    start_local_time time NOT NULL,
    end_local_time time NOT NULL,
    capacity_override integer
        CHECK (capacity_override IS NULL OR (capacity_override >= 0 AND capacity_override <= 1000000)),
    effective_start_date date,
    effective_end_date date,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_slot_templates_pool_scope_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT production_slot_templates_line_scope_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines (id, business_id),
    CONSTRAINT production_slot_templates_window_check
        CHECK (start_local_time <> end_local_time),
    CONSTRAINT production_slot_templates_date_range_check
        CHECK (effective_start_date IS NULL OR effective_end_date IS NULL
               OR effective_start_date <= effective_end_date),
    CONSTRAINT production_slot_templates_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id)
);

CREATE INDEX production_slot_templates_pool_idx
    ON production_slot_templates (business_id, capacity_pool_id, status);

CREATE TABLE production_slots (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    capacity_pool_id text NOT NULL,
    production_line_id text,
    template_id text,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    capacity_override integer
        CHECK (capacity_override IS NULL OR (capacity_override >= 0 AND capacity_override <= 1000000)),
    label text NOT NULL DEFAULT '' CHECK (char_length(label) <= 120),
    status text NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN','CLOSED','BLACKOUT','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_slots_pool_scope_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT production_slots_line_scope_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines (id, business_id),
    CONSTRAINT production_slots_template_scope_fk
        FOREIGN KEY (template_id, business_id) REFERENCES production_slot_templates (id, business_id),
    CONSTRAINT production_slots_window_check CHECK (end_at > start_at),
    UNIQUE (id, business_id)
);

CREATE INDEX production_slots_pool_window_idx
    ON production_slots (capacity_pool_id, start_at, end_at) WHERE status <> 'ARCHIVED';
CREATE INDEX production_slots_branch_window_idx
    ON production_slots (business_id, branch_id, start_at);
-- One generated slot per template per window; a regeneration run is then
-- idempotent rather than duplicating every window it has already produced.
CREATE UNIQUE INDEX production_slots_template_window_unique
    ON production_slots (template_id, start_at) WHERE template_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Blackouts
-- ---------------------------------------------------------------------------
--
-- A blackout closes a window after the fact. Reservations that already exist
-- stay visible and stay accounted for — deleting them would erase production
-- someone may already have started — but no new reservation may be taken
-- against a blacked-out window once the blackout commits.
CREATE TABLE production_blackouts (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text
        CHECK (capability IS NULL
               OR capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    capacity_pool_id text,
    production_line_id text,
    start_at timestamptz NOT NULL,
    end_at timestamptz NOT NULL,
    reason_code text NOT NULL
        CHECK (reason_code IN ('MAINTENANCE','STAFF_SHORTAGE','PRIVATE_PRODUCTION',
                               'HOLIDAY','EQUIPMENT_FAILURE','CLOSURE','OTHER')),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','CANCELLED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_blackouts_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT production_blackouts_pool_scope_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT production_blackouts_line_scope_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines (id, business_id),
    CONSTRAINT production_blackouts_window_check CHECK (end_at > start_at),
    UNIQUE (id, business_id)
);

CREATE INDEX production_blackouts_window_idx
    ON production_blackouts (business_id, branch_id, start_at, end_at) WHERE status = 'ACTIVE';
CREATE INDEX production_blackouts_pool_idx
    ON production_blackouts (capacity_pool_id, start_at, end_at) WHERE status = 'ACTIVE';

-- ---------------------------------------------------------------------------
-- Capacity reservations and allocations
-- ---------------------------------------------------------------------------
--
-- HELD and CONFIRMED consume capacity. RELEASED, CANCELLED and EXPIRED do not.
-- A HELD reservation stops consuming the moment its hold_expires_at passes,
-- decided by the authoritative clock at read time and NOT by a cleanup worker
-- having run: correctness that depends on a worker being on time is not
-- correctness.
CREATE TABLE capacity_reservations (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    source_type text NOT NULL
        CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                               'CUSTOM_CAKE_QUOTE','PRODUCTION_JOB','EXTERNAL_ORDER')),
    source_ref text CHECK (source_ref IS NULL OR char_length(source_ref) BETWEEN 1 AND 200),
    status text NOT NULL DEFAULT 'HELD'
        CHECK (status IN ('HELD','CONFIRMED','RELEASED','CANCELLED','EXPIRED')),
    hold_expires_at timestamptz,
    priority text NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('NORMAL','HIGH','URGENT')),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280),
    command_ref text CHECK (command_ref IS NULL OR char_length(command_ref) <= 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT capacity_reservations_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT capacity_reservations_window_check CHECK (window_end > window_start),
    -- A hold without an expiry would consume capacity forever while claiming to
    -- be temporary.
    CONSTRAINT capacity_reservations_hold_expiry_check
        CHECK (status <> 'HELD' OR hold_expires_at IS NOT NULL),
    UNIQUE (id, business_id)
);

CREATE INDEX capacity_reservations_window_idx
    ON capacity_reservations (business_id, branch_id, window_start, window_end);
CREATE INDEX capacity_reservations_source_idx
    ON capacity_reservations (business_id, source_type, source_ref);
CREATE INDEX capacity_reservations_hold_expiry_idx
    ON capacity_reservations (hold_expires_at) WHERE status = 'HELD';
CREATE INDEX capacity_reservations_active_idx
    ON capacity_reservations (business_id, status) WHERE status IN ('HELD','CONFIRMED');

CREATE TABLE capacity_reservation_allocations (
    id text PRIMARY KEY,
    reservation_id text NOT NULL,
    business_id text NOT NULL,
    capacity_pool_id text NOT NULL,
    quantity integer NOT NULL CHECK (quantity > 0 AND quantity <= 1000000),
    released_quantity integer NOT NULL DEFAULT 0
        CHECK (released_quantity >= 0 AND released_quantity <= 1000000),
    -- The window the resource is actually occupied for, buffers included. It is
    -- stored rather than recomputed so a later change to a pool's buffers never
    -- silently rewrites what an existing reservation was granted.
    effective_start timestamptz NOT NULL,
    effective_end timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capacity_reservation_allocations_reservation_fk
        FOREIGN KEY (reservation_id, business_id) REFERENCES capacity_reservations (id, business_id) ON DELETE CASCADE,
    CONSTRAINT capacity_reservation_allocations_pool_fk
        FOREIGN KEY (capacity_pool_id, business_id) REFERENCES production_capacity_pools (id, business_id),
    CONSTRAINT capacity_reservation_allocations_window_check
        CHECK (effective_end > effective_start),
    CONSTRAINT capacity_reservation_allocations_release_check
        CHECK (released_quantity <= quantity),
    -- One allocation row per pool per reservation, so "how much of this pool
    -- does this reservation hold" has exactly one answer.
    UNIQUE (reservation_id, capacity_pool_id)
);

CREATE INDEX capacity_reservation_allocations_pool_window_idx
    ON capacity_reservation_allocations (capacity_pool_id, effective_start, effective_end);

-- Append-only evidence of every allocation adjustment, so a partial release is
-- an event rather than an invisible edit of what was originally granted.
CREATE TABLE capacity_allocation_adjustments (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    reservation_id text NOT NULL,
    capacity_pool_id text NOT NULL,
    adjustment_type text NOT NULL
        CHECK (adjustment_type IN ('ALLOCATED','PARTIAL_RELEASE','FULL_RELEASE','RESCHEDULED')),
    previous_quantity integer NOT NULL CHECK (previous_quantity >= 0),
    new_quantity integer NOT NULL CHECK (new_quantity >= 0),
    actor_type text NOT NULL DEFAULT 'vendor_staff',
    actor_id text,
    correlation_id text,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capacity_allocation_adjustments_reservation_fk
        FOREIGN KEY (reservation_id, business_id) REFERENCES capacity_reservations (id, business_id) ON DELETE CASCADE
);

CREATE INDEX capacity_allocation_adjustments_reservation_idx
    ON capacity_allocation_adjustments (reservation_id, recorded_at DESC);

-- ---------------------------------------------------------------------------
-- Production jobs
-- ---------------------------------------------------------------------------
--
-- Vendor-owned operational work. This is NOT the canonical customer Order and
-- must never be read as one: it says the kitchen has work to do, not that a
-- customer bought something or paid for it.
CREATE TABLE production_jobs (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    reservation_id text,
    production_line_id text,
    source_type text NOT NULL
        CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                               'CUSTOM_CAKE_QUOTE','EXTERNAL_ORDER')),
    source_ref text CHECK (source_ref IS NULL OR char_length(source_ref) BETWEEN 1 AND 200),
    title text NOT NULL CHECK (char_length(title) BETWEEN 2 AND 200),
    status text NOT NULL DEFAULT 'PLANNED'
        CHECK (status IN ('PLANNED','QUEUED','IN_PROGRESS','BLOCKED','READY','COMPLETED','CANCELLED')),
    priority text NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('NORMAL','HIGH','URGENT')),
    planned_quantity integer NOT NULL DEFAULT 0 CHECK (planned_quantity >= 0),
    produced_quantity integer NOT NULL DEFAULT 0 CHECK (produced_quantity >= 0),
    waste_quantity integer NOT NULL DEFAULT 0 CHECK (waste_quantity >= 0),
    scheduled_start timestamptz,
    scheduled_end timestamptz,
    actual_start timestamptz,
    ready_at timestamptz,
    completed_at timestamptz,
    -- A PRODUCTION readiness window. It contains no travel time and is not a
    -- delivery estimate; naming it so would put a promise in front of a
    -- customer that this service has no authority to make.
    expected_ready_window_start timestamptz,
    expected_ready_window_end timestamptz,
    blocked_reason text NOT NULL DEFAULT '' CHECK (char_length(blocked_reason) <= 280),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_jobs_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT production_jobs_reservation_fk
        FOREIGN KEY (reservation_id, business_id) REFERENCES capacity_reservations (id, business_id),
    CONSTRAINT production_jobs_line_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines (id, business_id),
    CONSTRAINT production_jobs_schedule_check
        CHECK (scheduled_start IS NULL OR scheduled_end IS NULL OR scheduled_end > scheduled_start),
    CONSTRAINT production_jobs_ready_window_check
        CHECK (expected_ready_window_start IS NULL OR expected_ready_window_end IS NULL
               OR expected_ready_window_end >= expected_ready_window_start),
    UNIQUE (id, business_id)
);

CREATE INDEX production_jobs_queue_idx
    ON production_jobs (business_id, branch_id, status, scheduled_start);
CREATE INDEX production_jobs_source_idx
    ON production_jobs (business_id, source_type, source_ref);
CREATE INDEX production_jobs_reservation_idx
    ON production_jobs (reservation_id) WHERE reservation_id IS NOT NULL;

-- Append-only job history. A production timeline that could be edited would be
-- worthless the first time anyone needed it.
CREATE TABLE production_job_events (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    job_id text NOT NULL,
    event_type text NOT NULL
        CHECK (event_type IN ('CREATED','QUEUED','STARTED','BLOCKED','UNBLOCKED',
                              'READY','COMPLETED','CANCELLED','RESCHEDULED','QUANTITY_RECORDED')),
    from_status text,
    to_status text,
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280),
    actor_type text NOT NULL DEFAULT 'vendor_staff',
    actor_id text,
    correlation_id text,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT production_job_events_job_fk
        FOREIGN KEY (job_id, business_id) REFERENCES production_jobs (id, business_id) ON DELETE CASCADE
);

CREATE INDEX production_job_events_job_idx ON production_job_events (job_id, recorded_at DESC);

-- ---------------------------------------------------------------------------
-- Production calendar revisions
-- ---------------------------------------------------------------------------
--
-- The same monotonic-revision pattern Section 9B established for the food
-- catalogue, scoped to (business, branch, capability). Any committed change to
-- capacity, slots, blackouts or reservations advances it in the same
-- transaction, so a future consumer can tell that its view of the production
-- calendar is stale.
CREATE TABLE production_calendar_revisions (
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability text NOT NULL
        CHECK (capability IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, capability),
    CONSTRAINT production_calendar_revisions_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);
