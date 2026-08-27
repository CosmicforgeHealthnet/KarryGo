-- Fulfilment group, parcel and mission model.
--
-- This is the POST-ORDER operational fulfilment aggregate. It is deliberately
-- separate from commerce_fulfilment_plans, which is the PRE-ORDER candidate
-- plan bound to a checkout session: that plan answers "what would delivery
-- cost", this aggregate answers "what is actually being fulfilled, by whom,
-- in which packages, under which plan version".
--
-- Four invariants are carried by the schema itself rather than by handler
-- validation, because handler validation is not enforced against a concurrent
-- writer:
--
--   1. ONE COMMERCE ORDER HAS ONE ENDURING GROUP. Replanning, provider
--      reassignment and mission failure never create a second group.
--
--   2. PARCEL IDENTITY IS STABLE. A parcel row is created once and is never
--      deleted to simplify a workflow. Replan, reassignment, transfer and
--      return all keep the same parcel id.
--
--   3. PLAN VERSIONS ARE APPEND-ONLY AND MONOTONIC. unique(group_id, version)
--      makes two concurrent replans against v3 unable to both become v4; one
--      succeeds and the other sees a conflict.
--
--   4. A PLATFORM MISSION IS A PROJECTION. The canonical mission stays with
--      Dispatch/Hauling. This service records an OPAQUE external reference,
--      the authority that owns it, and a versioned status projection that a
--      late event cannot walk backwards.

-- -----------------------------------------------------------------------------
-- Fulfilment group: the enduring planning root for one commerce order.
-- -----------------------------------------------------------------------------

-- No status column by design. The group is an identity and a pointer to the
-- current plan; the commercial state of the purchase is already derived on
-- commerce_parent_orders and a second, separately-written copy of it could
-- only ever drift out of agreement with the first.
CREATE TABLE commerce_fulfilment_groups (
    id text PRIMARY KEY,

    parent_order_id text NOT NULL
        REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id text NOT NULL,

    -- The active plan version. Zero means planning has not produced a version
    -- yet, which is different from version 1 having been superseded.
    current_plan_id text,
    current_plan_version bigint NOT NULL DEFAULT 0
        CHECK (current_plan_version >= 0),

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- One group per commerce order. This is the constraint that makes
    -- "the group survives replanning" structural: a concurrent bootstrap
    -- cannot produce a second active group to plan against.
    UNIQUE (parent_order_id),

    -- Customer isolation carried by row shape, not only by query predicates.
    CONSTRAINT commerce_fulfilment_groups_order_scope_fk
        FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id),

    UNIQUE (id, parent_order_id),
    UNIQUE (id, customer_id),

    CONSTRAINT commerce_fulfilment_groups_plan_pointer_chk
        CHECK ((current_plan_id IS NULL) = (current_plan_version = 0))
);

CREATE INDEX commerce_fulfilment_groups_customer_idx
    ON commerce_fulfilment_groups(customer_id, created_at DESC, id);

-- -----------------------------------------------------------------------------
-- Fulfilment unit: one vendor order's pickup obligation at one branch.
-- -----------------------------------------------------------------------------

CREATE TABLE commerce_fulfilment_units (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    parent_order_id text NOT NULL,
    vendor_order_id text NOT NULL,

    -- Source authority. Resolved from the vendor order, never from a request
    -- body: a caller may not declare which vendor a unit belongs to.
    business_id text NOT NULL,
    branch_id text NOT NULL,

    pickup_address text NOT NULL DEFAULT '',
    pickup_latitude double precision NOT NULL DEFAULT 0,
    pickup_longitude double precision NOT NULL DEFAULT 0,

    service_class text NOT NULL DEFAULT '',
    vehicle_class text NOT NULL DEFAULT '',

    required_weight_grams bigint NOT NULL DEFAULT 0
        CHECK (required_weight_grams >= 0),
    required_package_count bigint NOT NULL DEFAULT 0
        CHECK (required_package_count >= 0),

    handling_requirements jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(handling_requirements) = 'array'),

    -- Readiness is operational evidence about the pickup obligation. It never
    -- settles or cancels the vendor order, which keeps this table unable to
    -- move money by itself.
    readiness text NOT NULL DEFAULT 'NOT_READY'
        CHECK (readiness IN ('NOT_READY','PACKING','READY','COLLECTED','CANCELLED')),

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- One obligation per vendor order. Two branches of the same business are
    -- two vendor orders and therefore two units; they are never merged just
    -- because the business id matches.
    UNIQUE (vendor_order_id),
    UNIQUE (group_id, vendor_order_id),
    UNIQUE (id, group_id),

    CONSTRAINT commerce_fulfilment_units_order_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id),

    CONSTRAINT commerce_fulfilment_units_group_scope_fk
        FOREIGN KEY (group_id, parent_order_id)
        REFERENCES commerce_fulfilment_groups(id, parent_order_id)
);

