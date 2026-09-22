package vendors

import (
	"testing"
)

func TestCapabilityCatalogueIsExact(t *testing.T) {
	want := []CapabilityType{CapabilityRestaurant, CapabilityFastFood, CapabilityCloudKitchen, CapabilityPastryBakery, CapabilityShopGrocery, CapabilityMarket, CapabilityWholesale, CapabilityCatering, CapabilityManufacturer}
	if len(SupportedCapabilityTypes) != len(want) {
		t.Fatalf("got %d capability types, want %d", len(SupportedCapabilityTypes), len(want))
	}
	for i := range want {
		if SupportedCapabilityTypes[i] != want[i] {
			t.Fatalf("capability[%d]=%q want %q", i, SupportedCapabilityTypes[i], want[i])
		}
	}
	for _, forbidden := range []CapabilityType{"PHARMACY", "DELIVERY", "ECOMMERCE"} {
		if IsSupportedCapability(forbidden) {
			t.Fatalf("unexpected capability %q", forbidden)
		}
	}
}

func TestEffectiveCapabilityRequiresEveryParentActive(t *testing.T) {
	if ok, reason := EffectiveCapabilityState(BusinessActive, CapabilityActive, BranchActive, BranchCapabilityActive); !ok || reason != "ACTIVE" {
		t.Fatalf("active chain = %v %q", ok, reason)
	}
	tests := []struct {
		b      BusinessStatus
		c      CapabilityStatus
		br     BranchStatus
		bc     BranchCapabilityStatus
		reason string
	}{
		{BusinessPaused, CapabilityActive, BranchActive, BranchCapabilityActive, "BUSINESS_NOT_ACTIVE"},
		{BusinessActive, CapabilityPaused, BranchActive, BranchCapabilityActive, "BUSINESS_CAPABILITY_NOT_ACTIVE"},
		{BusinessActive, CapabilityActive, BranchPaused, BranchCapabilityActive, "BRANCH_NOT_ACTIVE"},
		{BusinessActive, CapabilityActive, BranchActive, BranchCapabilityPaused, "BRANCH_CAPABILITY_NOT_ACTIVE"},
	}
	for _, tt := range tests {
		if ok, reason := EffectiveCapabilityState(tt.b, tt.c, tt.br, tt.bc); ok || reason != tt.reason {
			t.Errorf("got %v %q, want false %q", ok, reason, tt.reason)
		}
	}
}

func TestVendorTransitionsCannotSelfActivateDrafts(t *testing.T) {
	if vendorBusinessTransitionAllowed(BusinessDraft, BusinessActive) {
		t.Fatal("vendor must not activate a draft business")
	}
	if capabilityTransitionAllowed(CapabilityDraft, CapabilityActive) {
		t.Fatal("vendor must not activate a draft capability")
	}
	if branchTransitionAllowed(BranchDraft, BranchActive) {
		t.Fatal("vendor must not activate a draft branch")
	}
	if !vendorBusinessTransitionAllowed(BusinessActive, BusinessPaused) || !vendorBusinessTransitionAllowed(BusinessPaused, BusinessActive) {
		t.Fatal("active/paused operational transition should be supported")
	}
}

func TestRequiredHybridCapabilityCombinationsRemainValid(t *testing.T) {
	combinations := [][2]CapabilityType{{CapabilityCloudKitchen, CapabilityPastryBakery}, {CapabilityShopGrocery, CapabilityRestaurant}, {CapabilityShopGrocery, CapabilityPastryBakery}}
	for _, pair := range combinations {
		if !IsSupportedCapability(pair[0]) || !IsSupportedCapability(pair[1]) || pair[0] == pair[1] {
			t.Errorf("hybrid pair is not independently configurable: %v", pair)
		}
	}
	if CapabilityFamily(CapabilityCloudKitchen) != "FOOD" || CapabilityFamily(CapabilityShopGrocery) != "SHOP" {
		t.Fatal("hybrid capability families changed")
	}
}

func TestHoursRejectOvernightAndOverlap(t *testing.T) {
	if validateHours([]OperatingWindow{{Weekday: 1, OpensAt: "22:00", ClosesAt: "02:00"}}) == nil {
		t.Fatal("expected overnight interval rejection")
	}
	if validateHours([]OperatingWindow{{Weekday: 1, OpensAt: "09:00", ClosesAt: "12:00"}, {Weekday: 1, OpensAt: "11:00", ClosesAt: "13:00"}}) == nil {
		t.Fatal("expected overlap rejection")
	}
	if err := validateHours([]OperatingWindow{{Weekday: 1, OpensAt: "09:00", ClosesAt: "12:00"}, {Weekday: 1, OpensAt: "13:00", ClosesAt: "17:00"}, {Weekday: 2, IsClosed: true}}); err != nil {
		t.Fatalf("valid hours: %v", err)
	}
}

func TestMediaReferencesAreOpaque(t *testing.T) {
	if validateBrand(Brand{DisplayName: "Brand", MediaRefs: []MediaReference{{Kind: "LOGO", MediaRef: "https://cdn.example/logo.png"}}}) == nil {
		t.Fatal("expected URL rejection")
	}
	if err := validateBrand(Brand{DisplayName: "Brand", MediaRefs: []MediaReference{{Kind: "LOGO", MediaRef: "media/logo_123"}}}); err != nil {
		t.Fatalf("opaque ref rejected: %v", err)
	}
}
