package searchprojection

import (
	"context"
	"sort"
	"strings"
)

type FoodSearchEngine struct {
	backend FoodSearchEngineBackend

	distanceResolver FoodDistanceResolver
}

func NewFoodSearchEngine(
	backend FoodSearchEngineBackend,
) *FoodSearchEngine {
	return &FoodSearchEngine{
		backend: backend,
	}
}

func (
	engine *FoodSearchEngine,
) SetDistanceResolver(
	resolver FoodDistanceResolver,
) {
	engine.distanceResolver =
		resolver
}

type ShopSearchEngine struct {
	backend ShopSearchEngineBackend

	distanceResolver ShopDistanceResolver
}

func NewShopSearchEngine(
	backend ShopSearchEngineBackend,
) *ShopSearchEngine {
	return &ShopSearchEngine{
		backend: backend,
	}
}

func (
	engine *ShopSearchEngine,
) SetDistanceResolver(
	resolver ShopDistanceResolver,
) {
	engine.distanceResolver =
		resolver
}

type rankedFoodDocument struct {
	document FoodEngineDocument

	score int64

	priceMinor *int64

	distanceMeters *int64
}

type rankedShopDocument struct {
	document ShopEngineDocument

	score int64

	priceMinor *int64

	distanceMeters *int64
}

