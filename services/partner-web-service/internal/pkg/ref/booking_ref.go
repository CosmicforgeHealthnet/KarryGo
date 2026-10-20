// Package ref generates human-facing booking references using crypto/rand.
package ref

import (
	"crypto/rand"
	"math/big"
)

// charset excludes visually confusing characters (I, O, 0, 1).
const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateBookingRef returns a reference like "KGP-A2B3C4D5" (8 chars after the
// prefix), using crypto/rand — never math/rand.
func GenerateBookingRef() (string, error) {
	result := make([]byte, 8)

	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}

		result[i] = charset[n.Int64()]
	}

	return "KGP-" + string(result), nil
}
