-- Delivery requirement and vehicle selection evidence.
--
-- One immutable row per vehicle decision: the measurements it was taken from,
-- the capability policy in force, the operational reality at the time, every
-- class considered, and the class chosen with its reason.
--
-- WHY A TABLE AND NOT A COLUMN. commerce_fulfilment_plan_versions already has
-- routing_evidence, and a decision could have been buried in it. It is stored
-- separately because this evidence is READ BY PEOPLE, not only by the planner:
-- Support answers "why was this a van", operations counts how often the car
-- class is blocked by a disabled taxi integration, and both questions are
-- queries over reason codes and classes. A jsonb blob inside a plan row can be
-- read back; it cannot be counted without scanning every plan ever made.
--
-- WHAT THIS TABLE IS NOT. It is not a financial record. declared_value_minor is
-- risk metadata that may exclude a carrier class; it never becomes a fee, a
-- ledger entry or an insurance charge, and payment-wallet-service remains the
-- only authority for money. It is not a provider record either: nothing here
-- names a rider, a driver or a truck, because selecting a provider belongs to
-- the logistics services and selecting a REQUIREMENT belongs here.
--
-- APPEND-ONLY. A requirement that changes produces a new row against a new
-- plan version. Nothing here is ever updated, which is enforced below by
-- having no version column to bump and by the plan-scoped uniqueness: a plan
-- version is itself immutable, so a row bound to one cannot be rewritten
-- without rewriting history that other rows reference.

