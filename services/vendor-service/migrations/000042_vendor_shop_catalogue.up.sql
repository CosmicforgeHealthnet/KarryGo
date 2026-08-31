-- ===========================================================================
-- Section 12A — Shop / Grocery / Market / Wholesale commercial foundation
-- ===========================================================================
--
-- This migration establishes the Shop catalogue, the Nigerian trade
-- measurement domain, packaging, branch listings, retail/wholesale pricing,
-- fresh-goods configuration and handling metadata.
--
-- IT ESTABLISHES NO INVENTORY AUTHORITY. There is no on-hand balance, no
-- movement ledger, no reservation and no lot here, deliberately: Part B owns
-- stock, and a half-built balance column would be worse than none because
-- every reader would trust it.
--
-- MONEY IS INTEGER MINOR UNITS in the business's own currency (Section 6
-- owns the currency; no catalogue row restates it). QUANTITY IS INTEGER in a
-- canonical base unit, or an explicit bounded rational. Nothing here is a
-- float.

-- ---------------------------------------------------------------------------
-- Measurement definitions
-- ---------------------------------------------------------------------------
--
-- Trade measurement is a first-class domain, not a unit string. A definition
-- belongs to exactly one family; a family decides whether a canonical base
-- exists at all.
--
-- business_id NULL means a platform-seeded definition every tenant can see.
-- A non-NULL business_id is that tenant's own definition and is invisible to
-- every other tenant. owner_key exists so referencing rows can carry a
-- foreign key that proves, in the database, that they reference either a
-- platform definition or their OWN — never another business's.
--
-- base_quantity is the EXACT amount of the family's canonical base unit
-- (MASS → gram, VOLUME → millilitre, COUNT → unit) that one of this measure
-- equals, and is NULL whenever no such universal truth exists. That NULL is
-- the whole point: a Paint of rice and a Paint of beans are not the same
-- mass, so no row here may ever claim a container's weight.
CREATE TABLE shop_measure_definitions (
    id text PRIMARY KEY,
    business_id text REFERENCES vendor_businesses (id),
    owner_key text GENERATED ALWAYS AS (COALESCE(business_id, 'PLATFORM')) STORED,
    code text NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]{1,39}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),
    measurement_family text NOT NULL CHECK (measurement_family IN (
        'MASS','VOLUME','COUNT','LOCAL_CONTAINER','PACKAGE','GROUPED_COUNT',
        'LOAD','VARIABLE_WEIGHT','VARIABLE_COUNT','PRICE_PORTION','OTHER_CONTROLLED')),
    measure_origin text NOT NULL CHECK (measure_origin IN ('STANDARD','LOCAL','PACKAGE','VENDOR_DEFINED')),
    region_code text CHECK (region_code IS NULL OR region_code ~ '^[A-Z]{2}(-[A-Z0-9]{1,10})?$'),
    -- Exact conversion to the family's canonical base, or NULL for "no
    -- universal conversion exists".
    base_quantity bigint CHECK (base_quantity IS NULL OR (base_quantity > 0 AND base_quantity <= 1000000000)),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    -- Only the three convertible families may carry a base quantity at all.
    CONSTRAINT shop_measure_definitions_base_family_check CHECK (
        base_quantity IS NULL OR measurement_family IN ('MASS','VOLUME','COUNT')),
    -- A vendor definition is always VENDOR_DEFINED, and a platform definition
    -- never is: origin and ownership cannot disagree.
    CONSTRAINT shop_measure_definitions_origin_ownership_check CHECK (
        (business_id IS NULL AND measure_origin <> 'VENDOR_DEFINED')
        OR (business_id IS NOT NULL AND measure_origin = 'VENDOR_DEFINED')),
    UNIQUE (id, owner_key)
);

CREATE UNIQUE INDEX shop_measure_definitions_code_scope_uq
    ON shop_measure_definitions (owner_key, code);
CREATE INDEX shop_measure_definitions_family_idx
    ON shop_measure_definitions (owner_key, measurement_family, status);

