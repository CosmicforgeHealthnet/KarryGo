package commercemission

import (
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/rollout"
)

func TestCommerceBookingProtectionUsesServerOwnedVendorCommerceCohort(t *testing.T) {
	repository := NewRepository(nil).WithRolloutPolicy(rollout.Policy{
		Selector:                  rollout.NewSelector("", "VENDOR_COMMERCE", "", "express"),
		HandoverAvailable:         true,
		PickupEvidenceAvailable:   true,
		DeliveryEvidenceAvailable: true,
		ProviderTransferAvailable: true,
		Version:                   "commerce_rollout_test_v1",
	})

	protected := repository.protection("customer-a", "mission-a", "express")
	if protected.Mode != rollout.ModeProtected || !protected.HandoverRequired ||
		!protected.PickupEvidenceRequired || !protected.DeliveryEvidenceRequired ||
		!protected.ProviderTransferRequired {
		t.Fatalf("expected complete protected snapshot, got %+v", protected)
	}
	if protected.PolicyVersion != "commerce_rollout_test_v1" {
		t.Fatalf("unexpected policy version %q", protected.PolicyVersion)
	}

	standard := repository.protection("customer-a", "mission-a", "standard")
	if standard.Mode != rollout.ModeLegacy || standard.HandoverRequired {
		t.Fatalf("service-tier cohort must remain server-owned, got %+v", standard)
	}
}

func TestCommerceBookingProtectionDefaultsToLegacy(t *testing.T) {
	protection := NewRepository(nil).protection("customer-a", "mission-a", "express")
	if protection.Mode != rollout.ModeLegacy || protection.HandoverRequired {
		t.Fatalf("unconfigured rollout must preserve legacy behavior, got %+v", protection)
	}
}
