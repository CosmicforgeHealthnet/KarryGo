package cost

// Exact money and quantity arithmetic.
//
// Money is integer minor units. Quantity is an exact rational N/D. Nothing
// here is float: a recipe that uses 1/3 of a 850-kobo ingredient must not
// depend on binary floating point to decide what that costs.
//
// ROUNDING RULE
//
// A component's cost is quantity x unit cost, which is generally not a whole
// number of minor units. The repository had no existing minor-unit rounding
// convention when Section 16D was written (only overflow-safe integer
// multiplication in purchasing), so this one is defined here and used
// everywhere in this package:
//
//	ROUND HALF AWAY FROM ZERO, applied EXACTLY ONCE, at the component boundary.
//
// Rounding once per component rather than per intermediate step is the part
// that matters. Rounding intermediates would make a total depend on the order
// components happened to be summed in, which would make a historical snapshot
// irreproducible - the opposite of what this section exists to provide.
// Component amounts are exact integers, so the snapshot total is their exact
// sum with no further rounding.

import (
	"errors"
	"math/big"
)

// ErrInvalidQuantity is returned for a non-positive denominator or a negative
// quantity, both of which mean the caller's data is wrong rather than merely
// uncostable.
var ErrInvalidQuantity = errors.New("cost: invalid quantity")

// Quantity is an exact rational, mirroring how Sections 16B and 16C persist
// component quantities as numerator/denominator pairs.
type Quantity struct {
	Numerator   int64
	Denominator int64
}

