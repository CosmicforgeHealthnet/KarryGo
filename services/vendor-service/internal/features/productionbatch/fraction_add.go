package productionbatch

import "math/big"

func addFraction(left, right fraction) (fraction, error) {
	n := new(big.Int).Add(
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

	return fractionFromBig(n, d, true)
}