func (
	engine *FoodSearchEngine,
) Search(
	ctx context.Context,
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.backend == nil ||
		request.Generation <= 0 {
		return FoodEngineSearchResponse{},
			ErrInvalidArgument
	}

	normalizedQuery,
		queryTokens,
		err :=
		normalizeEngineQuery(
			request.Query,
		)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	request.BranchRef =
		strings.TrimSpace(
			request.BranchRef,
		)

	if request.ProfileCode == "" {
		request.ProfileCode =
			DefaultFoodRankingProfile
	}

	if request.Sort == "" {
		request.Sort =
			SortRelevance
	}

	if err =
		validateSearchSort(
			request.Sort,
		); err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	if request.PageSize == 0 {
		request.PageSize = 20
	}

	if request.PageSize < 1 ||
		request.PageSize >
			maxEnginePageSize {
		return FoodEngineSearchResponse{},
			ErrInvalidArgument
	}

	if len(request.Facets) >
		maxEngineFacets {
		return FoodEngineSearchResponse{},
			ErrInvalidArgument
	}

	request.Filters, err =
		normalizeFoodFilters(
			request.Filters,
		)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	profile, err :=
		engine.backend.
			LoadRankingProfile(
				ctx,
				request.ProfileCode,
				"FOOD",
			)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	if err =
		validateRankingProfile(
			profile,
			"FOOD",
		); err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	cursor, err :=
		decodeSearchCursor(
			request.Cursor,
			request.Sort,
		)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	if request.Sort ==
		SortDistance &&
		(request.DistanceContext == nil ||
			engine.distanceResolver == nil) {
		return FoodEngineSearchResponse{},
			ErrDistanceContextUnavailable
	}

	documents, err :=
		engine.backend.
			FindFoodCandidates(
				ctx,
				normalizedQuery,
				queryTokens,
				request.Generation,
				request.BranchRef,
				profile.MaxCandidates,
			)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	ranked :=
		make(
			[]rankedFoodDocument,
			0,
			len(documents),
		)

	for _, document := range documents {
		if !document.Active {
			continue
		}

		priceMinor :=
			foodPriceMinor(
				document,
			)

		if !foodFiltersMatch(
			document,
			request.Filters,
			priceMinor,
		) {
			continue
		}

		terms :=
			foodWeightedTerms(
				document,
			)

		textScore,
			matched :=
			scoreTextMatch(
				normalizedQuery,
				queryTokens,
				document.NormalizedName,
				terms,
				profile,
			)

		if !matched {
			continue
		}

		available, _ :=
			jsonBool(
				document.AvailabilitySummary,
				"available",
				"in_stock",
			)

		serviceable, serviceableExists :=
			jsonBool(
				document.ServiceabilityHint,
				"serviceable",
				"eligible",
			)

		if !serviceableExists {
			serviceable = false
		}

		trustScore :=
			jsonBasisPoints(
				document.TrustHint,
				"score_bps",
				"trust_bps",
			)

		if trustScore == 0 {
			trusted, exists :=
				jsonBool(
					document.TrustHint,
					"trusted",
					"eligible",
				)

			if exists &&
				trusted {
				trustScore = 10000
			}
		}

		reliabilityScore :=
			jsonBasisPoints(
				document.ReliabilityHint,
				"score_bps",
				"reliability_bps",
			)

		if reliabilityScore == 0 {
			healthy, exists :=
				jsonBool(
					document.ReliabilityHint,
					"healthy",
					"reliable",
				)

			if exists &&
				healthy {
				reliabilityScore = 10000
			}
		}

		commercialScore :=
			jsonBasisPoints(
				document.ReliabilityHint,
				"commercial_score_bps",
				"conversion_score_bps",
			)

		score :=
			applyNonTextBoost(
				textScore,
				profile,
				available,
				serviceable,
				trustScore,
				reliabilityScore,
				commercialScore,
				document.PromotionIndicator,
			)

		var distance *int64

		if request.DistanceContext != nil &&
			engine.distanceResolver != nil {
			value,
				ok,
				resolveErr :=
				engine.distanceResolver.
					ResolveFoodDistanceMeters(
						ctx,
						*request.DistanceContext,
						document,
					)

			if resolveErr != nil {
				return FoodEngineSearchResponse{},
					resolveErr
			}

			if ok {
				distance = &value
			}
		}

		if request.Sort ==
			SortDistance &&
			distance == nil {
			continue
		}

		ranked = append(
			ranked,
			rankedFoodDocument{
				document: document,

				score: score,

				priceMinor: priceMinor,

				distanceMeters: distance,
			},
		)
	}

	facets, err :=
		buildFoodFacets(
			ranked,
			request.Facets,
		)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	sortFoodResults(
		ranked,
		request.Sort,
	)

	ranked =
		afterFoodCursor(
			ranked,
			cursor,
			request.Sort,
		)

	hasMore :=
		len(ranked) >
			request.PageSize

	if hasMore {
		ranked =
			ranked[:request.PageSize]
	}

	hits :=
		make(
			[]FoodEngineHit,
			0,
			len(ranked),
		)

	for _, item := range ranked {
		hits = append(
			hits,
			FoodEngineHit{
				DocumentID: item.document.DocumentID,

				BusinessRef: item.document.BusinessRef,

				BranchRef: item.document.BranchRef,

				FoodItemRef: item.document.FoodItemRef,

				Name: item.document.Name,

				Score: item.score,

				PriceMinor: item.priceMinor,

				DistanceMeters: item.distanceMeters,

				Sponsored: false,

				PlacementType: PlacementOrganic,

				AvailabilityStatus: string(foodAvailabilityStatusFromDocument(item.document)),

				ServiceabilityStatus: string(foodServiceabilityStatusFromDocument(item.document)),

				SourceEvidence: buildFoodSearchSourceEvidence(item.document),

				ProjectionRevision: item.document.ProjectionRevision,
			},
		)
	}

	nextCursor := ""

	if hasMore &&
		len(ranked) > 0 {
		last :=
			ranked[len(ranked)-1]

		nextCursor, err =
			encodeSearchCursor(
				rankedSearchKey{
					Sort: request.Sort,

					Score: last.score,

					PriceMinor: last.priceMinor,

					DistanceMeters: last.distanceMeters,

					StableRef: last.document.DocumentID,
				},
			)

		if err != nil {
			return FoodEngineSearchResponse{},
				err
		}
	}

	return FoodEngineSearchResponse{
		ProfileCode: profile.ProfileCode,

		ProfileVersion: profile.Version,

		Hits: hits,

		Facets: facets,

		NextCursor: nextCursor,
	}, nil
}

