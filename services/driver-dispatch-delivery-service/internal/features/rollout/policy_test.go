package rollout

import "testing"

func TestSelectorIsServerOwnedAndRestrictive(t *testing.T) {
	selector := NewSelector("customer-internal", "VENDOR_COMMERCE", "partner:alpha", "express")
	tests := []struct {
		name string
		in   Context
		want bool
	}{
		{"selected customer and tier", Context{CustomerID: "customer-internal", ServiceTier: "EXPRESS"}, true},
		{"selected source and tier", Context{SourceType: "vendor_commerce", ServiceTier: "express"}, true},
		{"selected source ref and tier", Context{SourceRef: "PARTNER:ALPHA", ServiceTier: "express"}, true},
		{"wrong tier", Context{CustomerID: "customer-internal", ServiceTier: "standard"}, false},
		{"outside cohort", Context{CustomerID: "customer-public", ServiceTier: "express"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := selector.Selected(test.in); got != test.want {
				t.Fatalf("Selected()=%v want %v", got, test.want)
			}
		})
	}
}

func TestPolicyPinsRequirementsOnlyForSelectedCohort(t *testing.T) {
	policy := Policy{
		Selector:                  NewSelector("", "VENDOR_COMMERCE", "", ""),
		HandoverAvailable:         true,
		PickupEvidenceAvailable:   true,
		DeliveryEvidenceAvailable: true,
		ProviderTransferAvailable: true,
	}
	protected := policy.Protect(Context{SourceType: "VENDOR_COMMERCE"})
	if protected.Mode != ModeProtected || !protected.HandoverRequired || !protected.PickupEvidenceRequired || !protected.DeliveryEvidenceRequired || !protected.ProviderTransferRequired {
		t.Fatalf("selected work was not fully pinned: %+v", protected)
	}
	legacy := policy.Protect(Context{SourceType: "CUSTOMER"})
	if legacy.Mode != ModeLegacy || legacy.HandoverRequired {
		t.Fatalf("outside cohort must remain legacy: %+v", legacy)
	}
}

func TestPolicyFailsClosedWhenCapabilityUnavailable(t *testing.T) {
	policy := Policy{Selector: NewSelector("internal", "", "", ""), HandoverAvailable: false}
	result := policy.Protect(Context{CustomerID: "internal"})
	if result.Mode != ModeLegacy || result.HandoverRequired {
		t.Fatalf("disabled admission must not claim protected capability: %+v", result)
	}
}
