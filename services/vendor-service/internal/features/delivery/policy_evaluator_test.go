package delivery

import (
	"testing"
	"time"
)

func evaluatorPolicy(mode DeliveryMode) Policy {
	return Policy{
		ID: "p1", Mode: mode, FeeRule: FeeFlat, Enabled: true,
		Currency: "NGN", BaseFeeMinor: 1500, Version: 4,
		OperatingTimezone: "Africa/Lagos", RiderAvailability: RiderAvailable,
		MaxDeliveryRadiusMeters: intPtr(10000),
	}
}

func intPtr(v int) *int { return &v }

func evalRequest(policy Policy) EvaluationRequest {
	return EvaluationRequest{
		Policy: policy, Currency: "NGN", DistanceMeters: 3000,
		OrderSubtotalMinor: 10000, PlatformEligible: true,
		VendorRiderAvailability: RiderAvailable, PackageCount: 1,
		Now:                      time.Date(2026, time.January, 12, 10, 0, 0, 0, time.UTC),
		CustomerDeliveryFeeMinor: 1500, BeforeCustody: true,
	}
}

func TestEvaluatePlatformNeverSelectsVendor(t *testing.T) {
	req := evalRequest(evaluatorPolicy(ModePlatform))
	got := Evaluate(req)
	if got.Owner != OwnerPlatform || got.ReasonCode != ReasonModePlatform {
		t.Fatalf("got %+v", got)
	}
}

func TestEvaluateVendorNeverFallsBack(t *testing.T) {
	policy := evaluatorPolicy(ModeVendor)
	policy.RiderAvailability = RiderUnavailable
	req := evalRequest(policy)
	req.VendorRiderAvailability = ""
	got := Evaluate(req)
	if got.Owner != OwnerNone || got.Eligible || got.FallbackTrigger != "" {
		t.Fatalf("vendor must fail closed, got %+v", got)
	}
}

func TestEvaluateHybridOverflowTriggers(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Policy, *EvaluationRequest)
		want   OverflowTrigger
	}{
		{"distance", func(p *Policy, r *EvaluationRequest) { p.MaxDeliveryRadiusMeters = intPtr(1000) }, TriggerDistance},
		{"time", func(p *Policy, r *EvaluationRequest) {
			p.OperatingPeriods = []OperatingPeriod{{Weekday: 1, StartMinute: 600, EndMinute: 660}}
		}, TriggerTime},
		{"capacity", func(p *Policy, r *EvaluationRequest) { p.MaxPackageCount = int64Ptr(1); r.PackageCount = 2 }, TriggerCapacity},
		{"vehicle", func(p *Policy, r *EvaluationRequest) {
			p.SupportedVehicleClasses = []string{"VAN"}
			r.RequiredVehicleClass = "MOTORCYCLE"
		}, TriggerVehicle},
		{"sla", func(p *Policy, r *EvaluationRequest) { r.VendorEstimateMinutes = 60; r.PromisedMaxMinutes = 30 }, TriggerSLA},
		{"rider", func(p *Policy, r *EvaluationRequest) { r.VendorRiderAvailability = RiderUnavailable }, TriggerOwnRiderFirst},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := evaluatorPolicy(ModeHybrid)
			r := evalRequest(p)
			tc.mutate(&p, &r)
			r.Policy = p
			got := Evaluate(r)
			if got.Owner != OwnerPlatform || got.FallbackTrigger != tc.want || !got.Eligible {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
			if got.PriceGuarantee.CustomerFeeMinor != 1500 {
				t.Fatalf("price guarantee changed: %+v", got.PriceGuarantee)
			}
		})
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestEvaluateDistanceTiersHaveIntegerBoundaries(t *testing.T) {
	p := evaluatorPolicy(ModeVendor)
	p.FeeRule = FeeDistanceTier
	p.DistanceTiers = []DistanceTier{{MinDistanceMeters: 0, MaxDistanceMeters: 3000, FeeMinor: 1000}, {MinDistanceMeters: 3000, MaxDistanceMeters: 7000, FeeMinor: 2000}, {MinDistanceMeters: 7000, MaxDistanceMeters: 0, FeeMinor: 3000}}
	for _, tc := range []struct{ distance, fee int64 }{{2999, 1000}, {3000, 2000}, {6999, 2000}, {7000, 3000}} {
		r := evalRequest(p)
		r.DistanceMeters = tc.distance
		got := Evaluate(r)
		if got.FeeMinor != tc.fee || got.Owner != OwnerVendor {
			t.Fatalf("distance %d got %+v want fee %d", tc.distance, got, tc.fee)
		}
	}
}

