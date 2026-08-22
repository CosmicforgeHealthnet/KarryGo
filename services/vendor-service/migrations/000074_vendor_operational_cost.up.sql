-- Section 16D: operational cost policies, COGS calculation snapshots and
-- production cost evidence.
--
-- OPERATIONAL ANALYTICS ONLY. Nothing here is accounting. There is no ledger,
-- no journal, no settlement and no Wallet effect. Section 15 remains the sole
-- authority for purchase cost: vendor_purchase_cost_history is READ through a
-- port and is never copied into a second mutable source of truth. What this
-- schema owns is (a) the vendor's chosen costing policy and (b) immutable
-- evidence of calculations that were performed, so a historical figure can be
-- explained and reproduced rather than silently recomputed at today's prices.

-- ---------------------------------------------------------------------------
-- Cost policies
-- ---------------------------------------------------------------------------
--
-- Scope mirrors Section 16A's own layering: a business-level row is the
-- default, and an optional branch row overrides it. That is not a stylistic
-- choice - the ingredient -> inventory_item mapping itself lives on
-- vendor_ingredient_branch_configs, so the purchase evidence a policy resolves
-- against is already branch-specific.
--
-- subject_type keeps Packaging out of the Ingredient identity space: a policy
-- references either an ingredient or a packaging material, never both, and
-- never redefines master data that Section 16A owns.
CREATE TABLE vendor_cost_policies (
    id text PRIMARY KEY,

    business_id text NOT NULL,

    -- NULL means the business-level default. A non-NULL branch overrides it.
    branch_id text,

    subject_type text NOT NULL
        CHECK (subject_type IN ('INGREDIENT','PACKAGING')),
    subject_ref text NOT NULL
        CHECK (char_length(subject_ref) BETWEEN 1 AND 80),

    strategy text NOT NULL
        CHECK (
            strategy IN (
                'LATEST_ACCEPTED_PURCHASE_COST',
                'WEIGHTED_AVERAGE_PURCHASE_COST',
                'MANUAL_CONFIGURED_COST'
            )
        ),

    -- The currency the policy is willing to cost in. Purchase evidence in any
    -- other currency is not silently converted; it is excluded and the result
    -- is reported incomplete. There is no FX in this platform and this schema
    -- does not pretend otherwise.
    currency text NOT NULL
        CHECK (char_length(currency) = 3),

    -- Bounded weighted-average window. The weighting basis is accepted
    -- inventory quantity, over at most this many most-recent accepted receipts.
    -- Bounded by construction so a vendor with years of purchasing history
    -- cannot turn one cost read into an unbounded aggregation.
    weighted_average_receipt_limit integer NOT NULL DEFAULT 10
        CHECK (
            weighted_average_receipt_limit BETWEEN 1 AND 200
        ),

    -- Manual configured cost. Only consulted when strategy is
    -- MANUAL_CONFIGURED_COST; it can never silently outrank purchase evidence.
    manual_unit_cost_minor bigint
        CHECK (
            manual_unit_cost_minor IS NULL
            OR manual_unit_cost_minor >= 0
        ),
    manual_unit_code text
        CHECK (
            manual_unit_code IS NULL
            OR char_length(manual_unit_code) BETWEEN 1 AND 80
        ),
    manual_cost_reason text,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_by_staff_id text NOT NULL,
    updated_by_staff_id text NOT NULL,

    -- A manual strategy without a manual amount is not a policy, it is a hole
    -- that would later be papered over with a fabricated zero.
    CONSTRAINT vendor_cost_policies_manual_complete_ck
        CHECK (
            strategy <> 'MANUAL_CONFIGURED_COST'
            OR (
                manual_unit_cost_minor IS NOT NULL
                AND manual_unit_code IS NOT NULL
            )
        ),

    CONSTRAINT vendor_cost_policies_business_fk
        FOREIGN KEY (business_id)
        REFERENCES vendor_businesses(id),

    CONSTRAINT vendor_cost_policies_branch_fk
        FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    UNIQUE (id,business_id)
);

