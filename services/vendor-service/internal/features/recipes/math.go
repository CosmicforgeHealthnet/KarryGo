package recipes

import (
	"math/big"
)

type rational struct {
	N int64
	D int64
}

func normalizeRational(n, d int64) (rational, error) {
	if n <= 0 || d <= 0 {
		return rational{}, ErrInvalidInput
	}

	bn := big.NewInt(n)
	bd := big.NewInt(d)

	g := new(big.Int)
	g.GCD(nil, nil, bn, bd)

	bn.Div(bn, g)
	bd.Div(bd, g)

	if !bn.IsInt64() || !bd.IsInt64() {
		return rational{}, ErrInvalidInput
	}

	return rational{
		N: bn.Int64(),
		D: bd.Int64(),
	}, nil
}

func multiplyRational(
	left rational,
	right rational,
) (rational, error) {
	if left.N <= 0 ||
		left.D <= 0 ||
		right.N <= 0 ||
		right.D <= 0 {
		return rational{}, ErrInvalidInput
	}

	n := new(big.Int).Mul(
		big.NewInt(left.N),
		big.NewInt(right.N),
	)

	d := new(big.Int).Mul(
		big.NewInt(left.D),
		big.NewInt(right.D),
	)

	g := new(big.Int)
	g.GCD(nil, nil, n, d)

	n.Div(n, g)
	d.Div(d, g)

	if !n.IsInt64() ||
		!d.IsInt64() {
		return rational{}, ErrInvalidInput
	}

	return rational{
		N: n.Int64(),
		D: d.Int64(),
	}, nil
}

func divideRational(
	left rational,
	right rational,
) (rational, error) {
	if right.N <= 0 {
		return rational{}, ErrInvalidInput
	}

	return multiplyRational(
		left,
		rational{
			N: right.D,
			D: right.N,
		},
	)
}

func applyExpectedWaste(
	value rational,
	wasteBPS int,
) (rational, error) {
	if wasteBPS < 0 ||
		wasteBPS >= 10000 {
		return rational{}, ErrInvalidInput
	}

	if wasteBPS == 0 {
		return value, nil
	}

	// If w% of input is expected to be lost, planned gross input required
	// for the same usable output is:
	//
	// usable * 10000 / (10000-w)
	return multiplyRational(
		value,
		rational{
			N: 10000,
			D: int64(10000 - wasteBPS),
		},
	)
}
