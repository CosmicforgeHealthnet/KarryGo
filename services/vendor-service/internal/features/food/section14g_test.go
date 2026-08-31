package food

import (
	"context"
	"testing"
)

type section14GInventoryAuthority struct {
	calls int

	decision InventoryAvailabilityDecision
	err      error
}

func (
	a *section14GInventoryAuthority,
) ResolveFoodAvailability(
	context.Context,
	InventoryAvailabilityRequest,
) (
	InventoryAvailabilityDecision,
	error,
) {
	a.calls++

	return a.decision,
		a.err
}

func TestSection14GFoodManualDoesNotConsultInventory(
	t *testing.T,
) {
	authority :=
		&section14GInventoryAuthority{
			decision: InventoryAvailabilityDecision{
				Support: InventorySupportUnavailable,

				Sellable: false,
			},
		}

	service :=
		&Service{
			inventory: authority,
		}

	decision :=
		service.resolveInventoryAvailability(
			context.Background(),
			Item{
				ID: "food-item",

				BusinessID: "business",

				BranchID: "branch",
			},
			AvailabilitySource{
				Mode: SourceManual,
			},
		)

	if !decision.Sellable {
		t.Fatal(
			"MANUAL Food item was disabled by Inventory",
		)
	}

	if authority.calls != 0 {
		t.Fatalf(
			"inventory calls=%d want=0",
			authority.calls,
		)
	}
}

func TestSection14GFoodLinkedWithoutAuthorityFailsClosed(
	t *testing.T,
) {
	service :=
		&Service{}

	recipe :=
		"future-recipe"

	decision :=
		service.resolveInventoryAvailability(
			context.Background(),
			Item{
				ID: "food-item",

				BusinessID: "business",

				BranchID: "branch",
			},
			AvailabilitySource{
				Mode: SourceInventoryLinked,

				InventoryRecipeRef: &recipe,
			},
		)

	if decision.Sellable {
		t.Fatal(
			"linked Food item became sellable without Inventory authority",
		)
	}

	if decision.Support !=
		InventorySupportNotSupported {

		t.Fatalf(
			"support=%s",
			decision.Support,
		)
	}
}

func TestSection14GFoodLinkedMissingRecipeRefFailsClosed(
	t *testing.T,
) {
	service :=
		&Service{}

	decision :=
		service.resolveInventoryAvailability(
			context.Background(),
			Item{
				ID: "food-item",

				BusinessID: "business",

				BranchID: "branch",
			},
			AvailabilitySource{
				Mode: SourceInventoryLinked,
			},
		)

	if decision.Sellable {
		t.Fatal(
			"linked Food item with no recipe ref became sellable",
		)
	}

	if decision.Support !=
		InventorySupportNotConfigured {

		t.Fatalf(
			"support=%s",
			decision.Support,
		)
	}
}

func TestSection14GFoodInventoryRecovery(
	t *testing.T,
) {
	authority :=
		&section14GInventoryAuthority{
			decision: InventoryAvailabilityDecision{
				InventoryControlled: true,

				Support: InventorySupportAvailable,

				Sellable: true,

				InventoryRevision: 99,
			},
		}

	service :=
		&Service{
			inventory: authority,
		}

	recipe :=
		"future-recipe"

	decision :=
		service.resolveInventoryAvailability(
			context.Background(),
			Item{
				ID: "food-item",

				BusinessID: "business",

				BranchID: "branch",
			},
			AvailabilitySource{
				Mode: SourceInventoryLinked,

				InventoryRecipeRef: &recipe,
			},
		)

	if !decision.Sellable {
		t.Fatal(
			"Food sellability did not recover when authoritative Inventory became available",
		)
	}

	if decision.InventoryRevision != 99 {
		t.Fatalf(
			"inventory revision=%d want=99",
			decision.InventoryRevision,
		)
	}
}
