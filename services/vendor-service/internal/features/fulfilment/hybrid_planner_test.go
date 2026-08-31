package fulfilment

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
)

func TestPlannerHybridFallbackKeepsPlatformQuoteAndTrigger(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true
	p := evaluatorPolicyForPlanner(delivery.ModeHybrid)
	p.MaxDeliveryRadiusMeters = intPtrPlanner(1000)
	h.vendorOwn.policies["bizA|brA"] = p
	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusPlanned || len(plan.Missions) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Missions[0].Authority != AuthorityPlatform || plan.Missions[0].FallbackTrigger != string(delivery.TriggerDistance) {
		t.Fatalf("mission=%+v", plan.Missions[0])
	}
	if plan.Missions[0].CustomerPriceGuaranteeMinor != 150000 {
		t.Fatalf("price guarantee=%d", plan.Missions[0].CustomerPriceGuaranteeMinor)
	}
}

func TestPlannerVendorModeDoesNotSilentlyFallback(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true
	p := evaluatorPolicyForPlanner(delivery.ModeVendor)
	p.RiderAvailability = delivery.RiderUnavailable
	h.vendorOwn.policies["bizA|brA"] = p
	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusUnserviceable || len(plan.Missions) != 0 {
		t.Fatalf("vendor mode unexpectedly fell back: %+v", plan)
	}
}

func evaluatorPolicyForPlanner(mode delivery.DeliveryMode) delivery.Policy {
	return delivery.Policy{Mode: mode, FeeRule: delivery.FeeFlat, Enabled: true, Currency: "NGN", BaseFeeMinor: 1000, OperatingTimezone: "Africa/Lagos", RiderAvailability: delivery.RiderAvailable, Version: 5}
}

func intPtrPlanner(v int) *int { return &v }