CREATE INDEX commerce_fulfilment_units_group_idx
    ON commerce_fulfilment_units(group_id, id);
CREATE INDEX commerce_fulfilment_units_branch_idx
    ON commerce_fulfilment_units(business_id, branch_id, readiness);

-- -----------------------------------------------------------------------------
-- Parcel: the stable, traceable physical package.
-- -----------------------------------------------------------------------------

-- A parcel is not an order line, not a vendor order, not a unit and not a
-- mission. It is the box that physically moves, and its id is the thing that
-- has to still mean something after a replan, a provider change, a transfer
-- and a return.
CREATE TABLE commerce_fulfilment_parcels (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    unit_id text NOT NULL
        REFERENCES commerce_fulfilment_units(id) ON DELETE RESTRICT,
    parent_order_id text NOT NULL,
    vendor_order_id text NOT NULL,

    -- Source authority, denormalised from the unit so a cross-vendor parcel
    -- claim cannot be constructed by pointing at another vendor's unit.
    business_id text NOT NULL,
    branch_id text NOT NULL,

    -- Operational reference shown to staff, riders and the customer.
    parcel_ref text NOT NULL UNIQUE,

    -- Ordinal within the unit, so one unit split into two boxes is A and B
    -- rather than two rows nobody can tell apart.
    sequence integer NOT NULL CHECK (sequence > 0),

    status text NOT NULL DEFAULT 'CREATED'
        CHECK (status IN (
            'CREATED','PACKED','READY','IN_TRANSIT','DELIVERED',
            'RETURN_REQUIRED','RETURNING','RETURNED','FAILED','CANCELLED'
        )),

    -- Current custodian is a projection derived from custody evidence, never
    -- typed in directly by a caller.
    custodian_type text NOT NULL DEFAULT 'VENDOR'
        CHECK (custodian_type IN ('VENDOR','PLATFORM_PROVIDER','CUSTOMER','NONE')),
    custodian_ref text NOT NULL DEFAULT '',

    -- What the parcel contained at handover. Frozen once custody moves: a
    -- later catalogue edit must not silently rewrite delivered history.
    manifest jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(manifest) = 'object'),
    manifest_version bigint NOT NULL DEFAULT 1 CHECK (manifest_version > 0),
    sealed_at timestamptz,

    weight_grams bigint NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
    length_mm bigint NOT NULL DEFAULT 0 CHECK (length_mm >= 0),
    width_mm bigint NOT NULL DEFAULT 0 CHECK (width_mm >= 0),
    height_mm bigint NOT NULL DEFAULT 0 CHECK (height_mm >= 0),
    fragile boolean NOT NULL DEFAULT false,
    temperature_control text NOT NULL DEFAULT 'NONE'
        CHECK (temperature_control IN ('NONE','CHILLED','FROZEN','HOT')),

    -- Stable operation identity for packing, so a retried worker does not
    -- create a second physical parcel.
    idempotency_key text NOT NULL,

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (unit_id, idempotency_key),
    UNIQUE (unit_id, sequence),
    UNIQUE (id, group_id),
    UNIQUE (id, business_id),

    CONSTRAINT commerce_fulfilment_parcels_unit_scope_fk
        FOREIGN KEY (unit_id, group_id)
        REFERENCES commerce_fulfilment_units(id, group_id),

    -- A sealed parcel must carry the manifest it was sealed with.
    CONSTRAINT commerce_fulfilment_parcels_sealed_chk
        CHECK (sealed_at IS NULL OR manifest <> '{}'::jsonb)
);

