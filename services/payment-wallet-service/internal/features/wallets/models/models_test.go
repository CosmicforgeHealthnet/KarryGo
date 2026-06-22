package walletmodels

import "testing"

func TestCalculatePlatformFee(t *testing.T) {
	tests := []struct {
		name       string
		amountKobo int64
		bps        int64
		want       int64
	}{
		{name: "15 percent of 1000 NGN", amountKobo: 100000, bps: 1500, want: 15000},
		{name: "zero amount", amountKobo: 0, bps: 1500, want: 0},
		{name: "negative amount", amountKobo: -100, bps: 1500, want: 0},
		{name: "zero bps", amountKobo: 100000, bps: 0, want: 0},
		{name: "negative bps", amountKobo: 100000, bps: -10, want: 0},
		{name: "rounds down", amountKobo: 333, bps: 1500, want: 49},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculatePlatformFee(tt.amountKobo, tt.bps); got != tt.want {
				t.Fatalf("CalculatePlatformFee(%d, %d) = %d, want %d", tt.amountKobo, tt.bps, got, tt.want)
			}
		})
	}
}

func TestDefaultCurrency(t *testing.T) {
	if got := DefaultCurrency(""); got != CurrencyNGN {
		t.Fatalf("DefaultCurrency(\"\") = %q, want %q", got, CurrencyNGN)
	}
	if got := DefaultCurrency("USD"); got != "USD" {
		t.Fatalf("DefaultCurrency(\"USD\") = %q, want USD", got)
	}
}

func TestValidateBalanced(t *testing.T) {
	t.Run("balanced single currency passes", func(t *testing.T) {
		entries := []LedgerEntry{
			{Side: SideDebit, AmountKobo: 1000, Currency: CurrencyNGN},
			{Side: SideCredit, AmountKobo: 1000, Currency: CurrencyNGN},
		}
		if err := ValidateBalanced(entries); err != nil {
			t.Fatalf("expected balanced, got error: %v", err)
		}
	})

	t.Run("unbalanced fails", func(t *testing.T) {
		entries := []LedgerEntry{
			{Side: SideDebit, AmountKobo: 1000, Currency: CurrencyNGN},
			{Side: SideCredit, AmountKobo: 900, Currency: CurrencyNGN},
		}
		if err := ValidateBalanced(entries); err == nil {
			t.Fatal("expected unbalanced error, got nil")
		}
	})

	t.Run("fewer than two entries fails", func(t *testing.T) {
		entries := []LedgerEntry{{Side: SideDebit, AmountKobo: 1000, Currency: CurrencyNGN}}
		if err := ValidateBalanced(entries); err == nil {
			t.Fatal("expected error for single entry, got nil")
		}
	})

	t.Run("non-positive amount fails", func(t *testing.T) {
		entries := []LedgerEntry{
			{Side: SideDebit, AmountKobo: 0, Currency: CurrencyNGN},
			{Side: SideCredit, AmountKobo: 1000, Currency: CurrencyNGN},
		}
		if err := ValidateBalanced(entries); err == nil {
			t.Fatal("expected error for zero amount, got nil")
		}
	})

	t.Run("invalid side fails", func(t *testing.T) {
		entries := []LedgerEntry{
			{Side: "sideways", AmountKobo: 1000, Currency: CurrencyNGN},
			{Side: SideCredit, AmountKobo: 1000, Currency: CurrencyNGN},
		}
		if err := ValidateBalanced(entries); err == nil {
			t.Fatal("expected error for invalid side, got nil")
		}
	})

	t.Run("must balance per currency", func(t *testing.T) {
		// NGN balances, USD does not.
		entries := []LedgerEntry{
			{Side: SideDebit, AmountKobo: 1000, Currency: CurrencyNGN},
			{Side: SideCredit, AmountKobo: 1000, Currency: CurrencyNGN},
			{Side: SideDebit, AmountKobo: 500, Currency: "USD"},
			{Side: SideCredit, AmountKobo: 400, Currency: "USD"},
		}
		if err := ValidateBalanced(entries); err == nil {
			t.Fatal("expected per-currency imbalance error, got nil")
		}
	})

	t.Run("empty currency defaults to NGN and balances", func(t *testing.T) {
		entries := []LedgerEntry{
			{Side: SideDebit, AmountKobo: 1000},
			{Side: SideCredit, AmountKobo: 1000, Currency: CurrencyNGN},
		}
		if err := ValidateBalanced(entries); err != nil {
			t.Fatalf("expected empty currency to default to NGN, got error: %v", err)
		}
	})
}
