package searchprojection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type FoodProjectionInput struct {
	BusinessRef string
	BranchRef   string
	FoodItemRef string

	MenuRefs []string

	Name string

	Aliases  []string
	Synonyms []string

	CanonicalTaxonomy json.RawMessage

	ProviderTypes []string
	CuisineTerms  []string
	MealPeriods   []string
	DishTypes     []string
	FoodGroups    []string
	Occasions     []string

	DietaryClaims    []string
	PreferenceClaims []string

	AdditionalSearchTerms []string

	Active bool

	AvailabilitySummary   json.RawMessage
	EffectivePriceSummary json.RawMessage
	ServiceabilityHint    json.RawMessage
	TrustHint             json.RawMessage
	ReliabilityHint       json.RawMessage

	PromotionIndicator bool

	CatalogueRevision int64
	Generation        int64
}

type ShopProjectionInput struct {
	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Name  string
	Brand string

	Aliases  []string
	Synonyms []string

	CanonicalTaxonomy json.RawMessage

	TaxonomyTerms []string

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

	AdditionalSearchTerms []string

	CatalogueRevision int64
	InventoryRevision int64
	Generation        int64
}

type FoodProjector struct {
	backend FoodSearchBackend
}

func NewFoodProjector(
	backend FoodSearchBackend,
) *FoodProjector {
	return &FoodProjector{
		backend: backend,
	}
}

type ShopProjector struct {
	backend ShopSearchBackend
}

func NewShopProjector(
	backend ShopSearchBackend,
) *ShopProjector {
	return &ShopProjector{
		backend: backend,
	}
}

func deterministicDocumentID(
	prefix string,
	values ...string,
) string {
	raw := strings.Join(
		values,
		"|",
	)

	sum := sha256.Sum256(
		[]byte(raw),
	)

	return prefix +
		hex.EncodeToString(sum[:16])
}

func (p *FoodProjector) Project(
	ctx context.Context,
	input FoodProjectionInput,
) (ProjectionWriteResult, error) {
	if p == nil ||
		p.backend == nil {
		return ProjectionWriteResult{},
			ErrInvalidArgument
	}

	document, err :=
		BuildFoodSearchDocument(input)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	return p.backend.Upsert(
		ctx,
		document,
	)
}

func (p *ShopProjector) Project(
	ctx context.Context,
	input ShopProjectionInput,
) (ProjectionWriteResult, error) {
	if p == nil ||
		p.backend == nil {
		return ProjectionWriteResult{},
			ErrInvalidArgument
	}

	document, err :=
		BuildShopSearchDocument(input)
	if err != nil {
		return ProjectionWriteResult{},
			err
	}

	return p.backend.Upsert(
		ctx,
		document,
	)
}