CREATE INDEX commerce_fulfilment_parcels_unit_idx
    ON commerce_fulfilment_parcels(unit_id, sequence);
CREATE INDEX commerce_fulfilment_parcels_group_idx
    ON commerce_fulfilment_parcels(group_id, id);
CREATE INDEX commerce_fulfilment_parcels_order_idx
    ON commerce_fulfilment_parcels(vendor_order_id, id);

-- -----------------------------------------------------------------------------
-- Fulfilment plan version: one immutable planning decision.
-- -----------------------------------------------------------------------------

CREATE TABLE commerce_fulfilment_plan_versions (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,

    version bigint NOT NULL CHECK (version > 0),

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','ACTIVE','SUPERSEDED','ABANDONED')),

    -- Why this version exists. INITIAL_PLAN for the first; every later one
    -- names the operational condition that forced a replan, reusing the
    -- delivery-policy overflow vocabulary rather than inventing a parallel set.
    reason_code text NOT NULL
        CHECK (reason_code IN (
            'INITIAL_PLAN',
            'OWN_RIDER_FIRST',
            'DISTANCE_OVERFLOW',
            'TIME_OVERFLOW',
            'CAPACITY_OVERFLOW',
            'VEHICLE_OVERFLOW',
            'SLA_OVERFLOW',
            'PROVIDER_UNAVAILABLE',
            'VENDOR_UNAVAILABLE',
            'CUSTOMER_APPROVED_CHANGE',
            'MISSION_REPLACED',
            'RETURN',
            'TRANSFER'
        )),
    reason_detail text NOT NULL DEFAULT '',

    -- The planner and policy that produced this decision, so a plan can be
    -- explained against the rules in force when it was made rather than the
    -- rules in force when someone later reads it.
    planner_version text NOT NULL,
    policy_version bigint NOT NULL DEFAULT 0 CHECK (policy_version >= 0),

    delivery_mode text NOT NULL DEFAULT 'PLATFORM'
        CHECK (delivery_mode IN ('PLATFORM','VENDOR','HYBRID')),

    -- Planned cost EVIDENCE, not a ledger. payment-wallet-service remains the
    -- financial authority; this is what the plan believed the fulfilment would
    -- cost, kept so a later financial question can be answered against the
    -- plan that was actually in force.
    planned_cost_minor bigint NOT NULL DEFAULT 0
        CHECK (planned_cost_minor >= 0),
    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
    cost_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(cost_snapshot) = 'object'),
    routing_evidence jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(routing_evidence) = 'object'),

    -- Stable digest of the planning semantics, used for stale-plan detection
    -- and replan idempotency.
    fingerprint text NOT NULL DEFAULT '',

    supersedes_version bigint
        CHECK (supersedes_version IS NULL OR supersedes_version > 0),

    -- Stable identity of the operation that produced this version, so a
    -- retried replan reads back its own version instead of minting a new one.
    idempotency_key text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz,
    superseded_at timestamptz,

    -- Two concurrent replans against v3 cannot both become v4.
    UNIQUE (group_id, version),
    UNIQUE (group_id, idempotency_key),
    UNIQUE (id, group_id),

    CONSTRAINT commerce_fulfilment_plan_versions_initial_chk
        CHECK (
            (version = 1 AND reason_code = 'INITIAL_PLAN' AND supersedes_version IS NULL)
            OR (version > 1 AND supersedes_version IS NOT NULL AND supersedes_version < version)
        ),

    CONSTRAINT commerce_fulfilment_plan_versions_active_chk
        CHECK (status <> 'ACTIVE' OR activated_at IS NOT NULL)
);

