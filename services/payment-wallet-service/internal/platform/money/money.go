// Package money implements the authoritative Money value object for the
// payment-wallet-service financial foundation.
//
// Money is always an integer amount of minor units plus an explicit ISO-style
// three-letter currency. Floating point is never used for money arithmetic and
// amounts in different currencies are never combined: this service refuses
// mixed-currency arithmetic rather than silently converting.
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

// ErrNegative is returned where a non-negative amount is required.
var ErrNegative = errors.New("money: amount must not be negative")

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

// NewPositive constructs a Money value that must be strictly positive.
func NewPositive(amountMinor int64, currency string) (Money, error) {
	m, err := New(amountMinor, currency)
	if err != nil {
		return Money{}, err
	}
	if m.AmountMinor <= 0 {
		return Money{}, fmt.Errorf("money: amount must be positive")
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
func (m Money) IsZero() bool { return m.AmountMinor == 0 }

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m.AmountMinor < 0 }

// String renders the value for logs and statements. It never rounds.
func (m Money) String() string {
	return fmt.Sprintf("%d %s", m.AmountMinor, m.Currency)
}

// ApplyBasisPoints returns the share of m described by bps (1 bps = 0.01%).
//
// Truncating integer division is deliberate: the remainder is kept by the
// caller's residual line so that the split still sums to the gross amount and
// the journal balances. Rounding each share independently would leak minor
// units and produce unbalanced journals.
func (m Money) ApplyBasisPoints(bps int64) (Money, error) {
	if bps < 0 {
		return Money{}, fmt.Errorf("money: basis points must not be negative")
	}
	if bps > 10000 {
		return Money{}, fmt.Errorf("money: basis points must not exceed 10000")
	}
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	return Money{AmountMinor: (m.AmountMinor * bps) / 10000, Currency: m.Currency}, nil
}
