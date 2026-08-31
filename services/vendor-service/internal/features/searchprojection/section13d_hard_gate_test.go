package searchprojection

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13DHardGate(
	t *testing.T,
) {
	dsn :=
		os.Getenv(
			"VENDOR_SERVICE_TEST_DATABASE_URL",
		)

	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not configured",
		)
	}

	ctx, cancel :=
		context.WithTimeout(
			context.Background(),
			2*time.Minute,
		)
	defer cancel()

	pool, err :=
		pgxpool.New(
			ctx,
			dsn,
		)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err = pool.Exec(
		ctx,
		`
        TRUNCATE TABLE
            food_search_documents,
            shop_search_documents
        CASCADE
        `,
	); err != nil {
		t.Fatalf(
			"reset 13D projections: %v",
			err,
		)
	}

	capabilities, err :=
		InspectPostgresSearchCapabilities(
			ctx,
			pool,
		)
	if err != nil {
		t.Fatal(err)
	}

	if !capabilities.BuiltInTextSearch {
		t.Fatal(
			"PostgreSQL built-in text search unavailable",
		)
	}

	if !capabilities.GINAvailable {
		t.Fatal(
			"PostgreSQL GIN access method unavailable",
		)
	}

	foodBackend :=
		NewPostgresFoodSearchBackend(
			pool,
		)

	shopBackend :=
		NewPostgresShopSearchBackend(
			pool,
		)

	foodProjector :=
		NewFoodProjector(
			foodBackend,
		)

	shopProjector :=
		NewShopProjector(
			shopBackend,
		)

	// --------------------------------------------------------
	// FOOD: JOLLOF RICE
	// --------------------------------------------------------

	foodInput := FoodProjectionInput{
		BusinessRef: "business-food-13d",

		BranchRef: "branch-food-13d",

		FoodItemRef: "food-jollof-rice",

		MenuRefs: []string{
			"menu-lunch",
		},

		Name: "Jollof Rice",

		Aliases: []string{
			"Nigerian Jollof",
		},

		Synonyms: []string{},

		CanonicalTaxonomy: json.RawMessage(
			`{"dish_type":"RICE_MEALS"}`,
		),

		ProviderTypes: []string{
			"Restaurant",
		},

		CuisineTerms: []string{
			"Nigerian",
			"Local Dishes",
		},

		MealPeriods: []string{
			"Lunch",
			"Dinner",
		},

		DishTypes: []string{
			"Rice Meals",
		},

		FoodGroups: []string{
			"Rice",
		},

		Occasions: []string{
			"Family Meals",
		},

		DietaryClaims: []string{},

		PreferenceClaims: []string{},

		Active: true,

		AvailabilitySummary: json.RawMessage(
			`{"available":true}`,
		),

		EffectivePriceSummary: json.RawMessage(
			`{"currency":"NGN","minor":350000}`,
		),

		ServiceabilityHint: json.RawMessage(
			`{"serviceable":true}`,
		),

		TrustHint: json.RawMessage(
			`{"eligible":true}`,
		),

		ReliabilityHint: json.RawMessage(
			`{"healthy":true}`,
		),

		CatalogueRevision: 7,

		Generation: 1,
	}

	firstFood, err :=
		foodProjector.Project(
			ctx,
			foodInput,
		)
	if err != nil {
		t.Fatalf(
			"project Jollof Rice: %v",
			err,
		)
	}

	if !firstFood.Applied ||
		firstFood.ProjectionRevision != 1 {
		t.Fatalf(
			"first FOOD projection result=%+v",
			firstFood,
		)
	}

	// Same source revision is a no-op.
	secondFood, err :=
		foodProjector.Project(
			ctx,
			foodInput,
		)
	if err != nil {
		t.Fatalf(
			"reproject Jollof Rice: %v",
			err,
		)
	}

	if secondFood.Applied ||
		secondFood.ProjectionRevision != 1 {
		t.Fatalf(
			"FOOD idempotency result=%+v",
			secondFood,
		)
	}

	// --------------------------------------------------------
	// SHOP: RAW RICE 50KG
	// Includes Section 12-derived measurement/search vocabulary.
	// --------------------------------------------------------

	shopInput := ShopProjectionInput{
		BusinessRef: "business-shop-13d",

		BranchRef: "branch-shop-13d",

		ProductRef: "shop-raw-rice",

		VariantRef: "variant-50kg",

		ListingRef: "listing-raw-rice-50kg",

		Name: "Raw Rice 50kg",

		Brand: "Market Choice",

		Aliases: []string{
			"50kg Rice",
		},

		Synonyms: []string{
			"Rice Grain",
		},

		CanonicalTaxonomy: json.RawMessage(
			`{"category":"RICE_GRAINS"}`,
		),

		TaxonomyTerms: []string{
			"Foodstuff",
			"Rice and Grains",
		},

		SellUnits: []string{
			"Bag",
		},

		MeasureNames: []string{
			"Paint",
			"Half Paint",
			"Derica",
			"50kg",
		},

		LocalMeasureAliases: []string{
			"Paint Rubber",
		},

		PackagingTerms: []string{
			"Bag",
			"Crate",
			"Basket",
		},

		SKUTerms: []string{
			"RICE50KG",
		},

		BarcodeTerms: []string{
			"1234567890123",
		},

		RetailEligible: true,

		WholesaleEligible: true,

		Active: true,

		EffectivePriceSummary: json.RawMessage(
			`{"currency":"NGN","minor":7500000}`,
		),

		InventoryAvailabilitySummary: json.RawMessage(
			`{"available":true,"quantity":10}`,
		),

		FreshAttributes: []string{
			"Dry",
		},

		TemperatureZone: "Ambient",

		HandlingMetadata: json.RawMessage(
			`{"keep_dry":true}`,
		),

		CatalogueRevision: 9,

		InventoryRevision: 14,

		Generation: 1,
	}

	firstShop, err :=
		shopProjector.Project(
			ctx,
			shopInput,
		)
	if err != nil {
		t.Fatalf(
			"project Raw Rice 50kg: %v",
			err,
		)
	}

	if !firstShop.Applied ||
		firstShop.ProjectionRevision != 1 {
		t.Fatalf(
			"first SHOP projection result=%+v",
			firstShop,
		)
	}

	secondShop, err :=
		shopProjector.Project(
			ctx,
			shopInput,
		)
	if err != nil {
		t.Fatalf(
			"reproject Raw Rice 50kg: %v",
			err,
		)
	}

	if secondShop.Applied ||
		secondShop.ProjectionRevision != 1 {
		t.Fatalf(
			"SHOP idempotency result=%+v",
			secondShop,
		)
	}

	// --------------------------------------------------------
	// PHYSICAL / SEARCH ISOLATION
	// --------------------------------------------------------

	foodJollof, err :=
		foodBackend.Search(
			ctx,
			SearchRequest{
				Query: "Jollof",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(foodJollof) != 1 ||
		foodJollof[0].FoodItemRef !=
			"food-jollof-rice" {
		t.Fatalf(
			"FOOD Jollof results=%+v",
			foodJollof,
		)
	}

	foodRawRice, err :=
		foodBackend.Search(
			ctx,
			SearchRequest{
				Query: "Raw Rice",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(foodRawRice) != 0 {
		t.Fatalf(
			"SHOP Raw Rice leaked into FOOD: %+v",
			foodRawRice,
		)
	}

	shopRawRice, err :=
		shopBackend.Search(
			ctx,
			SearchRequest{
				Query: "Raw Rice",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(shopRawRice) != 1 ||
		shopRawRice[0].ProductRef !=
			"shop-raw-rice" {
		t.Fatalf(
			"SHOP Raw Rice results=%+v",
			shopRawRice,
		)
	}

	shopJollof, err :=
		shopBackend.Search(
			ctx,
			SearchRequest{
				Query: "Jollof",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(shopJollof) != 0 {
		t.Fatalf(
			"FOOD Jollof leaked into SHOP: %+v",
			shopJollof,
		)
	}

	// --------------------------------------------------------
	// SECTION 12 MEASUREMENT SEARCH EVIDENCE
	// --------------------------------------------------------

	for _, query := range []string{
		"50kg",
		"Paint",
		"Half Paint",
		"Derica",
		"Bag",
		"Crate",
		"Basket",
	} {
		hits, searchErr :=
			shopBackend.Search(
				ctx,
				SearchRequest{
					Query: query,

					Generation: 1,

					Limit: 10,
				},
			)
		if searchErr != nil {
			t.Fatalf(
				"search %q: %v",
				query,
				searchErr,
			)
		}

		if len(hits) != 1 ||
			hits[0].ProductRef !=
				"shop-raw-rice" {
			t.Fatalf(
				"measurement query %q hits=%+v",
				query,
				hits,
			)
		}
	}

	// --------------------------------------------------------
	// PHYSICAL TABLE COUNTS
	// --------------------------------------------------------

	var foodCount int
	var shopCount int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM food_search_documents
         WHERE generation=1
        `,
	).Scan(&foodCount); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM shop_search_documents
         WHERE generation=1
        `,
	).Scan(&shopCount); err != nil {
		t.Fatal(err)
	}

	if foodCount != 1 ||
		shopCount != 1 {
		t.Fatalf(
			"physical projection counts FOOD=%d SHOP=%d",
			foodCount,
			shopCount,
		)
	}
}