-- At most one active plan per group. A group whose pointer says v4 while v4
-- never durably activated is impossible, because activation and the pointer
-- move in one transaction.
CREATE UNIQUE INDEX commerce_fulfilment_plan_versions_one_active_idx
    ON commerce_fulfilment_plan_versions(group_id)
    WHERE status = 'ACTIVE';

CREATE INDEX commerce_fulfilment_plan_versions_history_idx
    ON commerce_fulfilment_plan_versions(group_id, version DESC);

ALTER TABLE commerce_fulfilment_groups
    ADD CONSTRAINT commerce_fulfilment_groups_current_plan_fk
    FOREIGN KEY (current_plan_id, id)
    REFERENCES commerce_fulfilment_plan_versions(id, group_id)
    DEFERRABLE INITIALLY DEFERRED;

-- -----------------------------------------------------------------------------
-- Plan mission slot: one planned mission inside one plan version.
-- -----------------------------------------------------------------------------

-- The slot is what makes mission creation idempotent. It is stable within a
-- plan version, so a recovery pass asks "does this slot already have a
-- mission" rather than "did I already send a request", which is a question
-- nothing durable can answer after a lost response.
CREATE TABLE commerce_fulfilment_plan_missions (
    id text PRIMARY KEY,

    plan_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_versions(id) ON DELETE RESTRICT,
    group_id text NOT NULL,

    slot_ref text NOT NULL,
    sequence integer NOT NULL CHECK (sequence > 0),

    -- Every planned mission has ONE executor at execution time. HYBRID is a
    -- planning behaviour that chooses between them; it is never itself an
    -- owner, which is why it is absent here.
    owner text NOT NULL CHECK (owner IN ('PLATFORM','VENDOR')),

    -- Controlled authority vocabulary. A caller can never widen this to an
    -- arbitrary service name.
    authority text NOT NULL
        CHECK (authority IN ('DISPATCH','HAULING','TAXI','VENDOR')),

    service_class text NOT NULL DEFAULT '',
    vehicle_class text NOT NULL DEFAULT '',

    planned_cost_minor bigint NOT NULL DEFAULT 0
        CHECK (planned_cost_minor >= 0),
    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),

    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (plan_id, slot_ref),
    UNIQUE (plan_id, sequence),
    UNIQUE (id, plan_id),
    UNIQUE (id, group_id),

    CONSTRAINT commerce_fulfilment_plan_missions_plan_scope_fk
        FOREIGN KEY (plan_id, group_id)
        REFERENCES commerce_fulfilment_plan_versions(id, group_id),

    -- A vendor-owned mission is executed by the vendor, and a platform
    -- mission never is. Collapsing the two would make "who is responsible"
    -- unanswerable at exactly the moment it matters.
    CONSTRAINT commerce_fulfilment_plan_missions_owner_authority_chk
        CHECK (
            (owner = 'VENDOR' AND authority = 'VENDOR')
            OR (owner = 'PLATFORM' AND authority IN ('DISPATCH','HAULING','TAXI'))
        )
);

CREATE INDEX commerce_fulfilment_plan_missions_plan_idx
    ON commerce_fulfilment_plan_missions(plan_id, sequence);

-- -----------------------------------------------------------------------------
-- Plan stop: the deterministic pickup/dropoff order of one mission slot.
-- -----------------------------------------------------------------------------

-- Persisted sequence, because unordered rows are not a route. A mission with
-- several compatible vendor pickups and one receiver destination is expressed
-- here as several PICKUP rows and exactly one DROPOFF row.
CREATE TABLE commerce_fulfilment_plan_stops (
    id text PRIMARY KEY,

    plan_mission_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,
    plan_id text NOT NULL,

    sequence integer NOT NULL CHECK (sequence > 0),
    stop_type text NOT NULL CHECK (stop_type IN ('PICKUP','DROPOFF')),

    unit_id text REFERENCES commerce_fulfilment_units(id) ON DELETE RESTRICT,
    business_id text NOT NULL DEFAULT '',
    branch_id text NOT NULL DEFAULT '',

    address text NOT NULL DEFAULT '',
    latitude double precision NOT NULL DEFAULT 0,
    longitude double precision NOT NULL DEFAULT 0,

    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (plan_mission_id, sequence),

    CONSTRAINT commerce_fulfilment_plan_stops_mission_scope_fk
        FOREIGN KEY (plan_mission_id, plan_id)
        REFERENCES commerce_fulfilment_plan_missions(id, plan_id),

    -- A pickup stop names the unit it collects; a dropoff stop does not.
    CONSTRAINT commerce_fulfilment_plan_stops_unit_chk
        CHECK ((stop_type = 'PICKUP') = (unit_id IS NOT NULL))
);