// NewQuantity validates a persisted numerator/denominator pair.
func NewQuantity(numerator, denominator int64) (Quantity, error) {
	if denominator <= 0 || numerator < 0 {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{Numerator: numerator, Denominator: denominator}, nil
}

// IsZero reports a quantity of exactly zero, which is a legitimate value (a
// component may be planned but not consumed) and is not the same as absent.
func (q Quantity) IsZero() bool { return q.Numerator == 0 }

// rat converts to an exact big.Rat. Using math/big rather than int64 is what
// makes the arithmetic overflow-safe: a large quantity multiplied by a large
// minor-unit cost can exceed int64 in the intermediate product even when the
// rounded result fits comfortably.
func (q Quantity) rat() *big.Rat {
	return new(big.Rat).SetFrac(
		big.NewInt(q.Numerator),
		big.NewInt(q.Denominator),
	)
}

// ApplyExpectedWaste grosses a NET quantity up to the gross quantity that must
// be issued for it, using Section 16C's rule:
//
//	gross = net * 10000 / (10000 - waste_bps)
//
// It is reproduced here rather than imported because Section 16C's copy is
// package-private; the identical result is asserted against a table of shared
// cases in the tests, so the two cannot drift silently.
//
// Callers must apply this exactly once. See the package doc.
func ApplyExpectedWaste(q Quantity, wasteBPS int) (Quantity, error) {
	if wasteBPS < 0 || wasteBPS >= 10000 {
		return Quantity{}, ErrInvalidQuantity
	}
	if wasteBPS == 0 {
		return q, nil
	}

	// Multiply the denominator rather than the numerator so the value stays
	// exact; reducing afterwards keeps the persisted pair small.
	numerator := new(big.Int).Mul(big.NewInt(q.Numerator), big.NewInt(10000))
	denominator := new(big.Int).Mul(
		big.NewInt(q.Denominator),
		big.NewInt(int64(10000-wasteBPS)),
	)

	rat := new(big.Rat).SetFrac(numerator, denominator)
	if !rat.Num().IsInt64() || !rat.Denom().IsInt64() {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{
		Numerator:   rat.Num().Int64(),
		Denominator: rat.Denom().Int64(),
	}, nil
}

// SubtractQuantity returns left-right, floored at zero. Used for the waste
// portion of a grossed-up quantity, where a negative result would mean the
// gross-up produced less than the net input and the arithmetic is wrong.
func SubtractQuantity(left, right Quantity) (Quantity, error) {
	difference := new(big.Rat).Sub(left.rat(), right.rat())
	if difference.Sign() < 0 {
		return Quantity{Numerator: 0, Denominator: 1}, nil
	}
	if !difference.Num().IsInt64() || !difference.Denom().IsInt64() {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{
		Numerator:   difference.Num().Int64(),
		Denominator: difference.Denom().Int64(),
	}, nil
}

// CostOf returns quantity x unitCostMinor in minor units, rounded half away
// from zero exactly once.
//
// unitCostMinor is the cost of ONE normalized inventory unit, which is what
// Section 15 stores in vendor_purchase_cost_history.inventory_unit_cost_minor,
// so no unit conversion happens here.
func CostOf(q Quantity, unitCostMinor int64) (int64, error) {
	if q.Denominator <= 0 || q.Numerator < 0 {
		return 0, ErrInvalidQuantity
	}
	if unitCostMinor < 0 {
		return 0, ErrInvalidQuantity
	}

	exact := new(big.Rat).Mul(q.rat(), new(big.Rat).SetInt64(unitCostMinor))
	return roundHalfAwayFromZero(exact), nil
}

// roundHalfAwayFromZero converts an exact rational to minor units.
//
// The classic 0.5 case is decided away from zero (2.5 -> 3), not to even, so
// the rule is stated in one sentence and produces the same answer on every
// machine and in every language a future reader might reimplement it in.
func roundHalfAwayFromZero(value *big.Rat) int64 {
	negative := value.Sign() < 0
	magnitude := new(big.Rat).Abs(value)

	numerator := magnitude.Num()
	denominator := magnitude.Denom()

	quotient, remainder := new(big.Int).QuoRem(
		numerator,
		denominator,
		new(big.Int),
	)

	// remainder*2 >= denominator  <=>  fractional part >= 0.5
	doubled := new(big.Int).Lsh(remainder, 1)
	if doubled.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}

	if !quotient.IsInt64() {
		// Saturate rather than silently wrap. A vendor cannot legitimately
		// reach 9.2e18 minor units, so this is a corrupt-input signal, and a
		// wrapped negative cost would be far worse than a clamped one.
		if negative {
			return -1 << 62
		}
		return 1 << 62
	}

	result := quotient.Int64()
	if negative {
		return -result
	}
	return result
}

// SumMinor adds component amounts with overflow safety. Components are already
// exact integers, so there is no rounding at this level.
func SumMinor(amounts ...int64) (int64, error) {
	total := big.NewInt(0)
	for _, amount := range amounts {
		total.Add(total, big.NewInt(amount))
	}
	if !total.IsInt64() {
		return 0, ErrInvalidQuantity
	}
	return total.Int64(), nil
}

// WeightedAverageMinor computes a quantity-weighted average unit cost in minor
// units over the supplied observations, rounded half away from zero once.
//
//	sum(quantity_i * unit_cost_i) / sum(quantity_i)
//
// Weighting is by accepted inventory quantity: a 100kg delivery must influence
// the average a hundred times more than a 1kg one. Returns ok=false when the
// total weight is zero, which the caller reports as no evidence rather than as
// a cost of zero.
func WeightedAverageMinor(quantities []int64, unitCosts []int64) (int64, bool) {
	if len(quantities) != len(unitCosts) || len(quantities) == 0 {
		return 0, false
	}

	weightedTotal := big.NewInt(0)
	totalWeight := big.NewInt(0)

	for i := range quantities {
		if quantities[i] <= 0 {
			continue
		}
		weightedTotal.Add(weightedTotal, new(big.Int).Mul(
			big.NewInt(quantities[i]),
			big.NewInt(unitCosts[i]),
		))
		totalWeight.Add(totalWeight, big.NewInt(quantities[i]))
	}

	if totalWeight.Sign() == 0 {
		return 0, false
	}

	return roundHalfAwayFromZero(new(big.Rat).SetFrac(weightedTotal, totalWeight)), true
}

// VarianceMinor returns actual-expected. Signed on purpose: a batch that came
// in under plan is as operationally interesting as one that overran, and
// clamping it at zero would hide exactly that.
func VarianceMinor(actualMinor, expectedMinor int64) int64 {
	difference := new(big.Int).Sub(big.NewInt(actualMinor), big.NewInt(expectedMinor))
	if !difference.IsInt64() {
		if difference.Sign() < 0 {
			return -1 << 62
		}
		return 1 << 62
	}
	return difference.Int64()
}

// PerUnitMinor divides a total across produced units, rounded half away from
// zero. ok=false when the output quantity is zero or absent, so a caller
// reports "not meaningful" instead of dividing by zero or showing the total as
// if it were a per-unit figure.
func PerUnitMinor(totalMinor int64, output Quantity) (int64, bool) {
	if output.Denominator <= 0 || output.Numerator <= 0 {
		return 0, false
	}
	exact := new(big.Rat).Quo(new(big.Rat).SetInt64(totalMinor), output.rat())
	return roundHalfAwayFromZero(exact), true
}