func (
	engine *ShopSearchEngine,
) Search(
	ctx context.Context,
	request ShopEngineSearchRequest,
) (
	ShopEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.backend == nil ||
		request.Generation <= 0 {
		return ShopEngineSearchResponse{},
			ErrInvalidArgument
	}

	normalizedQuery,
		queryTokens,
		err :=
		normalizeEngineQuery(
			request.Query,
		)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	request.BranchRef =
		strings.TrimSpace(
			request.BranchRef,
		)

	if request.ProfileCode == "" {
		request.ProfileCode =
			DefaultShopRankingProfile
	}

	if request.Sort == "" {
		request.Sort =
			SortRelevance
	}

	if err =
		validateSearchSort(
			request.Sort,
		); err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	if request.PageSize == 0 {
		request.PageSize = 20
	}

	if request.PageSize < 1 ||
		request.PageSize >
			maxEnginePageSize {
		return ShopEngineSearchResponse{},
			ErrInvalidArgument
	}

	if len(request.Facets) >
		maxEngineFacets {
		return ShopEngineSearchResponse{},
			ErrInvalidArgument
	}

	request.Filters, err =
		normalizeShopFilters(
			request.Filters,
		)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	profile, err :=
		engine.backend.
			LoadRankingProfile(
				ctx,
				request.ProfileCode,
				"SHOP",
			)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	if err =
		validateRankingProfile(
			profile,
			"SHOP",
		); err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	cursor, err :=
		decodeSearchCursor(
			request.Cursor,
			request.Sort,
		)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	if request.Sort ==
		SortDistance &&
		(request.DistanceContext == nil ||
			engine.distanceResolver == nil) {
		return ShopEngineSearchResponse{},
			ErrDistanceContextUnavailable
	}

	documents, err :=
		engine.backend.
			FindShopCandidates(
				ctx,
				normalizedQuery,
				queryTokens,
				request.Generation,
				request.BranchRef,
				profile.MaxCandidates,
			)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	ranked :=
		make(
			[]rankedShopDocument,
			0,
			len(documents),
		)

	for _, document := range documents {
		if !document.Active {
			continue
		}

		priceMinor :=
			shopPriceMinor(
				document,
			)

		if !shopFiltersMatch(
			document,
			request.Filters,
			priceMinor,
		) {
			continue
		}

		terms :=
			shopWeightedTerms(
				document,
			)

		textScore,
			matched :=
			scoreTextMatch(
				normalizedQuery,
				queryTokens,
				document.NormalizedName,
				terms,
				profile,
			)

		if !matched {
			continue
		}

		available, _ :=
			jsonBool(
				document.InventoryAvailabilitySummary,
				"available",
				"in_stock",
			)

		serviceable, _ :=
			jsonBool(
				document.HandlingMetadata,
				"serviceable",
				"eligible",
			)
		trustScore :=
			jsonBasisPoints(
				document.HandlingMetadata,
				"trust_bps",
				"trust_score_bps",
			)

		reliabilityScore :=
			jsonBasisPoints(
				document.HandlingMetadata,
				"reliability_bps",
				"reliability_score_bps",
			)

		commercialScore :=
			jsonBasisPoints(
				document.HandlingMetadata,
				"commercial_score_bps",
				"conversion_score_bps",
			)

		score :=
			applyNonTextBoost(
				textScore,
				profile,
				available,
				serviceable,
				trustScore,
				reliabilityScore,
				commercialScore,
				document.PromotionIndicator,
			)

		var distance *int64

		if request.DistanceContext != nil &&
			engine.distanceResolver != nil {
			value,
				ok,
				resolveErr :=
				engine.distanceResolver.
					ResolveShopDistanceMeters(
						ctx,
						*request.DistanceContext,
						document,
					)

			if resolveErr != nil {
				return ShopEngineSearchResponse{},
					resolveErr
			}

			if ok {
				distance = &value
			}
		}

		if request.Sort ==
			SortDistance &&
			distance == nil {
			continue
		}

		ranked = append(
			ranked,
			rankedShopDocument{
				document: document,

				score: score,

				priceMinor: priceMinor,

				distanceMeters: distance,
			},
		)
	}

	facets, err :=
		buildShopFacets(
			ranked,
			request.Facets,
		)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	sortShopResults(
		ranked,
		request.Sort,
	)

	ranked =
		afterShopCursor(
			ranked,
			cursor,
			request.Sort,
		)

	hasMore :=
		len(ranked) >
			request.PageSize

	if hasMore {
		ranked =
			ranked[:request.PageSize]
	}

	hits :=
		make(
			[]ShopEngineHit,
			0,
			len(ranked),
		)

	for _, item := range ranked {
		hits = append(
			hits,
			ShopEngineHit{
				DocumentID: item.document.DocumentID,

				BusinessRef: item.document.BusinessRef,

				BranchRef: item.document.BranchRef,

				ProductRef: item.document.ProductRef,

				VariantRef: item.document.VariantRef,

				ListingRef: item.document.ListingRef,

				Name: item.document.Name,

				Brand: item.document.Brand,

				Score: item.score,

				PriceMinor: item.priceMinor,

				DistanceMeters: item.distanceMeters,

				Sponsored: false,

				PlacementType: PlacementOrganic,

				AvailabilityStatus: string(shopInventoryStatusFromDocument(item.document)),

				ServiceabilityStatus: string(shopServiceabilityStatusFromDocument(item.document)),

				SourceEvidence: buildShopSearchSourceEvidence(item.document),

				ProjectionRevision: item.document.ProjectionRevision,
			},
		)
	}

	nextCursor := ""

	if hasMore &&
		len(ranked) > 0 {
		last :=
			ranked[len(ranked)-1]

		nextCursor, err =
			encodeSearchCursor(
				rankedSearchKey{
					Sort: request.Sort,

					Score: last.score,

					PriceMinor: last.priceMinor,

					DistanceMeters: last.distanceMeters,

					StableRef: last.document.DocumentID,
				},
			)

		if err != nil {
			return ShopEngineSearchResponse{},
				err
		}
	}

	return ShopEngineSearchResponse{
		ProfileCode: profile.ProfileCode,

		ProfileVersion: profile.Version,

		Hits: hits,

		Facets: facets,

		NextCursor: nextCursor,
	}, nil
}

