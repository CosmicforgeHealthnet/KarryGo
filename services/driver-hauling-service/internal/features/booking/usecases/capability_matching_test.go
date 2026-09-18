package bookingusecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

// fakeCapability is a controllable capability authority.
type fakeCapability struct {
	trucks          map[string][]cargofit.TruckCapability
	loadErr         error
	recorded        []CapabilityDecision
	recordErr       error
	certifications  bool
	seenRequirement cargofit.Requirement
}

func (f *fakeCapability) LoadProviderTrucks(
	_ context.Context, _ []string, requirement cargofit.Requirement,
) (map[string][]cargofit.TruckCapability, error) {
	f.seenRequirement = requirement
	return f.trucks, f.loadErr
}

func (f *fakeCapability) RecordDecisions(_ context.Context, decisions []CapabilityDecision) error {
	f.recorded = append(f.recorded, decisions...)
	return f.recordErr
}

func (f *fakeCapability) PolicyVersion(context.Context) string { return "hauling_capability_v1" }

func (f *fakeCapability) ResolveProduct(cargofit.Requirement) (string, bool) {
	return "HAUL_GENERAL", f.certifications
}

func bigTruck(id string) cargofit.TruckCapability {
	return cargofit.TruckCapability{
		TruckID: id, TruckType: "box", Active: true, Verified: true,
		MaxPayloadGrams: 5_000_000, MaxPayloadGramsKnown: true,
		CapabilityVersion: 7,
	}
}

func smallTruck(id string) cargofit.TruckCapability {
	truck := bigTruck(id)
	truck.MaxPayloadGrams = 100_000
	return truck
}

func serviceWith(capability CapabilityAuthority, config CapabilityMatchingConfig) *BookingService {
	return &BookingService{capability: capability, capabilityMatching: config}
}

func candidates(ids ...string) []availabilityrepositories.ProviderStatus {
	out := make([]availabilityrepositories.ProviderStatus, 0, len(ids))
	for _, id := range ids {
		out = append(out, availabilityrepositories.ProviderStatus{ProviderID: id})
	}
	return out
}

func twoTonneBooking() bookingmodels.Booking {
	return bookingmodels.Booking{ID: "booking-1", CargoWeightKg: 2_000}
}

// Enforced mode removes the candidates that cannot carry the load.
func TestCapabilityFilterRemovesUnfitCandidatesWhenEnforced(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"fits":      {bigTruck("t1")},
		"too-small": {smallTruck("t2")},
	}}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("fits", "too-small"), map[string]float64{"fits": 1, "too-small": 2})

	if len(eligible) != 1 || eligible[0].ProviderID != "fits" {
		t.Fatalf("eligible = %+v, want only the fitting provider", eligible)
	}
	if len(decisions) != 2 {
		t.Fatalf("recorded %d decisions, want one per candidate", len(decisions))
	}
	for _, decision := range decisions {
		if decision.Shadow {
			t.Fatal("enforced decision was recorded as shadow")
		}
		if decision.PolicyVersion == "" {
			t.Fatal("decision recorded without a policy version")
		}
	}
}

// This is the fail-open the old matcher had: an unresolved truck fell through
// to proximity-only and got the job. Every candidate must now be refused, and
// refused as a DEPENDENCY problem rather than a physical one.
func TestCapabilityStoreOutageRefusesEveryoneAsDependencyFailure(t *testing.T) {
	capability := &fakeCapability{loadErr: errors.New("postgres down")}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("p1", "p2"), map[string]float64{})

	if len(eligible) != 0 {
		t.Fatalf("an outage still produced %d eligible candidates", len(eligible))
	}
	for _, decision := range decisions {
		if !strings.Contains(decision.ReasonCodes, capabilityfit.ReasonDependencyUnavailable) {
			t.Fatalf("outage recorded as %q, want %s",
				decision.ReasonCodes, capabilityfit.ReasonDependencyUnavailable)
		}
	}
}

// A provider with no active truck is a different situation from a store that
// did not answer, and the evidence must say which.
func TestProviderWithNoTruckIsUnknownNotDependencyFailure(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{}}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	_, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("p1"), map[string]float64{})

	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if !strings.Contains(decisions[0].ReasonCodes, capabilityfit.ReasonCapabilityUnknown) {
		t.Fatalf("reason = %q, want %s", decisions[0].ReasonCodes, capabilityfit.ReasonCapabilityUnknown)
	}
	if strings.Contains(decisions[0].ReasonCodes, capabilityfit.ReasonDependencyUnavailable) {
		t.Fatal("a provider with no truck was reported as a dependency outage")
	}
}

// Shadow mode is the whole rollout story: the decision is computed and
// recorded, and selection is untouched.
func TestShadowModeRecordsWithoutChangingSelection(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"too-small": {smallTruck("t1")},
	}}
	config := CapabilityMatchingConfig{Enabled: true, FailClosed: false}
	if !config.Shadow() {
		t.Fatal("Enabled without FailClosed is not shadow")
	}
	service := serviceWith(capability, config)

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("too-small"), map[string]float64{})

	// capabilityFilter itself always reports the truthful eligible set; it is
	// eligibleProviders that keeps the legacy list in shadow mode. What must
	// be true here is that the evidence is marked shadow so an operator can
	// tell a measured decision from an enforced one.
	if len(eligible) != 0 {
		t.Fatalf("shadow evaluation found %d eligible, want 0", len(eligible))
	}
	if len(decisions) != 1 || !decisions[0].Shadow {
		t.Fatalf("decision not marked shadow: %+v", decisions)
	}
}

