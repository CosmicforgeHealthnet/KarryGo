-- Section 16A
-- Ingredients, Packaging Materials, Exact Units and Inventory Mapping.
--
-- This migration creates Vendor-owned production master data only.
--
-- It does NOT create:
--   recipe/BOM authority
--   production batches
--   stock balances
--   stock movements
--   purchase orders
--   Wallet/accounting state
--
-- Section 14 Inventory remains physical-stock authority.

-- ============================================================
-- INGREDIENT MASTER
-- ============================================================

CREATE TABLE vendor_ingredients (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    code text NOT NULL
        CHECK (char_length(code) BETWEEN 1 AND 80),

    name text NOT NULL
        CHECK (char_length(name) BETWEEN 1 AND 200),

    description text NOT NULL DEFAULT ''
        CHECK (char_length(description) <= 4000),

    measurement_family text NOT NULL
        CHECK (
            measurement_family IN (
                'MASS',
                'VOLUME',
                'COUNT'
            )
        ),

    default_unit_code text NOT NULL
        CHECK (char_length(default_unit_code) BETWEEN 1 AND 80),

    inventory_control_mode text NOT NULL DEFAULT 'UNTRACKED'
        CHECK (
            inventory_control_mode IN (
                'UNTRACKED',
                'INVENTORY_CONTROLLED'
            )
        ),

    perishable boolean NOT NULL DEFAULT false,

    requires_expiry boolean NOT NULL DEFAULT false,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'DRAFT',
                'ACTIVE',
                'INACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE(business_id,code),
    UNIQUE(id,business_id)
);

CREATE INDEX vendor_ingredients_business_status_name_idx
ON vendor_ingredients(
    business_id,
    status,
    name,
    id
);

CREATE INDEX vendor_ingredients_business_unit_idx
ON vendor_ingredients(
    business_id,
    measurement_family,
    default_unit_code
);

-- ============================================================
-- INGREDIENT-SPECIFIC EXACT/DISPLAY CONVERSIONS
-- ============================================================