func (
	engine *FoodSearchEngine,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.backend == nil ||
		request.Generation <= 0 {
		return nil,
			ErrInvalidArgument
	}

	normalized, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return nil, err
	}

	if request.Limit == 0 {
		request.Limit = 10
	}

	if request.Limit < 1 ||
		request.Limit >
			maxSuggestionLimit {
		return nil,
			ErrInvalidArgument
	}

	return engine.backend.
		SuggestFood(
			ctx,
			normalized,
			request.Generation,
			request.Limit,
		)
}

func (
	engine *ShopSearchEngine,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.backend == nil ||
		request.Generation <= 0 {
		return nil,
			ErrInvalidArgument
	}

	normalized, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return nil, err
	}

	if request.Limit == 0 {
		request.Limit = 10
	}

	if request.Limit < 1 ||
		request.Limit >
			maxSuggestionLimit {
		return nil,
			ErrInvalidArgument
	}

	return engine.backend.
		SuggestShop(
			ctx,
			normalized,
			request.Generation,
			request.Limit,
		)
}

func validateSearchSort(
	sortMode SearchSort,
) error {
	switch sortMode {
	case SortRelevance,
		SortPriceLowToHigh,
		SortPriceHighToLow,
		SortDistance:
		return nil

	default:
		return ErrInvalidArgument
	}
}

