package checkout

import (
	"errors"
	"math"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

// Money arithmetic for the checkout candidate.
//
// Two figures are produced per line, and both are preparation for Section
// 17D's quote rather than a quote themselves:
//
//   - quoted_total_minor is what the line costs if it is fulfilled exactly as
//     ordered.
//   - max_authorized_total_minor is the most the line could legitimately cost
//     once a variable-weight item is actually weighed, given the sell unit's
//     own overfill tolerance.
//
// For every fixed line the two are equal, because there is nothing to vary.
// Nothing here rounds down: a maximum that under-states the possible amount
// would be worse than useless to the section that has to authorize against it.

// ErrMoneyOverflow means a total could not be represented. Refusing is the
// only honest answer; a wrapped total would be a wrong price.
var ErrMoneyOverflow = errors.New(
	"commerce checkout monetary total overflowed",
)

const bpsScale = int64(10000)

func mulChecked(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}

	if a < 0 || b < 0 {
		return 0, ErrMoneyOverflow
	}

	if a > math.MaxInt64/b {
		return 0, ErrMoneyOverflow
	}

	return a * b, nil
}

func addChecked(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, ErrMoneyOverflow
	}

	if a > math.MaxInt64-b {
		return 0, ErrMoneyOverflow
	}

	return a + b, nil
}

// applyOverfillCeiling grows an amount by an overfill tolerance in basis
// points, rounding up.
func applyOverfillCeiling(
	amountMinor int64,
	overfillBps int,
) (int64, error) {
	if amountMinor < 0 {
		return 0, ErrMoneyOverflow
	}

	if overfillBps <= 0 {
		return amountMinor, nil
	}

	if overfillBps > int(bpsScale) {
		// A tolerance above 100% is not a tolerance. The Shop schema already
		// caps it at 10000 bps; refuse rather than silently clamp, because a
		// value outside the schema means the input did not come from Shop.
		return 0, ErrMoneyOverflow
	}

	scaled, err := mulChecked(
		amountMinor,
		bpsScale+int64(overfillBps),
	)
	if err != nil {
		return 0, err
	}

	// Ceiling division: the maximum authorized amount must never be less than
	// the real maximum.
	return (scaled + bpsScale - 1) / bpsScale, nil
}

// LineTotals computes one line's quoted and maximum authorized totals.
//
// A variable-measure line is quoted on the expected measure the customer
// chose, and its maximum carries the sell unit's overfill tolerance. Under
// tolerance is deliberately not applied anywhere: it can only reduce the
// final amount, and a smaller quote would be a promise Section 17B has no
// authority to make.
func LineTotals(
	unitPriceMinor int64,
	quantity int64,
	pricingBasis string,
	overfillBps int,
) (int64, int64, error) {
	if unitPriceMinor < 0 || quantity < 0 {
		return 0, 0, ErrMoneyOverflow
	}

	quoted, err := mulChecked(unitPriceMinor, quantity)

	if err != nil {
		return 0, 0, err
	}

	if pricingBasis != cart.PricingBasisVariableMeasure {
		return quoted, quoted, nil
	}

	maxAuthorized, err := applyOverfillCeiling(
		quoted,
		overfillBps,
	)
	if err != nil {
		return 0, 0, err
	}

	return quoted, maxAuthorized, nil
}

// SumTotals adds line totals with an explicit overflow refusal.
func SumTotals(values ...int64) (int64, error) {
	total := int64(0)

	for _, value := range values {
		next, err := addChecked(total, value)

		if err != nil {
			return 0, err
		}

		total = next
	}

	return total, nil
}

// ReservationQuantity computes the Section 14 reservation quantity for one
// line from its basis.
//
// A SELL_UNIT line reserves the ordered count. A STOCK_MEASURE line reserves
// the expected measure for every ordered unit, which is exactly the basis a
// later actual fulfilment measurement is compared against. There is no third
// case: a basis Section 17B cannot establish never reaches this function.
func ReservationQuantity(
	basis ReservationBasis,
	quantity int64,
	expectedMeasureQuantity int64,
) (int64, error) {
	switch basis {
	case BasisNone:
		return 0, nil

	case BasisSellUnit:
		if quantity <= 0 {
			return 0, ErrMoneyOverflow
		}

		return quantity, nil

	case BasisStockMeasure:
		if quantity <= 0 || expectedMeasureQuantity <= 0 {
			return 0, ErrMoneyOverflow
		}

		return mulChecked(
			expectedMeasureQuantity,
			quantity,
		)

	default:
		return 0, ErrMoneyOverflow
	}
}
