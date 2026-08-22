-- Section 16C
-- Production Batches, Inventory Consumption and Prepared Stock.
--
-- Authority boundaries:
--   Sections 10/11 production owns capacity pools, slots, reservations and jobs.
--   Section 14 Inventory owns all physical stock.
--   Section 16A owns Ingredient / Packaging master data.
--   Section 16B owns Recipe definitions and immutable versions.
--   Section 16C owns operational production execution only.
--
-- This migration intentionally contains NO COGS / financial / Wallet authority.

-- ============================================================
-- PACKAGING CONSUMPTION POINT
-- ============================================================

ALTER TABLE vendor_packaging_branch_configs
ADD COLUMN production_consumption_point text NOT NULL DEFAULT 'FULFILMENT'
CHECK (
    production_consumption_point IN (
        'FULFILMENT',
        'PRODUCTION'
    )
);

-- ============================================================
-- PREPARED-STOCK / OPERATING POLICY
-- ============================================================

CREATE TABLE vendor_production_stock_policies (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    branch_id text NOT NULL,

    target_type text NOT NULL
        CHECK (
            target_type IN (
                'FOOD_ITEM',
                'PASTRY_PRODUCT',
                'CLOUD_KITCHEN_PACKAGE'
            )
        ),

    target_ref text NOT NULL
        CHECK (char_length(target_ref) BETWEEN 1 AND 200),

    target_variant_ref text NOT NULL DEFAULT ''
        CHECK (char_length(target_variant_ref) <= 200),

    operating_model text NOT NULL DEFAULT 'ON_DEMAND'
        CHECK (
            operating_model IN (
                'ON_DEMAND',
                'BATCH_STOCK'
            )
        ),

    prepared_inventory_item_id text,

    -- Exact quantity of prepared Inventory units created per one production
    -- output unit. No floating point.
    output_inventory_numerator bigint NOT NULL DEFAULT 1
        CHECK (output_inventory_numerator > 0),

    output_inventory_denominator bigint NOT NULL DEFAULT 1
        CHECK (output_inventory_denominator > 0),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_stock_policies_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_production_stock_policies_inventory_fk
        FOREIGN KEY(
            prepared_inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items(
            id,
            business_id,
            branch_id
        ),

    UNIQUE(id,business_id)
);

CREATE UNIQUE INDEX vendor_production_stock_policy_live_target_uq
ON vendor_production_stock_policies(
    business_id,
    branch_id,
    target_type,
    target_ref,
    target_variant_ref
)
WHERE status='ACTIVE';

CREATE INDEX vendor_production_stock_policy_branch_idx
ON vendor_production_stock_policies(
    business_id,
    branch_id,
    status,
    target_type,
    target_ref
);

-- ============================================================
-- PRODUCTION BATCH
-- ============================================================

CREATE TABLE vendor_production_batches (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    branch_id text NOT NULL,

    recipe_id text NOT NULL,
    recipe_version_id text NOT NULL,
    recipe_version_number integer NOT NULL
        CHECK (recipe_version_number > 0),

    target_type text NOT NULL
        CHECK (
            target_type IN (
                'FOOD_ITEM',
                'PASTRY_PRODUCT',
                'CLOUD_KITCHEN_PACKAGE'
            )
        ),

    target_ref text NOT NULL
        CHECK (char_length(target_ref) BETWEEN 1 AND 200),

    target_variant_ref text NOT NULL DEFAULT ''
        CHECK (char_length(target_variant_ref) <= 200),

    -- Existing Sections 10/11 production-job integration seam.
    -- 16C does not create another capacity/job engine.
    production_job_id text,

    planned_output_numerator bigint NOT NULL
        CHECK (planned_output_numerator > 0),

    planned_output_denominator bigint NOT NULL DEFAULT 1
        CHECK (planned_output_denominator > 0),

    output_unit_code text NOT NULL
        CHECK (char_length(output_unit_code) BETWEEN 1 AND 80),

    expected_output_numerator bigint NOT NULL
        CHECK (expected_output_numerator > 0),

    expected_output_denominator bigint NOT NULL DEFAULT 1
        CHECK (expected_output_denominator > 0),

    recipe_expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (
            recipe_expected_waste_bps >= 0
            AND recipe_expected_waste_bps < 10000
        ),

    multiplier_numerator bigint NOT NULL
        CHECK (multiplier_numerator > 0),

    multiplier_denominator bigint NOT NULL
        CHECK (multiplier_denominator > 0),

    operating_model text NOT NULL
        CHECK (
            operating_model IN (
                'ON_DEMAND',
                'BATCH_STOCK'
            )
        ),

    -- Frozen from the active production-stock policy when the batch is
    -- created. Later policy changes cannot rewrite historical production.
    prepared_inventory_item_id text,

    output_inventory_numerator bigint NOT NULL DEFAULT 1
        CHECK (output_inventory_numerator > 0),

    output_inventory_denominator bigint NOT NULL DEFAULT 1
        CHECK (output_inventory_denominator > 0),

    inventory_reservation_id text,

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'PLANNED',
                'IN_PROGRESS',
                'COMPLETED',
                'CANCELLED',
                'FAILED'
            )
        ),

    actual_consumption_recorded boolean NOT NULL DEFAULT false,

    actual_output_numerator bigint NOT NULL DEFAULT 0
        CHECK (actual_output_numerator >= 0),

    actual_output_denominator bigint NOT NULL DEFAULT 1
        CHECK (actual_output_denominator > 0),

    created_by_staff_id text,
    planned_by_staff_id text,
    started_by_staff_id text,
    completed_by_staff_id text,
    cancelled_by_staff_id text,

    planned_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,

    cancel_reason text NOT NULL DEFAULT ''
        CHECK (char_length(cancel_reason) <= 1000),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batches_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_production_batches_recipe_fk
        FOREIGN KEY(recipe_id,business_id)
        REFERENCES vendor_recipes(id,business_id),

    CONSTRAINT vendor_production_batches_recipe_version_fk
        FOREIGN KEY(recipe_version_id,business_id)
        REFERENCES vendor_recipe_versions(id,business_id),

    CONSTRAINT vendor_production_batches_job_fk
        FOREIGN KEY(production_job_id,business_id)
        REFERENCES production_jobs(id,business_id),

    CONSTRAINT vendor_production_batches_output_inventory_fk
        FOREIGN KEY(
            prepared_inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items(
            id,
            business_id,
            branch_id
        ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_production_batches_branch_status_idx
ON vendor_production_batches(
    business_id,
    branch_id,
    status,
    created_at DESC,
    id
);

CREATE INDEX vendor_production_batches_recipe_history_idx
ON vendor_production_batches(
    business_id,
    recipe_id,
    recipe_version_id,
    created_at DESC,
    id
);

CREATE INDEX vendor_production_batches_target_idx
ON vendor_production_batches(
    business_id,
    target_type,
    target_ref,
    target_variant_ref,
    status
);

CREATE INDEX vendor_production_batches_job_idx
ON vendor_production_batches(
    business_id,
    production_job_id
)
WHERE production_job_id IS NOT NULL;

-- ============================================================
-- FROZEN INGREDIENT REQUIREMENTS
-- ============================================================

CREATE TABLE vendor_production_batch_components (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    batch_id text NOT NULL,
    ingredient_id text NOT NULL,

    recipe_component_id text NOT NULL,

    unit_code text NOT NULL
        CHECK (char_length(unit_code) BETWEEN 1 AND 80),

    planned_quantity_numerator bigint NOT NULL
        CHECK (planned_quantity_numerator > 0),

    planned_quantity_denominator bigint NOT NULL
        CHECK (planned_quantity_denominator > 0),

    expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (
            expected_waste_bps >= 0
            AND expected_waste_bps < 10000
        ),

    optional boolean NOT NULL DEFAULT false,

    inventory_controlled boolean NOT NULL DEFAULT false,

    inventory_item_id text,

    reserved_quantity bigint NOT NULL DEFAULT 0
        CHECK (reserved_quantity >= 0),

    actual_used_numerator bigint NOT NULL DEFAULT 0
        CHECK (actual_used_numerator >= 0),

    actual_used_denominator bigint NOT NULL DEFAULT 1
        CHECK (actual_used_denominator > 0),

    actual_waste_numerator bigint NOT NULL DEFAULT 0
        CHECK (actual_waste_numerator >= 0),

    actual_waste_denominator bigint NOT NULL DEFAULT 1
        CHECK (actual_waste_denominator > 0),

    actual_recorded boolean NOT NULL DEFAULT false,

    -- Signed numerator intentionally permits under/over planned variance.
    variance_numerator bigint NOT NULL DEFAULT 0,
    variance_denominator bigint NOT NULL DEFAULT 1
        CHECK (variance_denominator > 0),

    sequence integer NOT NULL DEFAULT 0
        CHECK (sequence >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_components_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_production_batch_components_ingredient_fk
        FOREIGN KEY(ingredient_id,business_id)
        REFERENCES vendor_ingredients(id,business_id),

    UNIQUE(
        business_id,
        batch_id,
        ingredient_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_production_batch_components_batch_idx
ON vendor_production_batch_components(
    business_id,
    batch_id,
    sequence,
    id
);

CREATE INDEX vendor_production_batch_components_inventory_idx
ON vendor_production_batch_components(
    business_id,
    inventory_item_id,
    batch_id
)
WHERE inventory_item_id IS NOT NULL;

-- ============================================================
-- FROZEN PACKAGING REQUIREMENTS
-- ============================================================

CREATE TABLE vendor_production_batch_packaging (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    batch_id text NOT NULL,
    packaging_material_id text NOT NULL,

    recipe_packaging_requirement_id text NOT NULL,

    unit_code text NOT NULL
        CHECK (char_length(unit_code) BETWEEN 1 AND 80),

    planned_quantity_numerator bigint NOT NULL
        CHECK (planned_quantity_numerator > 0),

    planned_quantity_denominator bigint NOT NULL
        CHECK (planned_quantity_denominator > 0),

    production_consumption_point text NOT NULL
        CHECK (
            production_consumption_point IN (
                'FULFILMENT',
                'PRODUCTION'
            )
        ),

    inventory_controlled boolean NOT NULL DEFAULT false,

    inventory_item_id text,

    reserved_quantity bigint NOT NULL DEFAULT 0
        CHECK (reserved_quantity >= 0),

    actual_used_numerator bigint NOT NULL DEFAULT 0
        CHECK (actual_used_numerator >= 0),

    actual_used_denominator bigint NOT NULL DEFAULT 1
        CHECK (actual_used_denominator > 0),

    actual_waste_numerator bigint NOT NULL DEFAULT 0
        CHECK (actual_waste_numerator >= 0),

    actual_waste_denominator bigint NOT NULL DEFAULT 1
        CHECK (actual_waste_denominator > 0),

    actual_recorded boolean NOT NULL DEFAULT false,

    variance_numerator bigint NOT NULL DEFAULT 0,
    variance_denominator bigint NOT NULL DEFAULT 1
        CHECK (variance_denominator > 0),

    sequence integer NOT NULL DEFAULT 0
        CHECK (sequence >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_packaging_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_production_batch_packaging_material_fk
        FOREIGN KEY(packaging_material_id,business_id)
        REFERENCES vendor_packaging_materials(id,business_id),

    UNIQUE(
        business_id,
        batch_id,
        packaging_material_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_production_batch_packaging_batch_idx
ON vendor_production_batch_packaging(
    business_id,
    batch_id,
    sequence,
    id
);

CREATE INDEX vendor_production_batch_packaging_inventory_idx
ON vendor_production_batch_packaging(
    business_id,
    inventory_item_id,
    batch_id
)
WHERE inventory_item_id IS NOT NULL;

-- ============================================================
-- BATCH -> INVENTORY RESERVATION EVIDENCE
-- ============================================================

CREATE TABLE vendor_production_batch_reservations (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    batch_id text NOT NULL,

    reservation_kind text NOT NULL
        CHECK (
            reservation_kind IN (
                'PRIMARY',
                'TOP_UP'
            )
        ),

    sequence integer NOT NULL
        CHECK (sequence > 0),

    inventory_reservation_id text NOT NULL,

    source_ref text NOT NULL,

    status text NOT NULL DEFAULT 'HELD'
        CHECK (
            status IN (
                'HELD',
                'CONFIRMED',
                'COMMITTED',
                'RELEASED',
                'CANCELLED'
            )
        ),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_reservations_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id)
        ON DELETE CASCADE,

    CONSTRAINT vendor_production_batch_reservations_inventory_fk
        FOREIGN KEY(
            inventory_reservation_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_reservations(
            id,
            business_id,
            branch_id
        ),

    UNIQUE(
        business_id,
        batch_id,
        reservation_kind,
        sequence
    ),

    UNIQUE(
        business_id,
        inventory_reservation_id
    )
);

CREATE INDEX vendor_production_batch_reservations_batch_idx
ON vendor_production_batch_reservations(
    business_id,
    batch_id,
    sequence
);

-- ============================================================
-- ACTUAL WASTE EVIDENCE
-- ============================================================

CREATE TABLE vendor_production_batch_waste_events (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    batch_id text NOT NULL,

    material_kind text NOT NULL
        CHECK (
            material_kind IN (
                'INGREDIENT',
                'PACKAGING'
            )
        ),

    material_ref text NOT NULL,

    quantity_numerator bigint NOT NULL
        CHECK (quantity_numerator > 0),

    quantity_denominator bigint NOT NULL
        CHECK (quantity_denominator > 0),

    unit_code text NOT NULL,

    reason_code text NOT NULL
        CHECK (char_length(btrim(reason_code)) > 0),

    actor_ref text,

    recorded_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_waste_events_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id)
);

CREATE INDEX vendor_production_batch_waste_events_batch_idx
ON vendor_production_batch_waste_events(
    business_id,
    batch_id,
    recorded_at,
    id
);

CREATE OR REPLACE FUNCTION vendor_production_waste_evidence_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'production waste evidence is immutable';
END;
$$;

CREATE TRIGGER vendor_production_waste_evidence_immutable_trigger
BEFORE UPDATE OR DELETE
ON vendor_production_batch_waste_events
FOR EACH ROW
EXECUTE FUNCTION vendor_production_waste_evidence_immutable();

-- ============================================================
-- FINAL INVENTORY ALLOCATION EVIDENCE
-- ============================================================

CREATE TABLE vendor_production_batch_inventory_splits (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    batch_id text NOT NULL,

    inventory_reservation_id text NOT NULL,
    inventory_item_id text NOT NULL,
    lot_id text NOT NULL,

    reserved_quantity bigint NOT NULL
        CHECK (reserved_quantity > 0),

    used_quantity bigint NOT NULL DEFAULT 0
        CHECK (used_quantity >= 0),

    waste_quantity bigint NOT NULL DEFAULT 0
        CHECK (waste_quantity >= 0),

    released_quantity bigint NOT NULL DEFAULT 0
        CHECK (released_quantity >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_inventory_splits_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id),

    CONSTRAINT vendor_production_batch_inventory_splits_lot_fk
        FOREIGN KEY(
            lot_id,
            business_id,
            inventory_item_id
        )
        REFERENCES inventory_lots(
            id,
            business_id,
            inventory_item_id
        ),

    CONSTRAINT vendor_production_batch_inventory_split_total_ck
        CHECK (
            used_quantity
            + waste_quantity
            + released_quantity
            = reserved_quantity
        ),

    UNIQUE(
        business_id,
        batch_id,
        inventory_reservation_id,
        inventory_item_id,
        lot_id
    )
);

CREATE INDEX vendor_production_batch_inventory_splits_batch_idx
ON vendor_production_batch_inventory_splits(
    business_id,
    batch_id,
    inventory_item_id,
    lot_id
);

-- ============================================================
-- PREPARED STOCK OUTPUT EVIDENCE
-- ============================================================

CREATE TABLE vendor_production_batch_outputs (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    batch_id text NOT NULL,

    inventory_item_id text NOT NULL,

    quantity bigint NOT NULL
        CHECK (quantity > 0),

    inventory_source_ref text NOT NULL,

    inventory_lot_id text,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_production_batch_outputs_batch_fk
        FOREIGN KEY(batch_id,business_id)
        REFERENCES vendor_production_batches(id,business_id),

    CONSTRAINT vendor_production_batch_outputs_inventory_fk
        FOREIGN KEY(
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items(
            id,
            business_id,
            branch_id
        ),

    UNIQUE(
        business_id,
        batch_id
    )
);

CREATE INDEX vendor_production_batch_outputs_inventory_idx
ON vendor_production_batch_outputs(
    business_id,
    branch_id,
    inventory_item_id,
    created_at DESC
);

-- ============================================================
-- INVENTORY AUTHORITY EXACTLY-ONCE MARKER
-- ============================================================

CREATE TABLE inventory_production_postings (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,

    batch_ref text NOT NULL,

    request_fingerprint text NOT NULL,

    result_json jsonb NOT NULL,

    posted_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE(
        business_id,
        batch_ref
    )
);

CREATE INDEX inventory_production_postings_branch_idx
ON inventory_production_postings(
    business_id,
    branch_id,
    posted_at DESC
);

-- ============================================================
-- IAM
-- ============================================================

INSERT INTO vendor_permissions(
    id,
    code,
    description,
    category,
    is_high_risk
)
VALUES
(
    'perm_vendor_production_waste_record',
    'vendor.production.waste.record',
    'Record actual ingredient or packaging waste against a production batch.',
    'production',
    false
),
(
    'perm_vendor_production_prepared_stock',
    'vendor.production.prepared_stock',
    'Configure and create prepared-stock Inventory output from production batches.',
    'production',
    true
)
ON CONFLICT DO NOTHING;

-- Owner / General Manager.
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
      'vendor.production.waste.record',
      'vendor.production.prepared_stock'
  )
WHERE EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id=role_ref.role_id
)
ON CONFLICT DO NOTHING;

-- Branch Manager remains runtime branch-scoped.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_branch_manager',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.production.waste.record',
    'vendor.production.prepared_stock'
)
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_branch_manager'
)
ON CONFLICT DO NOTHING;

-- Kitchen/Pastry may record waste and execute prepared-stock production,
-- still constrained to their effective branch assignment.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_kitchen_pastry',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.production.waste.record',
    'vendor.production.prepared_stock'
)
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_kitchen_pastry'
)
ON CONFLICT DO NOTHING;

-- Shop/Inventory receives prepared-stock authority, not recipe authority.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_shop_inventory',p.id
FROM vendor_permissions p
WHERE p.code='vendor.production.prepared_stock'
AND EXISTS (
    SELECT 1 FROM vendor_roles r WHERE r.id='role_shop_inventory'
)
ON CONFLICT DO NOTHING;