func foodWeightedTerms(
	document FoodEngineDocument,
) []weightedSearchTerm {
	terms :=
		[]weightedSearchTerm{
			{
				value: document.Name,
				kind:  searchTermName,
			},
		}

	appendTerms :=
		func(
			values []string,
			kind searchTermKind,
		) {
			for _, value := range values {
				terms = append(
					terms,
					weightedSearchTerm{
						value: value,
						kind:  kind,
					},
				)
			}
		}

	appendTerms(
		document.Aliases,
		searchTermAlias,
	)

	appendTerms(
		document.Synonyms,
		searchTermSynonym,
	)

	appendTerms(
		document.ProviderTypes,
		searchTermCategory,
	)

	appendTerms(
		document.CuisineTerms,
		searchTermCategory,
	)

	appendTerms(
		document.MealPeriods,
		searchTermCategory,
	)

	appendTerms(
		document.DishTypes,
		searchTermCategory,
	)

	appendTerms(
		document.FoodGroups,
		searchTermCategory,
	)

	appendTerms(
		document.Occasions,
		searchTermCategory,
	)

	appendTerms(
		document.DietaryClaims,
		searchTermTag,
	)

	appendTerms(
		document.PreferenceClaims,
		searchTermTag,
	)

	return terms
}

func shopWeightedTerms(
	document ShopEngineDocument,
) []weightedSearchTerm {
	terms :=
		[]weightedSearchTerm{
			{
				value: document.Name,
				kind:  searchTermName,
			},
			{
				value: document.Brand,
				kind:  searchTermBrand,
			},
		}

	appendTerms :=
		func(
			values []string,
			kind searchTermKind,
		) {
			for _, value := range values {
				terms = append(
					terms,
					weightedSearchTerm{
						value: value,
						kind:  kind,
					},
				)
			}
		}

	appendTerms(
		document.Aliases,
		searchTermAlias,
	)

	appendTerms(
		document.Synonyms,
		searchTermSynonym,
	)

	appendTerms(
		flattenJSONStrings(
			document.CanonicalTaxonomy,
		),
		searchTermCategory,
	)

	appendTerms(
		document.SellUnits,
		searchTermMeasurement,
	)

	appendTerms(
		document.MeasureNames,
		searchTermMeasurement,
	)

	appendTerms(
		document.LocalMeasureAliases,
		searchTermMeasurement,
	)

	appendTerms(
		document.PackagingTerms,
		searchTermMeasurement,
	)

	appendTerms(
		document.SKUTerms,
		searchTermIdentifier,
	)

	appendTerms(
		document.BarcodeTerms,
		searchTermIdentifier,
	)

	appendTerms(
		document.FreshAttributes,
		searchTermTag,
	)

	if document.TemperatureZone != "" {
		appendTerms(
			[]string{
				document.TemperatureZone,
			},
			searchTermTag,
		)
	}

	return terms
}

func foodPriceMinor(
	document FoodEngineDocument,
) *int64 {
	value, ok :=
		jsonInt64(
			document.EffectivePriceSummary,
			"minor",
			"price_minor",
			"minimum_minor",
		)

	if !ok {
		return nil
	}

	return &value
}

func shopPriceMinor(
	document ShopEngineDocument,
) *int64 {
	value, ok :=
		jsonInt64(
			document.EffectivePriceSummary,
			"minor",
			"price_minor",
			"minimum_minor",
		)

	if !ok {
		return nil
	}

	return &value
}

