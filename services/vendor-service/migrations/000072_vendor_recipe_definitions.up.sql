-- Section 16B
-- Recipes/BOM, immutable versions, exact component normalization,
-- expected yield and expected waste.
--
-- Authority boundaries:
--   Food/Pastry/Cloud Kitchen own sellable catalogue targets.
--   Section 16A owns Ingredient/Packaging master data.
--   Section 14 Inventory owns physical stock.
--   Section 16B owns production definitions only.
--
-- This migration DOES NOT introduce production batches, actual stock
-- consumption, actual waste or COGS.

-- ============================================================
-- RECIPE MASTER
-- ============================================================

CREATE TABLE vendor_recipes (
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

    -- Optional typed child/configuration reference.
    --
    -- FOOD_ITEM:
    --   food_item_variants.id
    --
    -- PASTRY_PRODUCT:
    --   pastry_options.id attached to that product
    --
    -- CLOUD_KITCHEN_PACKAGE:
    --   must remain empty
    target_variant_ref text NOT NULL DEFAULT ''
        CHECK (char_length(target_variant_ref) <= 200),

    name text NOT NULL
        CHECK (char_length(name) BETWEEN 1 AND 200),

    internal_description text NOT NULL DEFAULT ''
        CHECK (char_length(internal_description) <= 4000),

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

    CONSTRAINT vendor_recipes_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    UNIQUE(id,business_id)
);

CREATE UNIQUE INDEX vendor_recipes_live_target_uq
ON vendor_recipes(
    business_id,
    target_type,
    target_ref,
    target_variant_ref
)
WHERE status <> 'ARCHIVED';

CREATE INDEX vendor_recipes_business_branch_status_idx
ON vendor_recipes(
    business_id,
    branch_id,
    status,
    name,
    id
);

CREATE INDEX vendor_recipes_target_lookup_idx
ON vendor_recipes(
    business_id,
    target_type,
    target_ref,
    target_variant_ref,
    status
);

-- ============================================================
-- RECIPE VERSION
-- ============================================================

CREATE TABLE vendor_recipe_versions (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    recipe_id text NOT NULL,

    version_number integer NOT NULL
        CHECK (version_number > 0),

    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (
            status IN (
                'DRAFT',
                'ACTIVE',
                'RETIRED'
            )
        ),

    internal_notes text NOT NULL DEFAULT ''
        CHECK (char_length(internal_notes) <= 4000),

    expected_output_numerator bigint NOT NULL
        CHECK (expected_output_numerator > 0),

    expected_output_denominator bigint NOT NULL DEFAULT 1
        CHECK (expected_output_denominator > 0),

    output_unit_code text NOT NULL
        CHECK (char_length(output_unit_code) BETWEEN 1 AND 80),

    serving_count bigint NOT NULL DEFAULT 0
        CHECK (serving_count >= 0),

    -- General planning assumption. Individual ingredient components may
    -- override it with their own expected_waste_bps.
    expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (
            expected_waste_bps >= 0
            AND expected_waste_bps < 10000
        ),

    activated_at timestamptz,
    retired_at timestamptz,

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_recipe_versions_recipe_fk
        FOREIGN KEY(recipe_id,business_id)
        REFERENCES vendor_recipes(id,business_id),

    UNIQUE(
        business_id,
        recipe_id,
        version_number
    ),

    UNIQUE(id,business_id)
);

CREATE UNIQUE INDEX vendor_recipe_versions_one_active_uq
ON vendor_recipe_versions(
    business_id,
    recipe_id
)
WHERE status='ACTIVE';

CREATE INDEX vendor_recipe_versions_history_idx
ON vendor_recipe_versions(
    business_id,
    recipe_id,
    version_number DESC,
    id
);

-- ============================================================
-- INGREDIENT COMPONENTS
-- ============================================================