-- Aliases are presentation. They are never calculation identity: every
-- computation resolves the canonical code, so "Paint Rubber" and "Paint
-- Bucket" cannot silently become two different measures.
CREATE TABLE shop_measure_aliases (
    id text PRIMARY KEY,
    measure_definition_id text NOT NULL,
    owner_key text NOT NULL,
    business_id text REFERENCES vendor_businesses (id),
    alias text NOT NULL CHECK (char_length(alias) BETWEEN 1 AND 80),
    region_code text CHECK (region_code IS NULL OR region_code ~ '^[A-Z]{2}(-[A-Z0-9]{1,10})?$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_measure_aliases_definition_fk
        FOREIGN KEY (measure_definition_id, owner_key)
        REFERENCES shop_measure_definitions (id, owner_key) ON DELETE CASCADE,
    -- A tenant may add its own regional/trade alias to a platform definition
    -- (for example PAINT -> "Paint Bucket"), but it may never add an alias to
    -- another tenant's custom definition. NULL business_id is reserved for a
    -- future platform-owned alias seed.
    CONSTRAINT shop_measure_aliases_owner_check CHECK (
        owner_key = 'PLATFORM' OR owner_key = business_id)
);

CREATE UNIQUE INDEX shop_measure_aliases_unique
    ON shop_measure_aliases (measure_definition_id, COALESCE(business_id, 'PLATFORM'), lower(alias));
CREATE INDEX shop_measure_aliases_lookup_idx
    ON shop_measure_aliases (owner_key, business_id, lower(alias));

-- ---------------------------------------------------------------------------
-- Branch operating models
-- ---------------------------------------------------------------------------
--
-- An operating model refines Shop behaviour; it is not a capability and not a
-- service. A branch may legitimately run more than one (a foodstuff dealer
-- that also wholesales), so enabled models are rows and the primary is an
-- optional pointer at one of them.
CREATE TABLE shop_branch_operating_models (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text NOT NULL,
    primary_model text CHECK (primary_model IS NULL OR primary_model IN (
        'SUPERMARKET','GROCERY_SHOP','FOODSTUFF_DEALER','MARKET_TRADER','WHOLESALER')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_branch_operating_models_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    UNIQUE (branch_id),
    UNIQUE (id, business_id)
);

CREATE TABLE shop_branch_operating_model_modes (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    config_id text NOT NULL,
    branch_id text NOT NULL,
    operating_model text NOT NULL CHECK (operating_model IN (
        'SUPERMARKET','GROCERY_SHOP','FOODSTUFF_DEALER','MARKET_TRADER','WHOLESALER')),
    -- The Vendor capability this model was admitted under, recorded so a later
    -- capability change is auditable rather than silently retroactive.
    admitted_capability text NOT NULL CHECK (admitted_capability IN ('SHOP_GROCERY','MARKET','WHOLESALE')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_branch_operating_model_modes_config_fk
        FOREIGN KEY (config_id, business_id)
        REFERENCES shop_branch_operating_models (id, business_id) ON DELETE CASCADE,
    UNIQUE (config_id, operating_model),
    UNIQUE (branch_id, operating_model)
);

CREATE INDEX shop_branch_operating_model_modes_branch_idx
    ON shop_branch_operating_model_modes (business_id, branch_id);

-- ---------------------------------------------------------------------------
-- Category taxonomy
-- ---------------------------------------------------------------------------
--
-- A bounded tree. Depth is stored rather than recomputed so the bound is a
-- database fact; the parent scope foreign key makes a cross-tenant parent
-- impossible rather than merely rejected by a handler.
CREATE TABLE shop_categories (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    parent_id text,
    parent_key text GENERATED ALWAYS AS (COALESCE(parent_id, '')) STORED,
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 120),
    slug text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,79}$'),
    description text CHECK (description IS NULL OR char_length(description) <= 2000),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    depth smallint NOT NULL DEFAULT 0 CHECK (depth BETWEEN 0 AND 5),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_categories_no_self_parent CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT shop_categories_parent_scope_fk
        FOREIGN KEY (parent_id, business_id) REFERENCES shop_categories (id, business_id),
    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX shop_categories_slug_uq
    ON shop_categories (business_id, parent_key, slug);
CREATE INDEX shop_categories_tree_idx
    ON shop_categories (business_id, parent_key, position, id);
CREATE INDEX shop_categories_status_idx
    ON shop_categories (business_id, status);

-- Direct SQL must not be able to corrupt the taxonomy even if it bypasses
-- the service's bounded traversal. UNION (not UNION ALL) also guarantees the
-- validation query terminates if it is inspecting legacy-corrupt data.
CREATE FUNCTION shop_categories_validate_tree() RETURNS trigger AS $$
DECLARE
    parent_status text;
BEGIN
    IF NEW.status = 'ARCHIVED' AND EXISTS (
        SELECT 1 FROM shop_categories child
        WHERE child.business_id=NEW.business_id AND child.parent_id=NEW.id
          AND child.status <> 'ARCHIVED'
    ) THEN
        RAISE EXCEPTION 'shop_categories: archive active children first'
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT status INTO parent_status FROM shop_categories
    WHERE business_id=NEW.business_id AND id=NEW.parent_id;
    IF NEW.status <> 'ARCHIVED' AND parent_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'shop_categories: an active category cannot use an archived parent'
            USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (
        WITH RECURSIVE up AS (
            SELECT id,parent_id FROM shop_categories
            WHERE business_id=NEW.business_id AND id=NEW.parent_id
            UNION
            SELECT c.id,c.parent_id FROM shop_categories c
            JOIN up ON c.id=up.parent_id AND c.business_id=NEW.business_id
        )
        SELECT 1 FROM up WHERE id=NEW.id
    ) THEN
        RAISE EXCEPTION 'shop_categories: parent would create a cycle'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shop_categories_validate_tree_trigger
    BEFORE INSERT OR UPDATE OF parent_id,status ON shop_categories
    FOR EACH ROW EXECUTE FUNCTION shop_categories_validate_tree();

-- ---------------------------------------------------------------------------
-- Product master
-- ---------------------------------------------------------------------------
--
-- Brand, manufacturer and barcode are OPTIONAL by design. "Fresh Tomatoes"
-- from a market trader has none of them and is not a lesser product for it;
-- requiring them would be a European supermarket assumption imposed on a
-- Nigerian market stall.
CREATE TABLE shop_products (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    category_id text NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 200),
    description text CHECK (description IS NULL OR char_length(description) <= 4000),
    brand text CHECK (brand IS NULL OR char_length(brand) BETWEEN 1 AND 120),
    manufacturer text CHECK (manufacturer IS NULL OR char_length(manufacturer) BETWEEN 1 AND 160),
    origin_country char(2) CHECK (origin_country IS NULL OR origin_country ~ '^[A-Z]{2}$'),
    product_class text NOT NULL CHECK (product_class IN (
        'PACKAGED','DRY_BULK','FRESH_PRODUCE','MEAT','FISH','FROZEN','CHILLED','DAIRY',
        'BEVERAGE','BAKED_PACKAGED','HOUSEHOLD_GROCERY','LOCAL_FOODSTUFF','OTHER')),
    -- Optional, safe, self-declared metadata. Storing a registration reference
    -- is NOT a claim that KarryGo verified it with any authority.
    regulatory_reference text CHECK (regulatory_reference IS NULL OR char_length(regulatory_reference) BETWEEN 1 AND 80),
    manufacturer_code text CHECK (manufacturer_code IS NULL OR char_length(manufacturer_code) BETWEEN 1 AND 80),
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','PAUSED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_products_category_scope_fk
        FOREIGN KEY (category_id, business_id) REFERENCES shop_categories (id, business_id),
    UNIQUE (id, business_id)
);

CREATE INDEX shop_products_business_status_idx ON shop_products (business_id, status, created_at DESC);
CREATE INDEX shop_products_category_idx ON shop_products (business_id, category_id, status);
CREATE INDEX shop_products_brand_idx ON shop_products (business_id, lower(brand)) WHERE brand IS NOT NULL;
CREATE INDEX shop_products_class_idx ON shop_products (business_id, product_class);

CREATE TABLE shop_product_media (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    media_ref text NOT NULL CHECK (char_length(media_ref) BETWEEN 1 AND 200),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_product_media_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id) ON DELETE CASCADE,
    UNIQUE (product_id, media_ref)
);

CREATE INDEX shop_product_media_order_idx ON shop_product_media (product_id, position, id);