CREATE TABLE vendor_ingredient_unit_conversions (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    ingredient_id text NOT NULL,

    measurement_family text NOT NULL
        CHECK (
            measurement_family IN (
                'MASS',
                'VOLUME',
                'COUNT'
            )
        ),

    from_unit_code text NOT NULL
        CHECK (char_length(from_unit_code) BETWEEN 1 AND 80),

    to_unit_code text NOT NULL
        CHECK (char_length(to_unit_code) BETWEEN 1 AND 80),

    conversion_mode text NOT NULL
        CHECK (
            conversion_mode IN (
                'EXACT_AUTHORITATIVE',
                'DISPLAY_ONLY'
            )
        ),

    numerator bigint NOT NULL
        CHECK (numerator > 0),

    denominator bigint NOT NULL
        CHECK (denominator > 0),

    source_context text NOT NULL DEFAULT ''
        CHECK (char_length(source_context) <= 1000),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'INACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_ingredient_unit_conversions_ingredient_fk
        FOREIGN KEY(ingredient_id,business_id)
        REFERENCES vendor_ingredients(id,business_id),

    CONSTRAINT vendor_ingredient_unit_conversions_distinct_unit_ck
        CHECK (from_unit_code <> to_unit_code),

    UNIQUE(
        business_id,
        ingredient_id,
        from_unit_code,
        to_unit_code
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_ingredient_conversions_lookup_idx
ON vendor_ingredient_unit_conversions(
    business_id,
    ingredient_id,
    status,
    from_unit_code
);

-- ============================================================
-- INGREDIENT BRANCH CONFIGURATION / INVENTORY LINKAGE
-- ============================================================

CREATE TABLE vendor_ingredient_branch_configs (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    ingredient_id text NOT NULL,

    enabled boolean NOT NULL DEFAULT true,

    production_eligible boolean NOT NULL DEFAULT true,

    inventory_control_mode_override text
        CHECK (
            inventory_control_mode_override IS NULL
            OR inventory_control_mode_override IN (
                'UNTRACKED',
                'INVENTORY_CONTROLLED'
            )
        ),

    inventory_item_id text,

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_ingredient_branch_configs_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_ingredient_branch_configs_ingredient_fk
        FOREIGN KEY(ingredient_id,business_id)
        REFERENCES vendor_ingredients(id,business_id),

    CONSTRAINT vendor_ingredient_branch_configs_inventory_fk
        FOREIGN KEY(inventory_item_id,business_id,branch_id)
        REFERENCES inventory_items(id,business_id,branch_id),

    UNIQUE(
        business_id,
        branch_id,
        ingredient_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_ingredient_branch_inventory_idx
ON vendor_ingredient_branch_configs(
    business_id,
    branch_id,
    inventory_item_id
)
WHERE inventory_item_id IS NOT NULL;

-- ============================================================
-- PACKAGING MATERIAL MASTER
-- ============================================================

CREATE TABLE vendor_packaging_materials (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses(id),

    code text NOT NULL
        CHECK (char_length(code) BETWEEN 1 AND 80),

    name text NOT NULL
        CHECK (char_length(name) BETWEEN 1 AND 200),

    description text NOT NULL DEFAULT ''
        CHECK (char_length(description) <= 4000),

    measurement_family text NOT NULL
        CHECK (
            measurement_family IN (
                'MASS',
                'VOLUME',
                'COUNT'
            )
        ),

    default_unit_code text NOT NULL
        CHECK (char_length(default_unit_code) BETWEEN 1 AND 80),

    inventory_control_mode text NOT NULL DEFAULT 'UNTRACKED'
        CHECK (
            inventory_control_mode IN (
                'UNTRACKED',
                'INVENTORY_CONTROLLED'
            )
        ),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'DRAFT',
                'ACTIVE',
                'INACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE(business_id,code),
    UNIQUE(id,business_id)
);

CREATE INDEX vendor_packaging_materials_business_status_name_idx
ON vendor_packaging_materials(
    business_id,
    status,
    name,
    id
);

-- ============================================================
-- PACKAGING-SPECIFIC UNIT CONFIGURATION
-- ============================================================

CREATE TABLE vendor_packaging_unit_conversions (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    packaging_material_id text NOT NULL,

    measurement_family text NOT NULL
        CHECK (
            measurement_family IN (
                'MASS',
                'VOLUME',
                'COUNT'
            )
        ),

    from_unit_code text NOT NULL
        CHECK (char_length(from_unit_code) BETWEEN 1 AND 80),

    to_unit_code text NOT NULL
        CHECK (char_length(to_unit_code) BETWEEN 1 AND 80),

    conversion_mode text NOT NULL
        CHECK (
            conversion_mode IN (
                'EXACT_AUTHORITATIVE',
                'DISPLAY_ONLY'
            )
        ),

    numerator bigint NOT NULL
        CHECK (numerator > 0),

    denominator bigint NOT NULL
        CHECK (denominator > 0),

    source_context text NOT NULL DEFAULT ''
        CHECK (char_length(source_context) <= 1000),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'INACTIVE',
                'ARCHIVED'
            )
        ),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_packaging_unit_conversions_material_fk
        FOREIGN KEY(packaging_material_id,business_id)
        REFERENCES vendor_packaging_materials(id,business_id),

    CONSTRAINT vendor_packaging_unit_conversions_distinct_unit_ck
        CHECK (from_unit_code <> to_unit_code),

    UNIQUE(
        business_id,
        packaging_material_id,
        from_unit_code,
        to_unit_code
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_packaging_conversions_lookup_idx
ON vendor_packaging_unit_conversions(
    business_id,
    packaging_material_id,
    status,
    from_unit_code
);

-- ============================================================
-- PACKAGING BRANCH CONFIGURATION / INVENTORY LINKAGE
-- ============================================================

CREATE TABLE vendor_packaging_branch_configs (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    branch_id text NOT NULL,
    packaging_material_id text NOT NULL,

    enabled boolean NOT NULL DEFAULT true,

    production_eligible boolean NOT NULL DEFAULT true,

    inventory_control_mode_override text
        CHECK (
            inventory_control_mode_override IS NULL
            OR inventory_control_mode_override IN (
                'UNTRACKED',
                'INVENTORY_CONTROLLED'
            )
        ),

    inventory_item_id text,

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_packaging_branch_configs_branch_fk
        FOREIGN KEY(branch_id,business_id)
        REFERENCES vendor_branches(id,business_id),

    CONSTRAINT vendor_packaging_branch_configs_material_fk
        FOREIGN KEY(packaging_material_id,business_id)
        REFERENCES vendor_packaging_materials(id,business_id),

    CONSTRAINT vendor_packaging_branch_configs_inventory_fk
        FOREIGN KEY(inventory_item_id,business_id,branch_id)
        REFERENCES inventory_items(id,business_id,branch_id),

    UNIQUE(
        business_id,
        branch_id,
        packaging_material_id
    ),

    UNIQUE(id,business_id)
);

CREATE INDEX vendor_packaging_branch_inventory_idx
ON vendor_packaging_branch_configs(
    business_id,
    branch_id,
    inventory_item_id
)
WHERE inventory_item_id IS NOT NULL;

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
    'perm_vendor_ingredients_read',
    'vendor.ingredients.read',
    'Read Vendor ingredient master data.',
    'production',
    false
),
(
    'perm_vendor_ingredients_manage',
    'vendor.ingredients.manage',
    'Manage Vendor ingredient master data and exact conversions.',
    'production',
    false
),
(
    'perm_vendor_ingredients_inventory_mapping',
    'vendor.ingredients.inventory_mapping',
    'Configure branch Ingredient to Inventory mappings.',
    'production',
    false
),
(
    'perm_vendor_packaging_read',
    'vendor.packaging.read',
    'Read Vendor packaging-material master data.',
    'production',
    false
),
(
    'perm_vendor_packaging_manage',
    'vendor.packaging.manage',
    'Manage Vendor packaging-material master data and units.',
    'production',
    false
),
(
    'perm_vendor_packaging_inventory_mapping',
    'vendor.packaging.inventory_mapping',
    'Configure branch Packaging to Inventory mappings.',
    'production',
    false
)
ON CONFLICT DO NOTHING;

-- Owner / General Manager: full capability.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT role_ids.role_id,p.id
FROM (
    VALUES
        ('role_owner'),
        ('role_general_manager')
) AS role_ids(role_id)
JOIN vendor_permissions p
  ON p.code IN (
      'vendor.ingredients.read',
      'vendor.ingredients.manage',
      'vendor.ingredients.inventory_mapping',
      'vendor.packaging.read',
      'vendor.packaging.manage',
      'vendor.packaging.inventory_mapping'
  )
WHERE EXISTS (
    SELECT 1
      FROM vendor_roles vr
     WHERE vr.id=role_ids.role_id
)
ON CONFLICT DO NOTHING;

-- Branch Manager: operational read + own-scope branch linkage/config.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.ingredients.read',
    'vendor.ingredients.inventory_mapping',
    'vendor.packaging.read',
    'vendor.packaging.inventory_mapping'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles vr
     WHERE vr.id='role_branch_manager'
)
ON CONFLICT DO NOTHING;

-- Kitchen/Pastry: operational read.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_kitchen_pastry',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.ingredients.read',
    'vendor.packaging.read'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles vr
     WHERE vr.id='role_kitchen_pastry'
)
ON CONFLICT DO NOTHING;

-- Shop / Inventory: read + Inventory linkage.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_shop_inventory',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.ingredients.read',
    'vendor.ingredients.inventory_mapping',
    'vendor.packaging.read',
    'vendor.packaging.inventory_mapping'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles vr
     WHERE vr.id='role_shop_inventory'
)
ON CONFLICT DO NOTHING;

-- Analyst/Auditor: read only.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',p.id
FROM vendor_permissions p
WHERE p.code IN (
    'vendor.ingredients.read',
    'vendor.packaging.read'
)
AND EXISTS (
    SELECT 1
      FROM vendor_roles vr
     WHERE vr.id='role_analyst_auditor'
)
ON CONFLICT DO NOTHING;