-- One ACTIVE policy per subject per scope. Two partial indexes because
-- PostgreSQL treats NULL branch_id as distinct in a plain UNIQUE, which would
-- allow several competing business-level defaults.
CREATE UNIQUE INDEX vendor_cost_policies_business_scope_uq
    ON vendor_cost_policies (business_id,subject_type,subject_ref)
    WHERE branch_id IS NULL AND status = 'ACTIVE';

CREATE UNIQUE INDEX vendor_cost_policies_branch_scope_uq
    ON vendor_cost_policies (business_id,branch_id,subject_type,subject_ref)
    WHERE branch_id IS NOT NULL AND status = 'ACTIVE';

-- Resolution path: branch override first, then business default.
CREATE INDEX vendor_cost_policies_lookup_idx
    ON vendor_cost_policies (business_id,subject_type,subject_ref,status);

CREATE INDEX vendor_cost_policies_branch_idx
    ON vendor_cost_policies (business_id,branch_id,status);

-- ---------------------------------------------------------------------------
-- Cost calculation snapshots
-- ---------------------------------------------------------------------------
--
-- calculation_type separates a recalculable preview from historical evidence.
-- RECIPE_PREVIEW may be superseded freely. The other three are immutable once
-- written: a completed batch's cost must not drift because an ingredient got
-- more expensive afterwards.
CREATE TABLE vendor_cost_snapshots (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text,

    calculation_type text NOT NULL
        CHECK (
            calculation_type IN (
                'RECIPE_PREVIEW',
                'RECIPE_SNAPSHOT',
                'PRODUCTION_EXPECTED',
                'PRODUCTION_ACTUAL'
            )
        ),

    -- Bumped when the calculation algorithm itself changes, so an old snapshot
    -- is never mistaken for one produced by today's engine.
    calculation_version integer NOT NULL DEFAULT 1
        CHECK (calculation_version > 0),

    recipe_id text,
    recipe_version_id text,
    batch_id text,

    -- Recorded so a snapshot explains which policy produced it even after the
    -- policy is edited or archived.
    strategy text NOT NULL
        CHECK (
            strategy IN (
                'LATEST_ACCEPTED_PURCHASE_COST',
                'WEIGHTED_AVERAGE_PURCHASE_COST',
                'MANUAL_CONFIGURED_COST',
                'MIXED'
            )
        ),

    currency text NOT NULL
        CHECK (char_length(currency) = 3),

    -- COMPLETE means every component resolved. Anything else means the total
    -- is not the whole story, and the reason is on the component rows.
    completeness text NOT NULL
        CHECK (
            completeness IN (
                'COMPLETE',
                'INCOMPLETE',
                'UNKNOWN'
            )
        ),

    -- Totals in minor units. Nullable only when nothing could be resolved at
    -- all; a resolved-but-zero total is a real 0, never a stand-in for absence.
    total_cost_minor bigint
        CHECK (total_cost_minor IS NULL OR total_cost_minor >= 0),
    ingredient_cost_minor bigint
        CHECK (ingredient_cost_minor IS NULL OR ingredient_cost_minor >= 0),
    packaging_cost_minor bigint
        CHECK (packaging_cost_minor IS NULL OR packaging_cost_minor >= 0),
    waste_cost_minor bigint
        CHECK (waste_cost_minor IS NULL OR waste_cost_minor >= 0),

    -- Denominators for per-unit reads, carried so a later read need not guess
    -- what the batch or recipe yielded at calculation time.
    output_quantity_numerator bigint,
    output_quantity_denominator bigint
        CHECK (
            output_quantity_denominator IS NULL
            OR output_quantity_denominator > 0
        ),
    serving_count bigint
        CHECK (serving_count IS NULL OR serving_count >= 0),

    expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (expected_waste_bps >= 0 AND expected_waste_bps < 10000),

    calculated_at timestamptz NOT NULL DEFAULT now(),
    calculated_by_staff_id text NOT NULL,
    correlation_id text NOT NULL DEFAULT '',

    -- Idempotency for explicit recalculation commands: a retried command
    -- returns the original snapshot instead of writing a second one.
    command_ref text,

    CONSTRAINT vendor_cost_snapshots_business_fk
        FOREIGN KEY (business_id)
        REFERENCES vendor_businesses(id),

    CONSTRAINT vendor_cost_snapshots_branch_fk
        FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_cost_snapshots_recipe_fk
        FOREIGN KEY (recipe_id,business_id)
        REFERENCES vendor_recipes(id,business_id),

    CONSTRAINT vendor_cost_snapshots_recipe_version_fk
        FOREIGN KEY (recipe_version_id,business_id)
        REFERENCES vendor_recipe_versions(id,business_id),

    CONSTRAINT vendor_cost_snapshots_batch_fk
        FOREIGN KEY (batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id),

    -- A production calculation without a batch, or a recipe calculation
    -- without a recipe version, could not be reproduced.
    CONSTRAINT vendor_cost_snapshots_subject_ck
        CHECK (
            (
                calculation_type IN ('RECIPE_PREVIEW','RECIPE_SNAPSHOT')
                AND recipe_id IS NOT NULL
                AND recipe_version_id IS NOT NULL
            )
            OR (
                calculation_type IN ('PRODUCTION_EXPECTED','PRODUCTION_ACTUAL')
                AND batch_id IS NOT NULL
            )
        ),

    UNIQUE (id,business_id)
);

