package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	DefaultFoodRankingProfile = "FOOD_DEFAULT_V1"

	DefaultShopRankingProfile = "SHOP_DEFAULT_V1"

	maxEngineQueryTokens   = 8
	maxEnginePageSize      = 100
	maxEngineFacets        = 8
	maxFacetValues         = 20
	maxFilterValuesPerKind = 20
	maxSuggestionLimit     = 20
)

var (
	ErrInvalidCursor = errors.New("invalid search cursor")

	ErrDistanceContextUnavailable = errors.New("distance sorting requires legitimate distance context")

	ErrRankingProfileInvalid = errors.New("ranking profile invalid")

	// ErrRankingProfileNotFound is a Section 13I administration error: the
	// named profile does not exist inside the addressed namespace. It is
	// deliberately not distinguishable from "exists in the other namespace",
	// so a caller cannot enumerate SHOP profiles through the FOOD path.
	ErrRankingProfileNotFound = errors.New("ranking profile not found")
)

type PlacementType string

const (
	PlacementOrganic PlacementType = "ORGANIC"

	PlacementSponsored PlacementType = "SPONSORED"
)

type SearchSort string

const (
	SortRelevance SearchSort = "RELEVANCE"

	SortPriceLowToHigh SearchSort = "PRICE_LOW_TO_HIGH"

	SortPriceHighToLow SearchSort = "PRICE_HIGH_TO_LOW"

	SortDistance SearchSort = "DISTANCE"
)

type RankingProfile struct {
	ID string

	ProfileCode   string
	NamespaceCode string
	Version       int

	ExactNameWeight int
	PrefixWeight    int
	FullTextWeight  int
	AliasWeight     int
	SynonymWeight   int
	CategoryWeight  int
	TagWeight       int

	BrandWeight       int
	IdentifierWeight  int
	MeasurementWeight int

	TypoWeight int

	AvailabilityWeight   int
	ServiceabilityWeight int
	TrustWeight          int
	ReliabilityWeight    int
	DistanceWeight       int
	CommercialWeight     int
	PromotionWeight      int

	MaxNonTextBoost int
	MaxTypoDistance int
	MaxCandidates   int
}

type SearchDistanceContext struct {
	Latitude  float64
	Longitude float64
}

type FoodDistanceResolver interface {
	ResolveFoodDistanceMeters(
		ctx context.Context,
		distanceContext SearchDistanceContext,
		document FoodEngineDocument,
	) (
		int64,
		bool,
		error,
	)
}

type ShopDistanceResolver interface {
	ResolveShopDistanceMeters(
		ctx context.Context,
		distanceContext SearchDistanceContext,
		document ShopEngineDocument,
	) (
		int64,
		bool,
		error,
	)
}

type FoodSearchFilters struct {
	ProviderTypes []string
	Cuisines      []string
	MealPeriods   []string
	DishTypes     []string
	FoodGroups    []string
	Occasions     []string

	DietaryClaims    []string
	PreferenceClaims []string

	MinPriceMinor *int64
	MaxPriceMinor *int64

	Available   *bool
	Serviceable *bool
	Trusted     *bool
}

type ShopSearchFilters struct {
	CanonicalCategories []string
	Brands              []string

	RetailEligible    *bool
	WholesaleEligible *bool

	MinPriceMinor *int64
	MaxPriceMinor *int64

	Available *bool

	OperatingModels []string
	ProductClasses  []string

	SellUnits    []string
	Measurements []string

	FreshAttributes  []string
	TemperatureZones []string
}

type FoodEngineSearchRequest struct {
	Query string

	Generation int64
	BranchRef  string

	ProfileCode string

	Filters FoodSearchFilters

	Facets []string

	Sort SearchSort

	PageSize int
	Cursor   string

	DistanceContext *SearchDistanceContext
}

type ShopEngineSearchRequest struct {
	Query string

	Generation int64
	BranchRef  string

	ProfileCode string

	Filters ShopSearchFilters

	Facets []string

	Sort SearchSort

	PageSize int
	Cursor   string

	DistanceContext *SearchDistanceContext
}

type SearchFacetValue struct {
	Value string
	Count int
}

type SearchFacet struct {
	Name string

	Values []SearchFacetValue
}