-- Exactly one receiver destination per mission slot.
CREATE UNIQUE INDEX commerce_fulfilment_plan_stops_one_dropoff_idx
    ON commerce_fulfilment_plan_stops(plan_mission_id)
    WHERE stop_type = 'DROPOFF';

CREATE INDEX commerce_fulfilment_plan_stops_mission_idx
    ON commerce_fulfilment_plan_stops(plan_mission_id, sequence);

-- -----------------------------------------------------------------------------
-- Parcel assignment: which parcels ride which planned mission.
-- -----------------------------------------------------------------------------

-- Assignment is per plan version, so moving a parcel onto a different mission
-- is a new plan version rather than an edit that erases where it used to be.
-- Parcel identity is untouched by it.
CREATE TABLE commerce_fulfilment_plan_mission_parcels (
    plan_mission_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,
    parcel_id text NOT NULL
        REFERENCES commerce_fulfilment_parcels(id) ON DELETE RESTRICT,
    plan_id text NOT NULL,
    group_id text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (plan_mission_id, parcel_id),

    CONSTRAINT commerce_fulfilment_plan_mission_parcels_mission_scope_fk
        FOREIGN KEY (plan_mission_id, plan_id)
        REFERENCES commerce_fulfilment_plan_missions(id, plan_id),

    -- A mission slot may only carry parcels from its own group.
    CONSTRAINT commerce_fulfilment_plan_mission_parcels_parcel_scope_fk
        FOREIGN KEY (parcel_id, group_id)
        REFERENCES commerce_fulfilment_parcels(id, group_id),

    CONSTRAINT commerce_fulfilment_plan_mission_parcels_slot_scope_fk
        FOREIGN KEY (plan_mission_id, group_id)
        REFERENCES commerce_fulfilment_plan_missions(id, group_id)
);

-- A parcel rides at most one mission slot per plan version.
CREATE UNIQUE INDEX commerce_fulfilment_plan_mission_parcels_one_per_plan_idx
    ON commerce_fulfilment_plan_mission_parcels(plan_id, parcel_id);

CREATE INDEX commerce_fulfilment_plan_mission_parcels_parcel_idx
    ON commerce_fulfilment_plan_mission_parcels(parcel_id, plan_id);

-- -----------------------------------------------------------------------------
-- Platform mission reference: a projection of somebody else's mission.
-- -----------------------------------------------------------------------------

