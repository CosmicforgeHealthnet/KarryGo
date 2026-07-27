package money

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewValidatesCurrency(t *testing.T) {
	if _, err := New(700000, "NGN"); err != nil {
		t.Fatalf("valid money rejected: %v", err)
	}
	for _, currency := range []string{"", "ngn", "NGNX", "N1N", "12"} {
		if _, err := New(100, currency); err == nil {
			t.Fatalf("currency %q should be rejected", currency)
		}
	}
}

func TestJSONShape(t *testing.T) {
	m, _ := New(700000, "NGN")
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"amountMinor":700000,"currency":"NGN"}` {
		t.Fatalf("unexpected JSON: %s", raw)
	}

	var decoded Money
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != m {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}

func TestArithmeticRejectsCurrencyMismatch(t *testing.T) {
	ngn, _ := New(1000, "NGN")
	usd, _ := New(1000, "USD")

	if _, err := ngn.Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add should fail with ErrCurrencyMismatch, got %v", err)
	}
	if _, err := ngn.Sub(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Sub should fail with ErrCurrencyMismatch, got %v", err)
	}
	if _, err := ngn.Compare(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Compare should fail with ErrCurrencyMismatch, got %v", err)
	}
}

func TestArithmeticSameCurrency(t *testing.T) {
	a, _ := New(1500, "NGN")
	b, _ := New(500, "NGN")

	sum, err := a.Add(b)
	if err != nil || sum.AmountMinor != 2000 {
		t.Fatalf("Add = %+v, %v", sum, err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.AmountMinor != 1000 {
		t.Fatalf("Sub = %+v, %v", diff, err)
	}
	cmp, err := a.Compare(b)
	if err != nil || cmp != 1 {
		t.Fatalf("Compare = %d, %v", cmp, err)
	}
}