type FoodEngineHit struct {
	DocumentID string

	BusinessRef string
	BranchRef   string
	FoodItemRef string

	Name string

	Score int64

	PriceMinor *int64

	DistanceMeters *int64

	Sponsored bool

	PlacementType PlacementType

	AvailabilityStatus   string
	ServiceabilityStatus string

	SourceEvidence     SearchSourceEvidence
	ProjectionRevision int64
}

type ShopEngineHit struct {
	DocumentID string

	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Name  string
	Brand string

	Score int64

	PriceMinor *int64

	DistanceMeters *int64

	Sponsored bool

	PlacementType PlacementType

	AvailabilityStatus   string
	ServiceabilityStatus string

	SourceEvidence     SearchSourceEvidence
	ProjectionRevision int64
}

type FoodEngineSearchResponse struct {
	ProfileCode    string
	ProfileVersion int

	Hits []FoodEngineHit

	Facets []SearchFacet

	NextCursor string
}

type ShopEngineSearchResponse struct {
	ProfileCode    string
	ProfileVersion int

	Hits []ShopEngineHit

	Facets []SearchFacet

	NextCursor string
}

type SuggestionRequest struct {
	Query string

	Generation int64

	Limit int
}

type SearchSuggestion struct {
	Text string

	NormalizedText string

	SourceType string
}

type FoodEngineDocument struct {
	DocumentID string

	Generation int64

	BusinessRef string
	BranchRef   string
	FoodItemRef string

	Name           string
	NormalizedName string

	Aliases  []string
	Synonyms []string

	ProviderTypes []string
	CuisineTerms  []string
	MealPeriods   []string
	DishTypes     []string
	FoodGroups    []string
	Occasions     []string

	DietaryClaims    []string
	PreferenceClaims []string

	Active bool

	AvailabilitySummary   json.RawMessage
	EffectivePriceSummary json.RawMessage
	ServiceabilityHint    json.RawMessage
	TrustHint             json.RawMessage
	ReliabilityHint       json.RawMessage

	PromotionIndicator bool

	CatalogueRevision  int64
	ProjectionRevision int64
}

type ShopEngineDocument struct {
	DocumentID string

	Generation int64

	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Name           string
	Brand          string
	NormalizedName string

	Aliases  []string
	Synonyms []string

	CanonicalTaxonomy json.RawMessage

	SellUnits           []string
	MeasureNames        []string
	LocalMeasureAliases []string
	PackagingTerms      []string

	SKUTerms     []string
	BarcodeTerms []string

	RetailEligible    bool
	WholesaleEligible bool
	Active            bool

	EffectivePriceSummary        json.RawMessage
	InventoryAvailabilitySummary json.RawMessage

	PreorderHint  bool
	BackorderHint bool

	FreshAttributes []string

	TemperatureZone string

	HandlingMetadata json.RawMessage

	PromotionIndicator bool

	CatalogueRevision  int64
	InventoryRevision  int64
	ProjectionRevision int64
}

type FoodSearchEngineBackend interface {
	LoadRankingProfile(
		ctx context.Context,
		profileCode string,
		namespaceCode string,
	) (
		RankingProfile,
		error,
	)

	FindFoodCandidates(
		ctx context.Context,
		normalizedQuery string,
		queryTokens []string,
		generation int64,
		branchRef string,
		limit int,
	) (
		[]FoodEngineDocument,
		error,
	)

	SuggestFood(
		ctx context.Context,
		normalizedPrefix string,
		generation int64,
		limit int,
	) (
		[]SearchSuggestion,
		error,
	)
}

type ShopSearchEngineBackend interface {
	LoadRankingProfile(
		ctx context.Context,
		profileCode string,
		namespaceCode string,
	) (
		RankingProfile,
		error,
	)

	FindShopCandidates(
		ctx context.Context,
		normalizedQuery string,
		queryTokens []string,
		generation int64,
		branchRef string,
		limit int,
	) (
		[]ShopEngineDocument,
		error,
	)

	SuggestShop(
		ctx context.Context,
		normalizedPrefix string,
		generation int64,
		limit int,
	) (
		[]SearchSuggestion,
		error,
	)
}