-- external_ref is OPAQUE. Nothing in this service parses meaning out of it;
-- the owning authority is named in its own column precisely so nobody is ever
-- tempted to infer it from the identifier's shape.
CREATE TABLE commerce_platform_mission_refs (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    plan_id text NOT NULL,
    plan_mission_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,

    authority text NOT NULL CHECK (authority IN ('DISPATCH','HAULING','TAXI')),

    -- Our own operation identity, and the idempotency key the authority keys
    -- on. Unique here and there means a retried creation can never dispatch a
    -- second vehicle for the same goods.
    mission_ref text NOT NULL UNIQUE,

    external_ref text NOT NULL DEFAULT '',

    link_state text NOT NULL DEFAULT 'REQUESTED'
        CHECK (link_state IN (
            'REQUESTED','LINKED','REFUSED','FAILED','SUPERSEDED','REPLACED'
        )),

    -- Cached operational status for vendor coordination. The canonical status
    -- lives with the owning authority; this copy always names the event that
    -- produced it and refuses to move backwards.
    status_projection text NOT NULL DEFAULT '',
    status_source_version bigint NOT NULL DEFAULT 0
        CHECK (status_source_version >= 0),
    status_source_ref text NOT NULL DEFAULT '',
    status_updated_at timestamptz,

    refusal_code text NOT NULL DEFAULT '',
    failure_reason text NOT NULL DEFAULT '',

    -- Mission replacement keeps both rows. Overwriting external_ref in place
    -- would erase the operational history the replacement is evidence of.
    replaces_id text REFERENCES commerce_platform_mission_refs(id) ON DELETE RESTRICT,
    replacement_reason text NOT NULL DEFAULT '',

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    linked_at timestamptz,

    UNIQUE (id, group_id),

    CONSTRAINT commerce_platform_mission_refs_slot_scope_fk
        FOREIGN KEY (plan_mission_id, plan_id)
        REFERENCES commerce_fulfilment_plan_missions(id, plan_id),

    CONSTRAINT commerce_platform_mission_refs_group_scope_fk
        FOREIGN KEY (plan_mission_id, group_id)
        REFERENCES commerce_fulfilment_plan_missions(id, group_id),

    CONSTRAINT commerce_platform_mission_refs_linked_chk
        CHECK (link_state <> 'LINKED' OR (length(btrim(external_ref)) > 0 AND linked_at IS NOT NULL)),

    CONSTRAINT commerce_platform_mission_refs_refused_chk
        CHECK (link_state <> 'REFUSED' OR length(btrim(refusal_code)) > 0),

    CONSTRAINT commerce_platform_mission_refs_status_chk
        CHECK ((status_source_version = 0) = (status_projection = ''))
);

-- One live canonical mission per plan mission slot. A superseded or replaced
-- reference stays readable but no longer occupies the slot.
CREATE UNIQUE INDEX commerce_platform_mission_refs_one_live_slot_idx
    ON commerce_platform_mission_refs(plan_mission_id)
    WHERE link_state IN ('REQUESTED','LINKED');

-- The same canonical mission is never linked twice under one authority.
CREATE UNIQUE INDEX commerce_platform_mission_refs_external_idx
    ON commerce_platform_mission_refs(authority, external_ref)
    WHERE external_ref <> '';

CREATE INDEX commerce_platform_mission_refs_group_idx
    ON commerce_platform_mission_refs(group_id, created_at DESC, id);

-- -----------------------------------------------------------------------------
-- Vendor mission: canonical ONLY when the vendor delivers.
-- -----------------------------------------------------------------------------

-- This is the one mission this service genuinely owns, and it exists because
-- the vendor itself is the executor. It is not a platform mission reference
-- and the two are deliberately different tables: collapsing them would make
-- "who owns this mission's truth" ambiguous.
--
-- The status vocabulary is the delivery feature's existing ExecutionState,
-- reused verbatim rather than re-invented, so vendor-owned execution has one
-- lifecycle across the service.
CREATE TABLE commerce_vendor_missions (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    plan_id text NOT NULL,
    plan_mission_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    -- The vendor's own rider, drawn from vendor staff. This service does not
    -- keep a second rider identity and a vendor rider is never a marketplace
    -- provider.
    assigned_staff_id text REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    assigned_at timestamptz,

    status text NOT NULL DEFAULT 'PLANNED'
        CHECK (status IN (
            'PLANNED','ASSIGNED','READY_FOR_PICKUP','CUSTODY_TRANSFERRED',
            'OUT_FOR_DELIVERY','DELIVERED','FAILED','RETURN_REQUIRED','RETURNED'
        )),

    -- Handover credential and proof references belong to the authority that
    -- issued them. This service records the reference, never the secret.
    handover_ref text NOT NULL DEFAULT '',
    proof_ref text NOT NULL DEFAULT '',

    failure_reason text NOT NULL DEFAULT '',

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (plan_mission_id),
    UNIQUE (id, group_id),
    UNIQUE (id, business_id),

    CONSTRAINT commerce_vendor_missions_slot_scope_fk
        FOREIGN KEY (plan_mission_id, plan_id)
        REFERENCES commerce_fulfilment_plan_missions(id, plan_id),

    CONSTRAINT commerce_vendor_missions_group_scope_fk
        FOREIGN KEY (plan_mission_id, group_id)
        REFERENCES commerce_fulfilment_plan_missions(id, group_id),

    CONSTRAINT commerce_vendor_missions_assigned_chk
        CHECK (status = 'PLANNED' OR assigned_staff_id IS NOT NULL),

    CONSTRAINT commerce_vendor_missions_assignment_time_chk
        CHECK ((assigned_staff_id IS NULL) = (assigned_at IS NULL))
);

