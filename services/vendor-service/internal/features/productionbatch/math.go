package productionbatch

import (
	"fmt"
	"errors"
	"math"
	"math/big"
)

var (
	ErrInvalidInput       = errors.New("production batch invalid input")
	ErrNotFound           = errors.New("production batch not found")
	ErrStateConflict      = errors.New("production batch state conflict")
	ErrVersionConflict    = errors.New("production batch version conflict")
	ErrInventoryRequired  = errors.New("production batch inventory mapping required")
	ErrInventoryShortage  = errors.New("production batch inventory shortage")
	ErrTopUpRequired      = errors.New("production batch additional inventory reservation required")
	ErrCorrectionRequired = errors.New("production batch inventory correction required")
	ErrCapacityConflict   = errors.New("production batch production job conflict")
	// ErrConsumptionRequired wraps ErrStateConflict so existing checks still match.
	ErrConsumptionRequired = fmt.Errorf("%w: actual consumption not recorded", ErrStateConflict)
)

type fraction struct {
	N int64
	D int64
}

func gcd64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func normalizeFraction(n, d int64, allowZero bool) (fraction, error) {
	if d <= 0 || (!allowZero && n <= 0) || (allowZero && n < 0) {
		return fraction{}, ErrInvalidInput
	}

	if n == 0 {
		return fraction{N: 0, D: 1}, nil
	}

	g := gcd64(n, d)
	return fraction{N: n / g, D: d / g}, nil
}

func fractionFromBig(n, d *big.Int, allowZero bool) (fraction, error) {
	if d.Sign() <= 0 ||
		(!allowZero && n.Sign() <= 0) ||
		(allowZero && n.Sign() < 0) {
		return fraction{}, ErrInvalidInput
	}

	if n.Sign() == 0 {
		return fraction{N: 0, D: 1}, nil
	}

	absN := new(big.Int).Abs(new(big.Int).Set(n))
	g := new(big.Int).GCD(nil, nil, absN, d)

	n.Quo(n, g)
	d.Quo(d, g)

	if !n.IsInt64() || !d.IsInt64() {
		return fraction{}, ErrInvalidInput
	}

	return fraction{N: n.Int64(), D: d.Int64()}, nil
}

func multiplyFraction(left, right fraction) (fraction, error) {
	n := new(big.Int).Mul(
		big.NewInt(left.N),
		big.NewInt(right.N),
	)

	d := new(big.Int).Mul(
		big.NewInt(left.D),
		big.NewInt(right.D),
	)

	return fractionFromBig(n, d, left.N == 0 || right.N == 0)
}

func divideFraction(left, right fraction) (fraction, error) {
	if right.N <= 0 {
		return fraction{}, ErrInvalidInput
	}

	n := new(big.Int).Mul(
		big.NewInt(left.N),
		big.NewInt(right.D),
	)

	d := new(big.Int).Mul(
		big.NewInt(left.D),
		big.NewInt(right.N),
	)

	return fractionFromBig(n, d, left.N == 0)
}

// Matches Section 16B:
// requirement * 10000 / (10000 - expectedWasteBPS).
func applyExpectedWaste(value fraction, wasteBPS int) (fraction, error) {
	if wasteBPS < 0 || wasteBPS >= 10000 {
		return fraction{}, ErrInvalidInput
	}

	if wasteBPS == 0 {
		return value, nil
	}

	return multiplyFraction(
		value,
		fraction{
			N: 10000,
			D: int64(10000 - wasteBPS),
		},
	)
}

func subtractFraction(left, right fraction) (fraction, error) {
	n := new(big.Int).Sub(
		new(big.Int).Mul(
			big.NewInt(left.N),
			big.NewInt(right.D),
		),
		new(big.Int).Mul(
			big.NewInt(right.N),
			big.NewInt(left.D),
		),
	)

	d := new(big.Int).Mul(
		big.NewInt(left.D),
		big.NewInt(right.D),
	)

	if d.Sign() <= 0 {
		return fraction{}, ErrInvalidInput
	}

	if n.Sign() == 0 {
		return fraction{N: 0, D: 1}, nil
	}

	abs := new(big.Int).Abs(new(big.Int).Set(n))
	g := new(big.Int).GCD(nil, nil, abs, d)

	n.Quo(n, g)
	d.Quo(d, g)

	if !n.IsInt64() || !d.IsInt64() {
		return fraction{}, ErrInvalidInput
	}

	return fraction{N: n.Int64(), D: d.Int64()}, nil
}

func toExactInt64(value fraction) (int64, error) {
	if value.D <= 0 || value.N < 0 {
		return 0, ErrInvalidInput
	}

	if value.N == 0 {
		return 0, nil
	}

	if value.N%value.D != 0 {
		return 0, ErrInventoryRequired
	}

	out := value.N / value.D
	if out < 0 || out > math.MaxInt64 {
		return 0, ErrInvalidInput
	}

	return out, nil
}
