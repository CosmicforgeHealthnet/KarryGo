package assignment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

// fakeAuthority is a controllable capability authority.
type fakeAuthority struct {
	bikes           map[string][]parcelfit.Capability
	loadErr         error
	recorded        []CapabilityDecision
	recordErr       error
	certifications  bool
	seenRequirement parcelfit.Requirement
}

func (f *fakeAuthority) LoadProviderBikes(
	_ context.Context, _ []string, requirement parcelfit.Requirement,
) (map[string][]parcelfit.Capability, error) {
	f.seenRequirement = requirement
	return f.bikes, f.loadErr
}

func (f *fakeAuthority) RecordDecisions(_ context.Context, decisions []CapabilityDecision) error {
	f.recorded = append(f.recorded, decisions...)
	return f.recordErr
}

func (f *fakeAuthority) PolicyVersion(context.Context) string { return "dispatch_capability_v1" }

func (f *fakeAuthority) ResolveProduct(requirement parcelfit.Requirement) (string, bool) {
	if requirement.NeedsThermal() {
		return "DISPATCH_COLD_CHAIN", f.certifications
	}
	return "DISPATCH_STANDARD", f.certifications
}

func capableBike(id string) parcelfit.Capability {
	return parcelfit.Capability{
		BikeID: id, BikeType: "dispatch_bike", Active: true, Verified: true,
		MaxPayloadGrams: 20_000, MaxPayloadGramsKnown: true,
		BoxLengthMm: 500, BoxWidthMm: 400, BoxHeightMm: 400, BoxDimsKnown: true,
		CapabilityVersion: 5,
	}
}

func smallBike(id string) parcelfit.Capability {
	bike := capableBike(id)
	bike.MaxPayloadGrams = 3_000
	return bike
}

func candidates(ids ...string) []Candidate {
	out := make([]Candidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, Candidate{ProviderID: id})
	}
	return out
}

func tenKgParcel() parcelfit.Requirement {
	return parcelfit.Requirement{WeightGrams: 10_000}
}

// Enforced mode removes the candidates whose bikes cannot carry the parcel.
func TestGateRemovesUnfitCandidatesWhenEnforced(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"fits":      {capableBike("b1")},
		"too-small": {smallBike("b2")},
	}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	result := gate.Apply(context.Background(), "mission-1", tenKgParcel(),
		candidates("fits", "too-small"))

	if len(result) != 1 || result[0].ProviderID != "fits" {
		t.Fatalf("result = %+v, want only the fitting provider", result)
	}
	if len(authority.recorded) != 2 {
		t.Fatalf("recorded %d decisions, want one per candidate", len(authority.recorded))
	}
	for _, decision := range authority.recorded {
		if decision.Shadow {
			t.Fatal("enforced decision recorded as shadow")
		}
		if decision.PolicyVersion == "" {
			t.Fatal("decision recorded without a policy version")
		}
		if decision.MissionID != "mission-1" {
			t.Fatalf("decision lost its mission: %+v", decision)
		}
	}
}

// Shadow mode is the whole rollout story: the decision is computed and
// recorded, and the candidate list is returned untouched.
func TestShadowModeRecordsWithoutChangingTheCandidateList(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"too-small": {smallBike("b1")},
	}}
	config := CapabilityConfig{Enabled: true, Enforce: false}
	if !config.Shadow() {
		t.Fatal("Enabled without Enforce is not shadow")
	}
	gate := NewCapabilityGate(authority, config)

	input := candidates("too-small")
	result := gate.Apply(context.Background(), "mission-1", tenKgParcel(), input)

	if len(result) != len(input) {
		t.Fatalf("shadow mode dropped a candidate: %d of %d", len(result), len(input))
	}
	if len(authority.recorded) != 1 || !authority.recorded[0].Shadow {
		t.Fatalf("decision not recorded as shadow: %+v", authority.recorded)
	}
}

// A store outage must refuse everyone, and refuse them as a DEPENDENCY problem
// rather than a physical one. The legacy SQL had no way to express this at all.
func TestStoreOutageRefusesEveryoneAsDependencyFailure(t *testing.T) {
	authority := &fakeAuthority{loadErr: errors.New("postgres down")}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	result := gate.Apply(context.Background(), "mission-1", tenKgParcel(), candidates("p1", "p2"))

	if len(result) != 0 {
		t.Fatalf("an outage still produced %d candidates", len(result))
	}
	for _, decision := range authority.recorded {
		if !strings.Contains(decision.ReasonCodes, capabilityfit.ReasonDependencyUnavailable) {
			t.Fatalf("outage recorded as %q", decision.ReasonCodes)
		}
	}
}