func BuildFoodSearchDocument(
	input FoodProjectionInput,
) (FoodSearchDocument, error) {
	input.BusinessRef =
		strings.TrimSpace(input.BusinessRef)

	input.BranchRef =
		strings.TrimSpace(input.BranchRef)

	input.FoodItemRef =
		strings.TrimSpace(input.FoodItemRef)

	if input.BusinessRef == "" ||
		input.BranchRef == "" ||
		input.FoodItemRef == "" ||
		strings.TrimSpace(input.Name) == "" ||
		input.CatalogueRevision <= 0 ||
		input.Generation <= 0 {
		return FoodSearchDocument{},
			ErrInvalidArgument
	}

	canonicalTaxonomy, err :=
		normalizedJSONObject(
			input.CanonicalTaxonomy,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	availability, err :=
		normalizedJSONObject(
			input.AvailabilitySummary,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	price, err :=
		normalizedJSONObject(
			input.EffectivePriceSummary,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	serviceability, err :=
		normalizedJSONObject(
			input.ServiceabilityHint,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	trust, err :=
		normalizedJSONObject(
			input.TrustHint,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	reliability, err :=
		normalizedJSONObject(
			input.ReliabilityHint,
		)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	normalizedName,
		groups,
		tokens,
		searchText,
		err := buildSearchMaterial(
		input.Name,
		input.Aliases,
		input.Synonyms,
		input.ProviderTypes,
		input.CuisineTerms,
		input.MealPeriods,
		input.DishTypes,
		input.FoodGroups,
		input.Occasions,
		input.DietaryClaims,
		input.PreferenceClaims,
		input.AdditionalSearchTerms,
	)
	if err != nil {
		return FoodSearchDocument{},
			err
	}

	return FoodSearchDocument{
		DocumentID: deterministicDocumentID(
			"food_",
			input.BusinessRef,
			input.BranchRef,
			input.FoodItemRef,
		),

		Generation: input.Generation,

		BusinessRef: input.BusinessRef,

		BranchRef: input.BranchRef,

		FoodItemRef: input.FoodItemRef,

		MenuRefs: append(
			[]string(nil),
			input.MenuRefs...,
		),

		Name: strings.TrimSpace(
			input.Name,
		),

		NormalizedName: normalizedName,

		SearchTokens: tokens,

		Aliases: groups[0],

		Synonyms: groups[1],

		CanonicalTaxonomy: canonicalTaxonomy,

		ProviderTypes: groups[2],

		CuisineTerms: groups[3],

		MealPeriods: groups[4],

		DishTypes: groups[5],

		FoodGroups: groups[6],

		Occasions: groups[7],

		DietaryClaims: groups[8],

		PreferenceClaims: groups[9],

		Active: input.Active,

		AvailabilitySummary: availability,

		EffectivePriceSummary: price,

		ServiceabilityHint: serviceability,

		TrustHint: trust,

		ReliabilityHint: reliability,

		PromotionIndicator: input.PromotionIndicator,

		CatalogueRevision: input.CatalogueRevision,

		SearchText: searchText,
	}.normalizeArrays(), nil
}

func BuildShopSearchDocument(
	input ShopProjectionInput,
) (ShopSearchDocument, error) {
	input.BusinessRef =
		strings.TrimSpace(input.BusinessRef)

	input.BranchRef =
		strings.TrimSpace(input.BranchRef)

	input.ProductRef =
		strings.TrimSpace(input.ProductRef)

	input.VariantRef =
		strings.TrimSpace(input.VariantRef)

	input.ListingRef =
		strings.TrimSpace(input.ListingRef)

	if input.BusinessRef == "" ||
		input.BranchRef == "" ||
		input.ProductRef == "" ||
		strings.TrimSpace(input.Name) == "" ||
		input.CatalogueRevision <= 0 ||
		input.InventoryRevision <= 0 ||
		input.Generation <= 0 {
		return ShopSearchDocument{},
			ErrInvalidArgument
	}

	canonicalTaxonomy, err :=
		normalizedJSONObject(
			input.CanonicalTaxonomy,
		)
	if err != nil {
		return ShopSearchDocument{},
			err
	}

	price, err :=
		normalizedJSONObject(
			input.EffectivePriceSummary,
		)
	if err != nil {
		return ShopSearchDocument{},
			err
	}

	inventory, err :=
		normalizedJSONObject(
			input.InventoryAvailabilitySummary,
		)
	if err != nil {
		return ShopSearchDocument{},
			err
	}

	handling, err :=
		normalizedJSONObject(
			input.HandlingMetadata,
		)
	if err != nil {
		return ShopSearchDocument{},
			err
	}

	normalizedName,
		groups,
		tokens,
		searchText,
		err := buildSearchMaterial(
		input.Name,
		input.Aliases,
		input.Synonyms,
		input.TaxonomyTerms,
		input.SellUnits,
		input.MeasureNames,
		input.LocalMeasureAliases,
		input.PackagingTerms,
		input.SKUTerms,
		input.BarcodeTerms,
		input.FreshAttributes,
		[]string{
			input.Brand,
			input.TemperatureZone,
		},
		input.AdditionalSearchTerms,
	)
	if err != nil {
		return ShopSearchDocument{},
			err
	}

	return ShopSearchDocument{
		DocumentID: deterministicDocumentID(
			"shop_",
			input.BusinessRef,
			input.BranchRef,
			input.ProductRef,
			input.VariantRef,
			input.ListingRef,
		),

		Generation: input.Generation,

		BusinessRef: input.BusinessRef,

		BranchRef: input.BranchRef,

		ProductRef: input.ProductRef,

		VariantRef: input.VariantRef,

		ListingRef: input.ListingRef,

		Name: strings.TrimSpace(
			input.Name,
		),

		Brand: strings.TrimSpace(
			input.Brand,
		),

		NormalizedName: normalizedName,

		SearchTokens: tokens,

		Aliases: groups[0],

		Synonyms: groups[1],

		CanonicalTaxonomy: canonicalTaxonomy,

		SellUnits: groups[3],

		MeasureNames: groups[4],

		LocalMeasureAliases: groups[5],

		PackagingTerms: groups[6],

		SKUTerms: groups[7],

		BarcodeTerms: groups[8],

		RetailEligible: input.RetailEligible,

		WholesaleEligible: input.WholesaleEligible,

		Active: input.Active,

		EffectivePriceSummary: price,

		InventoryAvailabilitySummary: inventory,

		PreorderHint: input.PreorderHint,

		BackorderHint: input.BackorderHint,

		FreshAttributes: groups[9],

		TemperatureZone: strings.TrimSpace(
			input.TemperatureZone,
		),

		HandlingMetadata: handling,

		CatalogueRevision: input.CatalogueRevision,

		InventoryRevision: input.InventoryRevision,

		SearchText: searchText,
	}.normalizeArrays(), nil
}
