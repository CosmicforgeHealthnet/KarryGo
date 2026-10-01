package fulfilment

import (
	"context"
	"errors"
	"testing"
)

type fakeCapabilityAuthority struct {
	capability ServiceCapability
	err        error
	calls      int
}

func (f *fakeCapabilityAuthority) Capability(context.Context) (ServiceCapability, error) {
	f.calls++

	if f.err != nil {
		return ServiceCapability{}, f.err
	}

	return f.capability, nil
}

// A live capability contract opens consolidation without a deployment flag:
// the authority states it can drive two collection points, and the planner
// consolidates exactly as it would have with the configured map.
func TestPlan_LiveCapabilityContractOpensConsolidation(t *testing.T) {
	h := twoVendorHarness(t, separateLat, separateLng, map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 170000, minutes: 26},
	})

	authority := &fakeCapabilityAuthority{capability: ServiceCapability{
		Supported: true, MaximumPickups: 3, SecurePickupHandover: true, Version: "taxi|multi=true",
	}}
	h.capabilityAuthorities = map[Service]CapabilityAuthority{ServiceDispatch: authority}

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertSelected(t, plan, CandidateConsolidated)

	if authority.calls == 0 {
		t.Fatal("capability was assumed rather than read from the authority")
	}

	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	if !consolidated.ServiceCapabilities[ServiceDispatch] {
		t.Fatal("candidate evidence lost the capability it was decided under")
	}
}

// An authority that cannot be read is not a capable authority, even where the
// deployment configured it as one. A remembered capability would submit a
// mission the runtime has just stopped accepting.
func TestPlan_UnreadableCapabilityOverridesConfiguredSupport(t *testing.T) {
	h := twoVendorHarness(t, separateLat, separateLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	h.multiPickup = map[Service]bool{ServiceDispatch: true}
	h.capabilityAuthorities = map[Service]CapabilityAuthority{
		ServiceDispatch: &fakeCapabilityAuthority{err: errors.New("authority unreachable")},
	}

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan, CandidateConsolidated, ReasonCandidateMultiStopUnsupported)
	assertSelected(t, plan, CandidateSplitParallel)
}

// A stop limit is a real limit: the authority drives multi-stop missions but
// not one with this many collection points, so the plan splits instead of
// growing a mission the authority would refuse.
func TestPlan_StopLimitSplitsRatherThanOverfillingAMission(t *testing.T) {
	h := twoVendorHarness(t, separateLat, separateLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	h.capabilityAuthorities = map[Service]CapabilityAuthority{
		ServiceDispatch: &fakeCapabilityAuthority{capability: ServiceCapability{Supported: true, MaximumPickups: 1}},
	}

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan, CandidateConsolidated, ReasonCandidateMultiStopLimitExceeded)
	assertSelected(t, plan, CandidateSplitParallel)

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2 drivable missions", len(plan.Missions))
	}
}

func TestRuntimeLimitsAreStatedOnce(t *testing.T) {
	capabilities := map[Service]ServiceCapability{
		ServiceTaxi: {Supported: true, MaximumPickups: 3},
	}

	if _, ok := withinRuntimeLimits(ServiceTaxi, 1, true, nil); !ok {
		t.Fatal("a single pickup needs no multi-stop capability at all")
	}

	if reason, _ := withinRuntimeLimits(ServiceHauling, 2, false, capabilities); reason != ReasonCandidateMultiStopUnsupported {
		t.Fatalf("unstated authority = %s", reason)
	}

	if reason, _ := withinRuntimeLimits(ServiceTaxi, 4, false, capabilities); reason != ReasonCandidateMultiStopLimitExceeded {
		t.Fatalf("over the stop limit = %s", reason)
	}

	if reason, _ := withinRuntimeLimits(ServiceTaxi, 2, true, capabilities); reason != ReasonCandidateSecureHandoverUnsupported {
		t.Fatalf("high-value load without a verified handover = %s", reason)
	}

	if _, ok := withinRuntimeLimits(ServiceTaxi, 2, false, capabilities); !ok {
		t.Fatal("two stops within the stated limit must be executable")
	}
}
