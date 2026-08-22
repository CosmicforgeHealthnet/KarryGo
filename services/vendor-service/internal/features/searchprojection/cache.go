package searchprojection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

var ErrSearchCacheConfigInvalid = errors.New(
	"invalid search cache configuration",
)

type SearchCacheNamespace string

const (
	SearchCacheNamespaceFood SearchCacheNamespace = "FOOD"

	SearchCacheNamespaceShop SearchCacheNamespace = "SHOP"
)

type SearchCacheClass string

const (
	SearchCacheClassQuery SearchCacheClass = "QUERY"

	SearchCacheClassFacet SearchCacheClass = "FACET"

	SearchCacheClassSuggestion SearchCacheClass = "SUGGESTION"

	SearchCacheClassTaxonomy SearchCacheClass = "TAXONOMY"
)

const (
	foodSearchCachePrefix = "vendor:search:food"

	shopSearchCachePrefix = "vendor:search:shop"

	maxTargetedCacheInvalidationKeys = 128
)

type SearchCacheTTLConfig struct {
	QueryTTL      time.Duration
	FacetTTL      time.Duration
	SuggestionTTL time.Duration
	TaxonomyTTL   time.Duration
}

func DefaultSearchCacheTTLConfig() SearchCacheTTLConfig {
	return SearchCacheTTLConfig{
		QueryTTL: 30 * time.Second,

		FacetTTL: 45 * time.Second,

		SuggestionTTL: 2 * time.Minute,

		TaxonomyTTL: 5 * time.Minute,
	}
}

func (
	config SearchCacheTTLConfig,
) Validate() error {
	values := []time.Duration{
		config.QueryTTL,
		config.FacetTTL,
		config.SuggestionTTL,
		config.TaxonomyTTL,
	}

	for _, value := range values {
		if value < time.Second ||
			value > 15*time.Minute {
			return ErrSearchCacheConfigInvalid
		}
	}

	return nil
}

func (
	config SearchCacheTTLConfig,
) TTLFor(
	class SearchCacheClass,
) (
	time.Duration,
	error,
) {
	switch class {
	case SearchCacheClassQuery:
		return config.QueryTTL, nil

	case SearchCacheClassFacet:
		return config.FacetTTL, nil

	case SearchCacheClassSuggestion:
		return config.SuggestionTTL, nil

	case SearchCacheClassTaxonomy:
		return config.TaxonomyTTL, nil

	default:
		return 0,
			ErrSearchCacheConfigInvalid
	}
}

type SearchCache interface {
	Get(
		ctx context.Context,
		key string,
	) (
		[]byte,
		bool,
		error,
	)

	Set(
		ctx context.Context,
		key string,
		value []byte,
		ttl time.Duration,
	) error

	Delete(
		ctx context.Context,
		keys ...string,
	) error
}

type SearchCacheRevisionProvider interface {
	CurrentSearchCacheRevision(
		ctx context.Context,
		namespace SearchCacheNamespace,
	) (
		int64,
		error,
	)
}

type FoodSearchOrigin interface {
	Search(
		ctx context.Context,
		request FoodEngineSearchRequest,
	) (
		FoodEngineSearchResponse,
		error,
	)

	Suggest(
		ctx context.Context,
		request SuggestionRequest,
	) (
		[]SearchSuggestion,
		error,
	)
}

type ShopSearchOrigin interface {
	Search(
		ctx context.Context,
		request ShopEngineSearchRequest,
	) (
		ShopEngineSearchResponse,
		error,
	)

	Suggest(
		ctx context.Context,
		request SuggestionRequest,
	) (
		[]SearchSuggestion,
		error,
	)
}

type cacheIdentity struct {
	Namespace SearchCacheNamespace `json:"namespace"`

	Class SearchCacheClass `json:"class"`

	Generation int64 `json:"generation"`

	CacheRevision int64 `json:"cache_revision"`

	ProfileCode string `json:"profile_code,omitempty"`

	Request any `json:"request"`
}