func foodFiltersMatch(
	document FoodEngineDocument,
	filter FoodSearchFilters,
	price *int64,
) bool {
	if !stringSlicesIntersect(
		filter.ProviderTypes,
		document.ProviderTypes,
	) ||
		!stringSlicesIntersect(
			filter.Cuisines,
			document.CuisineTerms,
		) ||
		!stringSlicesIntersect(
			filter.MealPeriods,
			document.MealPeriods,
		) ||
		!stringSlicesIntersect(
			filter.DishTypes,
			document.DishTypes,
		) ||
		!stringSlicesIntersect(
			filter.FoodGroups,
			document.FoodGroups,
		) ||
		!stringSlicesIntersect(
			filter.Occasions,
			document.Occasions,
		) ||
		!stringSlicesIntersect(
			filter.DietaryClaims,
			document.DietaryClaims,
		) ||
		!stringSlicesIntersect(
			filter.PreferenceClaims,
			document.PreferenceClaims,
		) {
		return false
	}

	if !priceFilterMatches(
		filter.MinPriceMinor,
		filter.MaxPriceMinor,
		price,
	) {
		return false
	}

	available, availableExists :=
		jsonBool(
			document.AvailabilitySummary,
			"available",
			"in_stock",
		)

	if !boolFilterMatches(
		filter.Available,
		available,
		availableExists,
	) {
		return false
	}

	serviceable, serviceableExists :=
		jsonBool(
			document.ServiceabilityHint,
			"serviceable",
			"eligible",
		)

	if !boolFilterMatches(
		filter.Serviceable,
		serviceable,
		serviceableExists,
	) {
		return false
	}

	trusted, trustedExists :=
		jsonBool(
			document.TrustHint,
			"trusted",
			"eligible",
		)

	if !boolFilterMatches(
		filter.Trusted,
		trusted,
		trustedExists,
	) {
		return false
	}

	return true
}

func shopFiltersMatch(
	document ShopEngineDocument,
	filter ShopSearchFilters,
	price *int64,
) bool {
	if !stringSlicesIntersect(
		filter.CanonicalCategories,
		flattenJSONStrings(
			document.CanonicalTaxonomy,
		),
	) ||
		!stringSlicesIntersect(
			filter.Brands,
			[]string{
				document.Brand,
			},
		) ||
		!stringSlicesIntersect(
			filter.SellUnits,
			document.SellUnits,
		) ||
		!stringSlicesIntersect(
			filter.Measurements,
			append(
				append(
					append(
						[]string{},
						document.MeasureNames...,
					),
					document.LocalMeasureAliases...,
				),
				document.PackagingTerms...,
			),
		) ||
		!stringSlicesIntersect(
			filter.FreshAttributes,
			document.FreshAttributes,
		) ||
		!stringSlicesIntersect(
			filter.TemperatureZones,
			[]string{
				document.TemperatureZone,
			},
		) {
		return false
	}

	if filter.RetailEligible != nil &&
		document.RetailEligible !=
			*filter.RetailEligible {
		return false
	}

	if filter.WholesaleEligible != nil &&
		document.WholesaleEligible !=
			*filter.WholesaleEligible {
		return false
	}

	if !priceFilterMatches(
		filter.MinPriceMinor,
		filter.MaxPriceMinor,
		price,
	) {
		return false
	}

	available, availableExists :=
		jsonBool(
			document.InventoryAvailabilitySummary,
			"available",
			"in_stock",
		)

	if !boolFilterMatches(
		filter.Available,
		available,
		availableExists,
	) {
		return false
	}

	operatingModel, _ :=
		jsonString(
			document.HandlingMetadata,
			"operating_model",
		)

	if len(filter.OperatingModels) > 0 &&
		!stringSlicesIntersect(
			filter.OperatingModels,
			[]string{
				operatingModel,
			},
		) {
		return false
	}

	productClass, ok :=
		jsonString(
			document.CanonicalTaxonomy,
			"product_class",
		)

	if !ok {
		productClass, _ =
			jsonString(
				document.HandlingMetadata,
				"product_class",
			)
	}

	if len(filter.ProductClasses) > 0 &&
		!stringSlicesIntersect(
			filter.ProductClasses,
			[]string{
				productClass,
			},
		) {
		return false
	}

	return true
}