CREATE INDEX commerce_vendor_missions_branch_idx
    ON commerce_vendor_missions(business_id, branch_id, status, created_at DESC);
CREATE INDEX commerce_vendor_missions_group_idx
    ON commerce_vendor_missions(group_id, id);
CREATE INDEX commerce_vendor_missions_rider_idx
    ON commerce_vendor_missions(assigned_staff_id, status)
    WHERE assigned_staff_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Parcel custody events: the append-only chain that makes a journey provable.
-- -----------------------------------------------------------------------------

-- Vendor Service never fabricates a logistics fact. A row whose source is a
-- logistics authority is written only from that authority's own signed,
-- version-stamped evidence, and the unique key on (parcel, source, event) is
-- what makes an at-least-once delivery safe to replay.
CREATE TABLE commerce_parcel_custody_events (
    id text PRIMARY KEY,

    parcel_id text NOT NULL
        REFERENCES commerce_fulfilment_parcels(id) ON DELETE RESTRICT,
    group_id text NOT NULL,

    event_type text NOT NULL
        CHECK (event_type IN (
            'CREATED','PACKED','READY','HANDED_OVER','TRANSFERRED',
            'DELIVERED','FAILED','RETURN_INITIATED','RETURNED'
        )),

    from_custodian_type text NOT NULL DEFAULT 'NONE'
        CHECK (from_custodian_type IN ('VENDOR','PLATFORM_PROVIDER','CUSTOMER','NONE')),
    from_custodian_ref text NOT NULL DEFAULT '',
    to_custodian_type text NOT NULL
        CHECK (to_custodian_type IN ('VENDOR','PLATFORM_PROVIDER','CUSTOMER','NONE')),
    to_custodian_ref text NOT NULL DEFAULT '',

    source text NOT NULL
        CHECK (source IN ('VENDOR','DISPATCH','HAULING','TAXI','PLATFORM')),
    source_event_id text NOT NULL,
    source_version bigint NOT NULL DEFAULT 0 CHECK (source_version >= 0),

    platform_mission_ref_id text
        REFERENCES commerce_platform_mission_refs(id) ON DELETE RESTRICT,
    vendor_mission_id text
        REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,

    handover_ref text NOT NULL DEFAULT '',
    proof_ref text NOT NULL DEFAULT '',
    reason_code text NOT NULL DEFAULT '',

    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (parcel_id, source, source_event_id),

    CONSTRAINT commerce_parcel_custody_events_parcel_scope_fk
        FOREIGN KEY (parcel_id, group_id)
        REFERENCES commerce_fulfilment_parcels(id, group_id),

    -- Exactly one executing mission, never both and never neither for an
    -- event that describes movement.
    CONSTRAINT commerce_parcel_custody_events_mission_chk
        CHECK (
            event_type IN ('CREATED','PACKED','READY')
            OR (platform_mission_ref_id IS NULL) <> (vendor_mission_id IS NULL)
        )
);

CREATE INDEX commerce_parcel_custody_events_parcel_idx
    ON commerce_parcel_custody_events(parcel_id, occurred_at, id);

-- -----------------------------------------------------------------------------
-- Return journey: a returned parcel is the SAME parcel.
-- -----------------------------------------------------------------------------

