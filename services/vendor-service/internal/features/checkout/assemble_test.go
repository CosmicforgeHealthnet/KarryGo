package checkout

import (
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

func shopLineOutcome(
	business string,
	branch string,
	quoted int64,
	requiresStock bool,
) lineOutcome {
	return lineOutcome{
		line: Line{
			ID:         newID("chkl_"),
			Domain:     cart.DomainShop,
			BusinessID: business,
			BranchID:   branch,
			Quantity:   1,

			Currency:                "NGN",
			QuotedTotalMinor:        quoted,
			MaxAuthorizedTotalMinor: quoted,

			Status:     LineValid,
			ReasonCode: ReasonSellable,

			ReservationBasis: BasisNone,
		},
		requiresStock: requiresStock,
	}
}

func TestAssembleGroupsByVendorBranchInAStableOrder(t *testing.T) {
	t.Parallel()

	service := &Service{}

	groups, failure := service.assemble(
		cart.Cart{Domain: cart.DomainShop},
		[]lineOutcome{
			shopLineOutcome("vendor-b", "branch-9", 300, true),
			shopLineOutcome("vendor-a", "branch-2", 200, true),
			shopLineOutcome("vendor-a", "branch-1", 100, true),
			shopLineOutcome("vendor-a", "branch-1", 50, true),
		},
	)

	if failure != "" {
		t.Fatalf("failure=%q want none", failure)
	}

	if len(groups) != 3 {
		t.Fatalf("groups=%d want=3", len(groups))
	}

	expected := [][2]string{
		{"vendor-a", "branch-1"},
		{"vendor-a", "branch-2"},
		{"vendor-b", "branch-9"},
	}

	for index, want := range expected {
		if groups[index].BusinessID != want[0] ||
			groups[index].BranchID != want[1] {
			t.Fatalf("group %d = %s/%s want %s/%s",
				index,
				groups[index].BusinessID,
				groups[index].BranchID,
				want[0],
				want[1],
			)
		}
	}

	if len(groups[0].Lines) != 2 {
		t.Fatalf("first group lines=%d want=2", len(groups[0].Lines))
	}

	if groups[0].QuotedTotalMinor != 150 {
		t.Fatalf("first group quoted=%d want=150", groups[0].QuotedTotalMinor)
	}

	for _, group := range groups {
		if group.ReservationStatus != ReservationPending {
			t.Fatalf("group %s reservation=%s want PENDING",
				group.ID, group.ReservationStatus)
		}

		// The idempotency key a crash has to be recoverable from must exist
		// before anything is reserved, and it must be per group.
		if group.ReservationSourceRef != group.ID {
			t.Fatalf("group %s source ref=%q", group.ID, group.ReservationSourceRef)
		}
	}
}

func TestAssembleRefusesAMixedCurrencyCheckout(t *testing.T) {
	t.Parallel()

	service := &Service{}

	second := shopLineOutcome("vendor-a", "branch-1", 100, true)
	second.line.Currency = "USD"

	groups, failure := service.assemble(
		cart.Cart{Domain: cart.DomainShop},
		[]lineOutcome{
			shopLineOutcome("vendor-a", "branch-1", 100, true),
			second,
		},
	)

	if failure != FailureCurrencyMismatch {
		t.Fatalf("failure=%q want=%q", failure, FailureCurrencyMismatch)
	}

	// Nothing may be reserved for a candidate that will not be validated.
	for _, group := range groups {
		if group.RequiresStockReservation ||
			group.ReservationStatus != ReservationNotRequired ||
			group.ReservationSourceRef != "" {
			t.Fatalf("failed candidate still claims a hold: %+v", group)
		}
	}
}

func TestAssembleRefusesAFoodCartThatSpansTwoBranches(t *testing.T) {
	t.Parallel()

	service := &Service{}

	first := shopLineOutcome("vendor-a", "branch-1", 100, false)
	first.line.Domain = cart.DomainFood

	second := shopLineOutcome("vendor-a", "branch-2", 100, false)
	second.line.Domain = cart.DomainFood

	_, failure := service.assemble(
		cart.Cart{Domain: cart.DomainFood},
		[]lineOutcome{first, second},
	)

	if failure != FailureCartGroupingViolation {
		t.Fatalf("failure=%q want=%q", failure, FailureCartGroupingViolation)
	}
}

func TestAssembleFailsWhenAnyLineIsBlocking(t *testing.T) {
	t.Parallel()

	service := &Service{}

	blocked := shopLineOutcome("vendor-a", "branch-1", 0, false)
	blocked.line.Status = LineUnavailable
	blocked.line.ReasonCode = "INSUFFICIENT_STOCK"
	blocked.line.Currency = ""

	_, failure := service.assemble(
		cart.Cart{Domain: cart.DomainShop},
		[]lineOutcome{
			shopLineOutcome("vendor-a", "branch-1", 100, true),
			blocked,
		},
	)

	if failure != FailureLineNotSellable {
		t.Fatalf("failure=%q want=%q", failure, FailureLineNotSellable)
	}
}

// A CHANGED line is not a refusal. Section 17B reports the difference and
// still holds the stock; Section 17D decides whether the customer accepts it.
func TestAssembleAcceptsAChangedPriceAndStillReserves(t *testing.T) {
	t.Parallel()

	service := &Service{}

	changed := shopLineOutcome("vendor-a", "branch-1", 120, true)
	changed.line.Status = LineChanged
	changed.line.ReasonCode = ReasonPriceChanged
	changed.line.PriceChanged = true

	groups, failure := service.assemble(
		cart.Cart{Domain: cart.DomainShop},
		[]lineOutcome{changed},
	)

	if failure != "" {
		t.Fatalf("failure=%q want none", failure)
	}

	if groups[0].ReservationStatus != ReservationPending {
		t.Fatalf("reservation=%s want PENDING", groups[0].ReservationStatus)
	}
}

func TestBuildLineOutcomeRefusesAShopLineWithNoInventoryIdentity(t *testing.T) {
	t.Parallel()

	outcome, err := buildLineOutcome(
		cart.CartItem{
			ID:           "citem-1",
			Domain:       cart.DomainShop,
			BusinessID:   "vendor-a",
			BranchID:     "branch-1",
			ItemRef:      "product-1",
			Quantity:     1,
			PricingBasis: cart.PricingBasisFixedUnit,
		},
		LineDecision{
			Sellable:         true,
			BusinessID:       "vendor-a",
			BranchID:         "branch-1",
			UnitPriceMinor:   500,
			Currency:         "NGN",
			ReservationBasis: BasisSellUnit,
			// No inventory identity, which must fail closed rather than be
			// treated as unlimited stock.
			InventoryItemRef:    "",
			ReservationQuantity: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.line.Status != LineInvalid ||
		outcome.line.ReasonCode != ReasonInventoryIdentityMissing {
		t.Fatalf("status=%s reason=%s", outcome.line.Status, outcome.line.ReasonCode)
	}

	if outcome.requiresStock {
		t.Fatal("a line with no inventory identity still asked for a hold")
	}
}

func TestBuildLineOutcomeRefusesAShopLineWithNoReservationBasis(t *testing.T) {
	t.Parallel()

	outcome, err := buildLineOutcome(
		cart.CartItem{
			ID:           "citem-1",
			Domain:       cart.DomainShop,
			BusinessID:   "vendor-a",
			BranchID:     "branch-1",
			ItemRef:      "product-1",
			Quantity:     1,
			PricingBasis: cart.PricingBasisFixedUnit,
		},
		LineDecision{
			Sellable:       true,
			BusinessID:     "vendor-a",
			BranchID:       "branch-1",
			UnitPriceMinor: 500,
			Currency:       "NGN",
			// The Shop authority could not establish a basis, which is the
			// EXACT_CONVERSION / fractional-sell-unit case.
			ReservationBasis: BasisNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.line.Status != LineInvalid ||
		outcome.line.ReasonCode != ReasonReservationBasisUnknown {
		t.Fatalf("status=%s reason=%s", outcome.line.Status, outcome.line.ReasonCode)
	}
}

// Food reserves nothing in Section 17B, and that is not a defect in the line.
func TestBuildLineOutcomeAcceptsAFoodLineWithoutAnyReservation(t *testing.T) {
	t.Parallel()

	outcome, err := buildLineOutcome(
		cart.CartItem{
			ID:                "citem-1",
			Domain:            cart.DomainFood,
			BusinessID:        "vendor-a",
			BranchID:          "branch-1",
			ItemRef:           "item-1",
			MenuRef:           "menu-1",
			Quantity:          2,
			UnitPriceMinor:    750,
			PriceReference:    "price-v1",
			CatalogueRevision: 4,
			PricingBasis:      cart.PricingBasisFixedUnit,
		},
		LineDecision{
			Sellable:          true,
			BusinessID:        "vendor-a",
			BranchID:          "branch-1",
			UnitPriceMinor:    750,
			Currency:          "NGN",
			PriceReference:    "price-v1",
			CatalogueRevision: 4,
			ReservationBasis:  BasisNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.line.Status != LineValid {
		t.Fatalf("status=%s reason=%s", outcome.line.Status, outcome.line.ReasonCode)
	}

	if outcome.requiresStock {
		t.Fatal("a Food line asked for a stock hold")
	}

	if outcome.line.QuotedTotalMinor != 1500 ||
		outcome.line.MaxAuthorizedTotalMinor != 1500 {
		t.Fatalf("quoted=%d maximum=%d",
			outcome.line.QuotedTotalMinor,
			outcome.line.MaxAuthorizedTotalMinor,
		)
	}
}

// The cart's own grouping evidence is not authority. A listing that moved
// branch must not silently re-group the checkout under the new one.
func TestBuildLineOutcomeRefusesALineThatMovedBranch(t *testing.T) {
	t.Parallel()

	outcome, err := buildLineOutcome(
		cart.CartItem{
			ID:           "citem-1",
			Domain:       cart.DomainShop,
			BusinessID:   "vendor-a",
			BranchID:     "branch-1",
			ItemRef:      "product-1",
			Quantity:     1,
			PricingBasis: cart.PricingBasisFixedUnit,
		},
		LineDecision{
			Sellable:   true,
			BusinessID: "vendor-a",
			BranchID:   "branch-2",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.line.Status != LineInvalid ||
		outcome.line.ReasonCode != ReasonGroupingViolation {
		t.Fatalf("status=%s reason=%s", outcome.line.Status, outcome.line.ReasonCode)
	}
}

func TestBuildLineOutcomeReportsAPriceChangeWithoutRepricingSilently(t *testing.T) {
	t.Parallel()

	outcome, err := buildLineOutcome(
		cart.CartItem{
			ID:                "citem-1",
			Domain:            cart.DomainFood,
			BusinessID:        "vendor-a",
			BranchID:          "branch-1",
			ItemRef:           "item-1",
			MenuRef:           "menu-1",
			Quantity:          1,
			UnitPriceMinor:    500,
			PriceReference:    "price-v1",
			CatalogueRevision: 3,
			PricingBasis:      cart.PricingBasisFixedUnit,
		},
		LineDecision{
			Sellable:          true,
			BusinessID:        "vendor-a",
			BranchID:          "branch-1",
			UnitPriceMinor:    650,
			Currency:          "NGN",
			PriceReference:    "price-v2",
			CatalogueRevision: 4,
			ReservationBasis:  BasisNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if outcome.line.Status != LineChanged ||
		outcome.line.ReasonCode != ReasonPriceChanged {
		t.Fatalf("status=%s reason=%s", outcome.line.Status, outcome.line.ReasonCode)
	}

	// Both the old and the new figure survive, because the customer has to be
	// shown what actually changed.
	if outcome.line.SnapshotUnitPriceMinor != 500 ||
		outcome.line.UnitPriceMinor != 650 {
		t.Fatalf("snapshot=%d current=%d",
			outcome.line.SnapshotUnitPriceMinor,
			outcome.line.UnitPriceMinor,
		)
	}

	if !outcome.line.PriceChanged || !outcome.line.CatalogueChanged {
		t.Fatalf("change flags price=%v catalogue=%v",
			outcome.line.PriceChanged,
			outcome.line.CatalogueChanged,
		)
	}
}

func TestNewServiceRefusesAnUnconfiguredOrAbsurdTTL(t *testing.T) {
	t.Parallel()

	store := NewStore(nil)

	for name, ttl := range map[string]time.Duration{
		"unset":     0,
		"negative":  -time.Minute,
		"too short": MinReservationTTL - time.Second,
		"too long":  MaxReservationTTL + time.Second,
	} {
		if _, err := NewService(Options{
			Store:          store,
			ReservationTTL: ttl,
		}); err == nil {
			t.Fatalf("%s TTL was accepted", name)
		} else if !errors.Is(err, ErrDependencyMissing) &&
			!errors.Is(err, ErrInvalidTTL) {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}
