package fulfilment

import (
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
)

func TestReplanBeforeCustodyPreservesIdentityAndPrice(t *testing.T) {
	plan := Plan{Status: StatusPlanned, PlanRevision: 2, Version: 4, Missions: []Mission{{
		ID: "mission-1", Authority: AuthorityVendorOwn, DeliveryMode: "HYBRID",
		CustomerPriceGuaranteeMinor: 1500, GroupIDs: []string{"group-1"},
	}}, Groups: []PlanGroup{{Authority: AuthorityVendorOwn, MissionID: "mission-1"}}}
	platform := Mission{ID: "new-platform-id", Authority: AuthorityPlatform, Quote: QuoteEvidence{QuoteID: "dispatch-q1"}, Currency: "NGN"}
	got, err := ReplanBeforeCustody(FallbackInput{Plan: plan, PlatformMission: platform, Trigger: "DISTANCE_OVERFLOW", IdempotencyKey: "fallback-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Missions[0].ID != "mission-1" || got.Missions[0].CustomerPriceGuaranteeMinor != 1500 || got.Missions[0].FallbackTrigger != "DISTANCE_OVERFLOW" {
		t.Fatalf("identity/price not preserved: %+v", got.Missions[0])
	}
	if got.Groups[0].Authority != AuthorityPlatform {
		t.Fatalf("group owner=%s", got.Groups[0].Authority)
	}
	retry, err := ReplanBeforeCustody(FallbackInput{Plan: got, PlatformMission: platform, Trigger: "DISTANCE_OVERFLOW", IdempotencyKey: "fallback-1"})
	if err != nil || retry.PlanRevision != got.PlanRevision || retry.Missions[0].ID != got.Missions[0].ID {
		t.Fatalf("retry was not idempotent: %+v err=%v", retry, err)
	}
}

func TestHybridPlatformFundingPreservesGuaranteeAndAttributesGap(t *testing.T) {
	base := planned{deliveryMode: string(delivery.ModeHybrid), fallbackTrigger: string(delivery.TriggerDistance), priceGuaranteeMinor: 1500}
	tests := []struct {
		name     string
		sponsor  delivery.FallbackFundingSponsor
		shareBPS int
		vendor   int64
		platform int64
	}{
		{name: "vendor", sponsor: delivery.FallbackFundingVendor, shareBPS: 10000, vendor: 800},
		{name: "platform", sponsor: delivery.FallbackFundingPlatform, platform: 800},
		{name: "combined", sponsor: delivery.FallbackFundingCombined, shareBPS: 3750, vendor: 300, platform: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base
			input.hybridFallbackFunding, input.hybridVendorShareBPS = tt.sponsor, tt.shareBPS
			got := hybridPlatformFunding(input, 2300)
			if got.GrossMinor != 2300 || got.CustomerMinor != 1500 || got.VendorFundedMinor != tt.vendor || got.PlatformFundedMinor != tt.platform || !got.Balanced() {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestPostCustodyFallbackRefusesSilentSwitch(t *testing.T) {
	if err := PostCustodyFallback(CustodyVendor); err == nil {
		t.Fatal("expected transfer/return requirement")
	}
}
