package inventory

import (
	"context"
	"testing"
)

type section14GRecipeResolver struct {
	calls int

	result RecipeAvailabilityResult
	err    error
}

func (
	r *section14GRecipeResolver,
) ResolveRecipeAvailability(
	context.Context,
	RecipeAvailabilityRequest,
) (
	RecipeAvailabilityResult,
	error,
) {
	r.calls++

	return r.result,
		r.err
}

func TestSection14GManualFoodBypassesInventory(
	t *testing.T,
) {
	resolver :=
		&section14GRecipeResolver{
			result: RecipeAvailabilityResult{
				Support: FoodAvailabilityUnavailable,

				Sellable: false,
			},
		}

	service :=
		&StockService{
			recipeAvailability: resolver,
		}

	result, err :=
		service.ResolveFoodAvailability(
			context.Background(),
			FoodAvailabilityRequest{
				BusinessID: "business",

				BranchID: "branch",

				FoodItemRef: "food-item",

				Mode: FoodAvailabilityModeManual,
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if !result.Sellable {
		t.Fatal(
			"MANUAL Food item was disabled by inventory",
		)
	}

	if result.InventoryControlled {
		t.Fatal(
			"MANUAL Food item became inventory-controlled",
		)
	}

	if resolver.calls != 0 {
		t.Fatalf(
			"recipe resolver calls=%d want=0",
			resolver.calls,
		)
	}
}

func TestSection14GLinkedFoodWithoutRecipeAuthorityFailsClosed(
	t *testing.T,
) {
	service :=
		&StockService{}

	result, err :=
		service.ResolveFoodAvailability(
			context.Background(),
			FoodAvailabilityRequest{
				BusinessID: "business",

				BranchID: "branch",

				FoodItemRef: "food-item",

				Mode: FoodAvailabilityModeInventoryLinked,

				InventoryRecipeRef: "recipe-future-1",
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if result.Sellable {
		t.Fatal(
			"INVENTORY_LINKED Food item became sellable without recipe authority",
		)
	}

	if result.Support !=
		FoodAvailabilityNotSupported {

		t.Fatalf(
			"support=%s want=%s",
			result.Support,
			FoodAvailabilityNotSupported,
		)
	}
}

func TestSection14GLinkedFoodCanRecoverWhenFutureAuthoritySaysAvailable(
	t *testing.T,
) {
	resolver :=
		&section14GRecipeResolver{
			result: RecipeAvailabilityResult{
				Support: FoodAvailabilityAvailable,

				Sellable: true,

				InventoryRevision: 42,
			},
		}

	service :=
		&StockService{
			recipeAvailability: resolver,
		}

	result, err :=
		service.ResolveFoodAvailability(
			context.Background(),
			FoodAvailabilityRequest{
				BusinessID: "business",

				BranchID: "branch",

				FoodItemRef: "food-item",

				Mode: FoodAvailabilityModeInventoryLinked,

				InventoryRecipeRef: "recipe-future-1",
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if !result.Sellable {
		t.Fatal(
			"future authoritative availability could not recover Food sellability",
		)
	}

	if result.InventoryRevision != 42 {
		t.Fatalf(
			"revision=%d want=42",
			result.InventoryRevision,
		)
	}
}

func TestSection14GAvailabilityTransitionEvents(
	t *testing.T,
) {
	available :=
		true

	if event :=
		availabilityTransitionEvent(
			&available,
			false,
		); event !=
		EventInventoryItemOutOfStock {

		t.Fatalf(
			"stock-out event=%s",
			event,
		)
	}

	unavailable :=
		false

	if event :=
		availabilityTransitionEvent(
			&unavailable,
			true,
		); event !=
		EventInventoryItemAvailable {

		t.Fatalf(
			"recovery event=%s",
			event,
		)
	}

	if event :=
		availabilityTransitionEvent(
			&available,
			true,
		); event != "" {

		t.Fatalf(
			"unchanged availability emitted transition=%s",
			event,
		)
	}
}

func TestSection14GProjectionInvalidationEventVocabulary(
	t *testing.T,
) {
	if EventInventoryBalanceChanged !=
		"vendor.inventory.balance.changed" {

		t.Fatal(
			"balance invalidation event changed",
		)
	}

	if EventInventoryItemOutOfStock !=
		"vendor.inventory.item.out_of_stock" {

		t.Fatal(
			"stock-out event changed",
		)
	}

	if EventInventoryItemAvailable !=
		"vendor.inventory.item.available" {

		t.Fatal(
			"available event changed",
		)
	}
}