func canonicalStringValues(
	values []string,
) []string {
	if len(values) == 0 {
		return nil
	}

	result := make(
		[]string,
		0,
		len(values),
	)

	seen := make(
		map[string]struct{},
	)

	for _, value := range values {
		normalized, err :=
			NormalizeText(value)

		if err != nil ||
			normalized == "" {
			normalized =
				strings.ToLower(
					strings.TrimSpace(value),
				)
		}

		if normalized == "" {
			continue
		}

		if _, exists :=
			seen[normalized]; exists {
			continue
		}

		seen[normalized] =
			struct{}{}

		result = append(
			result,
			normalized,
		)
	}

	sort.Strings(result)

	return result
}

func canonicalFoodSearchRequest(
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchRequest,
	error,
) {
	normalizedQuery, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return FoodEngineSearchRequest{},
			err
	}

	request.Query =
		normalizedQuery

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

	if request.PageSize == 0 {
		request.PageSize = 20
	}

	request.Facets =
		canonicalStringValues(
			request.Facets,
		)

	request.Filters.ProviderTypes =
		canonicalStringValues(
			request.Filters.ProviderTypes,
		)

	request.Filters.Cuisines =
		canonicalStringValues(
			request.Filters.Cuisines,
		)

	request.Filters.MealPeriods =
		canonicalStringValues(
			request.Filters.MealPeriods,
		)

	request.Filters.DishTypes =
		canonicalStringValues(
			request.Filters.DishTypes,
		)

	request.Filters.FoodGroups =
		canonicalStringValues(
			request.Filters.FoodGroups,
		)

	request.Filters.Occasions =
		canonicalStringValues(
			request.Filters.Occasions,
		)

	request.Filters.DietaryClaims =
		canonicalStringValues(
			request.Filters.DietaryClaims,
		)

	request.Filters.PreferenceClaims =
		canonicalStringValues(
			request.Filters.PreferenceClaims,
		)

	return request, nil
}

func canonicalShopSearchRequest(
	request ShopEngineSearchRequest,
) (
	ShopEngineSearchRequest,
	error,
) {
	normalizedQuery, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return ShopEngineSearchRequest{},
			err
	}

	request.Query =
		normalizedQuery

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

	if request.PageSize == 0 {
		request.PageSize = 20
	}

	request.Facets =
		canonicalStringValues(
			request.Facets,
		)

	request.Filters.CanonicalCategories =
		canonicalStringValues(
			request.Filters.CanonicalCategories,
		)

	request.Filters.Brands =
		canonicalStringValues(
			request.Filters.Brands,
		)

	request.Filters.OperatingModels =
		canonicalStringValues(
			request.Filters.OperatingModels,
		)

	request.Filters.ProductClasses =
		canonicalStringValues(
			request.Filters.ProductClasses,
		)

	request.Filters.SellUnits =
		canonicalStringValues(
			request.Filters.SellUnits,
		)

	request.Filters.Measurements =
		canonicalStringValues(
			request.Filters.Measurements,
		)

	request.Filters.FreshAttributes =
		canonicalStringValues(
			request.Filters.FreshAttributes,
		)

	request.Filters.TemperatureZones =
		canonicalStringValues(
			request.Filters.TemperatureZones,
		)

	return request, nil
}

