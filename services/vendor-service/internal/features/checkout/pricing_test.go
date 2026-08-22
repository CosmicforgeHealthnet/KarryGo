package checkout

import (
	"errors"
	"math"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

func TestLineTotalsFixedLineHasNoAuthorizedHeadroom(t *testing.T) {
	t.Parallel()

	for _, basis := range []string{
		cart.PricingBasisFixedUnit,
		cart.PricingBasisFixedMeasure,
	} {
		quoted, maximum, err := LineTotals(1250, 3, basis, 500)

		if err != nil {
			t.Fatalf("%s: %v", basis, err)
		}

		if quoted != 3750 {
			t.Fatalf("%s: quoted=%d want=3750", basis, quoted)
		}

		// A fixed line cannot vary, so an overfill tolerance configured on
		// the sell unit must not create authorized headroom against it.
		if maximum != quoted {
			t.Fatalf("%s: maximum=%d want=%d", basis, maximum, quoted)
		}
	}
}

func TestLineTotalsVariableMeasureCarriesOverfillTolerance(t *testing.T) {
	t.Parallel()

	quoted, maximum, err := LineTotals(
		1000,
		2,
		cart.PricingBasisVariableMeasure,
		500,
	)
	if err != nil {
		t.Fatal(err)
	}

	if quoted != 2000 {
		t.Fatalf("quoted=%d want=2000", quoted)
	}

	if maximum != 2100 {
		t.Fatalf("maximum=%d want=2100", maximum)
	}
}

// A maximum authorized amount that rounds DOWN would under-state what the
// vendor may legitimately charge once the item is weighed. Rounding up is the
// only safe direction, and this pins it against a value that does not divide
// evenly.
func TestLineTotalsRoundsTheAuthorizedMaximumUp(t *testing.T) {
	t.Parallel()

	_, maximum, err := LineTotals(
		333,
		1,
		cart.PricingBasisVariableMeasure,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	// 333 * 10001 / 10000 = 333.0333, so the only defensible answer is 334.
	if maximum != 334 {
		t.Fatalf("maximum=%d want=334", maximum)
	}
}

func TestLineTotalsZeroToleranceLeavesTheQuoteAlone(t *testing.T) {
	t.Parallel()

	quoted, maximum, err := LineTotals(
		999,
		7,
		cart.PricingBasisVariableMeasure,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	if quoted != maximum {
		t.Fatalf("quoted=%d maximum=%d want equal", quoted, maximum)
	}
}

func TestLineTotalsRefusesOverflowRatherThanWrapping(t *testing.T) {
	t.Parallel()

	if _, _, err := LineTotals(
		math.MaxInt64/2,
		4,
		cart.PricingBasisFixedUnit,
		0,
	); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("err=%v want ErrMoneyOverflow", err)
	}

	if _, _, err := LineTotals(
		math.MaxInt64/2,
		1,
		cart.PricingBasisVariableMeasure,
		1000,
	); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("overfill err=%v want ErrMoneyOverflow", err)
	}
}

// A tolerance outside the Shop schema's own 0-10000 bps range did not come
// from Shop. Refusing beats clamping: clamping would silently produce a
// maximum nobody configured.
func TestLineTotalsRefusesToleranceOutsideTheShopSchemaRange(t *testing.T) {
	t.Parallel()

	if _, _, err := LineTotals(
		100,
		1,
		cart.PricingBasisVariableMeasure,
		10001,
	); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("err=%v want ErrMoneyOverflow", err)
	}
}

func TestSumTotalsRefusesOverflow(t *testing.T) {
	t.Parallel()

	total, err := SumTotals(1, 2, 3)

	if err != nil || total != 6 {
		t.Fatalf("total=%d err=%v", total, err)
	}

	if _, err = SumTotals(math.MaxInt64, 1); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("err=%v want ErrMoneyOverflow", err)
	}
}

func TestReservationQuantityBySellUnitIsTheOrderedCount(t *testing.T) {
	t.Parallel()

	quantity, err := ReservationQuantity(BasisSellUnit, 4, 0)

	if err != nil || quantity != 4 {
		t.Fatalf("quantity=%d err=%v", quantity, err)
	}
}

// A variable-fulfilment line reserves in the stock measure, once per ordered
// unit. That is the same basis a later actual fulfilment measurement is
// compared against, which is why it may not be the ordered count.
func TestReservationQuantityByStockMeasureMultipliesTheExpectedMeasure(t *testing.T) {
	t.Parallel()

	quantity, err := ReservationQuantity(BasisStockMeasure, 3, 500)

	if err != nil || quantity != 1500 {
		t.Fatalf("quantity=%d err=%v", quantity, err)
	}
}

func TestReservationQuantityRefusesAnIncoherentBasis(t *testing.T) {
	t.Parallel()

	none, err := ReservationQuantity(BasisNone, 5, 5)

	if err != nil || none != 0 {
		t.Fatalf("none=%d err=%v", none, err)
	}

	if _, err = ReservationQuantity(BasisStockMeasure, 3, 0); err == nil {
		t.Fatal("a stock-measure line with no expected measure was accepted")
	}

	if _, err = ReservationQuantity(BasisSellUnit, 0, 0); err == nil {
		t.Fatal("a sell-unit line with no quantity was accepted")
	}

	if _, err = ReservationQuantity(
		ReservationBasis("SOMETHING_ELSE"),
		1,
		1,
	); err == nil {
		t.Fatal("an unknown reservation basis was accepted")
	}
}