func TestProviderWithNoBikeIsUnknownNotDependencyFailure(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	gate.Apply(context.Background(), "mission-1", tenKgParcel(), candidates("p1"))

	if len(authority.recorded) != 1 {
		t.Fatalf("recorded %d decisions, want 1", len(authority.recorded))
	}
	if !strings.Contains(authority.recorded[0].ReasonCodes, capabilityfit.ReasonCapabilityUnknown) {
		t.Fatalf("reason = %q, want %s",
			authority.recorded[0].ReasonCodes, capabilityfit.ReasonCapabilityUnknown)
	}
	if strings.Contains(authority.recorded[0].ReasonCodes, capabilityfit.ReasonDependencyUnavailable) {
		t.Fatal("a provider with no bike was reported as a dependency outage")
	}
}

// A rider brings one machine and chooses which, so any fitting bike makes them
// eligible.
func TestProviderIsEligibleIfAnyBikeFits(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"mixed": {smallBike("small"), capableBike("big")},
	}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	result := gate.Apply(context.Background(), "mission-1", tenKgParcel(), candidates("mixed"))

	if len(result) != 1 {
		t.Fatal("a provider with one fitting bike was refused")
	}
	if authority.recorded[0].BikeID != "big" {
		t.Fatalf("evidence named %q, want the bike that fitted", authority.recorded[0].BikeID)
	}
}

// An accept or a direct assignment must not route around the gate discovery
// applies. That would be the obvious way to hand a rider a parcel their bike
// cannot carry.
func TestAllowsRunsTheSameGateForANamedProvider(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"too-small": {smallBike("b1")},
		"fits":      {capableBike("b2")},
	}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	allowed, reason := gate.Allows(context.Background(), "mission-1", tenKgParcel(), "too-small")
	if allowed {
		t.Fatal("an unfit named provider was allowed")
	}
	if !strings.Contains(reason, capabilityfit.ReasonCapacityExceeded) {
		t.Fatalf("reason = %q", reason)
	}

	allowed, _ = gate.Allows(context.Background(), "mission-1", tenKgParcel(), "fits")
	if !allowed {
		t.Fatal("a fitting named provider was refused")
	}
}

// In shadow mode Allows must not block, or shadow would silently be enforcing
// on the accept path while only measuring on the discovery path.
func TestAllowsDoesNotBlockInShadowMode(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"too-small": {smallBike("b1")},
	}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: false})

	if allowed, _ := gate.Allows(context.Background(), "m", tenKgParcel(), "too-small"); !allowed {
		t.Fatal("shadow mode blocked an accept")
	}
}

// With no authority wired the gate is inert, so a deployment that has not
// enabled Section 8 behaves exactly as it did before.
func TestInactiveGateIsAPassThrough(t *testing.T) {
	for _, gate := range []*CapabilityGate{
		nil,
		NewCapabilityGate(nil, CapabilityConfig{Enabled: true, Enforce: true}),
		NewCapabilityGate(&fakeAuthority{}, CapabilityConfig{Enabled: false}),
	} {
		input := candidates("p1", "p2")
		if result := gate.Apply(context.Background(), "m", tenKgParcel(), input); len(result) != len(input) {
			t.Fatalf("inactive gate dropped candidates: %d of %d", len(result), len(input))
		}
		if allowed, _ := gate.Allows(context.Background(), "m", tenKgParcel(), "p1"); !allowed {
			t.Fatal("inactive gate blocked an accept")
		}
	}
}

// Evidence is valuable, but never at the cost of an assignment.
func TestEvidenceWriteFailureDoesNotFailTheAssignment(t *testing.T) {
	authority := &fakeAuthority{
		bikes:     map[string][]parcelfit.Capability{"p1": {capableBike("b1")}},
		recordErr: errors.New("operations table unavailable"),
	}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	if result := gate.Apply(context.Background(), "m", tenKgParcel(), candidates("p1")); len(result) != 1 {
		t.Fatal("an evidence-write failure changed the assignment outcome")
	}
}

// The requirement reaching the store must carry the resolved product, so
// eligibility can be projected before the fit engine runs.
func TestRequirementReachesTheAuthorityWithItsProduct(t *testing.T) {
	authority := &fakeAuthority{certifications: true,
		bikes: map[string][]parcelfit.Capability{"p1": {capableBike("b1")}}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true})

	requirement := tenKgParcel()
	requirement.TemperatureClass = "CHILLED"
	gate.Apply(context.Background(), "m", requirement, candidates("p1"))

	if authority.seenRequirement.ProductCode != "DISPATCH_COLD_CHAIN" {
		t.Fatalf("product = %q, want DISPATCH_COLD_CHAIN", authority.seenRequirement.ProductCode)
	}
	if !authority.seenRequirement.CertificationRequired {
		t.Fatal("certification requirement was not passed to the store")
	}
}

// ── Requirement derivation from the mission's own parcel rows ─────────────────