func buildSearchCacheKey(
	namespace SearchCacheNamespace,
	class SearchCacheClass,
	generation int64,
	cacheRevision int64,
	profileCode string,
	request any,
) (
	string,
	error,
) {
	if generation <= 0 ||
		cacheRevision <= 0 {
		return "",
			ErrInvalidArgument
	}

	var prefix string

	switch namespace {
	case SearchCacheNamespaceFood:
		prefix =
			foodSearchCachePrefix

	case SearchCacheNamespaceShop:
		prefix =
			shopSearchCachePrefix

	default:
		return "",
			ErrInvalidArgument
	}

	switch class {
	case SearchCacheClassQuery,
		SearchCacheClassFacet,
		SearchCacheClassSuggestion,
		SearchCacheClassTaxonomy:

	default:
		return "",
			ErrInvalidArgument
	}

	raw, err :=
		json.Marshal(
			cacheIdentity{
				Namespace: namespace,

				Class: class,

				Generation: generation,

				CacheRevision: cacheRevision,

				ProfileCode: profileCode,

				Request: request,
			},
		)
	if err != nil {
		return "", err
	}

	sum :=
		sha256.Sum256(raw)

	return prefix +
			":" +
			strings.ToLower(
				string(class),
			) +
			":g" +
			int64String(generation) +
			":r" +
			int64String(cacheRevision) +
			":" +
			hex.EncodeToString(sum[:]),
		nil
}

func int64String(
	value int64,
) string {
	return json.Number(
		stringInt64(value),
	).String()
}

func stringInt64(
	value int64,
) string {
	if value == 0 {
		return "0"
	}

	negative :=
		value < 0

	if negative {
		value = -value
	}

	var buffer [20]byte

	position :=
		len(buffer)

	for value > 0 {
		position--

		buffer[position] =
			byte(
				'0' +
					value%10,
			)

		value /= 10
	}

	if negative {
		position--

		buffer[position] = '-'
	}

	return string(
		buffer[position:],
	)
}

func buildFoodSearchCacheKey(
	request FoodEngineSearchRequest,
	cacheRevision int64,
	class SearchCacheClass,
) (
	string,
	error,
) {
	request, err :=
		canonicalFoodSearchRequest(
			request,
		)
	if err != nil {
		return "", err
	}

	return buildSearchCacheKey(
		SearchCacheNamespaceFood,
		class,
		request.Generation,
		cacheRevision,
		request.ProfileCode,
		request,
	)
}

func buildShopSearchCacheKey(
	request ShopEngineSearchRequest,
	cacheRevision int64,
	class SearchCacheClass,
) (
	string,
	error,
) {
	request, err :=
		canonicalShopSearchRequest(
			request,
		)
	if err != nil {
		return "", err
	}

	return buildSearchCacheKey(
		SearchCacheNamespaceShop,
		class,
		request.Generation,
		cacheRevision,
		request.ProfileCode,
		request,
	)
}

func buildSuggestionCacheKey(
	namespace SearchCacheNamespace,
	request SuggestionRequest,
	cacheRevision int64,
) (
	string,
	error,
) {
	if request.Generation <= 0 ||
		cacheRevision <= 0 {
		return "",
			ErrInvalidArgument
	}

	normalized, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return "", err
	}

	if request.Limit == 0 {
		request.Limit = 10
	}

	request.Query =
		normalized

	return buildSearchCacheKey(
		namespace,
		SearchCacheClassSuggestion,
		request.Generation,
		cacheRevision,
		"",
		request,
	)
}

type CachedFoodSearchEngine struct {
	origin FoodSearchOrigin

	cache SearchCache

	revisions SearchCacheRevisionProvider

	ttl SearchCacheTTLConfig
}

func NewCachedFoodSearchEngine(
	origin FoodSearchOrigin,
	cache SearchCache,
	revisions SearchCacheRevisionProvider,
	ttl SearchCacheTTLConfig,
) (
	*CachedFoodSearchEngine,
	error,
) {
	if origin == nil {
		return nil,
			ErrInvalidArgument
	}

	if ttl == (SearchCacheTTLConfig{}) {
		ttl =
			DefaultSearchCacheTTLConfig()
	}

	if err :=
		ttl.Validate(); err != nil {
		return nil, err
	}

	return &CachedFoodSearchEngine{
		origin: origin,

		cache: cache,

		revisions: revisions,

		ttl: ttl,
	}, nil
}

