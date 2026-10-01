// Package money is pricing-service's exact arithmetic.
//
// Every amount is a whole number of kobo held in an int64. There is no
// floating-point arithmetic anywhere in this package, and the architecture test
// forbids float64 from appearing in it or in the pricing engine. This is not
// fastidiousness: the legacy formulas this service replaces multiply money by
// float64 kilometres, so the same trip can price differently depending on how
// the distance happened to be represented. A quote that cannot be reproduced
// bit-for-bit cannot be reconciled against months later.
//
// Rates are therefore expressed as integers over an integer unit ("kobo per
// 1000 metres"), multipliers as exact rationals ("135/100"), and every rounding
// step names its own mode. Intermediate products are computed with math/big, so
// a large haulage job cannot silently overflow.
package money

import (
	"errors"
	"fmt"
	"math/big"
)

// ErrOverflow is returned when a result cannot be represented in an int64.
// Pricing refuses to produce a wrong number: an impossible amount is an error,
// never a wrapped one.
var ErrOverflow = errors.New("money: amount overflows int64")

// RoundingMode selects how a fractional kobo becomes a whole kobo. Each legacy
// formula used a different one, so a policy version must state which it means
// rather than inheriting a language default.
type RoundingMode string

const (
	// Truncate drops the fraction, rounding toward zero. Used by the legacy
	// haulage weight surcharge (integer division) and the legacy gateway fare.
	Truncate RoundingMode = "TRUNCATE"
	// HalfAwayFromZero rounds .5 away from zero, which is what Go's math.Round
	// does and therefore what the legacy dispatch and haulage formulas do.
	HalfAwayFromZero RoundingMode = "HALF_AWAY_FROM_ZERO"
	// HalfUp rounds .5 toward positive infinity. For non-negative money this
	// equals HalfAwayFromZero; they differ only for negative amounts, which is
	// why they are distinct modes rather than aliases.
	HalfUp RoundingMode = "HALF_UP"
	// Floor rounds toward negative infinity. Used for the platform fee, which
	// the wallet computes as amount*bps/10000 with integer division.
	Floor RoundingMode = "FLOOR"
)

// Valid reports whether m is a known mode. An unknown mode in a stored policy
// must fail loudly rather than fall back to some default.
func (m RoundingMode) Valid() bool {
	switch m {
	case Truncate, HalfAwayFromZero, HalfUp, Floor:
		return true
	}
	return false
}

// Rational is an exact fraction, used for multipliers such as an express
// surcharge (135/100) or a percentage (1/10). Den must not be zero.
type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

// Valid reports whether r can be applied.
func (r Rational) Valid() bool { return r.Den != 0 }

func (r Rational) String() string { return fmt.Sprintf("%d/%d", r.Num, r.Den) }

// Multiply returns amount * r, rounded with mode.
//
// The multiplication happens at full precision before rounding, so
// (amount * 135) / 100 is computed exactly and rounded once. Rounding at each
// step instead would drift.
func Multiply(amount int64, r Rational, mode RoundingMode) (int64, error) {
	if !r.Valid() {
		return 0, errors.New("money: rational has zero denominator")
	}
	if !mode.Valid() {
		return 0, fmt.Errorf("money: unknown rounding mode %q", mode)
	}
	num := new(big.Int).Mul(big.NewInt(amount), big.NewInt(r.Num))
	return divide(num, big.NewInt(r.Den), mode)
}

// PerUnit returns rate * quantity / unit, rounded with mode.
//
// This is how a distance or time component is priced: a per-kilometre rate is
// kobo per 1000 metres, and a per-minute rate is kobo per 60 seconds, so the
// quantity stays an exact integer of metres or seconds. Expressing it this way
// is what removes float kilometres from the calculation entirely.
func PerUnit(rate int64, quantity int64, unit int64, mode RoundingMode) (int64, error) {
	if unit == 0 {
		return 0, errors.New("money: unit must not be zero")
	}
	if !mode.Valid() {
		return 0, fmt.Errorf("money: unknown rounding mode %q", mode)
	}
	num := new(big.Int).Mul(big.NewInt(rate), big.NewInt(quantity))
	return divide(num, big.NewInt(unit), mode)
}

// BasisPoints returns amount * bps / 10000, rounded with mode. Commission and
// percentage surcharges are expressed in basis points so they stay integers.
func BasisPoints(amount int64, bps int64, mode RoundingMode) (int64, error) {
	return PerUnit(amount, bps, 10_000, mode)
}

// RoundToStep rounds amount to the nearest multiple of step using mode. Taxi
// rounds to 50 kobo and dispatch rounds to 50 kobo; haulage does not round at
// all, which a policy expresses as step 0 (or 1).
func RoundToStep(amount int64, step int64, mode RoundingMode) (int64, error) {
	if step <= 1 {
		return amount, nil // step 0 or 1 means "no rounding"
	}
	if !mode.Valid() {
		return 0, fmt.Errorf("money: unknown rounding mode %q", mode)
	}
	steps, err := PerUnit(amount, 1, step, mode)
	if err != nil {
		return 0, err
	}
	product := new(big.Int).Mul(big.NewInt(steps), big.NewInt(step))
	return toInt64(product)
}

// Sum adds amounts, refusing to overflow. Component sums must equal the total
// on every quote, so this is used wherever conservation is asserted.
func Sum(amounts ...int64) (int64, error) {
	total := new(big.Int)
	for _, amount := range amounts {
		total.Add(total, big.NewInt(amount))
	}
	return toInt64(total)
}

// AtLeast raises amount to floor, which is how a minimum fare is applied. The
// order matters and belongs to the policy: the legacy dispatch formula rounds
// first and then applies its minimum.
func AtLeast(amount int64, floor int64) int64 {
	if amount < floor {
		return floor
	}
	return amount
}

// divide returns num/den rounded with mode, exactly.
func divide(num, den *big.Int, mode RoundingMode) (int64, error) {
	if den.Sign() == 0 {
		return 0, errors.New("money: division by zero")
	}
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() == 0 {
		return toInt64(quotient)
	}

	// negative reports whether the exact result is below zero. QuoRem truncates
	// toward zero, so the remainder carries the sign of the numerator.
	negative := (num.Sign() < 0) != (den.Sign() < 0)

	switch mode {
	case Truncate:
		// Already truncated toward zero by QuoRem.
	case Floor:
		if negative {
			quotient.Sub(quotient, big.NewInt(1))
		}
	case HalfAwayFromZero, HalfUp:
		// Compare |remainder|*2 against |den| to decide whether the fraction
		// reaches one half, without ever leaving integer arithmetic.
		twiceRemainder := new(big.Int).Abs(remainder)
		twiceRemainder.Lsh(twiceRemainder, 1)
		absDen := new(big.Int).Abs(den)
		atLeastHalf := twiceRemainder.Cmp(absDen) >= 0

		if atLeastHalf {
			if negative {
				// HalfUp sends an exact half toward positive infinity, so a
				// negative exact half truncates instead of stepping away.
				exactlyHalf := twiceRemainder.Cmp(absDen) == 0
				if !(mode == HalfUp && exactlyHalf) {
					quotient.Sub(quotient, big.NewInt(1))
				}
			} else {
				quotient.Add(quotient, big.NewInt(1))
			}
		}
	default:
		return 0, fmt.Errorf("money: unknown rounding mode %q", mode)
	}
	return toInt64(quotient)
}

func toInt64(value *big.Int) (int64, error) {
	if !value.IsInt64() {
		return 0, fmt.Errorf("%w: %s", ErrOverflow, value.String())
	}
	return value.Int64(), nil
}