CREATE TABLE shop_product_operating_models (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    operating_model text NOT NULL CHECK (operating_model IN (
        'SUPERMARKET','GROCERY_SHOP','FOODSTUFF_DEALER','MARKET_TRADER','WHOLESALER')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_product_operating_models_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id) ON DELETE CASCADE,
    UNIQUE (product_id, operating_model)
);

-- ---------------------------------------------------------------------------
-- Variants / SKUs
-- ---------------------------------------------------------------------------
--
-- Physical metadata is integer in a fixed base (grams, millilitres,
-- millimetres). Prices are integer minor units. A NULL wholesale base means
-- "this SKU has no wholesale price of its own", which is different from zero.
CREATE TABLE shop_variants (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    sku text CHECK (sku IS NULL OR sku ~ '^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    package_level text NOT NULL DEFAULT 'EACH' CHECK (package_level IN (
        'EACH','SACHET','PACK','ROLL','BUNDLE','TRAY','BAG','SACK','CARTON','CASE',
        'CRATE','DRUM','PALLET','OTHER_CONTROLLED')),
    pack_quantity integer NOT NULL DEFAULT 1 CHECK (pack_quantity BETWEEN 1 AND 100000),
    net_weight_grams bigint CHECK (net_weight_grams IS NULL OR (net_weight_grams > 0 AND net_weight_grams <= 100000000)),
    net_volume_ml bigint CHECK (net_volume_ml IS NULL OR (net_volume_ml > 0 AND net_volume_ml <= 100000000)),
    length_mm integer CHECK (length_mm IS NULL OR (length_mm > 0 AND length_mm <= 100000)),
    width_mm integer CHECK (width_mm IS NULL OR (width_mm > 0 AND width_mm <= 100000)),
    height_mm integer CHECK (height_mm IS NULL OR (height_mm > 0 AND height_mm <= 100000)),
    retail_price_minor bigint CHECK (retail_price_minor IS NULL OR (retail_price_minor >= 0 AND retail_price_minor <= 99999999999)),
    wholesale_price_minor bigint CHECK (wholesale_price_minor IS NULL OR (wholesale_price_minor >= 0 AND wholesale_price_minor <= 99999999999)),
    temperature_zone text NOT NULL DEFAULT 'AMBIENT' CHECK (temperature_zone IN ('AMBIENT','FRESH','CHILLED','FROZEN')),
    consolidation_class text CHECK (consolidation_class IS NULL OR consolidation_class IN (
        'GENERAL','DRY_GOODS','FRESH_PRODUCE','RAW_PROTEIN','FROZEN','CHILLED','LIQUID','FRAGILE','BULK_LOAD')),
    separation_group text CHECK (separation_group IS NULL OR char_length(separation_group) BETWEEN 1 AND 60),
    stackable boolean NOT NULL DEFAULT true,
    large_load boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','PAUSED','ARCHIVED')),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_variants_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id),
    UNIQUE (id, business_id),
    UNIQUE (id, product_id)
);

CREATE UNIQUE INDEX shop_variants_sku_uq ON shop_variants (business_id, upper(sku)) WHERE sku IS NOT NULL;
CREATE INDEX shop_variants_product_idx ON shop_variants (product_id, position, id);
CREATE INDEX shop_variants_business_status_idx ON shop_variants (business_id, status);

