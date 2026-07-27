// Package money implements the platform Money value object.
//
// Money is always an integer amount of minor units plus an explicit ISO-style
// three-letter currency. Floating point is never used for money arithmetic,
// and amounts in different currencies cannot be combined or compared without
// explicit conversion (which this service intentionally does not implement).
package money

import (
	"errors"
	"fmt"
	"regexp"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ErrCurrencyMismatch is returned when two Money values with different
// currencies are combined or compared.
var ErrCurrencyMismatch = errors.New("money: currency mismatch")

// Money is an immutable amount of a single currency in minor units.
type Money struct {
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// New validates and constructs a Money value.
func New(amountMinor int64, currency string) (Money, error) {
	m := Money{AmountMinor: amountMinor, Currency: currency}
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	return m, nil
}

// Validate checks the currency format.
func (m Money) Validate() error {
	if !currencyPattern.MatchString(m.Currency) {
		return fmt.Errorf("money: currency %q must be a three-letter uppercase code", m.Currency)
	}
	return nil
}

// Add returns m + other, failing on currency mismatch.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{AmountMinor: m.AmountMinor + other.AmountMinor, Currency: m.Currency}, nil
}

// Sub returns m - other, failing on currency mismatch.
func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{AmountMinor: m.AmountMinor - other.AmountMinor, Currency: m.Currency}, nil
}

// Compare returns -1, 0 or 1, failing on currency mismatch.
func (m Money) Compare(other Money) (int, error) {
	if m.Currency != other.Currency {
		return 0, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	switch {
	case m.AmountMinor < other.AmountMinor:
		return -1, nil
	case m.AmountMinor > other.AmountMinor:
		return 1, nil
	default:
		return 0, nil
	}
}

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool {
	return m.AmountMinor == 0
}