-- Exactly one immutable production cost record of each kind per batch. This is
-- the "no duplicate historical production-cost evidence" invariant, enforced by
-- the database rather than by hoping two concurrent finalizations do not race.
CREATE UNIQUE INDEX vendor_cost_snapshots_production_uq
    ON vendor_cost_snapshots (business_id,batch_id,calculation_type)
    WHERE batch_id IS NOT NULL
      AND calculation_type IN ('PRODUCTION_EXPECTED','PRODUCTION_ACTUAL');

-- Retry of an explicit recalculation command must not create a second row.
CREATE UNIQUE INDEX vendor_cost_snapshots_command_uq
    ON vendor_cost_snapshots (business_id,command_ref)
    WHERE command_ref IS NOT NULL;

-- Recipe cost history, newest first, bounded by recipe or version.
CREATE INDEX vendor_cost_snapshots_recipe_history_idx
    ON vendor_cost_snapshots (business_id,recipe_id,calculated_at DESC)
    WHERE recipe_id IS NOT NULL;

CREATE INDEX vendor_cost_snapshots_version_history_idx
    ON vendor_cost_snapshots (business_id,recipe_version_id,calculated_at DESC)
    WHERE recipe_version_id IS NOT NULL;

CREATE INDEX vendor_cost_snapshots_batch_idx
    ON vendor_cost_snapshots (business_id,batch_id,calculation_type)
    WHERE batch_id IS NOT NULL;

CREATE INDEX vendor_cost_snapshots_branch_idx
    ON vendor_cost_snapshots (business_id,branch_id,calculated_at DESC);