func buildFoodFacets(
	results []rankedFoodDocument,
	requested []string,
) (
	[]SearchFacet,
	error,
) {
	if len(requested) == 0 {
		return nil, nil
	}

	if len(requested) >
		maxEngineFacets {
		return nil,
			ErrInvalidArgument
	}

	facets :=
		make(
			[]SearchFacet,
			0,
			len(requested),
		)

	for _, rawName := range requested {
		name :=
			strings.ToUpper(
				strings.TrimSpace(
					rawName,
				),
			)

		counts :=
			make(
				map[string]int,
			)

		for _, result := range results {
			var values []string

			switch name {
			case "PROVIDER_TYPE":
				values =
					result.document.ProviderTypes

			case "CUISINE":
				values =
					result.document.CuisineTerms

			case "MEAL_PERIOD":
				values =
					result.document.MealPeriods

			case "DISH_TYPE":
				values =
					result.document.DishTypes

			case "FOOD_GROUP":
				values =
					result.document.FoodGroups

			case "OCCASION":
				values =
					result.document.Occasions

			case "DIETARY":
				values =
					result.document.DietaryClaims

			case "PREFERENCE":
				values =
					result.document.PreferenceClaims

			case "AVAILABILITY":
				available, exists :=
					jsonBool(
						result.document.AvailabilitySummary,
						"available",
						"in_stock",
					)

				if exists {
					if available {
						values =
							[]string{
								"IN_STOCK",
							}
					} else {
						values =
							[]string{
								"UNAVAILABLE",
							}
					}
				}

			default:
				return nil,
					ErrInvalidArgument
			}

			for _, value := range values {
				normalized, err :=
					NormalizeText(value)

				if err != nil ||
					normalized == "" {
					continue
				}

				counts[normalized]++
			}
		}

		facets = append(
			facets,
			SearchFacet{
				Name: name,

				Values: sortFacetValues(
					counts,
				),
			},
		)
	}

	return facets, nil
}

func buildShopFacets(
	results []rankedShopDocument,
	requested []string,
) (
	[]SearchFacet,
	error,
) {
	if len(requested) == 0 {
		return nil, nil
	}

	if len(requested) >
		maxEngineFacets {
		return nil,
			ErrInvalidArgument
	}

	facets :=
		make(
			[]SearchFacet,
			0,
			len(requested),
		)

	for _, rawName := range requested {
		name :=
			strings.ToUpper(
				strings.TrimSpace(
					rawName,
				),
			)

		counts :=
			make(
				map[string]int,
			)

		for _, result := range results {
			var values []string

			switch name {
			case "CATEGORY":
				values =
					flattenJSONStrings(
						result.document.CanonicalTaxonomy,
					)

			case "BRAND":
				values =
					[]string{
						result.document.Brand,
					}

			case "SELL_UNIT":
				values =
					result.document.SellUnits

			case "MEASUREMENT":
				values =
					append(
						append(
							append(
								[]string{},
								result.document.MeasureNames...,
							),
							result.document.LocalMeasureAliases...,
						),
						result.document.PackagingTerms...,
					)

			case "FRESH_ATTRIBUTE":
				values =
					result.document.FreshAttributes

			case "TEMPERATURE_ZONE":
				values =
					[]string{
						result.document.TemperatureZone,
					}

			case "AVAILABILITY":
				available, exists :=
					jsonBool(
						result.document.InventoryAvailabilitySummary,
						"available",
						"in_stock",
					)

				if exists &&
					available {
					values =
						[]string{
							"IN_STOCK",
						}
				} else if result.document.PreorderHint {
					values =
						[]string{
							"PREORDER",
						}
				} else if result.document.BackorderHint {
					values =
						[]string{
							"BACKORDER",
						}
				} else if exists {
					values =
						[]string{
							"UNAVAILABLE",
						}
				}

			default:
				return nil,
					ErrInvalidArgument
			}

			for _, value := range values {
				normalized, err :=
					NormalizeText(value)

				if err != nil ||
					normalized == "" {
					continue
				}

				counts[normalized]++
			}
		}

		facets = append(
			facets,
			SearchFacet{
				Name: name,

				Values: sortFacetValues(
					counts,
				),
			},
		)
	}

	return facets, nil
}

