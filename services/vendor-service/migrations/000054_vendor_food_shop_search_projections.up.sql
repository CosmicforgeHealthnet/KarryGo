-- Section 13D
-- Physically isolated FOOD and SHOP search projections.
--
-- These tables are READ PROJECTIONS ONLY.
-- They never become catalogue/product authority.
--
-- Authoritative chain:
--
-- Food / Shop source domain
--          |
--          v
-- Search projection
--          |
--          v
-- Search backend
--
-- No CREATE EXTENSION is intentionally present here.

CREATE TABLE food_search_documents (
    generation BIGINT NOT NULL
        CHECK (generation > 0),

    document_id TEXT NOT NULL,

    business_ref TEXT NOT NULL,
    branch_ref TEXT NOT NULL,
    food_item_ref TEXT NOT NULL,

    menu_refs TEXT[] NOT NULL DEFAULT '{}'::text[],

    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,

    search_tokens TEXT[] NOT NULL DEFAULT '{}'::text[],
    aliases TEXT[] NOT NULL DEFAULT '{}'::text[],
    synonyms TEXT[] NOT NULL DEFAULT '{}'::text[],

    canonical_taxonomy JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(canonical_taxonomy) = 'object'),

    provider_types TEXT[] NOT NULL DEFAULT '{}'::text[],
    cuisine_terms TEXT[] NOT NULL DEFAULT '{}'::text[],
    meal_periods TEXT[] NOT NULL DEFAULT '{}'::text[],
    dish_types TEXT[] NOT NULL DEFAULT '{}'::text[],
    food_groups TEXT[] NOT NULL DEFAULT '{}'::text[],
    occasions TEXT[] NOT NULL DEFAULT '{}'::text[],

    dietary_claims TEXT[] NOT NULL DEFAULT '{}'::text[],
    preference_claims TEXT[] NOT NULL DEFAULT '{}'::text[],

    active BOOLEAN NOT NULL DEFAULT TRUE,

    availability_summary JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(availability_summary) = 'object'),

    effective_price_summary JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(effective_price_summary) = 'object'),

    serviceability_hint JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(serviceability_hint) = 'object'),

    trust_hint JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(trust_hint) = 'object'),

    reliability_hint JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(reliability_hint) = 'object'),

    promotion_indicator BOOLEAN NOT NULL DEFAULT FALSE,

    catalogue_revision BIGINT NOT NULL
        CHECK (catalogue_revision > 0),

    projection_revision BIGINT NOT NULL DEFAULT 1
        CHECK (projection_revision > 0),

    search_text TEXT NOT NULL,

    search_vector TSVECTOR
        GENERATED ALWAYS AS (
            to_tsvector(
                'simple'::regconfig,
                search_text
            )
        ) STORED,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (
        generation,
        document_id
    ),

    CONSTRAINT food_search_document_identity_uq
        UNIQUE (
            generation,
            branch_ref,
            food_item_ref
        ),

    CONSTRAINT food_search_document_refs_ck
        CHECK (
            btrim(document_id) <> ''
            AND btrim(business_ref) <> ''
            AND btrim(branch_ref) <> ''
            AND btrim(food_item_ref) <> ''
            AND btrim(name) <> ''
            AND btrim(normalized_name) <> ''
            AND btrim(search_text) <> ''
        )
);

CREATE INDEX food_search_documents_vector_gin_idx
    ON food_search_documents
    USING GIN (search_vector);

CREATE INDEX food_search_documents_generation_active_idx
    ON food_search_documents (
        generation,
        active,
        branch_ref
    );

CREATE INDEX food_search_documents_source_idx
    ON food_search_documents (
        branch_ref,
        food_item_ref,
        generation
    );

CREATE INDEX food_search_documents_catalogue_revision_idx
    ON food_search_documents (
        generation,
        catalogue_revision
    );


CREATE TABLE shop_search_documents (
    generation BIGINT NOT NULL
        CHECK (generation > 0),

    document_id TEXT NOT NULL,

    business_ref TEXT NOT NULL,
    branch_ref TEXT NOT NULL,

    product_ref TEXT NOT NULL,
    variant_ref TEXT,
    listing_ref TEXT,

    name TEXT NOT NULL,
    brand TEXT,

    normalized_name TEXT NOT NULL,

    search_tokens TEXT[] NOT NULL DEFAULT '{}'::text[],
    aliases TEXT[] NOT NULL DEFAULT '{}'::text[],
    synonyms TEXT[] NOT NULL DEFAULT '{}'::text[],

    canonical_taxonomy JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(canonical_taxonomy) = 'object'),

    sell_units TEXT[] NOT NULL DEFAULT '{}'::text[],
    measure_names TEXT[] NOT NULL DEFAULT '{}'::text[],
    local_measure_aliases TEXT[] NOT NULL DEFAULT '{}'::text[],
    packaging_terms TEXT[] NOT NULL DEFAULT '{}'::text[],

    sku_terms TEXT[] NOT NULL DEFAULT '{}'::text[],
    barcode_terms TEXT[] NOT NULL DEFAULT '{}'::text[],

    retail_eligible BOOLEAN NOT NULL DEFAULT TRUE,
    wholesale_eligible BOOLEAN NOT NULL DEFAULT FALSE,

    active BOOLEAN NOT NULL DEFAULT TRUE,

    effective_price_summary JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(effective_price_summary) = 'object'),

    inventory_availability_summary JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(inventory_availability_summary) = 'object'),

    preorder_hint BOOLEAN NOT NULL DEFAULT FALSE,
    backorder_hint BOOLEAN NOT NULL DEFAULT FALSE,

    fresh_attributes TEXT[] NOT NULL DEFAULT '{}'::text[],

    temperature_zone TEXT,

    handling_metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(handling_metadata) = 'object'),

    catalogue_revision BIGINT NOT NULL
        CHECK (catalogue_revision > 0),

    inventory_revision BIGINT NOT NULL
        CHECK (inventory_revision > 0),

    projection_revision BIGINT NOT NULL DEFAULT 1
        CHECK (projection_revision > 0),

    search_text TEXT NOT NULL,

    search_vector TSVECTOR
        GENERATED ALWAYS AS (
            to_tsvector(
                'simple'::regconfig,
                search_text
            )
        ) STORED,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (
        generation,
        document_id
    ),

    CONSTRAINT shop_search_document_refs_ck
        CHECK (
            btrim(document_id) <> ''
            AND btrim(business_ref) <> ''
            AND btrim(branch_ref) <> ''
            AND btrim(product_ref) <> ''
            AND btrim(name) <> ''
            AND btrim(normalized_name) <> ''
            AND btrim(search_text) <> ''
        )
);

CREATE INDEX shop_search_documents_vector_gin_idx
    ON shop_search_documents
    USING GIN (search_vector);

CREATE INDEX shop_search_documents_generation_active_idx
    ON shop_search_documents (
        generation,
        active,
        branch_ref
    );

CREATE INDEX shop_search_documents_source_idx
    ON shop_search_documents (
        branch_ref,
        product_ref,
        variant_ref,
        listing_ref,
        generation
    );

CREATE INDEX shop_search_documents_revision_idx
    ON shop_search_documents (
        generation,
        catalogue_revision,
        inventory_revision
    );


COMMENT ON TABLE food_search_documents IS
    'Derived FOOD search projection. Food catalogue remains authoritative.';

COMMENT ON TABLE shop_search_documents IS
    'Derived SHOP search projection. Shop catalogue/inventory remain authoritative.';