package searchprojection

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type foodSearchDocumentJSON struct {
	DocumentID string `json:"document_id"`

	Generation int64 `json:"generation"`

	BusinessRef string `json:"business_ref"`
	BranchRef   string `json:"branch_ref"`
	FoodItemRef string `json:"food_item_ref"`

	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`

	Aliases  []string `json:"aliases"`
	Synonyms []string `json:"synonyms"`

	ProviderTypes []string `json:"provider_types"`
	CuisineTerms  []string `json:"cuisine_terms"`
	MealPeriods   []string `json:"meal_periods"`
	DishTypes     []string `json:"dish_types"`
	FoodGroups    []string `json:"food_groups"`
	Occasions     []string `json:"occasions"`

	DietaryClaims    []string `json:"dietary_claims"`
	PreferenceClaims []string `json:"preference_claims"`

	Active bool `json:"active"`

	AvailabilitySummary   json.RawMessage `json:"availability_summary"`
	EffectivePriceSummary json.RawMessage `json:"effective_price_summary"`
	ServiceabilityHint    json.RawMessage `json:"serviceability_hint"`
	TrustHint             json.RawMessage `json:"trust_hint"`
	ReliabilityHint       json.RawMessage `json:"reliability_hint"`

	PromotionIndicator bool `json:"promotion_indicator"`

	CatalogueRevision  int64 `json:"catalogue_revision"`
	ProjectionRevision int64 `json:"projection_revision"`
}

type shopSearchDocumentJSON struct {
	DocumentID string `json:"document_id"`

	Generation int64 `json:"generation"`

	BusinessRef string `json:"business_ref"`
	BranchRef   string `json:"branch_ref"`

	ProductRef string  `json:"product_ref"`
	VariantRef *string `json:"variant_ref"`
	ListingRef *string `json:"listing_ref"`

	Name           string  `json:"name"`
	Brand          *string `json:"brand"`
	NormalizedName string  `json:"normalized_name"`

	Aliases  []string `json:"aliases"`
	Synonyms []string `json:"synonyms"`

	CanonicalTaxonomy json.RawMessage `json:"canonical_taxonomy"`

	SellUnits           []string `json:"sell_units"`
	MeasureNames        []string `json:"measure_names"`
	LocalMeasureAliases []string `json:"local_measure_aliases"`
	PackagingTerms      []string `json:"packaging_terms"`

	SKUTerms     []string `json:"sku_terms"`
	BarcodeTerms []string `json:"barcode_terms"`

	RetailEligible    bool `json:"retail_eligible"`
	WholesaleEligible bool `json:"wholesale_eligible"`
	Active            bool `json:"active"`

	EffectivePriceSummary        json.RawMessage `json:"effective_price_summary"`
	InventoryAvailabilitySummary json.RawMessage `json:"inventory_availability_summary"`

	PreorderHint  bool `json:"preorder_hint"`
	BackorderHint bool `json:"backorder_hint"`

	FreshAttributes []string `json:"fresh_attributes"`

	TemperatureZone *string `json:"temperature_zone"`

	HandlingMetadata json.RawMessage `json:"handling_metadata"`

	PromotionIndicator bool `json:"promotion_indicator"`

	CatalogueRevision  int64 `json:"catalogue_revision"`
	InventoryRevision  int64 `json:"inventory_revision"`
	ProjectionRevision int64 `json:"projection_revision"`
}

func loadRankingProfile(
	ctx context.Context,
	pool *pgxpool.Pool,
	profileCode string,
	namespaceCode string,
) (
	RankingProfile,
	error,
) {
	if pool == nil ||
		profileCode == "" ||
		namespaceCode == "" {
		return RankingProfile{},
			ErrInvalidArgument
	}

	var profile RankingProfile

	err := pool.QueryRow(
		ctx,
		`
        SELECT
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
          FROM search_ranking_profiles
         WHERE profile_code=$1
           AND namespace_code=$2
           AND active=TRUE
        `,
		profileCode,
		namespaceCode,
	).Scan(
		&profile.ID,
		&profile.ProfileCode,
		&profile.NamespaceCode,
		&profile.Version,

		&profile.ExactNameWeight,
		&profile.PrefixWeight,
		&profile.FullTextWeight,
		&profile.AliasWeight,
		&profile.SynonymWeight,
		&profile.CategoryWeight,
		&profile.TagWeight,
		&profile.BrandWeight,
		&profile.IdentifierWeight,
		&profile.MeasurementWeight,
		&profile.TypoWeight,

		&profile.AvailabilityWeight,
		&profile.ServiceabilityWeight,
		&profile.TrustWeight,
		&profile.ReliabilityWeight,
		&profile.DistanceWeight,
		&profile.CommercialWeight,
		&profile.PromotionWeight,

		&profile.MaxNonTextBoost,
		&profile.MaxTypoDistance,
		&profile.MaxCandidates,
	)
	if err != nil {
		return RankingProfile{},
			err
	}

	return profile, nil
}

func (
	backend *PostgresFoodSearchBackend,
) LoadRankingProfile(
	ctx context.Context,
	profileCode string,
	namespaceCode string,
) (
	RankingProfile,
	error,
) {
	if backend == nil ||
		backend.pool == nil {
		return RankingProfile{},
			ErrInvalidArgument
	}

	return loadRankingProfile(
		ctx,
		backend.pool,
		profileCode,
		namespaceCode,
	)
}

func (
	backend *PostgresShopSearchBackend,
) LoadRankingProfile(
	ctx context.Context,
	profileCode string,
	namespaceCode string,
) (
	RankingProfile,
	error,
) {
	if backend == nil ||
		backend.pool == nil {
		return RankingProfile{},
			ErrInvalidArgument
	}

	return loadRankingProfile(
		ctx,
		backend.pool,
		profileCode,
		namespaceCode,
	)
}
func (
	backend *PostgresFoodSearchBackend,
) FindFoodCandidates(
	ctx context.Context,
	normalizedQuery string,
	queryTokens []string,
	generation int64,
	branchRef string,
	limit int,
) (
	[]FoodEngineDocument,
	error,
) {
	if backend == nil ||
		backend.pool == nil ||
		generation <= 0 ||
		normalizedQuery == "" ||
		len(queryTokens) == 0 ||
		limit < 1 ||
		limit > 2000 {
		return nil,
			ErrInvalidArgument
	}

	rows, err :=
		backend.pool.Query(
			ctx,
			`
            WITH query_tokens AS (
                SELECT DISTINCT
                    token
                FROM unnest($2::text[]) AS token
                WHERE token <> ''
            ),
            candidate_ids AS (
                SELECT
                    document_id
                FROM food_search_documents
                WHERE generation=$3
                  AND active=TRUE
                  AND (
                      $4=''
                      OR branch_ref=$4
                  )
                  AND search_vector @@
                      plainto_tsquery(
                          'simple'::regconfig,
                          $1
                      )

                UNION

                SELECT DISTINCT
                    terms.document_id
                FROM food_search_terms terms
                CROSS JOIN query_tokens query_token
                JOIN food_search_documents document
                  ON document.generation=terms.generation
                 AND document.document_id=terms.document_id
                WHERE terms.generation=$3
                  AND terms.term_scope='TOKEN'
                  AND document.active=TRUE
                  AND (
                      $4=''
                      OR document.branch_ref=$4
                  )
                  AND (
                      terms.normalized_term =
                          query_token.token

                      OR terms.normalized_term
                          LIKE query_token.token || '%'

                      OR (
                          terms.first_char =
                              left(query_token.token, 1)

                          AND terms.char_length_value
                              BETWEEN
                                  greatest(
                                      1,
                                      char_length(query_token.token) - 2
                                  )
                                  AND
                                  char_length(query_token.token) + 2
                      )
                  )
            )
            SELECT to_jsonb(document)
            FROM food_search_documents document
            JOIN candidate_ids candidate
              ON candidate.document_id=document.document_id
            WHERE document.generation=$3
              AND document.active=TRUE
              AND (
                  $4=''
                  OR document.branch_ref=$4
              )
            ORDER BY document.document_id
            LIMIT $5
            `,
			normalizedQuery,
			queryTokens,
			generation,
			branchRef,
			limit,
		)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result :=
		make(
			[]FoodEngineDocument,
			0,
		)

	for rows.Next() {
		var raw []byte

		if err = rows.Scan(
			&raw,
		); err != nil {
			return nil, err
		}

		var decoded foodSearchDocumentJSON

		if err = json.Unmarshal(
			raw,
			&decoded,
		); err != nil {
			return nil, err
		}

		result = append(
			result,
			FoodEngineDocument{
				DocumentID: decoded.DocumentID,

				Generation: decoded.Generation,

				BusinessRef: decoded.BusinessRef,

				BranchRef: decoded.BranchRef,

				FoodItemRef: decoded.FoodItemRef,

				Name: decoded.Name,

				NormalizedName: decoded.NormalizedName,

				Aliases: decoded.Aliases,

				Synonyms: decoded.Synonyms,

				ProviderTypes: decoded.ProviderTypes,

				CuisineTerms: decoded.CuisineTerms,

				MealPeriods: decoded.MealPeriods,

				DishTypes: decoded.DishTypes,

				FoodGroups: decoded.FoodGroups,

				Occasions: decoded.Occasions,

				DietaryClaims: decoded.DietaryClaims,

				PreferenceClaims: decoded.PreferenceClaims,

				Active: decoded.Active,

				AvailabilitySummary: decoded.AvailabilitySummary,

				EffectivePriceSummary: decoded.EffectivePriceSummary,

				ServiceabilityHint: decoded.ServiceabilityHint,

				TrustHint: decoded.TrustHint,

				ReliabilityHint: decoded.ReliabilityHint,

				PromotionIndicator: decoded.PromotionIndicator,

				CatalogueRevision: decoded.CatalogueRevision,

				ProjectionRevision: decoded.ProjectionRevision,
			},
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (
	backend *PostgresShopSearchBackend,
) FindShopCandidates(
	ctx context.Context,
	normalizedQuery string,
	queryTokens []string,
	generation int64,
	branchRef string,
	limit int,
) (
	[]ShopEngineDocument,
	error,
) {
	if backend == nil ||
		backend.pool == nil ||
		generation <= 0 ||
		normalizedQuery == "" ||
		len(queryTokens) == 0 ||
		limit < 1 ||
		limit > 2000 {
		return nil,
			ErrInvalidArgument
	}

	rows, err :=
		backend.pool.Query(
			ctx,
			`
            WITH query_tokens AS (
                SELECT DISTINCT
                    token
                FROM unnest($2::text[]) AS token
                WHERE token <> ''
            ),
            candidate_ids AS (
                SELECT
                    document_id
                FROM shop_search_documents
                WHERE generation=$3
                  AND active=TRUE
                  AND (
                      $4=''
                      OR branch_ref=$4
                  )
                  AND search_vector @@
                      plainto_tsquery(
                          'simple'::regconfig,
                          $1
                      )

                UNION

                SELECT DISTINCT
                    terms.document_id
                FROM shop_search_terms terms
                CROSS JOIN query_tokens query_token
                JOIN shop_search_documents document
                  ON document.generation=terms.generation
                 AND document.document_id=terms.document_id
                WHERE terms.generation=$3
                  AND terms.term_scope='TOKEN'
                  AND document.active=TRUE
                  AND (
                      $4=''
                      OR document.branch_ref=$4
                  )
                  AND (
                      terms.normalized_term =
                          query_token.token

                      OR terms.normalized_term
                          LIKE query_token.token || '%'

                      OR (
                          terms.first_char =
                              left(query_token.token, 1)

                          AND terms.char_length_value
                              BETWEEN
                                  greatest(
                                      1,
                                      char_length(query_token.token) - 2
                                  )
                                  AND
                                  char_length(query_token.token) + 2
                      )
                  )
            )
            SELECT to_jsonb(document)
            FROM shop_search_documents document
            JOIN candidate_ids candidate
              ON candidate.document_id=document.document_id
            WHERE document.generation=$3
              AND document.active=TRUE
              AND (
                  $4=''
                  OR document.branch_ref=$4
              )
            ORDER BY document.document_id
            LIMIT $5
            `,
			normalizedQuery,
			queryTokens,
			generation,
			branchRef,
			limit,
		)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result :=
		make(
			[]ShopEngineDocument,
			0,
		)

	for rows.Next() {
		var raw []byte

		if err = rows.Scan(
			&raw,
		); err != nil {
			return nil, err
		}

		var decoded shopSearchDocumentJSON

		if err = json.Unmarshal(
			raw,
			&decoded,
		); err != nil {
			return nil, err
		}

		variantRef := ""
		listingRef := ""
		brand := ""
		temperatureZone := ""

		if decoded.VariantRef != nil {
			variantRef =
				*decoded.VariantRef
		}

		if decoded.ListingRef != nil {
			listingRef =
				*decoded.ListingRef
		}

		if decoded.Brand != nil {
			brand =
				*decoded.Brand
		}

		if decoded.TemperatureZone != nil {
			temperatureZone =
				*decoded.TemperatureZone
		}

		result = append(
			result,
			ShopEngineDocument{
				DocumentID: decoded.DocumentID,

				Generation: decoded.Generation,

				BusinessRef: decoded.BusinessRef,

				BranchRef: decoded.BranchRef,

				ProductRef: decoded.ProductRef,

				VariantRef: variantRef,

				ListingRef: listingRef,

				Name: decoded.Name,

				Brand: brand,

				NormalizedName: decoded.NormalizedName,

				Aliases: decoded.Aliases,

				Synonyms: decoded.Synonyms,

				CanonicalTaxonomy: decoded.CanonicalTaxonomy,

				SellUnits: decoded.SellUnits,

				MeasureNames: decoded.MeasureNames,

				LocalMeasureAliases: decoded.LocalMeasureAliases,

				PackagingTerms: decoded.PackagingTerms,

				SKUTerms: decoded.SKUTerms,

				BarcodeTerms: decoded.BarcodeTerms,

				RetailEligible: decoded.RetailEligible,

				WholesaleEligible: decoded.WholesaleEligible,

				Active: decoded.Active,

				EffectivePriceSummary: decoded.EffectivePriceSummary,

				InventoryAvailabilitySummary: decoded.InventoryAvailabilitySummary,

				PreorderHint: decoded.PreorderHint,

				BackorderHint: decoded.BackorderHint,

				FreshAttributes: decoded.FreshAttributes,

				TemperatureZone: temperatureZone,

				HandlingMetadata: decoded.HandlingMetadata,

				PromotionIndicator: decoded.PromotionIndicator,

				CatalogueRevision: decoded.CatalogueRevision,

				InventoryRevision: decoded.InventoryRevision,

				ProjectionRevision: decoded.ProjectionRevision,
			},
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (
	backend *PostgresFoodSearchBackend,
) SuggestFood(
	ctx context.Context,
	normalizedPrefix string,
	generation int64,
	limit int,
) (
	[]SearchSuggestion,
	error,
) {
	if backend == nil ||
		backend.pool == nil ||
		generation <= 0 ||
		normalizedPrefix == "" ||
		limit < 1 ||
		limit > maxSuggestionLimit {
		return nil,
			ErrInvalidArgument
	}

	rows, err :=
		backend.pool.Query(
			ctx,
			`
            SELECT
                min(display_term),
                normalized_term,
                term_kind
            FROM food_search_terms
            WHERE generation=$1
              AND term_scope='PHRASE'
              AND normalized_term LIKE $2 || '%'
            GROUP BY
                normalized_term,
                term_kind
            ORDER BY
                count(*) DESC,
                normalized_term ASC,
                term_kind ASC
            LIMIT $3
            `,
			generation,
			normalizedPrefix,
			limit,
		)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result :=
		make(
			[]SearchSuggestion,
			0,
			limit,
		)

	for rows.Next() {
		var suggestion SearchSuggestion

		if err = rows.Scan(
			&suggestion.Text,
			&suggestion.NormalizedText,
			&suggestion.SourceType,
		); err != nil {
			return nil, err
		}

		result = append(
			result,
			suggestion,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (
	backend *PostgresShopSearchBackend,
) SuggestShop(
	ctx context.Context,
	normalizedPrefix string,
	generation int64,
	limit int,
) (
	[]SearchSuggestion,
	error,
) {
	if backend == nil ||
		backend.pool == nil ||
		generation <= 0 ||
		normalizedPrefix == "" ||
		limit < 1 ||
		limit > maxSuggestionLimit {
		return nil,
			ErrInvalidArgument
	}

	rows, err :=
		backend.pool.Query(
			ctx,
			`
            SELECT
                min(display_term),
                normalized_term,
                term_kind
            FROM shop_search_terms
            WHERE generation=$1
              AND term_scope='PHRASE'
              AND normalized_term LIKE $2 || '%'
            GROUP BY
                normalized_term,
                term_kind
            ORDER BY
                count(*) DESC,
                normalized_term ASC,
                term_kind ASC
            LIMIT $3
            `,
			generation,
			normalizedPrefix,
			limit,
		)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result :=
		make(
			[]SearchSuggestion,
			0,
			limit,
		)

	for rows.Next() {
		var suggestion SearchSuggestion

		if err = rows.Scan(
			&suggestion.Text,
			&suggestion.NormalizedText,
			&suggestion.SourceType,
		); err != nil {
			return nil, err
		}

		result = append(
			result,
			suggestion,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

var (
	_ FoodSearchEngineBackend = (*PostgresFoodSearchBackend)(nil)

	_ ShopSearchEngineBackend = (*PostgresShopSearchBackend)(nil)
)
