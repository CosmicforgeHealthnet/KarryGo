package cart

import (
	"errors"
	"testing"
)

func TestNormalizeSubstitutionPolicy(t *testing.T) {
	cases := []struct {
		domain Domain
		value  string
		want   string
		err    error
	}{
		{DomainShop, "", SubstitutionExactOnly, nil},
		{DomainShop, " contact_first ", SubstitutionContactFirst, nil},
		{DomainShop, "AUTO_REFUND", SubstitutionAutoRefund, nil},
		{DomainShop, "VENDOR_SUGGEST_AND_APPROVE", SubstitutionVendorSuggestAndApprove, nil},
		// Rules that need parameters the cart cannot yet carry are refused
		// rather than stored without them.
		{DomainShop, "SAME_BRAND", "", ErrInvalidSubstitutionPolicy},
		{DomainShop, "SIMILAR_WITHIN_PRICE_TOLERANCE", "", ErrInvalidSubstitutionPolicy},
		{DomainShop, "SPECIFIC_REPLACEMENT", "", ErrInvalidSubstitutionPolicy},
		{DomainShop, "ANYTHING", "", ErrInvalidSubstitutionPolicy},
		// Prepared food is never substituted.
		{DomainFood, "", SubstitutionExactOnly, nil},
		{DomainFood, "EXACT_ONLY", SubstitutionExactOnly, nil},
		{DomainFood, "CONTACT_FIRST", "", ErrInvalidSubstitutionPolicy},
	}
	for _, tc := range cases {
		got, err := NormalizeSubstitutionPolicy(tc.domain, tc.value)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("%s %q: got %q/%v, want %q/%v", tc.domain, tc.value, got, err, tc.want, tc.err)
		}
	}
}