// A provider brings one truck and chooses which, so any fitting truck makes
// them eligible.
func TestProviderIsEligibleIfAnyOfTheirTrucksFits(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"mixed": {smallTruck("small"), bigTruck("big")},
	}}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("mixed"), map[string]float64{})

	if len(eligible) != 1 {
		t.Fatal("a provider with one fitting truck was refused")
	}
	if decisions[0].TruckID != "big" {
		t.Fatalf("evidence named %q, want the truck that fitted", decisions[0].TruckID)
	}
}

// The requirement reaching the store must carry the product, so eligibility
// can be projected before the fit engine runs.
func TestRequirementReachesTheAuthorityWithItsProduct(t *testing.T) {
	capability := &fakeCapability{certifications: true,
		trucks: map[string][]cargofit.TruckCapability{"p1": {bigTruck("t1")}}}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	service.capabilityFilter(context.Background(), twoTonneBooking(), candidates("p1"), map[string]float64{})

	if capability.seenRequirement.ProductCode != "HAUL_GENERAL" {
		t.Fatalf("product = %q, want HAUL_GENERAL", capability.seenRequirement.ProductCode)
	}
	if !capability.seenRequirement.CertificationRequired {
		t.Fatal("certification requirement was not passed to the store")
	}
	if capability.seenRequirement.WeightGrams != 2_000_000 {
		t.Fatalf("weight = %d grams, want the booking's 2000kg converted",
			capability.seenRequirement.WeightGrams)
	}
}

// A fragile load must stay the way it was loaded, so a tipper is out even
// though nobody ticked "upright".
func TestFragileBookingImpliesUpright(t *testing.T) {
	booking := twoTonneBooking()
	booking.IsFragile = true
	requirement := requirementForBooking(booking)

	if !requirement.Fragile || !requirement.UprightRequired {
		t.Fatalf("fragile booking did not require upright: %+v", requirement)
	}

	tipper := cargofit.TruckCapability{
		TruckID: "t", TruckType: "tipper", Active: true, Verified: true,
		MaxPayloadGrams: 9_000_000, MaxPayloadGramsKnown: true,
		FragileSafe: capabilityfit.Yes,
	}
	if cargofit.Evaluate(cargofit.ResolutionFound, tipper, requirement).Eligible {
		t.Fatal("a tipper was offered a fragile load")
	}
}

// A legacy direct booking states no dimensions, and must keep matching.
func TestLegacyBookingWithoutDimensionsStillMatches(t *testing.T) {
	capability := &fakeCapability{trucks: map[string][]cargofit.TruckCapability{
		"p1": {bigTruck("t1")}, // internal dimensions deliberately unrecorded
	}}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	eligible, _ := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("p1"), map[string]float64{})

	if len(eligible) != 1 {
		t.Fatal("a legacy weight-only booking stopped matching an unsurveyed body")
	}
}

// Evidence is valuable, but never at the cost of a match.
func TestEvidenceWriteFailureDoesNotFailTheMatch(t *testing.T) {
	capability := &fakeCapability{
		trucks:    map[string][]cargofit.TruckCapability{"p1": {bigTruck("t1")}},
		recordErr: errors.New("operations table unavailable"),
	}
	service := serviceWith(capability, CapabilityMatchingConfig{Enabled: true, FailClosed: true})

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(),
		candidates("p1"), map[string]float64{})
	// recordCapabilityDecisions swallows the error; the test asserts it does
	// not panic and does not touch the eligible set.
	service.recordCapabilityDecisions(context.Background(), decisions)

	if len(eligible) != 1 {
		t.Fatal("an evidence-write failure changed the match outcome")
	}
}

// With no authority wired the filter is a pass-through, so a deployment that
// has not enabled Section 9 behaves exactly as it did before.
func TestNilAuthorityIsAPassThrough(t *testing.T) {
	service := serviceWith(nil, CapabilityMatchingConfig{Enabled: true, FailClosed: true})
	input := candidates("p1", "p2")

	eligible, decisions := service.capabilityFilter(context.Background(), twoTonneBooking(), input, nil)

	if len(eligible) != len(input) {
		t.Fatalf("pass-through dropped candidates: %d of %d", len(eligible), len(input))
	}
	if decisions != nil {
		t.Fatal("a pass-through produced evidence")
	}
}

func TestShortfallSummaryRanksByFrequency(t *testing.T) {
	summary := capabilityShortfall([]CapabilityDecision{
		{Decision: capabilityDecisionRejected, ReasonCodes: capabilityfit.ReasonCapacityExceeded},
		{Decision: capabilityDecisionRejected, ReasonCodes: capabilityfit.ReasonCapacityExceeded},
		{Decision: capabilityDecisionRejected, ReasonCodes: capabilityfit.ReasonCargoFitFailed},
		{Decision: capabilityDecisionEligible, ReasonCodes: capabilityfit.ReasonEligible},
	})
	if !strings.HasPrefix(summary, capabilityfit.ReasonCapacityExceeded) {
		t.Fatalf("summary = %q, want the commonest reason first", summary)
	}
	if strings.Contains(summary, capabilityfit.ReasonEligible) {
		t.Fatalf("summary %q counted an eligible candidate as a shortfall", summary)
	}
}