-- ---------------------------------------------------------------------------
-- Component-level cost evidence
-- ---------------------------------------------------------------------------
--
-- The point of this table is that a total is never presented without the
-- reasoning behind it. Each row records the quantity used, the exact cost
-- basis applied, where that basis came from, and what it produced - so a
-- figure from six months ago can be re-derived line by line.
CREATE TABLE vendor_cost_snapshot_components (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    snapshot_id text NOT NULL,

    line_number integer NOT NULL
        CHECK (line_number > 0),

    subject_type text NOT NULL
        CHECK (subject_type IN ('INGREDIENT','PACKAGING')),
    subject_ref text NOT NULL,

    -- The Section 16A branch mapping that identified the purchase evidence.
    inventory_item_id text,

    -- Exact rational quantity. No float money, and no float quantity either.
    quantity_numerator bigint NOT NULL
        CHECK (quantity_numerator >= 0),
    quantity_denominator bigint NOT NULL DEFAULT 1
        CHECK (quantity_denominator > 0),
    quantity_unit_code text NOT NULL DEFAULT '',

    -- Where the unit cost came from, and the evidence that proves it.
    cost_source text NOT NULL
        CHECK (
            cost_source IN (
                'LATEST_ACCEPTED_PURCHASE_COST',
                'WEIGHTED_AVERAGE_PURCHASE_COST',
                'MANUAL_CONFIGURED_COST',
                'UNRESOLVED'
            )
        ),

    -- For LATEST: the purchase cost history row. For WEIGHTED_AVERAGE: the
    -- most recent row in the window. For MANUAL: the policy row. Kept as a
    -- plain reference rather than a foreign key so that archiving a policy or
    -- correcting purchasing history can never rewrite or orphan settled
    -- historical evidence.
    source_evidence_ref text,
    source_receipt_count integer
        CHECK (source_receipt_count IS NULL OR source_receipt_count >= 0),

    -- Unit cost actually applied, in minor units per normalized inventory unit.
    unit_cost_minor bigint
        CHECK (unit_cost_minor IS NULL OR unit_cost_minor >= 0),
    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR char_length(currency) = 3),

    -- quantity x unit cost, rounded half-up to the minor unit exactly once, at
    -- the component boundary. Documented in internal/features/cost.
    component_cost_minor bigint
        CHECK (component_cost_minor IS NULL OR component_cost_minor >= 0),

    expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (expected_waste_bps >= 0 AND expected_waste_bps < 10000),

    -- The portion of component_cost_minor attributable to the expected-waste
    -- gross-up, so waste is visible rather than buried in the total.
    waste_cost_minor bigint
        CHECK (waste_cost_minor IS NULL OR waste_cost_minor >= 0),

    -- Why this line could not be costed, when it could not be.
    incomplete_reason text
        CHECK (
            incomplete_reason IS NULL
            OR incomplete_reason IN (
                'NO_PURCHASE_EVIDENCE',
                'NO_POLICY',
                'CURRENCY_MISMATCH',
                'NO_INVENTORY_MAPPING',
                'NO_MANUAL_COST',
                'UNIT_NOT_CONVERTIBLE'
            )
        ),

    CONSTRAINT vendor_cost_snapshot_components_snapshot_fk
        FOREIGN KEY (snapshot_id,business_id)
        REFERENCES vendor_cost_snapshots(id,business_id)
        ON DELETE CASCADE,

    -- A resolved line must carry its evidence; an unresolved line must say why.
    CONSTRAINT vendor_cost_snapshot_components_resolution_ck
        CHECK (
            (
                cost_source <> 'UNRESOLVED'
                AND unit_cost_minor IS NOT NULL
                AND component_cost_minor IS NOT NULL
                AND currency <> ''
                AND incomplete_reason IS NULL
            )
            OR (
                cost_source = 'UNRESOLVED'
                AND incomplete_reason IS NOT NULL
            )
        ),

    UNIQUE (snapshot_id,line_number),
    UNIQUE (id,business_id)
);

CREATE INDEX vendor_cost_snapshot_components_snapshot_idx
    ON vendor_cost_snapshot_components (snapshot_id,line_number);

-- The covering index for the highest-cost read.
--
-- That read aggregates a bounded window of recent snapshots. Without this
-- index PostgreSQL costs a hash join against the whole component table more
-- cheaply than the nested loop and reads every evidence row: measured on an
-- 80,000-row corpus, 24.8ms and growing with total evidence rather than with
-- the window. Neither an explicit id array, a LATERAL join nor a MATERIALIZED
-- CTE changed that choice - the cost model genuinely prefers the scan at this
-- selectivity.
--
-- Including the aggregated columns makes it an index-only scan, which the
-- planner does choose: 5.35ms on the same corpus, and bounded by the window.
-- The evidence is in internal/features/cost/queryplan_test.go.
CREATE INDEX vendor_cost_snapshot_components_window_idx
    ON vendor_cost_snapshot_components (business_id,snapshot_id)
    INCLUDE (subject_type,subject_ref,component_cost_minor,currency);

-- Powers "highest-cost ingredients" and "ingredient cost over time" without a
-- table scan per read.
CREATE INDEX vendor_cost_snapshot_components_subject_idx
    ON vendor_cost_snapshot_components
        (business_id,subject_type,subject_ref,component_cost_minor DESC);

-- ---------------------------------------------------------------------------
-- Snapshot immutability
-- ---------------------------------------------------------------------------
--
-- Enforced in the database, not only in the service, because "historical
-- evidence cannot change" is exactly the guarantee that must survive a future
-- handler written by someone who has not read this file. RECIPE_PREVIEW is
-- deliberately exempt: a preview is a recalculable convenience, not evidence.
CREATE OR REPLACE FUNCTION vendor_cost_snapshot_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.calculation_type = 'RECIPE_PREVIEW' THEN
        RETURN COALESCE(NEW, OLD);
    END IF;

    RAISE EXCEPTION
        'vendor_cost_snapshots row % is immutable historical cost evidence',
        OLD.id
        USING ERRCODE = 'restrict_violation';