func (
	engine *CachedFoodSearchEngine,
) Search(
	ctx context.Context,
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.origin == nil {
		return FoodEngineSearchResponse{},
			ErrInvalidArgument
	}

	// Redis/revision-cache infrastructure is optional.
	// PostgreSQL search remains the correctness path.
	if engine.cache == nil ||
		engine.revisions == nil {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	revision, err :=
		engine.revisions.
			CurrentSearchCacheRevision(
				ctx,
				SearchCacheNamespaceFood,
			)
	if err != nil ||
		revision <= 0 {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	class :=
		SearchCacheClassQuery

	if len(request.Facets) > 0 {
		class =
			SearchCacheClassFacet
	}

	key, err :=
		buildFoodSearchCacheKey(
			request,
			revision,
			class,
		)
	if err != nil {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	if raw,
		found,
		cacheErr :=
		engine.cache.Get(
			ctx,
			key,
		); cacheErr == nil &&
		found {
		var response FoodEngineSearchResponse

		if json.Unmarshal(
			raw,
			&response,
		) == nil {
			return response, nil
		}

		// Corrupt cache entry is never trusted.
		_ = engine.cache.Delete(
			ctx,
			key,
		)
	}

	response, err :=
		engine.origin.Search(
			ctx,
			request,
		)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	raw, marshalErr :=
		json.Marshal(response)

	if marshalErr == nil {
		if ttl,
			ttlErr :=
			engine.ttl.TTLFor(
				class,
			); ttlErr == nil {
			// Cache write failure is best effort.
			_ = engine.cache.Set(
				ctx,
				key,
				raw,
				ttl,
			)
		}
	}

	return response, nil
}

func (
	engine *CachedFoodSearchEngine,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.origin == nil {
		return nil,
			ErrInvalidArgument
	}

	if engine.cache == nil ||
		engine.revisions == nil {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	revision, err :=
		engine.revisions.
			CurrentSearchCacheRevision(
				ctx,
				SearchCacheNamespaceFood,
			)
	if err != nil ||
		revision <= 0 {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	key, err :=
		buildSuggestionCacheKey(
			SearchCacheNamespaceFood,
			request,
			revision,
		)
	if err != nil {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	if raw,
		found,
		cacheErr :=
		engine.cache.Get(
			ctx,
			key,
		); cacheErr == nil &&
		found {
		var response []SearchSuggestion

		if json.Unmarshal(
			raw,
			&response,
		) == nil {
			return response, nil
		}

		_ = engine.cache.Delete(
			ctx,
			key,
		)
	}

	response, err :=
		engine.origin.Suggest(
			ctx,
			request,
		)
	if err != nil {
		return nil, err
	}

	raw, marshalErr :=
		json.Marshal(response)

	if marshalErr == nil {
		if ttl,
			ttlErr :=
			engine.ttl.TTLFor(
				SearchCacheClassSuggestion,
			); ttlErr == nil {
			_ = engine.cache.Set(
				ctx,
				key,
				raw,
				ttl,
			)
		}
	}

	return response, nil
}

func (
	engine *CachedFoodSearchEngine,
) InvalidateExactKeys(
	ctx context.Context,
	keys ...string,
) error {
	if engine == nil ||
		engine.cache == nil {
		return nil
	}

	if len(keys) >
		maxTargetedCacheInvalidationKeys {
		return ErrInvalidArgument
	}

	return engine.cache.Delete(
		ctx,
		keys...,
	)
}

type CachedShopSearchEngine struct {
	origin ShopSearchOrigin

	cache SearchCache

	revisions SearchCacheRevisionProvider

	ttl SearchCacheTTLConfig
}

func NewCachedShopSearchEngine(
	origin ShopSearchOrigin,
	cache SearchCache,
	revisions SearchCacheRevisionProvider,
	ttl SearchCacheTTLConfig,
) (
	*CachedShopSearchEngine,
	error,
) {
	if origin == nil {
		return nil,
			ErrInvalidArgument
	}

	if ttl == (SearchCacheTTLConfig{}) {
		ttl =
			DefaultSearchCacheTTLConfig()
	}

	if err :=
		ttl.Validate(); err != nil {
		return nil, err
	}

	return &CachedShopSearchEngine{
		origin: origin,

		cache: cache,

		revisions: revisions,

		ttl: ttl,
	}, nil
}

func (
	engine *CachedShopSearchEngine,
) Search(
	ctx context.Context,
	request ShopEngineSearchRequest,
) (
	ShopEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.origin == nil {
		return ShopEngineSearchResponse{},
			ErrInvalidArgument
	}

	if engine.cache == nil ||
		engine.revisions == nil {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	revision, err :=
		engine.revisions.
			CurrentSearchCacheRevision(
				ctx,
				SearchCacheNamespaceShop,
			)
	if err != nil ||
		revision <= 0 {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	class :=
		SearchCacheClassQuery

	if len(request.Facets) > 0 {
		class =
			SearchCacheClassFacet
	}

	key, err :=
		buildShopSearchCacheKey(
			request,
			revision,
			class,
		)
	if err != nil {
		return engine.origin.Search(
			ctx,
			request,
		)
	}

	if raw,
		found,
		cacheErr :=
		engine.cache.Get(
			ctx,
			key,
		); cacheErr == nil &&
		found {
		var response ShopEngineSearchResponse

		if json.Unmarshal(
			raw,
			&response,
		) == nil {
			return response, nil
		}

		_ = engine.cache.Delete(
			ctx,
			key,
		)
	}

	response, err :=
		engine.origin.Search(
			ctx,
			request,
		)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	raw, marshalErr :=
		json.Marshal(response)

	if marshalErr == nil {
		if ttl,
			ttlErr :=
			engine.ttl.TTLFor(
				class,
			); ttlErr == nil {
			_ = engine.cache.Set(
				ctx,
				key,
				raw,
				ttl,
			)
		}
	}

	return response, nil
}

func (
	engine *CachedShopSearchEngine,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.origin == nil {
		return nil,
			ErrInvalidArgument
	}

	if engine.cache == nil ||
		engine.revisions == nil {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	revision, err :=
		engine.revisions.
			CurrentSearchCacheRevision(
				ctx,
				SearchCacheNamespaceShop,
			)
	if err != nil ||
		revision <= 0 {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	key, err :=
		buildSuggestionCacheKey(
			SearchCacheNamespaceShop,
			request,
			revision,
		)
	if err != nil {
		return engine.origin.Suggest(
			ctx,
			request,
		)
	}

	if raw,
		found,
		cacheErr :=
		engine.cache.Get(
			ctx,
			key,
		); cacheErr == nil &&
		found {
		var response []SearchSuggestion

		if json.Unmarshal(
			raw,
			&response,
		) == nil {
			return response, nil
		}

		_ = engine.cache.Delete(
			ctx,
			key,
		)
	}

	response, err :=
		engine.origin.Suggest(
			ctx,
			request,
		)
	if err != nil {
		return nil, err
	}

	raw, marshalErr :=
		json.Marshal(response)

	if marshalErr == nil {
		if ttl,
			ttlErr :=
			engine.ttl.TTLFor(
				SearchCacheClassSuggestion,
			); ttlErr == nil {
			_ = engine.cache.Set(
				ctx,
				key,
				raw,
				ttl,
			)
		}
	}

	return response, nil
}

func (
	engine *CachedShopSearchEngine,
) InvalidateExactKeys(
	ctx context.Context,
	keys ...string,
) error {
	if engine == nil ||
		engine.cache == nil {
		return nil
	}

	if len(keys) >
		maxTargetedCacheInvalidationKeys {
		return ErrInvalidArgument
	}

	return engine.cache.Delete(
		ctx,
		keys...,
	)
}

var (
	_ FoodSearchOrigin = (*FoodSearchEngine)(nil)

	_ ShopSearchOrigin = (*ShopSearchEngine)(nil)
)
