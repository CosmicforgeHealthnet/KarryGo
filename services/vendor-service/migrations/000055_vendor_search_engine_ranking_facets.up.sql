-- Section 13E
-- Search engine ranking, bounded term matching, facets and suggestions.
--
-- Ranking weights are configuration/data.
-- Search code must not scatter ranking weights through Go constants.
--
-- FOOD and SHOP search terms remain physically isolated.
--
-- No PostgreSQL extension is enabled by this migration.

CREATE TABLE search_ranking_profiles (
    id TEXT PRIMARY KEY,

    profile_code TEXT NOT NULL UNIQUE,
    namespace_code TEXT NOT NULL
        CHECK (namespace_code IN ('FOOD', 'SHOP')),

    version INTEGER NOT NULL
        CHECK (version > 0),

    active BOOLEAN NOT NULL DEFAULT TRUE,

    exact_name_weight INTEGER NOT NULL
        CHECK (exact_name_weight BETWEEN 0 AND 10000),

    prefix_weight INTEGER NOT NULL
        CHECK (prefix_weight BETWEEN 0 AND 10000),

    full_text_weight INTEGER NOT NULL
        CHECK (full_text_weight BETWEEN 0 AND 10000),

    alias_weight INTEGER NOT NULL
        CHECK (alias_weight BETWEEN 0 AND 10000),

    synonym_weight INTEGER NOT NULL
        CHECK (synonym_weight BETWEEN 0 AND 10000),

    category_weight INTEGER NOT NULL
        CHECK (category_weight BETWEEN 0 AND 10000),

    tag_weight INTEGER NOT NULL
        CHECK (tag_weight BETWEEN 0 AND 10000),

    brand_weight INTEGER NOT NULL
        CHECK (brand_weight BETWEEN 0 AND 10000),

    identifier_weight INTEGER NOT NULL
        CHECK (identifier_weight BETWEEN 0 AND 10000),

    measurement_weight INTEGER NOT NULL
        CHECK (measurement_weight BETWEEN 0 AND 10000),

    typo_weight INTEGER NOT NULL
        CHECK (typo_weight BETWEEN 0 AND 10000),

    availability_weight INTEGER NOT NULL
        CHECK (availability_weight BETWEEN 0 AND 10000),

    serviceability_weight INTEGER NOT NULL
        CHECK (serviceability_weight BETWEEN 0 AND 10000),

    trust_weight INTEGER NOT NULL
        CHECK (trust_weight BETWEEN 0 AND 10000),

    reliability_weight INTEGER NOT NULL
        CHECK (reliability_weight BETWEEN 0 AND 10000),

    distance_weight INTEGER NOT NULL
        CHECK (distance_weight BETWEEN 0 AND 10000),

    commercial_weight INTEGER NOT NULL
        CHECK (commercial_weight BETWEEN 0 AND 10000),

    promotion_weight INTEGER NOT NULL
        CHECK (promotion_weight BETWEEN 0 AND 10000),

    max_non_text_boost INTEGER NOT NULL
        CHECK (max_non_text_boost BETWEEN 0 AND 10000),

    max_typo_distance INTEGER NOT NULL
        CHECK (max_typo_distance BETWEEN 0 AND 2),

    max_candidates INTEGER NOT NULL
        CHECK (max_candidates BETWEEN 50 AND 2000),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT search_ranking_profile_namespace_version_uq
        UNIQUE (
            namespace_code,
            version
        ),

    CONSTRAINT search_ranking_profile_promotion_bound_ck
        CHECK (
            promotion_weight <= max_non_text_boost
        ),

    CONSTRAINT search_ranking_profile_non_text_relevance_ck
        CHECK (
            max_non_text_boost < exact_name_weight
        )
);

INSERT INTO search_ranking_profiles (
    id,
    profile_code,
    namespace_code,
    version,
    exact_name_weight,
    prefix_weight,
    full_text_weight,
    alias_weight,
    synonym_weight,
    category_weight,
    tag_weight,
    brand_weight,
    identifier_weight,
    measurement_weight,
    typo_weight,
    availability_weight,
    serviceability_weight,
    trust_weight,
    reliability_weight,
    distance_weight,
    commercial_weight,
    promotion_weight,
    max_non_text_boost,
    max_typo_distance,
    max_candidates
)
VALUES
(
    'ranking_food_default_v1',
    'FOOD_DEFAULT_V1',
    'FOOD',
    1,

    1200,
    700,
    850,
    800,
    700,
    520,
    480,

    0,
    0,
    0,

    420,

    70,
    70,
    55,
    55,
    40,
    35,
    25,

    250,
    2,
    750
),
(
    'ranking_shop_default_v1',
    'SHOP_DEFAULT_V1',
    'SHOP',
    1,

    1200,
    700,
    850,
    800,
    700,
    520,
    450,

    650,
    800,
    700,

    420,

    70,
    60,
    50,
    50,
    40,
    35,
    25,

    250,
    2,
    900
);