END $$;

CREATE TRIGGER vendor_cost_snapshots_immutable_trg
    BEFORE UPDATE OR DELETE ON vendor_cost_snapshots
    FOR EACH ROW
    EXECUTE FUNCTION vendor_cost_snapshot_immutable();

CREATE OR REPLACE FUNCTION vendor_cost_snapshot_component_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_type text;
BEGIN
    SELECT calculation_type INTO parent_type
      FROM vendor_cost_snapshots
     WHERE id = OLD.snapshot_id;

    -- Absent parent means the cascade is deleting a preview; allow it.
    IF parent_type IS NULL OR parent_type = 'RECIPE_PREVIEW' THEN
        RETURN COALESCE(NEW, OLD);
    END IF;

    RAISE EXCEPTION
        'vendor_cost_snapshot_components row % is immutable cost evidence',
        OLD.id
        USING ERRCODE = 'restrict_violation';
END $$;

CREATE TRIGGER vendor_cost_snapshot_components_immutable_trg
    BEFORE UPDATE OR DELETE ON vendor_cost_snapshot_components
    FOR EACH ROW
    EXECUTE FUNCTION vendor_cost_snapshot_component_immutable();

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------
--
-- Cost visibility is separable from recipe and production visibility: kitchen
-- staff run production without seeing what the vendor pays for chicken.
INSERT INTO vendor_permissions(
    id,
    code,
    description,
    category,
    is_high_risk
)
VALUES
(
    'perm_vendor_cost_read',
    'vendor.cost.read',
    'Read operational ingredient, packaging and recipe cost information.',
    'cost',
    false
),
(
    'perm_vendor_cost_policy_manage',
    'vendor.cost.policy.manage',
    'Create, change or archive operational cost policies and manual configured cost.',
    'cost',
    true
),
(
    'perm_vendor_production_cost_read',
    'vendor.production.cost.read',
    'Read expected, actual and variance operational cost for production batches.',
    'cost',
    false
)
ON CONFLICT DO NOTHING;

-- Owner and General Manager: full cost authority.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT role_ref.role_id,p.id
FROM (
    VALUES
        ('role_owner'),
        ('role_general_manager')
) AS role_ref(role_id)
JOIN vendor_permissions p
  ON p.code IN (
      'vendor.cost.read',
      'vendor.cost.policy.manage',
      'vendor.production.cost.read'
  )
WHERE EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id=role_ref.role_id
)
ON CONFLICT DO NOTHING;

-- Finance: reads cost and production cost, and owns cost policy, because
-- deciding how the business values its inputs is a finance decision.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_finance',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.cost.read',
    'vendor.cost.policy.manage',
    'vendor.production.cost.read'
)
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_finance'
)
ON CONFLICT DO NOTHING;

-- Branch Manager: reads cost, still runtime branch-scoped. No policy authority,
-- because a policy is a business-wide valuation decision.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_branch_manager',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.cost.read',
    'vendor.production.cost.read'
)
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_branch_manager'
)
ON CONFLICT DO NOTHING;

-- Analyst / Auditor: read-only. Analysing cost never implies changing how it
-- is calculated.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_analyst_auditor',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.cost.read',
    'vendor.production.cost.read'
)
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_analyst_auditor'
)
ON CONFLICT DO NOTHING;

-- Procurement: reads ingredient cost, since it negotiates the purchases the
-- cost is derived from. No production cost, no policy.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_procurement',p.id
FROM vendor_permissions p
WHERE p.code = 'vendor.cost.read'
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_procurement'
)
ON CONFLICT DO NOTHING;

-- role_kitchen_pastry is deliberately granted nothing here. Kitchen and pastry
-- staff keep every recipe and production permission Section 16B/16C gave them
-- and gain no cost visibility. That separation is the point of this section's
-- IAM, and it is asserted directly in the Section 16D certification tests.
