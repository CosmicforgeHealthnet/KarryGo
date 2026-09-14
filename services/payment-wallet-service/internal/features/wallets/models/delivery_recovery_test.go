package walletmodels_test

import (
	"testing"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

func TestSplitIsExactForCleanProportions(t *testing.T) {
	split, err := walletmodels.SplitDeliveryRecovery(1500, 3500, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if split.ProviderMinor != 300 || split.OrganizationMinor != 700 {
		t.Fatalf("split = %+v, want 300/700", split)
	}
}

// WAL-029 requirement 9. A salaried rider received nothing, so there is nothing
// to claw back from them. Recovering from a provider for money they never had
// is a real loss to a real person.
func TestASalariedProviderGivesBackNothing(t *testing.T) {
	split, err := walletmodels.SplitDeliveryRecovery(0, 5000, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if split.ProviderMinor != 0 {
		t.Fatalf("recovered %d from a provider who received nothing", split.ProviderMinor)
	}
	if split.OrganizationMinor != 4000 {
		t.Fatalf("organization gave back %d, want 4000", split.OrganizationMinor)
	}
}

// An independent-shaped split: the provider received everything.
func TestAProviderWhoReceivedEverythingGivesBackEverything(t *testing.T) {
	split, err := walletmodels.SplitDeliveryRecovery(5000, 0, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if split.ProviderMinor != 1234 || split.OrganizationMinor != 0 {
		t.Fatalf("split = %+v, want 1234/0", split)
	}
}

// Requirement 7: never create or destroy money through rounding. Exhaustive
// over a range that includes every fractional remainder these proportions can
// produce.
func TestRoundingNeverCreatesOrDestroysMoney(t *testing.T) {
	for _, received := range [][2]int64{
		{1500, 3500}, {1, 2}, {7, 11}, {1, 9999}, {3333, 3334}, {2, 2}, {999983, 17},
	} {
		provider, organization := received[0], received[1]
		total := provider + organization
		for recover := int64(0); recover <= total && recover <= 4000; recover++ {
			split, err := walletmodels.SplitDeliveryRecovery(provider, organization, recover)
			if err != nil {
				t.Fatalf("received %d/%d recover %d: %v", provider, organization, recover, err)
			}
			if split.TotalMinor() != recover {
				t.Fatalf("received %d/%d recover %d: split totals %d",
					provider, organization, recover, split.TotalMinor())
			}
			if split.ProviderMinor > provider || split.OrganizationMinor > organization {
				t.Fatalf("received %d/%d recover %d: split %+v exceeds what a party received",
					provider, organization, recover, split)
			}
			if split.ProviderMinor < 0 || split.OrganizationMinor < 0 {
				t.Fatalf("received %d/%d recover %d: negative share %+v", provider, organization, recover, split)
			}
		}
	}
}

// Requirement 14 in miniature: recovering the pool in pieces takes back exactly
// the pool, never a unit more.
func TestRepeatedPartialRecoveriesNeverExceedThePool(t *testing.T) {
	const provider, organization = 1500, 3500
	remainingProvider, remainingOrganization := int64(provider), int64(organization)
	var recovered int64
	for step := 0; step < 50 && remainingProvider+remainingOrganization > 0; step++ {
		want := int64(137)
		if want > remainingProvider+remainingOrganization {
			want = remainingProvider + remainingOrganization
		}
		split, err := walletmodels.SplitDeliveryRecovery(remainingProvider, remainingOrganization, want)
		if err != nil {
			t.Fatal(err)
		}
		remainingProvider -= split.ProviderMinor
		remainingOrganization -= split.OrganizationMinor
		recovered += split.TotalMinor()
		if remainingProvider < 0 || remainingOrganization < 0 {
			t.Fatalf("step %d drove a remaining balance negative: %d/%d", step, remainingProvider, remainingOrganization)
		}
	}
	if recovered != provider+organization {
		t.Fatalf("recovered %d in total, want exactly %d", recovered, provider+organization)
	}
}

// A replay must recover the same amounts. An unstable tie-break would make a
// retried refund take back a different amount than the first attempt.
func TestTheSplitIsDeterministic(t *testing.T) {
	first, err := walletmodels.SplitDeliveryRecovery(2500, 2500, 999)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		again, err := walletmodels.SplitDeliveryRecovery(2500, 2500, 999)
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("attempt %d produced %+v, first produced %+v", attempt, again, first)
		}
	}
}

func TestRecoveringMoreThanWasPaidIsRefused(t *testing.T) {
	if _, err := walletmodels.SplitDeliveryRecovery(1500, 3500, 5001); err == nil {
		t.Fatal("recovering more than was paid out was accepted")
	}
	if _, err := walletmodels.SplitDeliveryRecovery(1500, 3500, -1); err == nil {
		t.Fatal("a negative recovery was accepted")
	}
	if _, err := walletmodels.SplitDeliveryRecovery(-1, 3500, 10); err == nil {
		t.Fatal("a negative receipt was accepted")
	}
}

func TestRecoveringNothingFromNothingIsFine(t *testing.T) {
	split, err := walletmodels.SplitDeliveryRecovery(0, 0, 0)
	if err != nil || split.TotalMinor() != 0 {
		t.Fatalf("split=%+v err=%v", split, err)
	}
	if _, err := walletmodels.SplitDeliveryRecovery(0, 0, 1); err == nil {
		t.Fatal("recovering from an empty pool was accepted")
	}
}