-- Return state is tracked beside the parcel rather than by creating a
-- replacement parcel identity, which is the whole point: a customer, a vendor
-- and a support agent must all still be looking at the same box.
CREATE TABLE commerce_parcel_returns (
    id text PRIMARY KEY,

    parcel_id text NOT NULL
        REFERENCES commerce_fulfilment_parcels(id) ON DELETE RESTRICT,
    group_id text NOT NULL,

    reason_code text NOT NULL,

    -- The mission that failed to deliver, and the mission carrying it back.
    -- A return may legitimately need a NEW logistics mission; the parcel it
    -- carries is not new.
    origin_platform_mission_ref_id text
        REFERENCES commerce_platform_mission_refs(id) ON DELETE RESTRICT,
    origin_vendor_mission_id text
        REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    return_platform_mission_ref_id text
        REFERENCES commerce_platform_mission_refs(id) ON DELETE RESTRICT,
    return_vendor_mission_id text
        REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,

    state text NOT NULL DEFAULT 'INITIATED'
        CHECK (state IN ('INITIATED','IN_RETURN','RETURNED','ABANDONED')),

    idempotency_key text NOT NULL,

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,

    UNIQUE (parcel_id, idempotency_key),

    CONSTRAINT commerce_parcel_returns_parcel_scope_fk
        FOREIGN KEY (parcel_id, group_id)
        REFERENCES commerce_fulfilment_parcels(id, group_id)
);

-- One open return per parcel.
CREATE UNIQUE INDEX commerce_parcel_returns_one_open_idx
    ON commerce_parcel_returns(parcel_id)
    WHERE state IN ('INITIATED','IN_RETURN');

CREATE INDEX commerce_parcel_returns_group_idx
    ON commerce_parcel_returns(group_id, created_at DESC, id);

-- -----------------------------------------------------------------------------
-- Permissions
-- -----------------------------------------------------------------------------

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
 ('perm_fulfilment_read', 'vendor.fulfilment.read',
  'Read fulfilment groups, parcels, plans and mission projections for an authorised branch.',
  'delivery', false),
 ('perm_fulfilment_parcel_manage', 'vendor.fulfilment.parcel.manage',
  'Create and amend parcels and manifests for an authorised branch before handover.',
  'delivery', false),
 ('perm_fulfilment_mission_manage', 'vendor.fulfilment.mission.manage',
  'Assign and execute vendor-owned delivery missions for an authorised branch.',
  'delivery', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_owner'), ('role_general_manager'), ('role_branch_manager')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN (
    'vendor.fulfilment.read',
    'vendor.fulfilment.parcel.manage',
    'vendor.fulfilment.mission.manage'
)
ON CONFLICT DO NOTHING;

-- A vendor rider reads its own work and executes the mission assigned to it.
-- It cannot repack a parcel or reassign a mission to somebody else.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_vendor_rider', p.id
FROM vendor_permissions p
WHERE p.code = 'vendor.fulfilment.read'
ON CONFLICT DO NOTHING;

-- -----------------------------------------------------------------------------
-- Comments
-- -----------------------------------------------------------------------------

COMMENT ON TABLE commerce_fulfilment_groups IS
'The enduring fulfilment planning root for one commerce order. Survives replanning, provider reassignment and mission failure; unique(parent_order_id) makes a second active group impossible.';

COMMENT ON TABLE commerce_fulfilment_parcels IS
'Stable, traceable physical package. Identity survives replan, reassignment, mission replacement, transfer and return; parcels are never deleted and recreated to simplify a workflow.';

COMMENT ON TABLE commerce_fulfilment_plan_versions IS
'Append-only fulfilment plan history. Replanning creates the next version; unique(group_id, version) prevents two concurrent replans from both becoming the same version.';

COMMENT ON TABLE commerce_platform_mission_refs IS
'PROJECTION ONLY. The canonical mission is owned by Dispatch/Hauling/Taxi; external_ref is opaque and status_projection is refused when it would regress a newer source version.';

COMMENT ON TABLE commerce_vendor_missions IS
'Canonical delivery mission for VENDOR-OWNED delivery only. Platform-executed delivery has a commerce_platform_mission_refs projection instead and never a row here.';

COMMENT ON TABLE commerce_parcel_custody_events IS
'Append-only custody chain. Logistics-sourced rows are written only from the owning authority evidence; unique(parcel_id, source, source_event_id) makes at-least-once replay safe.';