CREATE TABLE vendor_recipe_components (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    recipe_version_id text NOT NULL,
    ingredient_id text NOT NULL,

    quantity_numerator bigint NOT NULL
        CHECK (quantity_numerator > 0),

    quantity_denominator bigint NOT NULL DEFAULT 1
        CHECK (quantity_denominator > 0),

    unit_code text NOT NULL
        CHECK (char_length(unit_code) BETWEEN 1 AND 80),

    -- Resolved against Section 16A authoritative conversion during
    -- activation. UNRESOLVED is permitted only for an operationally
    -- UNTRACKED ingredient.
    normalization_status text NOT NULL DEFAULT 'UNRESOLVED'
        CHECK (
            normalization_status IN (
                'UNRESOLVED',
                'EXACT'
            )
        ),

    normalized_quantity_numerator bigint NOT NULL DEFAULT 0
        CHECK (normalized_quantity_numerator >= 0),

    normalized_quantity_denominator bigint NOT NULL DEFAULT 1
        CHECK (normalized_quantity_denominator > 0),

    normalized_unit_code text NOT NULL DEFAULT ''
        CHECK (char_length(normalized_unit_code) <= 80),

    expected_waste_bps integer NOT NULL DEFAULT 0
        CHECK (
            expected_waste_bps >= 0
            AND expected_waste_bps < 10000
        ),

    is_optional boolean NOT NULL DEFAULT false,

    sequence integer NOT NULL DEFAULT 0
        CHECK (sequence >= 0),

    preparation_note text NOT NULL DEFAULT ''
        CHECK (char_length(preparation_note) <= 1000),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_recipe_components_version_fk
        FOREIGN KEY(recipe_version_id,business_id)
        REFERENCES vendor_recipe_versions(id,business_id),

    CONSTRAINT vendor_recipe_components_ingredient_fk
        FOREIGN KEY(ingredient_id,business_id)
        REFERENCES vendor_ingredients(id,business_id),

    UNIQUE(
        business_id,
        recipe_version_id,
        ingredient_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_recipe_components_version_sequence_idx
ON vendor_recipe_components(
    business_id,
    recipe_version_id,
    sequence,
    id
);

CREATE INDEX vendor_recipe_components_ingredient_idx
ON vendor_recipe_components(
    business_id,
    ingredient_id,
    recipe_version_id
);

-- ============================================================
-- PACKAGING REQUIREMENTS
-- ============================================================

CREATE TABLE vendor_recipe_packaging_requirements (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    recipe_version_id text NOT NULL,
    packaging_material_id text NOT NULL,

    quantity_numerator bigint NOT NULL
        CHECK (quantity_numerator > 0),

    quantity_denominator bigint NOT NULL DEFAULT 1
        CHECK (quantity_denominator > 0),

    unit_code text NOT NULL
        CHECK (char_length(unit_code) BETWEEN 1 AND 80),

    normalization_status text NOT NULL DEFAULT 'UNRESOLVED'
        CHECK (
            normalization_status IN (
                'UNRESOLVED',
                'EXACT'
            )
        ),

    normalized_quantity_numerator bigint NOT NULL DEFAULT 0
        CHECK (normalized_quantity_numerator >= 0),

    normalized_quantity_denominator bigint NOT NULL DEFAULT 1
        CHECK (normalized_quantity_denominator > 0),

    normalized_unit_code text NOT NULL DEFAULT ''
        CHECK (char_length(normalized_unit_code) <= 80),

    sequence integer NOT NULL DEFAULT 0
        CHECK (sequence >= 0),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_recipe_packaging_version_fk
        FOREIGN KEY(recipe_version_id,business_id)
        REFERENCES vendor_recipe_versions(id,business_id),

    CONSTRAINT vendor_recipe_packaging_material_fk
        FOREIGN KEY(packaging_material_id,business_id)
        REFERENCES vendor_packaging_materials(id,business_id),

    UNIQUE(
        business_id,
        recipe_version_id,
        packaging_material_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_recipe_packaging_version_sequence_idx
ON vendor_recipe_packaging_requirements(
    business_id,
    recipe_version_id,
    sequence,
    id
);

CREATE INDEX vendor_recipe_packaging_material_idx
ON vendor_recipe_packaging_requirements(
    business_id,
    packaging_material_id,
    recipe_version_id
);

-- ============================================================
-- VERSION IMMUTABILITY
-- ============================================================

CREATE OR REPLACE FUNCTION vendor_recipe_definition_child_mutability_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_status text;
BEGIN
    SELECT status
      INTO parent_status
      FROM vendor_recipe_versions
     WHERE id = COALESCE(NEW.recipe_version_id,OLD.recipe_version_id)
       AND business_id = COALESCE(NEW.business_id,OLD.business_id);

    IF parent_status IS NULL THEN
        RAISE EXCEPTION
            'recipe version does not exist';
    END IF;

    IF parent_status <> 'DRAFT' THEN
        RAISE EXCEPTION
            'recipe material composition is immutable outside DRAFT';
    END IF;

    RETURN COALESCE(NEW,OLD);
END;
$$;

CREATE TRIGGER vendor_recipe_components_mutability_guard
BEFORE INSERT OR UPDATE OR DELETE
ON vendor_recipe_components
FOR EACH ROW
EXECUTE FUNCTION vendor_recipe_definition_child_mutability_guard();

CREATE TRIGGER vendor_recipe_packaging_mutability_guard
BEFORE INSERT OR UPDATE OR DELETE
ON vendor_recipe_packaging_requirements
FOR EACH ROW
EXECUTE FUNCTION vendor_recipe_definition_child_mutability_guard();

CREATE OR REPLACE FUNCTION vendor_recipe_version_immutability_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status = 'DRAFT' THEN
        RETURN NEW;
    END IF;

    -- Once ACTIVE/RETIRED, only lifecycle/version metadata may change.
    IF
        NEW.recipe_id IS DISTINCT FROM OLD.recipe_id
        OR NEW.business_id IS DISTINCT FROM OLD.business_id
        OR NEW.version_number IS DISTINCT FROM OLD.version_number
        OR NEW.internal_notes IS DISTINCT FROM OLD.internal_notes
        OR NEW.expected_output_numerator IS DISTINCT FROM OLD.expected_output_numerator
        OR NEW.expected_output_denominator IS DISTINCT FROM OLD.expected_output_denominator
        OR NEW.output_unit_code IS DISTINCT FROM OLD.output_unit_code
        OR NEW.serving_count IS DISTINCT FROM OLD.serving_count
        OR NEW.expected_waste_bps IS DISTINCT FROM OLD.expected_waste_bps
    THEN
        RAISE EXCEPTION
            'historical recipe version material definition is immutable';
    END IF;

    IF OLD.status = 'RETIRED' AND NEW.status <> 'RETIRED' THEN
        RAISE EXCEPTION
            'retired recipe version cannot be reactivated';
    END IF;

    IF OLD.status = 'ACTIVE'
       AND NEW.status NOT IN ('ACTIVE','RETIRED')
    THEN
        RAISE EXCEPTION
            'active recipe version may only remain active or retire';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER vendor_recipe_version_immutability_guard
BEFORE UPDATE
ON vendor_recipe_versions
FOR EACH ROW
EXECUTE FUNCTION vendor_recipe_version_immutability_guard();

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
    'perm_vendor_recipes_read',
    'vendor.recipes.read',
    'Read Vendor recipe definitions and version history.',
    'production',
    false
),
(
    'perm_vendor_recipes_manage',
    'vendor.recipes.manage',
    'Manage Vendor recipe definitions and activate recipe versions.',
    'production',
    false
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
      'vendor.recipes.read',
      'vendor.recipes.manage'
  )
WHERE EXISTS (
    SELECT 1
      FROM vendor_roles r
     WHERE r.id=role_ref.role_id
)
ON CONFLICT DO NOTHING;

-- Kitchen / Pastry owns operational production definitions by default.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_kitchen_pastry',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.recipes.read',
    'vendor.recipes.manage'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles r
     WHERE r.id='role_kitchen_pastry'
)
ON CONFLICT DO NOTHING;

-- Branch Manager receives the same permission codes, but runtime assignment
-- scope still restricts the permission to the assigned branch.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_branch_manager',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.recipes.read',
    'vendor.recipes.manage'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles r
     WHERE r.id='role_branch_manager'
)
ON CONFLICT DO NOTHING;

-- Analyst/Auditor read-only.
INSERT INTO vendor_role_permissions(
    role_id,
    permission_id
)
SELECT 'role_analyst_auditor',p.id
FROM vendor_permissions p
WHERE p.code='vendor.recipes.read'
AND EXISTS (
    SELECT 1
      FROM vendor_roles r
     WHERE r.id='role_analyst_auditor'
)
ON CONFLICT DO NOTHING;