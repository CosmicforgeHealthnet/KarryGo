package walletusecases

import "testing"

func TestReferralLedgerOwner(t *testing.T) {
	cases := []struct {
		ownerType, ownerID, providerType string
		wantType, wantID                 string
		ok                               bool
	}{
		{"customer", "c-1", "", "customer", "c-1", true},
		{"provider", "p-1", "truck_provider", "provider", "truck_provider:p-1", true},
		{"provider", "p-1", "taxi", "provider", "taxi:p-1", true},
		{"provider", "p-1", "vendor_business", "", "", false},
		{"provider", "p-1", "", "", "", false},
		{"platform", "karry-go", "", "", "", false},
		{"customer", " ", "", "", "", false},
	}
	for _, tc := range cases {
		gotType, gotID, err := referralLedgerOwner(tc.ownerType, tc.ownerID, tc.providerType)
		if (err == nil) != tc.ok || gotType != tc.wantType || gotID != tc.wantID {
			t.Errorf("referralLedgerOwner(%q, %q, %q) = %q, %q, %v", tc.ownerType, tc.ownerID, tc.providerType, gotType, gotID, err)
		}
	}
}

func TestReferralPostingInputValidates(t *testing.T) {
	if _, err := referralPostingInput(ReferralPostingRequest{OwnerType: "customer", OwnerID: "c", Balance: "cash", AmountKobo: 0}); err == nil {
		t.Fatal("expected validation errors")
	}
	input, err := referralPostingInput(ReferralPostingRequest{
		OwnerType: "provider", OwnerID: "p", ProviderType: "dispatch", Balance: "earnings",
		AmountKobo: 50_000, IdempotencyKey: "k", Reference: "rw_1",
	})
	if err != nil || input.OwnerID != "dispatch:p" || input.OwnerType != "provider" {
		t.Fatalf("input = %+v, err = %v", input, err)
	}
}
