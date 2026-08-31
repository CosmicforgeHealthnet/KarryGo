package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid search projection argument")
	ErrNotFound        = errors.New("search projection not found")
)

type ProjectionWriteResult struct {
	Applied bool

	ProjectionRevision int64

	CatalogueRevision int64
	InventoryRevision int64
}

type SearchRequest struct {
	Query string

	Generation int64

	BranchRef string

	Limit int
}

type FoodSearchDocument struct {
	DocumentID string
	Generation int64

	BusinessRef string
	BranchRef   string
	FoodItemRef string

	MenuRefs []string

	Name           string
	NormalizedName string

	SearchTokens []string
	Aliases      []string
	Synonyms     []string

	CanonicalTaxonomy json.RawMessage

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

	CatalogueRevision int64

	ProjectionRevision int64

	SearchText string

	UpdatedAt time.Time
}

type ShopSearchDocument struct {
	DocumentID string
	Generation int64

	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Name  string
	Brand string

	NormalizedName string

	SearchTokens []string
	Aliases      []string
	Synonyms     []string

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

	CatalogueRevision int64
	InventoryRevision int64

	ProjectionRevision int64

	SearchText string

	UpdatedAt time.Time
}

type FoodSearchHit struct {
	DocumentID string

	BusinessRef string
	BranchRef   string
	FoodItemRef string

	Name string

	Generation int64

	ProjectionRevision int64

	Rank float32
}

type ShopSearchHit struct {
	DocumentID string

	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Name  string
	Brand string

	Generation int64

	ProjectionRevision int64

	Rank float32
}

// FoodSearchBackend is intentionally separate from ShopSearchBackend.
//
// A future OpenSearch / Elasticsearch / Typesense / Meilisearch adapter
// implements this interface without changing FOOD catalogue ownership.
type FoodSearchBackend interface {
	Upsert(
		ctx context.Context,
		document FoodSearchDocument,
	) (ProjectionWriteResult, error)

	Search(
		ctx context.Context,
		request SearchRequest,
	) ([]FoodSearchHit, error)
}

// ShopSearchBackend is intentionally separate from FoodSearchBackend.
//
// Physical/search-engine implementation can change without allowing
// SHOP search storage to become product, inventory or pricing authority.
type ShopSearchBackend interface {
	Upsert(
		ctx context.Context,
		document ShopSearchDocument,
	) (ProjectionWriteResult, error)

	Search(
		ctx context.Context,
		request SearchRequest,
	) ([]ShopSearchHit, error)
}

type PostgresSearchCapabilities struct {
	BuiltInTextSearch bool
	GINAvailable      bool

	PGTrgmAvailable bool
	PGTrgmInstalled bool
}