func sortFoodResults(
	results []rankedFoodDocument,
	sortMode SearchSort,
) {
	sort.SliceStable(
		results,
		func(
			i int,
			j int,
		) bool {
			return compareFoodResult(
				results[i],
				results[j],
				sortMode,
			) < 0
		},
	)
}

func sortShopResults(
	results []rankedShopDocument,
	sortMode SearchSort,
) {
	sort.SliceStable(
		results,
		func(
			i int,
			j int,
		) bool {
			return compareShopResult(
				results[i],
				results[j],
				sortMode,
			) < 0
		},
	)
}

func compareFoodResult(
	left rankedFoodDocument,
	right rankedFoodDocument,
	sortMode SearchSort,
) int {
	return compareRankedValues(
		left.score,
		right.score,
		left.priceMinor,
		right.priceMinor,
		left.distanceMeters,
		right.distanceMeters,
		left.document.DocumentID,
		right.document.DocumentID,
		sortMode,
	)
}

func compareShopResult(
	left rankedShopDocument,
	right rankedShopDocument,
	sortMode SearchSort,
) int {
	return compareRankedValues(
		left.score,
		right.score,
		left.priceMinor,
		right.priceMinor,
		left.distanceMeters,
		right.distanceMeters,
		left.document.DocumentID,
		right.document.DocumentID,
		sortMode,
	)
}

func compareRankedValues(
	leftScore int64,
	rightScore int64,
	leftPrice *int64,
	rightPrice *int64,
	leftDistance *int64,
	rightDistance *int64,
	leftRef string,
	rightRef string,
	sortMode SearchSort,
) int {
	switch sortMode {
	case SortPriceLowToHigh:
		if comparison :=
			compareNullableInt64(
				leftPrice,
				rightPrice,
				false,
			); comparison != 0 {
			return comparison
		}

	case SortPriceHighToLow:
		if comparison :=
			compareNullableInt64(
				leftPrice,
				rightPrice,
				true,
			); comparison != 0 {
			return comparison
		}

	case SortDistance:
		if comparison :=
			compareNullableInt64(
				leftDistance,
				rightDistance,
				false,
			); comparison != 0 {
			return comparison
		}
	}

	if leftScore != rightScore {
		if leftScore > rightScore {
			return -1
		}

		return 1
	}

	if leftRef < rightRef {
		return -1
	}

	if leftRef > rightRef {
		return 1
	}

	return 0
}

func compareNullableInt64(
	left *int64,
	right *int64,
	descending bool,
) int {
	if left == nil &&
		right == nil {
		return 0
	}

	if left == nil {
		return 1
	}

	if right == nil {
		return -1
	}

	if *left == *right {
		return 0
	}

	result := -1

	if *left > *right {
		result = 1
	}

	if descending {
		result =
			-result
	}

	return result
}

func afterFoodCursor(
	results []rankedFoodDocument,
	cursor rankedSearchKey,
	sortMode SearchSort,
) []rankedFoodDocument {
	if cursor.StableRef == "" {
		return results
	}

	for index, result := range results {
		comparison :=
			compareRankedValues(
				result.score,
				cursor.Score,
				result.priceMinor,
				cursor.PriceMinor,
				result.distanceMeters,
				cursor.DistanceMeters,
				result.document.DocumentID,
				cursor.StableRef,
				sortMode,
			)

		if comparison > 0 {
			return results[index:]
		}
	}

	return nil
}

func afterShopCursor(
	results []rankedShopDocument,
	cursor rankedSearchKey,
	sortMode SearchSort,
) []rankedShopDocument {
	if cursor.StableRef == "" {
		return results
	}

	for index, result := range results {
		comparison :=
			compareRankedValues(
				result.score,
				cursor.Score,
				result.priceMinor,
				cursor.PriceMinor,
				result.distanceMeters,
				cursor.DistanceMeters,
				result.document.DocumentID,
				cursor.StableRef,
				sortMode,
			)

		if comparison > 0 {
			return results[index:]
		}
	}

	return nil
}