CREATE TABLE shop_variant_handling_flags (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    variant_id text NOT NULL,
    handling_flag text NOT NULL CHECK (handling_flag IN (
        'FRAGILE','UPRIGHT_REQUIRED','CRUSH_SENSITIVE','LEAK_RISK','LIQUID',
        'ODOR_SENSITIVE','RAW_PROTEIN','KEEP_DRY','KEEP_COLD','STACK_RESTRICTED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_variant_handling_flags_variant_scope_fk
        FOREIGN KEY (variant_id, business_id) REFERENCES shop_variants (id, business_id) ON DELETE CASCADE,
    UNIQUE (variant_id, handling_flag)
);

-- ---------------------------------------------------------------------------
-- Trade identifiers
-- ---------------------------------------------------------------------------
--
-- Uniqueness is per business and per identifier type. KarryGo does not claim
-- a GTIN is globally unique or externally certified — storing a barcode is
-- storage, not verification.
CREATE TABLE shop_product_identifiers (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    variant_id text NOT NULL,
    identifier_type text NOT NULL CHECK (identifier_type IN (
        'INTERNAL_SKU','GTIN','EAN','UPC','BARCODE','MANUFACTURER_CODE','OTHER_CONTROLLED')),
    identifier_value text NOT NULL CHECK (identifier_value ~ '^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$'),
    package_level text CHECK (package_level IS NULL OR package_level IN (
        'EACH','SACHET','PACK','ROLL','BUNDLE','TRAY','BAG','SACK','CARTON','CASE',
        'CRATE','DRUM','PALLET','OTHER_CONTROLLED')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_product_identifiers_variant_scope_fk
        FOREIGN KEY (variant_id, business_id) REFERENCES shop_variants (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_product_identifiers_variant_product_fk
        FOREIGN KEY (variant_id, product_id) REFERENCES shop_variants (id, product_id),
    UNIQUE (id, business_id)
);

-- Numeric trade identifiers must not collide within a tenant. INTERNAL_SKU
-- and MANUFACTURER_CODE share the same rule; OTHER_CONTROLLED does not,
-- because its meaning is by definition vendor-private.
CREATE UNIQUE INDEX shop_product_identifiers_value_uq
    ON shop_product_identifiers (business_id, identifier_type, upper(identifier_value))
    WHERE status = 'ACTIVE' AND identifier_type <> 'OTHER_CONTROLLED';
CREATE INDEX shop_product_identifiers_lookup_idx
    ON shop_product_identifiers (business_id, upper(identifier_value)) WHERE status = 'ACTIVE';
CREATE INDEX shop_product_identifiers_variant_idx
    ON shop_product_identifiers (variant_id, identifier_type);

-- ---------------------------------------------------------------------------
-- Packaging hierarchy
-- ---------------------------------------------------------------------------
--
-- 1 carton = 24 packs is a CONFIGURED fact, never inferred from a name. Both
-- ends must belong to the same product, which the composite foreign keys make
-- structurally true rather than merely validated.
CREATE TABLE shop_package_relations (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    parent_variant_id text NOT NULL,
    child_variant_id text NOT NULL,
    units_per_parent integer NOT NULL CHECK (units_per_parent BETWEEN 1 AND 1000000),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_package_relations_no_self CHECK (parent_variant_id <> child_variant_id),
    CONSTRAINT shop_package_relations_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_package_relations_parent_fk
        FOREIGN KEY (parent_variant_id, product_id) REFERENCES shop_variants (id, product_id),
    CONSTRAINT shop_package_relations_child_fk
        FOREIGN KEY (child_variant_id, product_id) REFERENCES shop_variants (id, product_id),
    UNIQUE (parent_variant_id, child_variant_id),
    UNIQUE (id, business_id)
);

CREATE INDEX shop_package_relations_parent_idx ON shop_package_relations (parent_variant_id, status);
CREATE INDEX shop_package_relations_child_idx ON shop_package_relations (child_variant_id, status);

CREATE FUNCTION shop_package_relations_validate_tree() RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'ACTIVE' THEN
        RETURN NEW;
    END IF;
    IF NEW.parent_variant_id = NEW.child_variant_id OR EXISTS (
        WITH RECURSIVE down AS (
            SELECT child_variant_id AS id FROM shop_package_relations
            WHERE business_id=NEW.business_id AND parent_variant_id=NEW.child_variant_id
              AND status='ACTIVE' AND id<>NEW.id
            UNION
            SELECT r.child_variant_id FROM shop_package_relations r
            JOIN down ON r.parent_variant_id=down.id
            WHERE r.business_id=NEW.business_id AND r.status='ACTIVE' AND r.id<>NEW.id
        )
        SELECT 1 FROM down WHERE id=NEW.parent_variant_id
    ) THEN
        RAISE EXCEPTION 'shop_package_relations: relationship would create a cycle'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shop_package_relations_validate_tree_trigger
    BEFORE INSERT OR UPDATE OF parent_variant_id,child_variant_id,status ON shop_package_relations
    FOR EACH ROW EXECUTE FUNCTION shop_package_relations_validate_tree();

-- ---------------------------------------------------------------------------
-- Measure calibration
-- ---------------------------------------------------------------------------
--
-- The answer to "how much rice is in YOUR paint rubber". Per product, and
-- optionally per branch, because the honest answer differs by commodity, by
-- container and by trader. A calibration is explicitly APPROXIMATE evidence
-- and carries its own tolerance and source; nothing may promote it to an
-- exact conversion.
CREATE TABLE shop_measure_calibrations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    product_id text NOT NULL,
    variant_id text,
    measure_definition_id text NOT NULL,
    measure_owner_key text NOT NULL,
    approximate_weight_grams bigint CHECK (approximate_weight_grams IS NULL
        OR (approximate_weight_grams > 0 AND approximate_weight_grams <= 100000000)),
    approximate_volume_ml bigint CHECK (approximate_volume_ml IS NULL
        OR (approximate_volume_ml > 0 AND approximate_volume_ml <= 100000000)),
    tolerance_bps integer NOT NULL DEFAULT 0 CHECK (tolerance_bps BETWEEN 0 AND 10000),
    calibration_source text NOT NULL CHECK (calibration_source IN (
        'VENDOR_MEASURED','MANUFACTURER_DECLARED','PACKAGE_DECLARED','PLATFORM_VERIFIED','OTHER_CONTROLLED')),
    effective_from timestamptz NOT NULL DEFAULT now(),
    effective_to timestamptz,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_measure_calibrations_amount_present CHECK (
        approximate_weight_grams IS NOT NULL OR approximate_volume_ml IS NOT NULL),
    CONSTRAINT shop_measure_calibrations_window_check CHECK (
        effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT shop_measure_calibrations_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_measure_calibrations_variant_scope_fk
        FOREIGN KEY (variant_id, product_id) REFERENCES shop_variants (id, product_id),
    CONSTRAINT shop_measure_calibrations_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT shop_measure_calibrations_measure_fk
        FOREIGN KEY (measure_definition_id, measure_owner_key)
        REFERENCES shop_measure_definitions (id, owner_key),
    -- The tenant may reference a platform measure or its own. Never another's.
    CONSTRAINT shop_measure_calibrations_measure_owner_check CHECK (
        measure_owner_key = 'PLATFORM' OR measure_owner_key = business_id),
    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX shop_measure_calibrations_active_uq
    ON shop_measure_calibrations (product_id, COALESCE(variant_id, ''), COALESCE(branch_id, ''), measure_definition_id)
    WHERE status = 'ACTIVE';
CREATE INDEX shop_measure_calibrations_lookup_idx
    ON shop_measure_calibrations (business_id, product_id, status);

-- ---------------------------------------------------------------------------
-- Sell units
-- ---------------------------------------------------------------------------
--
-- The first-class answer to "in what units may a customer buy this". One
-- variant may have many: 1 Derica, ½ Paint, 1 Paint, a 50 kg bag.
--
-- FRACTIONS, NOT ENUM EXPLOSION. "Half Paint" is PAINT with numerator 1 and
-- denominator 2 — not a HALF_PAINT measure — so the measurement vocabulary
-- stays finite and the fraction stays exact rational arithmetic.
--
-- conversion_type is explicit and never inferred. EXACT carries an exact base
-- quantity; CALIBRATED_APPROXIMATE points at a calibration and must not be
-- treated as exact by any caller; NONE admits that no conversion is known.
CREATE TABLE shop_sell_units (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    product_id text NOT NULL,
    variant_id text NOT NULL,
    measure_definition_id text NOT NULL,
    measure_owner_key text NOT NULL,
    fraction_numerator integer NOT NULL DEFAULT 1 CHECK (fraction_numerator BETWEEN 1 AND 1000000),
    fraction_denominator integer NOT NULL DEFAULT 1 CHECK (fraction_denominator BETWEEN 1 AND 1000),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 120),
    measurement_mode text NOT NULL DEFAULT 'FIXED' CHECK (measurement_mode IN (
        'FIXED','VARIABLE_WEIGHT','VARIABLE_COUNT','LOCAL_CONTAINER','PACKAGE_COUNT','PRICE_PORTION')),
    conversion_type text NOT NULL DEFAULT 'NONE' CHECK (conversion_type IN (
        'EXACT','CALIBRATED_APPROXIMATE','NONE')),
    -- Exact conversion, in the canonical base of exact_base_family.
    exact_base_family text CHECK (exact_base_family IS NULL OR exact_base_family IN ('MASS','VOLUME','COUNT')),
    exact_base_quantity bigint CHECK (exact_base_quantity IS NULL
        OR (exact_base_quantity > 0 AND exact_base_quantity <= 100000000000)),
    calibration_id text,
    -- Non-authoritative display estimate for variable modes. Never used as a
    -- stock conversion.
    estimated_base_quantity bigint CHECK (estimated_base_quantity IS NULL
        OR (estimated_base_quantity > 0 AND estimated_base_quantity <= 100000000000)),
    underfill_tolerance_bps integer NOT NULL DEFAULT 0 CHECK (underfill_tolerance_bps BETWEEN 0 AND 10000),
    overfill_tolerance_bps integer NOT NULL DEFAULT 0 CHECK (overfill_tolerance_bps BETWEEN 0 AND 10000),
    actual_measurement_required boolean NOT NULL DEFAULT false,
    display_precision smallint NOT NULL DEFAULT 0 CHECK (display_precision BETWEEN 0 AND 3),
    price_minor bigint CHECK (price_minor IS NULL OR (price_minor >= 0 AND price_minor <= 99999999999)),
    wholesale_price_minor bigint CHECK (wholesale_price_minor IS NULL
        OR (wholesale_price_minor >= 0 AND wholesale_price_minor <= 99999999999)),
    retail_enabled boolean NOT NULL DEFAULT true,
    wholesale_enabled boolean NOT NULL DEFAULT false,
    minimum_order_quantity bigint NOT NULL DEFAULT 1 CHECK (minimum_order_quantity BETWEEN 1 AND 1000000000),
    order_multiple bigint NOT NULL DEFAULT 1 CHECK (order_multiple BETWEEN 1 AND 1000000000),
    maximum_order_quantity bigint CHECK (maximum_order_quantity IS NULL
        OR (maximum_order_quantity > 0 AND maximum_order_quantity <= 1000000000)),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','PAUSED','ARCHIVED')),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    -- An EXACT conversion must actually carry one, and only an EXACT one may.
    CONSTRAINT shop_sell_units_exact_check CHECK (
        (conversion_type = 'EXACT'
            AND exact_base_family IS NOT NULL AND exact_base_quantity IS NOT NULL
            AND calibration_id IS NULL)
        OR (conversion_type = 'CALIBRATED_APPROXIMATE'
            AND calibration_id IS NOT NULL
            AND exact_base_family IS NULL AND exact_base_quantity IS NULL)
        OR (conversion_type = 'NONE'
            AND calibration_id IS NULL
            AND exact_base_family IS NULL AND exact_base_quantity IS NULL)),
    -- A price portion IS a price: "₦500 of tomatoes" has no meaning without one.
    CONSTRAINT shop_sell_units_price_portion_check CHECK (
        measurement_mode <> 'PRICE_PORTION' OR price_minor IS NOT NULL),
    CONSTRAINT shop_sell_units_max_order_check CHECK (
        maximum_order_quantity IS NULL OR maximum_order_quantity >= minimum_order_quantity),
    CONSTRAINT shop_sell_units_variant_scope_fk
        FOREIGN KEY (variant_id, business_id) REFERENCES shop_variants (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_sell_units_variant_product_fk
        FOREIGN KEY (variant_id, product_id) REFERENCES shop_variants (id, product_id),
    CONSTRAINT shop_sell_units_measure_fk
        FOREIGN KEY (measure_definition_id, measure_owner_key)
        REFERENCES shop_measure_definitions (id, owner_key),
    CONSTRAINT shop_sell_units_measure_owner_check CHECK (
        measure_owner_key = 'PLATFORM' OR measure_owner_key = business_id),
    CONSTRAINT shop_sell_units_calibration_scope_fk
        FOREIGN KEY (calibration_id, business_id) REFERENCES shop_measure_calibrations (id, business_id),
    UNIQUE (id, business_id),
    UNIQUE (id, variant_id)
);

CREATE UNIQUE INDEX shop_sell_units_shape_uq
    ON shop_sell_units (variant_id, measure_definition_id, fraction_numerator, fraction_denominator)
    WHERE status <> 'ARCHIVED';
CREATE INDEX shop_sell_units_variant_idx ON shop_sell_units (variant_id, position, id);
CREATE INDEX shop_sell_units_product_idx ON shop_sell_units (business_id, product_id, status);

-- ---------------------------------------------------------------------------
-- Branch listings
-- ---------------------------------------------------------------------------
--
-- A Shop product does not sell from every branch merely because it exists.
-- The listing is the branch's commercial decision about one SKU.
--
-- THERE IS NO QUANTITY COLUMN HERE. Availability is Part B's authority, and a
-- convenient integer on this row would be a lie the moment anything sold.
CREATE TABLE shop_branch_listings (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text NOT NULL,
    product_id text NOT NULL,
    variant_id text NOT NULL,
    operating_model text CHECK (operating_model IS NULL OR operating_model IN (
        'SUPERMARKET','GROCERY_SHOP','FOODSTUFF_DEALER','MARKET_TRADER','WHOLESALER')),
    retail_enabled boolean NOT NULL DEFAULT true,
    wholesale_enabled boolean NOT NULL DEFAULT false,
    retail_price_override_minor bigint CHECK (retail_price_override_minor IS NULL
        OR (retail_price_override_minor >= 0 AND retail_price_override_minor <= 99999999999)),
    wholesale_price_override_minor bigint CHECK (wholesale_price_override_minor IS NULL
        OR (wholesale_price_override_minor >= 0 AND wholesale_price_override_minor <= 99999999999)),
    minimum_order_quantity bigint NOT NULL DEFAULT 1 CHECK (minimum_order_quantity BETWEEN 1 AND 1000000000),
    order_multiple bigint NOT NULL DEFAULT 1 CHECK (order_multiple BETWEEN 1 AND 1000000000),
    maximum_order_quantity bigint CHECK (maximum_order_quantity IS NULL
        OR (maximum_order_quantity > 0 AND maximum_order_quantity <= 1000000000)),
    -- Declared commercial intent only. Part B executes preorder/backorder; a
    -- policy recorded here promises nothing about stock.
    preorder_policy text NOT NULL DEFAULT 'NOT_ALLOWED'
        CHECK (preorder_policy IN ('NOT_ALLOWED','ALLOWED')),
    backorder_policy text NOT NULL DEFAULT 'NOT_ALLOWED'
        CHECK (backorder_policy IN ('NOT_ALLOWED','ALLOWED')),
    auto_hide_when_unavailable boolean NOT NULL DEFAULT true,
    logistics_available boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_branch_listings_max_order_check CHECK (
        maximum_order_quantity IS NULL OR maximum_order_quantity >= minimum_order_quantity),
    CONSTRAINT shop_branch_listings_sale_mode_check CHECK (retail_enabled OR wholesale_enabled),
    CONSTRAINT shop_branch_listings_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT shop_branch_listings_variant_scope_fk
        FOREIGN KEY (variant_id, business_id) REFERENCES shop_variants (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_branch_listings_variant_product_fk
        FOREIGN KEY (variant_id, product_id) REFERENCES shop_variants (id, product_id),
    UNIQUE (branch_id, variant_id),
    UNIQUE (id, business_id)
);

CREATE INDEX shop_branch_listings_branch_idx ON shop_branch_listings (business_id, branch_id, status);
CREATE INDEX shop_branch_listings_variant_idx ON shop_branch_listings (variant_id, status);
CREATE INDEX shop_branch_listings_product_idx ON shop_branch_listings (business_id, product_id);

CREATE TABLE shop_listing_sell_units (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    listing_id text NOT NULL,
    sell_unit_id text NOT NULL,
    variant_id text NOT NULL,
    -- Optional branch-specific calibrated conversion. The base sell unit is
    -- business-wide; a branch calibration belongs here so it cannot silently
    -- become the conversion used by every other branch.
    calibration_id text,
    enabled boolean NOT NULL DEFAULT true,
    price_override_minor bigint CHECK (price_override_minor IS NULL
        OR (price_override_minor >= 0 AND price_override_minor <= 99999999999)),
    wholesale_price_override_minor bigint CHECK (wholesale_price_override_minor IS NULL
        OR (wholesale_price_override_minor >= 0 AND wholesale_price_override_minor <= 99999999999)),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_listing_sell_units_listing_scope_fk
        FOREIGN KEY (listing_id, business_id) REFERENCES shop_branch_listings (id, business_id) ON DELETE CASCADE,
    -- The sell unit must belong to the very variant the listing lists.
    CONSTRAINT shop_listing_sell_units_unit_variant_fk
        FOREIGN KEY (sell_unit_id, variant_id) REFERENCES shop_sell_units (id, variant_id),
    CONSTRAINT shop_listing_sell_units_calibration_scope_fk
        FOREIGN KEY (calibration_id, business_id) REFERENCES shop_measure_calibrations (id, business_id),
    UNIQUE (listing_id, sell_unit_id),
    UNIQUE (id, business_id)
);

CREATE INDEX shop_listing_sell_units_order_idx ON shop_listing_sell_units (listing_id, position, id);

-- ---------------------------------------------------------------------------
-- Quantity breaks
-- ---------------------------------------------------------------------------
--
-- Wholesale tiering. Bands are half-open in intent but stored inclusive, and
-- an active band may not overlap another for the same scope — enforced by the
-- trigger below, because an overlap makes "the price" ambiguous and a pricing
-- ambiguity is a commercial defect, not a display problem.
CREATE TABLE shop_quantity_breaks (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    listing_id text NOT NULL,
    sell_unit_id text,
    sell_unit_key text GENERATED ALWAYS AS (COALESCE(sell_unit_id, '')) STORED,
    min_quantity bigint NOT NULL CHECK (min_quantity BETWEEN 1 AND 1000000000),
    max_quantity bigint CHECK (max_quantity IS NULL OR (max_quantity > 0 AND max_quantity <= 1000000000)),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0 AND unit_price_minor <= 99999999999),
    wholesale_only boolean NOT NULL DEFAULT true,
    lead_time_days integer CHECK (lead_time_days IS NULL OR (lead_time_days >= 0 AND lead_time_days <= 365)),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_quantity_breaks_bounds_check CHECK (
        max_quantity IS NULL OR max_quantity >= min_quantity),
    CONSTRAINT shop_quantity_breaks_listing_scope_fk
        FOREIGN KEY (listing_id, business_id) REFERENCES shop_branch_listings (id, business_id) ON DELETE CASCADE,
    CONSTRAINT shop_quantity_breaks_sell_unit_scope_fk
        FOREIGN KEY (sell_unit_id, business_id) REFERENCES shop_sell_units (id, business_id),
    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX shop_quantity_breaks_min_uq
    ON shop_quantity_breaks (listing_id, sell_unit_key, min_quantity) WHERE status = 'ACTIVE';
-- At most one open-ended band per scope: two would both claim "and above".
CREATE UNIQUE INDEX shop_quantity_breaks_open_ended_uq
    ON shop_quantity_breaks (listing_id, sell_unit_key)
    WHERE status = 'ACTIVE' AND max_quantity IS NULL;
CREATE INDEX shop_quantity_breaks_lookup_idx
    ON shop_quantity_breaks (listing_id, sell_unit_key, min_quantity) WHERE status = 'ACTIVE';

-- Overlap is refused by the database, not only by the service. btree_gist is
-- not assumed available, so this is a plain row trigger rather than an
-- exclusion constraint; the guarantee is the same.
CREATE FUNCTION shop_quantity_breaks_no_overlap() RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'ACTIVE' THEN
        RETURN NEW;
    END IF;
    IF EXISTS (
        SELECT 1 FROM shop_quantity_breaks other
        WHERE other.listing_id = NEW.listing_id
          AND other.sell_unit_key = COALESCE(NEW.sell_unit_id, '')
          AND other.id <> NEW.id
          AND other.status = 'ACTIVE'
          AND NEW.min_quantity <= COALESCE(other.max_quantity, 9223372036854775807)
          AND other.min_quantity <= COALESCE(NEW.max_quantity, 9223372036854775807)
    ) THEN
        RAISE EXCEPTION 'shop_quantity_breaks: band [%,%] overlaps an active band for this listing scope',
            NEW.min_quantity, NEW.max_quantity USING ERRCODE = 'exclusion_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shop_quantity_breaks_no_overlap_trigger
    BEFORE INSERT OR UPDATE ON shop_quantity_breaks
    FOR EACH ROW EXECUTE FUNCTION shop_quantity_breaks_no_overlap();

-- ---------------------------------------------------------------------------
-- Fresh-goods configuration
-- ---------------------------------------------------------------------------
--
-- Not every attribute applies to every product: descaling is meaningless for
-- rice and ripeness is meaningless for frozen chicken. The product class
-- decides which attribute types may be attached at all (enforced in the
-- service, where the class-to-attribute map lives), and the option vocabulary
-- is bounded per type.
CREATE TABLE shop_fresh_attributes (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    product_id text NOT NULL,
    attribute_type text NOT NULL CHECK (attribute_type IN (
        'RIPENESS','QUALITY_GRADE','SIZE_GRADE','COLOUR','FIRMNESS','CUT','CLEANING',
        'PEELING','TRIMMING','GUTTING','DESCALING','HEAD','BONE','PRESERVATION',
        'SELECTION','PACKAGING_PREFERENCE')),
    selection_required boolean NOT NULL DEFAULT false,
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_fresh_attributes_product_scope_fk
        FOREIGN KEY (product_id, business_id) REFERENCES shop_products (id, business_id) ON DELETE CASCADE,
    UNIQUE (product_id, attribute_type),
    UNIQUE (id, business_id)
);

CREATE TABLE shop_fresh_attribute_options (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    attribute_id text NOT NULL,
    option_value text NOT NULL CHECK (option_value ~ '^[A-Z][A-Z0-9_]{0,39}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),
    is_default boolean NOT NULL DEFAULT false,
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_fresh_attribute_options_attribute_scope_fk
        FOREIGN KEY (attribute_id, business_id) REFERENCES shop_fresh_attributes (id, business_id) ON DELETE CASCADE,
    UNIQUE (attribute_id, option_value),
    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX shop_fresh_attribute_options_default_uq
    ON shop_fresh_attribute_options (attribute_id) WHERE is_default AND status = 'ACTIVE';
CREATE INDEX shop_fresh_attribute_options_order_idx
    ON shop_fresh_attribute_options (attribute_id, position, id);

-- ---------------------------------------------------------------------------
-- Price history
-- ---------------------------------------------------------------------------
--
-- Append-only commercial evidence, enforced by trigger. A future Order will
-- snapshot an exact (subject_type, subject_ref, revision) rather than
-- recomputing a historical total from today's catalogue.
CREATE TABLE shop_price_versions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    subject_type text NOT NULL CHECK (subject_type IN (
        'VARIANT_RETAIL','VARIANT_WHOLESALE','SELL_UNIT_RETAIL','SELL_UNIT_WHOLESALE',
        'LISTING_RETAIL_OVERRIDE','LISTING_WHOLESALE_OVERRIDE',
        'LISTING_SELL_UNIT_RETAIL','LISTING_SELL_UNIT_WHOLESALE','QUANTITY_BREAK')),
    subject_ref text NOT NULL,
    product_ref text,
    variant_ref text,
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    previous_amount_minor bigint CHECK (previous_amount_minor IS NULL
        OR (previous_amount_minor >= 0 AND previous_amount_minor <= 99999999999)),
    amount_minor bigint CHECK (amount_minor IS NULL
        OR (amount_minor >= 0 AND amount_minor <= 99999999999)),
    previous_inherited boolean NOT NULL DEFAULT false,
    inherited boolean NOT NULL DEFAULT false,
    change_reason text NOT NULL CHECK (change_reason IN (
        'BASELINE_IMPORT','CREATED','PRICE_CHANGED','OVERRIDE_SET','OVERRIDE_CLEARED','ARCHIVED')),
    subject_version bigint NOT NULL CHECK (subject_version > 0),
    revision bigint NOT NULL CHECK (revision > 0),
    actor_type text NOT NULL DEFAULT 'vendor_staff',
    actor_id text,
    correlation_id text,
    command_ref text,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    effective_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_price_versions_inherit_check CHECK ((amount_minor IS NULL) = inherited),
    UNIQUE (subject_type, subject_ref, revision)
);

CREATE INDEX shop_price_versions_variant_idx
    ON shop_price_versions (business_id, variant_ref, recorded_at DESC) WHERE variant_ref IS NOT NULL;
CREATE INDEX shop_price_versions_product_idx
    ON shop_price_versions (business_id, product_ref, recorded_at DESC) WHERE product_ref IS NOT NULL;
CREATE INDEX shop_price_versions_subject_idx
    ON shop_price_versions (subject_type, subject_ref, revision DESC);
CREATE INDEX shop_price_versions_business_recorded_idx
    ON shop_price_versions (business_id, recorded_at DESC);

CREATE FUNCTION shop_price_versions_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'shop_price_versions is append-only: % is not permitted', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shop_price_versions_no_update_or_delete
    BEFORE UPDATE OR DELETE ON shop_price_versions
    FOR EACH ROW EXECUTE FUNCTION shop_price_versions_immutable();

-- ---------------------------------------------------------------------------
-- Catalogue revision
-- ---------------------------------------------------------------------------
--
-- Monotonic per business+branch, advanced inside the mutation's own
-- transaction. Part B's Shop discovery reads it to answer "has anything I
-- show changed" without diffing the catalogue.
CREATE TABLE shop_catalogue_revisions (
    business_id text NOT NULL,
    branch_id text NOT NULL,
    domain text NOT NULL DEFAULT 'SHOP' CHECK (domain = 'SHOP'),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, domain),
    CONSTRAINT shop_catalogue_revisions_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

-- ---------------------------------------------------------------------------
-- Platform measurement seed
-- ---------------------------------------------------------------------------
--
-- Standard measures carry an exact base quantity. Local containers, packages,
-- grouped counts and loads carry NONE — deliberately and permanently. There
-- is no row here that says a Paint, a Derica, a Mudu or a Bag weighs anything,
-- because that depends on the commodity, the container and the trader, and a
-- global constant would be wrong for every one of them. Vendors calibrate
-- their own containers per product in shop_measure_calibrations.
INSERT INTO shop_measure_definitions (id, business_id, code, display_name, measurement_family, measure_origin, base_quantity)
VALUES
    ('smd_gram',        NULL, 'GRAM',        'Gram',        'MASS',   'STANDARD', 1),
    ('smd_kilogram',    NULL, 'KILOGRAM',    'Kilogram',    'MASS',   'STANDARD', 1000),
    ('smd_millilitre',  NULL, 'MILLILITRE',  'Millilitre',  'VOLUME', 'STANDARD', 1),
    ('smd_centilitre',  NULL, 'CENTILITRE',  'Centilitre',  'VOLUME', 'STANDARD', 10),
    ('smd_litre',       NULL, 'LITRE',       'Litre',       'VOLUME', 'STANDARD', 1000),
    ('smd_piece',       NULL, 'PIECE',       'Piece',       'COUNT',  'STANDARD', 1),
    ('smd_dozen',       NULL, 'DOZEN',       'Dozen',       'COUNT',  'STANDARD', 12),

    ('smd_paint',           NULL, 'PAINT',           'Paint',           'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_paint_rubber',    NULL, 'PAINT_RUBBER',    'Paint Rubber',    'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_custard_rubber',  NULL, 'CUSTARD_RUBBER',  'Custard Rubber',  'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_derica',          NULL, 'DERICA',          'Derica',          'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_mudu',            NULL, 'MUDU',            'Mudu',            'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_congo',           NULL, 'CONGO',           'Congo',           'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_tiya',            NULL, 'TIYA',            'Tiya',            'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_kobiowu',         NULL, 'KOBIOWU',         'Kobiowu',         'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_milk_cup',        NULL, 'MILK_CUP',        'Milk Cup',        'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_cigarette_cup',   NULL, 'CIGARETTE_CUP',   'Cigarette Cup',   'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_cup',             NULL, 'CUP',             'Cup',             'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_bowl',            NULL, 'BOWL',            'Bowl',            'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_basin',           NULL, 'BASIN',           'Basin',           'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_basket',          NULL, 'BASKET',          'Basket',          'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_rubber',          NULL, 'RUBBER',          'Rubber',          'LOCAL_CONTAINER', 'LOCAL', NULL),
    ('smd_jerrycan',        NULL, 'JERRYCAN',        'Jerrycan',        'LOCAL_CONTAINER', 'LOCAL', NULL),

    ('smd_heap',    NULL, 'HEAP',   'Heap',   'GROUPED_COUNT', 'LOCAL', NULL),
    ('smd_bunch',   NULL, 'BUNCH',  'Bunch',  'GROUPED_COUNT', 'LOCAL', NULL),
    ('smd_bundle',  NULL, 'BUNDLE', 'Bundle', 'GROUPED_COUNT', 'LOCAL', NULL),
    ('smd_stalk',   NULL, 'STALK',  'Stalk',  'GROUPED_COUNT', 'LOCAL', NULL),
    ('smd_stick',   NULL, 'STICK',  'Stick',  'GROUPED_COUNT', 'LOCAL', NULL),
    ('smd_tuber',   NULL, 'TUBER',  'Tuber',  'GROUPED_COUNT', 'LOCAL', NULL),

    ('smd_bag',      NULL, 'BAG',     'Bag',     'PACKAGE', 'PACKAGE', NULL),
    ('smd_sack',     NULL, 'SACK',    'Sack',    'PACKAGE', 'PACKAGE', NULL),
    ('smd_crate',    NULL, 'CRATE',   'Crate',   'PACKAGE', 'PACKAGE', NULL),
    ('smd_carton',   NULL, 'CARTON',  'Carton',  'PACKAGE', 'PACKAGE', NULL),
    ('smd_tray',     NULL, 'TRAY',    'Tray',    'PACKAGE', 'PACKAGE', NULL),
    ('smd_drum',     NULL, 'DRUM',    'Drum',    'PACKAGE', 'PACKAGE', NULL),
    ('smd_bottle',   NULL, 'BOTTLE',  'Bottle',  'PACKAGE', 'PACKAGE', NULL),
    ('smd_can',      NULL, 'CAN',     'Can',     'PACKAGE', 'PACKAGE', NULL),
    ('smd_pack',     NULL, 'PACK',    'Pack',    'PACKAGE', 'PACKAGE', NULL),
    ('smd_sachet',   NULL, 'SACHET',  'Sachet',  'PACKAGE', 'PACKAGE', NULL),
    ('smd_roll',     NULL, 'ROLL',    'Roll',    'PACKAGE', 'PACKAGE', NULL),

    ('smd_wheelbarrow',  NULL, 'WHEELBARROW',  'Wheelbarrow',  'LOAD', 'LOCAL', NULL),
    ('smd_pickup_load',  NULL, 'PICKUP_LOAD',  'Pickup Load',  'LOAD', 'LOCAL', NULL),

    ('smd_price_portion', NULL, 'PRICE_PORTION', 'Price Portion', 'PRICE_PORTION', 'LOCAL', NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO shop_measure_aliases (id, measure_definition_id, owner_key, business_id, alias, region_code)
VALUES
    ('sma_paint_bucket',   'smd_paint',         'PLATFORM', NULL, 'Paint Bucket', 'NG'),
    ('sma_paint_rubber',   'smd_paint',         'PLATFORM', NULL, 'Paint Rubber', 'NG'),
    ('sma_rubber_paint',   'smd_paint_rubber',  'PLATFORM', NULL, 'Rubber',       'NG'),
    ('sma_custard',        'smd_custard_rubber','PLATFORM', NULL, 'Custard',      'NG'),
    ('sma_derica_cup',     'smd_derica',        'PLATFORM', NULL, 'Derica Cup',   'NG'),
    ('sma_mudu_measure',   'smd_mudu',          'PLATFORM', NULL, 'Muddu',        'NG'),
    ('sma_congo_measure',  'smd_congo',         'PLATFORM', NULL, 'Kongo',        'NG'),
    ('sma_milk_tin',       'smd_milk_cup',      'PLATFORM', NULL, 'Milk Tin',     'NG'),
    ('sma_egg_crate',      'smd_tray',          'PLATFORM', NULL, 'Egg Crate',    'NG'),
    ('sma_kg',             'smd_kilogram',      'PLATFORM', NULL, 'kg',           NULL),
    ('sma_litre_short',    'smd_litre',         'PLATFORM', NULL, 'L',            NULL)
ON CONFLICT (id) DO NOTHING;