CREATE TABLE food_search_terms (
    generation BIGINT NOT NULL,
    document_id TEXT NOT NULL,

    term_kind TEXT NOT NULL
        CHECK (
            term_kind IN (
                'NAME',
                'ALIAS',
                'SYNONYM',
                'CATEGORY',
                'TAG'
            )
        ),

    term_scope TEXT NOT NULL
        CHECK (
            term_scope IN (
                'PHRASE',
                'TOKEN'
            )
        ),

    display_term TEXT NOT NULL,
    normalized_term TEXT NOT NULL,

    first_char TEXT
        GENERATED ALWAYS AS (
            left(normalized_term, 1)
        ) STORED,

    char_length_value INTEGER
        GENERATED ALWAYS AS (
            char_length(normalized_term)
        ) STORED,

    PRIMARY KEY (
        generation,
        document_id,
        term_kind,
        term_scope,
        normalized_term
    ),

    FOREIGN KEY (
        generation,
        document_id
    )
    REFERENCES food_search_documents (
        generation,
        document_id
    )
    ON DELETE CASCADE,

    CHECK (
        btrim(display_term) <> ''
        AND btrim(normalized_term) <> ''
    )
);

CREATE INDEX food_search_terms_exact_idx
    ON food_search_terms (
        generation,
        normalized_term
    );

CREATE INDEX food_search_terms_prefix_idx
    ON food_search_terms (
        generation,
        normalized_term text_pattern_ops
    );

CREATE INDEX food_search_terms_typo_candidate_idx
    ON food_search_terms (
        generation,
        first_char,
        char_length_value
    )
    WHERE term_scope='TOKEN';

CREATE INDEX food_search_terms_document_idx
    ON food_search_terms (
        generation,
        document_id
    );


CREATE TABLE shop_search_terms (
    generation BIGINT NOT NULL,
    document_id TEXT NOT NULL,

    term_kind TEXT NOT NULL
        CHECK (
            term_kind IN (
                'NAME',
                'ALIAS',
                'SYNONYM',
                'CATEGORY',
                'TAG',
                'BRAND',
                'IDENTIFIER',
                'MEASUREMENT'
            )
        ),

    term_scope TEXT NOT NULL
        CHECK (
            term_scope IN (
                'PHRASE',
                'TOKEN'
            )
        ),

    display_term TEXT NOT NULL,
    normalized_term TEXT NOT NULL,

    first_char TEXT
        GENERATED ALWAYS AS (
            left(normalized_term, 1)
        ) STORED,

    char_length_value INTEGER
        GENERATED ALWAYS AS (
            char_length(normalized_term)
        ) STORED,

    PRIMARY KEY (
        generation,
        document_id,
        term_kind,
        term_scope,
        normalized_term
    ),

    FOREIGN KEY (
        generation,
        document_id
    )
    REFERENCES shop_search_documents (
        generation,
        document_id
    )
    ON DELETE CASCADE,

    CHECK (
        btrim(display_term) <> ''
        AND btrim(normalized_term) <> ''
    )
);

CREATE INDEX shop_search_terms_exact_idx
    ON shop_search_terms (
        generation,
        normalized_term
    );

CREATE INDEX shop_search_terms_prefix_idx
    ON shop_search_terms (
        generation,
        normalized_term text_pattern_ops
    );

CREATE INDEX shop_search_terms_typo_candidate_idx
    ON shop_search_terms (
        generation,
        first_char,
        char_length_value
    )
    WHERE term_scope='TOKEN';

CREATE INDEX shop_search_terms_document_idx
    ON shop_search_terms (
        generation,
        document_id
    );


CREATE OR REPLACE FUNCTION refresh_food_search_terms()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM food_search_terms
     WHERE generation = NEW.generation
       AND document_id = NEW.document_id;

    WITH phrases (
        term_kind,
        display_term,
        normalized_term
    ) AS (
        SELECT
            'NAME',
            NEW.name,
            NEW.normalized_name

        UNION ALL

        SELECT
            'ALIAS',
            value,
            value
        FROM unnest(NEW.aliases) AS value

        UNION ALL

        SELECT
            'SYNONYM',
            value,
            value
        FROM unnest(NEW.synonyms) AS value

        UNION ALL

        SELECT
            'CATEGORY',
            value,
            value
        FROM unnest(
            NEW.provider_types
            || NEW.cuisine_terms
            || NEW.meal_periods
            || NEW.dish_types
            || NEW.food_groups
            || NEW.occasions
        ) AS value

        UNION ALL

        SELECT
            'TAG',
            value,
            value
        FROM unnest(
            NEW.dietary_claims
            || NEW.preference_claims
        ) AS value
    ),
    clean_phrases AS (
        SELECT DISTINCT
            term_kind,
            btrim(display_term) AS display_term,
            lower(btrim(normalized_term)) AS normalized_term
        FROM phrases
        WHERE btrim(normalized_term) <> ''
    )
    INSERT INTO food_search_terms (
        generation,
        document_id,
        term_kind,
        term_scope,
        display_term,
        normalized_term
    )
    SELECT
        NEW.generation,
        NEW.document_id,
        term_kind,
        'PHRASE',
        display_term,
        normalized_term
    FROM clean_phrases

    UNION ALL

    SELECT DISTINCT
        NEW.generation,
        NEW.document_id,
        term_kind,
        'TOKEN',
        token,
        token
    FROM clean_phrases
    CROSS JOIN LATERAL
        unnest(
            string_to_array(
                normalized_term,
                ' '
            )
        ) AS token
    WHERE btrim(token) <> ''

    ON CONFLICT DO NOTHING;

    RETURN NEW;
