package searchprojection

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13EHardGate(
	t *testing.T,
) {
	dsn := os.Getenv(
		"VENDOR_SERVICE_TEST_DATABASE_URL",
	)

	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not configured",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	pool, err := pgxpool.New(
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
			"reset search projections: %v",
			err,
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

	foodEngine :=
		NewFoodSearchEngine(
			foodBackend,
		)

	shopEngine :=
		NewShopSearchEngine(
			shopBackend,
		)

	// ========================================================
	// FOOD FIXTURES
	// ========================================================

	foodFixtures := []FoodProjectionInput{
		{
			BusinessRef: "food-business-13e",

			BranchRef: "food-branch-13e",

			FoodItemRef: "food-jollof",

			Name: "Jollof Rice",

			CuisineTerms: []string{
				"Nigerian",
			},

			MealPeriods: []string{
				"Lunch",
				"Dinner",
			},

			DishTypes: []string{
				"Rice Meals",
			},

			AvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			EffectivePriceSummary: json.RawMessage(
				`{"minor":350000}`,
			),

			ServiceabilityHint: json.RawMessage(
				`{"serviceable":true}`,
			),

			TrustHint: json.RawMessage(
				`{"trusted":true}`,
			),

			ReliabilityHint: json.RawMessage(
				`{"healthy":true}`,
			),

			Active: true,

			CatalogueRevision: 1,

			Generation: 1,
		},
		{
			BusinessRef: "food-business-13e",

			BranchRef: "food-branch-13e",

			FoodItemRef: "food-shawarma",

			Name: "Chicken Shawarma",

			Aliases: []string{
				"Shawarma",
			},

			DishTypes: []string{
				"Shawarma",
			},

			AvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			ServiceabilityHint: json.RawMessage(
				`{"serviceable":true}`,
			),

			Active: true,

			CatalogueRevision: 1,

			Generation: 1,
		},
		{
			BusinessRef: "food-business-13e",

			BranchRef: "food-branch-13e",

			FoodItemRef: "food-moi-moi",

			Name: "Moi Moi",

			Aliases: []string{
				"Moin Moin",
				"Moimoi",
			},

			AvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			ServiceabilityHint: json.RawMessage(
				`{"serviceable":true}`,
			),

			Active: true,

			CatalogueRevision: 1,

			Generation: 1,
		},
		{
			BusinessRef: "food-business-13e",

			BranchRef: "food-branch-13e",

			FoodItemRef: "food-promoted-burger",

			Name: "Beef Burger",

			PromotionIndicator: true,

			AvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			ServiceabilityHint: json.RawMessage(
				`{"serviceable":true}`,
			),

			Active: true,

			CatalogueRevision: 1,

			Generation: 1,
		},
	}

	for _, fixture := range foodFixtures {
		if _, err = foodProjector.Project(
			ctx,
			fixture,
		); err != nil {
			t.Fatalf(
				"FOOD fixture %s: %v",
				fixture.FoodItemRef,
				err,
			)
		}
	}

	// ========================================================
	// SHOP FIXTURES
	// ========================================================

	shopFixtures := []ShopProjectionInput{
		{
			BusinessRef: "shop-business-13e",

			BranchRef: "shop-branch-13e",

			ProductRef: "shop-garri",

			VariantRef: "garri-retail",

			Name: "Garri",

			Brand: "Local Market",

			Aliases: []string{
				"Gari",
			},

			TaxonomyTerms: []string{
				"Garri and Cassava",
			},

			SellUnits: []string{
				"Paint",
			},

			MeasureNames: []string{
				"Paint",
				"Derica",
			},

			RetailEligible: true,

			Active: true,

			EffectivePriceSummary: json.RawMessage(
				`{"minor":300000}`,
			),

			InventoryAvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			CatalogueRevision: 1,

			InventoryRevision: 1,

			Generation: 1,
		},
		{
			BusinessRef: "shop-business-13e",

			BranchRef: "shop-branch-13e",

			ProductRef: "shop-beans",

			VariantRef: "beans-paint",

			Name: "Brown Beans",

			Brand: "Farm Choice",

			TaxonomyTerms: []string{
				"Beans and Pulses",
			},

			SellUnits: []string{
				"Paint",
			},

			MeasureNames: []string{
				"Paint",
				"Half Paint",
			},

			RetailEligible: true,

			WholesaleEligible: true,

			Active: true,

			EffectivePriceSummary: json.RawMessage(
				`{"minor":600000}`,
			),

			InventoryAvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			CatalogueRevision: 1,

			InventoryRevision: 1,

			Generation: 1,
		},
		{
			BusinessRef: "shop-business-13e",

			BranchRef: "shop-branch-13e",

			ProductRef: "shop-rice",

			VariantRef: "rice-50kg",

			Name: "Raw Rice 50kg",

			Brand: "Golden Grain",

			TaxonomyTerms: []string{
				"Rice and Grains",
			},

			SellUnits: []string{
				"Bag",
			},

			MeasureNames: []string{
				"50kg",
			},

			PackagingTerms: []string{
				"Bag",
			},

			SKUTerms: []string{
				"RICE50KG",
			},

			RetailEligible: true,

			WholesaleEligible: true,

			Active: true,

			EffectivePriceSummary: json.RawMessage(
				`{"minor":7500000}`,
			),

			InventoryAvailabilitySummary: json.RawMessage(
				`{"available":true}`,
			),

			CatalogueRevision: 1,

			InventoryRevision: 1,

			Generation: 1,
		},
	}

	for _, fixture := range shopFixtures {
		if _, err = shopProjector.Project(
			ctx,
			fixture,
		); err != nil {
			t.Fatalf(
				"SHOP fixture %s: %v",
				fixture.ProductRef,
				err,
			)
		}
	}

	// ========================================================
	// TYPO + ALIAS + MEASUREMENT HARD GATES
	// ========================================================

	assertFood := func(
		query string,
		expected string,
	) {
		response, searchErr :=
			foodEngine.Search(
				ctx,
				FoodEngineSearchRequest{
					Query: query,

					Generation: 1,

					PageSize: 10,

					Sort: SortRelevance,
				},
			)

		if searchErr != nil {
			t.Fatalf(
				"FOOD search %q: %v",
				query,
				searchErr,
			)
		}

		if len(response.Hits) == 0 ||
			response.Hits[0].FoodItemRef !=
				expected {
			t.Fatalf(
				"FOOD search %q hits=%+v",
				query,
				response.Hits,
			)
		}

		if response.Hits[0].Sponsored {
			t.Fatalf(
				"organic result marked sponsored: %+v",
				response.Hits[0],
			)
		}

		if response.Hits[0].PlacementType !=
			PlacementOrganic {
			t.Fatalf(
				"unexpected placement type: %+v",
				response.Hits[0],
			)
		}
	}

	assertFood(
		"jolof",
		"food-jollof",
	)

	assertFood(
		"shwarma",
		"food-shawarma",
	)

	assertFood(
		"moin moin",
		"food-moi-moi",
	)

	assertShop := func(
		query string,
		expected string,
	) {
		response, searchErr :=
			shopEngine.Search(
				ctx,
				ShopEngineSearchRequest{
					Query: query,

					Generation: 1,

					PageSize: 10,

					Sort: SortRelevance,
				},
			)

		if searchErr != nil {
			t.Fatalf(
				"SHOP search %q: %v",
				query,
				searchErr,
			)
		}

		if len(response.Hits) == 0 ||
			response.Hits[0].ProductRef !=
				expected {
			t.Fatalf(
				"SHOP search %q hits=%+v",
				query,
				response.Hits,
			)
		}
	}

	assertShop(
		"gari",
		"shop-garri",
	)

	assertShop(
		"paint beans",
		"shop-beans",
	)

	assertShop(
		"50kg rice",
		"shop-rice",
	)

	// ========================================================
	// PROMOTION CANNOT CREATE RELEVANCE
	// ========================================================

	response, err :=
		foodEngine.Search(
			ctx,
			FoodEngineSearchRequest{
				Query: "jollof",

				Generation: 1,

				PageSize: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	for _, hit := range response.Hits {
		if hit.FoodItemRef ==
			"food-promoted-burger" {
			t.Fatal(
				"irrelevant promoted result entered organic Jollof results",
			)
		}
	}

	// ========================================================
	// FACETS
	// ========================================================

	facetResponse, err :=
		shopEngine.Search(
			ctx,
			ShopEngineSearchRequest{
				Query: "beans",

				Generation: 1,

				PageSize: 10,

				Facets: []string{
					"BRAND",
					"MEASUREMENT",
					"AVAILABILITY",
				},
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(facetResponse.Facets) != 3 {
		t.Fatalf(
			"facet count=%d want=3",
			len(facetResponse.Facets),
		)
	}

	// ========================================================
	// SUGGESTIONS
	// ========================================================

	foodSuggestions, err :=
		foodEngine.Suggest(
			ctx,
			SuggestionRequest{
				Query: "jol",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(foodSuggestions) == 0 {
		t.Fatal(
			"FOOD suggestions empty",
		)
	}

	shopSuggestions, err :=
		shopEngine.Suggest(
			ctx,
			SuggestionRequest{
				Query: "gar",

				Generation: 1,

				Limit: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(shopSuggestions) == 0 {
		t.Fatal(
			"SHOP suggestions empty",
		)
	}

	// ========================================================
	// DISTANCE SAFETY
	// ========================================================

	_, err =
		foodEngine.Search(
			ctx,
			FoodEngineSearchRequest{
				Query: "rice",

				Generation: 1,

				Sort: SortDistance,
			},
		)

	if err !=
		ErrDistanceContextUnavailable {
		t.Fatalf(
			"distance safety err=%v",
			err,
		)
	}
}