func TestEvaluateOperatingHoursUseIANAOvernightWindow(t *testing.T) {
	p := evaluatorPolicy(ModeHybrid)
	p.OperatingTimezone = "America/New_York"
	p.OperatingPeriods = []OperatingPeriod{{Weekday: 1, StartMinute: 22 * 60, EndMinute: 2 * 60}}
	// Monday 22:30 New York, represented in UTC.
	r := evalRequest(p)
	r.Now = time.Date(2026, time.January, 13, 3, 30, 0, 0, time.UTC)
	if got := Evaluate(r); got.Owner != OwnerVendor {
		t.Fatalf("overnight period should be open, got %+v", got)
	}
	r.Now = time.Date(2026, time.January, 13, 6, 30, 0, 0, time.UTC)
	if got := Evaluate(r); got.Owner != OwnerVendor {
		t.Fatalf("after-midnight half of overnight period should be open, got %+v", got)
	}
	// Monday 03:30 New York is outside the window.
	r.Now = time.Date(2026, time.January, 12, 8, 30, 0, 0, time.UTC)
	if got := Evaluate(r); got.Owner != OwnerPlatform || got.FallbackTrigger != TriggerTime {
		t.Fatalf("outside overnight period should fall back, got %+v", got)
	}
}

func TestEvaluateOperatingHoursAcrossDSTTransitions(t *testing.T) {
	p := evaluatorPolicy(ModeVendor)
	p.OperatingTimezone = "America/New_York"
	p.OperatingPeriods = []OperatingPeriod{{Weekday: 7, StartMinute: 60, EndMinute: 4 * 60}}

	// 2026-03-08 07:30 UTC is 03:30 EDT after the spring-forward transition.
	r := evalRequest(p)
	r.Now = time.Date(2026, time.March, 8, 7, 30, 0, 0, time.UTC)
	if got := Evaluate(r); got.Owner != OwnerVendor {
		t.Fatalf("spring-forward local time should remain eligible, got %+v", got)
	}
	// 2026-11-01 06:30 UTC is 01:30 EST after the fall-back transition.
	r.Now = time.Date(2026, time.November, 1, 6, 30, 0, 0, time.UTC)
	if got := Evaluate(r); got.Owner != OwnerVendor {
		t.Fatalf("fall-back local time should remain eligible, got %+v", got)
	}
}

func TestPolicyValidationRejectsTierGapAndUnknownTrigger(t *testing.T) {
	p := evaluatorPolicy(ModeHybrid)
	p.DistanceTiers = []DistanceTier{{MinDistanceMeters: 0, MaxDistanceMeters: 1000, FeeMinor: 1}, {MinDistanceMeters: 1100, MaxDistanceMeters: 0, FeeMinor: 2}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected distance-tier gap rejection")
	}
	p.DistanceTiers = nil
	p.FallbackTriggers = []OverflowTrigger{"NOT_A_TRIGGER"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected unknown fallback trigger rejection")
	}
}

func TestExecutionTransitionRequiresExplicitCustody(t *testing.T) {
	if err := Transition(ExecutionReadyForPickup, ExecutionCustodyTransferred); err != nil {
		t.Fatal(err)
	}
	if err := Transition(ExecutionPlanned, ExecutionDelivered); err == nil {
		t.Fatal("delivery must not bypass custody evidence")
	}
}

func TestEvaluateZoneFeeUsesPriorityDeterministically(t *testing.T) {
	p := evaluatorPolicy(ModeVendor)
	p.FeeRule = FeeZoneBased
	p.ServiceZones = []ServiceZone{
		{ID: "outer", Enabled: true, MinDistanceMeters: 0, MaxDistanceMeters: 10000, FeeMinor: 2000, Priority: 1},
		{ID: "inner", Enabled: true, MinDistanceMeters: 0, MaxDistanceMeters: 10000, FeeMinor: 2500, Priority: 2},
	}
	r := evalRequest(p)
	r.DestinationZoneID = "inner"
	got := Evaluate(r)
	if got.Owner != OwnerVendor || got.FeeMinor != 2500 {
		t.Fatalf("got %+v", got)
	}
}

func TestEvaluateFreeThresholdKeepsGrossCostAndCustomerContributionSeparate(t *testing.T) {
	p := evaluatorPolicy(ModeVendor)
	p.FreeDeliveryThresholdMinor = int64Ptr(5000)
	r := evalRequest(p)
	got := Evaluate(r)
	if got.FeeMinor != 1500 || got.PriceGuarantee.CustomerFeeMinor != 0 {
		t.Fatalf("free threshold collapsed cost/contribution: %+v", got)
	}
}

func TestHybridPlatformFundingRequiresApprovalAndCombinedShare(t *testing.T) {
	policy := evaluatorPolicy(ModeHybrid)
	policy.HybridFallbackFunding = FallbackFundingPlatform
	policy.HybridVendorShareBPS = 0
	if policy.Validate() == nil {
		t.Fatal("platform-funded fallback without approval was accepted")
	}
	policy.HybridPlatformApprovalRef = "platform_approval_1"
	if err := policy.Validate(); err != nil {
		t.Fatalf("approved platform fallback rejected: %v", err)
	}
	policy.HybridFallbackFunding = FallbackFundingCombined
	policy.HybridVendorShareBPS = 3750
	if err := policy.Validate(); err != nil {
		t.Fatalf("approved combined fallback rejected: %v", err)
	}
}