END;
$$;

CREATE TRIGGER food_search_terms_refresh_trg
AFTER INSERT OR UPDATE
ON food_search_documents
FOR EACH ROW
EXECUTE FUNCTION refresh_food_search_terms();


CREATE OR REPLACE FUNCTION refresh_shop_search_terms()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM shop_search_terms
     WHERE generation = NEW.generation
       AND document_id = NEW.document_id;

    WITH json_category_phrases AS (
        SELECT
            value #>> '{}' AS value
        FROM jsonb_each(NEW.canonical_taxonomy)
        WHERE jsonb_typeof(value) = 'string'

        UNION ALL

        SELECT
            array_value
        FROM jsonb_each(NEW.canonical_taxonomy) entry
        CROSS JOIN LATERAL
            jsonb_array_elements_text(
                CASE
                    WHEN jsonb_typeof(entry.value) = 'array'
                        THEN entry.value
                    ELSE '[]'::jsonb
                END
            ) AS array_value
    ),
    phrases (
        term_kind,
        display_term,
        normalized_term
    ) AS (
        SELECT
            'NAME',
            NEW.name,
            NEW.normalized_name

        UNION ALL

        SELECT
            'ALIAS',
            value,
            value
        FROM unnest(NEW.aliases) AS value

        UNION ALL

        SELECT
            'SYNONYM',
            value,
            value
        FROM unnest(NEW.synonyms) AS value

        UNION ALL

        SELECT
            'CATEGORY',
            value,
            lower(value)
        FROM json_category_phrases

        UNION ALL

        SELECT
            'BRAND',
            NEW.brand,
            lower(NEW.brand)
        WHERE NEW.brand IS NOT NULL
          AND btrim(NEW.brand) <> ''

        UNION ALL

        SELECT
            'IDENTIFIER',
            value,
            lower(value)
        FROM unnest(
            NEW.sku_terms
            || NEW.barcode_terms
        ) AS value

        UNION ALL

        SELECT
            'MEASUREMENT',
            value,
            value
        FROM unnest(
            NEW.sell_units
            || NEW.measure_names
            || NEW.local_measure_aliases
            || NEW.packaging_terms
        ) AS value

        UNION ALL

        SELECT
            'TAG',
            value,
            value
        FROM unnest(
            NEW.fresh_attributes
        ) AS value

        UNION ALL

        SELECT
            'TAG',
            NEW.temperature_zone,
            lower(NEW.temperature_zone)
        WHERE NEW.temperature_zone IS NOT NULL
          AND btrim(NEW.temperature_zone) <> ''
    ),
    clean_phrases AS (
        SELECT DISTINCT
            term_kind,
            btrim(display_term) AS display_term,
            lower(btrim(normalized_term)) AS normalized_term
        FROM phrases
        WHERE normalized_term IS NOT NULL
          AND btrim(normalized_term) <> ''
    )
    INSERT INTO shop_search_terms (
        generation,
        document_id,
        term_kind,
        term_scope,
        display_term,
        normalized_term
    )
    SELECT
        NEW.generation,
        NEW.document_id,
        term_kind,
        'PHRASE',
        display_term,
        normalized_term
    FROM clean_phrases

    UNION ALL

    SELECT DISTINCT
        NEW.generation,
        NEW.document_id,
        term_kind,
        'TOKEN',
        token,
        token
    FROM clean_phrases
    CROSS JOIN LATERAL
        unnest(
            string_to_array(
                normalized_term,
                ' '
            )
        ) AS token
    WHERE btrim(token) <> ''

    ON CONFLICT DO NOTHING;

    RETURN NEW;
END;
$$;

CREATE TRIGGER shop_search_terms_refresh_trg
AFTER INSERT OR UPDATE
ON shop_search_documents
FOR EACH ROW
EXECUTE FUNCTION refresh_shop_search_terms();


-- Backfill sidecar terms safely when upgrading a database that already
-- contains Section 13D projection documents.

UPDATE food_search_documents
SET updated_at = updated_at;

UPDATE shop_search_documents
SET updated_at = updated_at;


COMMENT ON TABLE search_ranking_profiles IS
    'Versioned bounded ranking configuration. Ranking weights are data, not scattered application constants.';

COMMENT ON TABLE food_search_terms IS
    'Derived FOOD search-term sidecar used for indexed exact, prefix, typo candidate and suggestion lookup.';

COMMENT ON TABLE shop_search_terms IS
    'Derived SHOP search-term sidecar used for indexed exact, prefix, typo candidate and suggestion lookup.';