func TestRequirementAggregatesWeightAndMaximizesDimensions(t *testing.T) {
	requirement := RequirementFromMissionSnapshot([]ParcelSnapshot{
		{WeightGrams: 3_000, VolumeMl: 4_000, LengthMm: 300, WidthMm: 200, HeightMm: 150, PackageCount: 1},
		{WeightGrams: 2_500, VolumeMl: 1_000, LengthMm: 180, WidthMm: 400, HeightMm: 120, PackageCount: 2},
	})

	// Weight, volume and count accumulate because the rider carries them all.
	if requirement.WeightGrams != 5_500 {
		t.Fatalf("weight = %d, want 5500", requirement.WeightGrams)
	}
	if requirement.VolumeMl != 5_000 {
		t.Fatalf("volume = %d, want 5000", requirement.VolumeMl)
	}
	if requirement.PackageCount != 3 {
		t.Fatalf("count = %d, want 3", requirement.PackageCount)
	}
	// Dimensions take the maximum, because the box has to fit the biggest
	// single item, not the sum of every item's edges.
	if requirement.LengthMm != 300 || requirement.WidthMm != 400 || requirement.HeightMm != 150 {
		t.Fatalf("dimensions = %dx%dx%d, want 300x400x150",
			requirement.LengthMm, requirement.WidthMm, requirement.HeightMm)
	}
}

// One fragile item makes the whole consignment fragile: they travel together.
func TestHandlingIsUnionedAcrossParcels(t *testing.T) {
	requirement := RequirementFromMissionSnapshot([]ParcelSnapshot{
		{WeightGrams: 1_000, Handling: json.RawMessage(`{"codes":["FRAGILE"]}`)},
		{WeightGrams: 1_000, Handling: json.RawMessage(`{}`)},
	})
	if !requirement.Fragile {
		t.Fatal("a fragile parcel did not make the consignment fragile")
	}
	if !requirement.UprightRequired {
		t.Fatal("fragile did not imply upright")
	}
}

// A consignment with one frozen item must be carried frozen. Taking the last
// parcel's class instead would silently thaw it.
func TestStrictestTemperatureClassWins(t *testing.T) {
	requirement := RequirementFromMissionSnapshot([]ParcelSnapshot{
		{Handling: json.RawMessage(`{"temperature_class":"FROZEN"}`)},
		{Handling: json.RawMessage(`{"temperature_class":"CHILLED"}`)},
		{Handling: json.RawMessage(`{"temperature_class":"AMBIENT"}`)},
	})
	if requirement.TemperatureClass != "FROZEN" {
		t.Fatalf("class = %q, want FROZEN", requirement.TemperatureClass)
	}
}

// A malformed handling snapshot must not fail the whole assignment; it simply
// contributes no handling constraints.
func TestMalformedHandlingSnapshotDegradesGracefully(t *testing.T) {
	requirement := RequirementFromMissionSnapshot([]ParcelSnapshot{
		{WeightGrams: 2_000, Handling: json.RawMessage(`not json`)},
	})
	if requirement.WeightGrams != 2_000 {
		t.Fatalf("physical facts lost: %+v", requirement)
	}
	if requirement.Fragile || requirement.EnclosureRequired {
		t.Fatalf("malformed snapshot invented handling: %+v", requirement)
	}
}

// A mission with no parcel rows -- legacy or non-commerce -- yields an empty
// requirement, which constrains nothing and cannot change its outcome.
func TestNoParcelsYieldsAnUnconstrainedRequirement(t *testing.T) {
	requirement := RequirementFromMissionSnapshot(nil)
	if requirement.WeightGrams != 0 || requirement.HasDimensions() || requirement.NeedsThermal() {
		t.Fatalf("empty snapshot produced constraints: %+v", requirement)
	}
	assertNoRefusal(t, parcelfit.Evaluate(parcelfit.ResolutionFound, capableBike("b1"), requirement))
}

func assertNoRefusal(t *testing.T, decision *capabilityfit.Decision) {
	t.Helper()
	if !decision.Eligible {
		t.Fatalf("unconstrained requirement refused: %v", decision.Reasons)
	}
}

func TestShortfallRanksByFrequency(t *testing.T) {
	summary := shortfall([]CapabilityDecision{
		{Decision: decisionRejected, ReasonCodes: capabilityfit.ReasonCapacityExceeded},
		{Decision: decisionRejected, ReasonCodes: capabilityfit.ReasonCapacityExceeded},
		{Decision: decisionRejected, ReasonCodes: capabilityfit.ReasonCargoFitFailed},
		{Decision: decisionEligible, ReasonCodes: capabilityfit.ReasonEligible},
	})
	if !strings.HasPrefix(summary, capabilityfit.ReasonCapacityExceeded) {
		t.Fatalf("summary = %q, want the commonest reason first", summary)
	}
	if strings.Contains(summary, capabilityfit.ReasonEligible) {
		t.Fatalf("summary %q counted an eligible candidate as a shortfall", summary)
	}
}