CREATE TABLE commerce_delivery_requirement_snapshots (
    id text PRIMARY KEY,

    group_id text NOT NULL
        REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,

    -- The plan version this decision belongs to. Every snapshot belongs to
    -- exactly one immutable plan version, which is what makes the snapshot
    -- immutable too.
    plan_id text NOT NULL
        REFERENCES commerce_fulfilment_plan_versions(id) ON DELETE RESTRICT,

    -- The mission slot the decision was taken for. Empty when the decision
    -- covers the group as a whole rather than one planned mission.
    slot_ref text NOT NULL DEFAULT '',

    business_id text NOT NULL,
    branch_id text NOT NULL,

    -- -----------------------------------------------------------------------
    -- What was decided
    -- -----------------------------------------------------------------------

    -- The smallest class that can PHYSICALLY carry the load, whether or not
    -- anything can execute it. CAR is a legitimate value here and is never a
    -- legitimate value in selected_vehicle_class: that difference is the whole
    -- point of storing both.
    minimum_physical_class text NOT NULL DEFAULT ''
        CHECK (minimum_physical_class IN ('', 'MOTORCYCLE','CAR','VAN','TRUCK')),

    -- The executable answer. Both halves are set together or neither is: a
    -- stored class with no service is a mission nobody owns.
    selected_vehicle_class text NOT NULL DEFAULT ''
        CHECK (selected_vehicle_class IN ('', 'MOTORCYCLE','VAN','TRUCK')),
    selected_service text NOT NULL DEFAULT ''
        CHECK (selected_service IN ('', 'DISPATCH','HAULING','VENDOR_OWN_DELIVERY')),

    executable boolean NOT NULL DEFAULT false,

    -- The controlled reason vocabulary. Constrained in the database and not
    -- only in Go, because a reason code nobody can count is not evidence, and
    -- the code is what Support reads and operations aggregates.
    primary_reason text NOT NULL
        CHECK (primary_reason IN (
            'VEHICLE_SELECTED',
            'NO_SUITABLE_VEHICLE',
            'NO_SUITABLE_EXECUTABLE_SERVICE',
            'TEMPERATURE_CONFLICT',
            'SEPARATION_CONFLICT',
            'MANUAL_REVIEW_REQUIRED',
            'METADATA_INCOMPLETE'
        )),

    -- Supporting codes, ordered and deduplicated by the engine so two
    -- identical decisions serialize identically.
    supporting_reasons jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(supporting_reasons) = 'array'),

    -- Every class the engine walked, with its own outcome. Stored so the
    -- ladder that was NOT taken is still readable: "why not a motorcycle" is
    -- asked far more often than "why a van".
    candidates jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(candidates) = 'array'),

    -- -----------------------------------------------------------------------
    -- What it was decided from
    -- -----------------------------------------------------------------------

    -- Which evidence the decision actually used, and where it came from.
    measurement_state text NOT NULL DEFAULT 'ESTIMATED'
        CHECK (measurement_state IN ('ESTIMATED','CONFIRMED')),
    measurement_source text NOT NULL DEFAULT ''
        CHECK (measurement_source IN (
            '', 'CATALOG_ESTIMATE','VENDOR_DECLARED',
            'PACKING_CONFIRMED','SYSTEM_CALCULATED'
        )),

    -- Canonical integer units throughout: grams, millilitres, millimetres,
    -- metres. A threshold compared in floating point behaves differently on
    -- either side of a rounding error, which is not acceptable for a safety
    -- limit.
    total_weight_grams bigint NOT NULL DEFAULT 0
        CHECK (total_weight_grams >= 0),
    total_volume_ml bigint NOT NULL DEFAULT 0
        CHECK (total_volume_ml >= 0),

    -- The volume actually compared against each class, after any
    -- non-stackable derate. Stored beside the raw total so the comparison is
    -- reproducible from this row alone.
    effective_volume_ml bigint NOT NULL DEFAULT 0
        CHECK (effective_volume_ml >= 0),

    longest_dimension_mm integer NOT NULL DEFAULT 0
        CHECK (longest_dimension_mm >= 0),
    package_count bigint NOT NULL DEFAULT 0
        CHECK (package_count >= 0),

    route_distance_meters bigint NOT NULL DEFAULT 0
        CHECK (route_distance_meters >= 0),
    pickup_count integer NOT NULL DEFAULT 1
        CHECK (pickup_count >= 0),

    risk_class text NOT NULL DEFAULT ''
        CHECK (risk_class IN ('', 'STANDARD','HIGH_VALUE')),

    -- Risk metadata only. See the table note: this never moves money.
    declared_value_minor bigint NOT NULL DEFAULT 0
        CHECK (declared_value_minor >= 0),
    declared_value_currency text NOT NULL DEFAULT ''
        CHECK (declared_value_currency = ''
               OR declared_value_currency ~ '^[A-Z]{3}$'),

    -- -----------------------------------------------------------------------
    -- The rules it was decided under
    -- -----------------------------------------------------------------------

    -- The version identifies the rules; the snapshot embeds them. Both are
    -- kept because a version string alone stops explaining anything the day
    -- its thresholds are edited out of the code.
    policy_version text NOT NULL,

    -- The complete evidence document: requirement, policy, serviceability and
    -- decision. It is the replay input - a stored decision must be
    -- reproducible with no live service reachable.
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(evidence) = 'object'),

    -- Stable digest of the decision-bearing content, excluding identifiers and
    -- timestamps. Two rows with the same fingerprint are the same decision,
    -- which is how a replan detects that nothing operationally changed and
    -- avoids minting a plan version that says nothing.
    fingerprint text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    -- One decision per mission slot per plan version. A second decision for
    -- the same slot is a new plan version, never an overwrite.
    UNIQUE (plan_id, slot_ref),
    UNIQUE (id, group_id),

    CONSTRAINT commerce_delivery_requirement_snapshots_plan_scope_fk
        FOREIGN KEY (plan_id, group_id)
        REFERENCES commerce_fulfilment_plan_versions(id, group_id),

    -- An executable decision names both halves of the answer; a refused one
    -- names neither. Enforced here and not only in Go, because a half-written
    -- decision is exactly what a crash between two updates would leave.
    CONSTRAINT commerce_delivery_requirement_snapshots_executable_chk
        CHECK (
            (executable
             AND selected_vehicle_class <> ''
             AND selected_service <> ''
             AND primary_reason = 'VEHICLE_SELECTED')
            OR
            (NOT executable
             AND selected_vehicle_class = ''
             AND selected_service = ''
             AND primary_reason <> 'VEHICLE_SELECTED')
        ),

    -- The car class can be a physical requirement and can never be an
    -- executed one. driver-taxi-service has no goods-delivery mode, so a row
    -- claiming a car was executed would be a delivery nobody performed. The
    -- selected_vehicle_class CHECK above already excludes it; this states the
    -- rule where a reader will look for it.
    CONSTRAINT commerce_delivery_requirement_snapshots_no_car_execution_chk
        CHECK (selected_vehicle_class <> 'CAR'),

    -- A value without a currency cannot be compared to anything.
    CONSTRAINT commerce_delivery_requirement_snapshots_value_currency_chk
        CHECK (declared_value_minor = 0 OR declared_value_currency <> '')
);

-- Support answers "why was this order a van" from the group.
CREATE INDEX commerce_delivery_requirement_snapshots_group_idx
    ON commerce_delivery_requirement_snapshots(group_id, created_at DESC);

-- Operations counts refusals and escalations by reason and by class.
CREATE INDEX commerce_delivery_requirement_snapshots_reason_idx
    ON commerce_delivery_requirement_snapshots(primary_reason, created_at DESC);

-- "How often did the disabled taxi integration force a larger vehicle" is a
-- question about the gap between the physical requirement and what ran.
CREATE INDEX commerce_delivery_requirement_snapshots_class_idx
    ON commerce_delivery_requirement_snapshots(
        minimum_physical_class, selected_vehicle_class
    );

-- A replan asks whether the decision actually changed before writing a new
-- plan version.
CREATE INDEX commerce_delivery_requirement_snapshots_fingerprint_idx
    ON commerce_delivery_requirement_snapshots(group_id, fingerprint);
