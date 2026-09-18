package cargofit

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

func intPtr(v int) *int { return &v }

// A fully-surveyed box truck. Tests start from this and break one thing at a
// time, so a refusal always has exactly one cause.
func surveyedBoxTruck() TruckCapability {
	return TruckCapability{
		TruckID: "truck-1", ProviderID: "provider-1", TruckType: "box",
		Active: true, Verified: true,
		MaxPayloadGrams: 3_000_000, MaxPayloadGramsKnown: true,
		InternalLengthMm: 4_000, InternalWidthMm: 2_000, InternalHeightMm: 2_200,
		InternalDimsKnown: true,
		UsableVolumeMl:    17_600_000, UsableVolumeMlKnown: true,
		OpeningWidthMm: 1_900, OpeningHeightMm: 2_100, OpeningDimsKnown: true,
		Enclosed: capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		UprightSafe: capabilityfit.Yes, FragileSafe: capabilityfit.Yes,
		TailLift: capabilityfit.Yes, Crane: capabilityfit.No,
		LoadingRamp: capabilityfit.Yes, Hydraulic: capabilityfit.Yes,
		CapabilityVersion: 3,
	}
}

func assertEligible(t *testing.T, decision *capabilityfit.Decision) {
	t.Helper()
	if !decision.Eligible {
		t.Fatalf("expected eligible, refused with %v (%v)", decision.Reasons, decision.Details)
	}
}

func assertRefused(t *testing.T, decision *capabilityfit.Decision, reason string) {
	t.Helper()
	if decision.Eligible {
		t.Fatalf("expected refusal %s, got eligible", reason)
	}
	for _, got := range decision.Reasons {
		if got == reason {
			return
		}
	}
	t.Fatalf("reasons %v do not include %s (%v)", decision.Reasons, reason, decision.Details)
}

// ── Fail-closed truck resolution (Section 9.6) ────────────────────────────────

// This is the behaviour the old matcher got wrong. `if err == nil { checks }`
// meant an unreachable store and a deleted truck both fell through to
// proximity-only matching, so a truck nobody could describe still got the job.
func TestUnresolvedTruckCannotReachScoring(t *testing.T) {
	requirement := Requirement{WeightGrams: 2_000_000}

	missing := Evaluate(ResolutionMissing, TruckCapability{}, requirement)
	assertRefused(t, missing, capabilityfit.ReasonCapabilityUnknown)

	unattempted := Evaluate(ResolutionUnattempted, TruckCapability{}, requirement)
	assertRefused(t, unattempted, capabilityfit.ReasonDependencyUnavailable)
}

// A dependency outage must stay distinguishable from a physical mismatch, so
// an operator can tell "our store is down" from "this truck is too small".
func TestDependencyOutageIsDistinctFromPhysicalMismatch(t *testing.T) {
	outage := Evaluate(ResolutionFailed, TruckCapability{}, Requirement{WeightGrams: 1})
	if got := outage.PrimaryReason(); got != capabilityfit.ReasonDependencyUnavailable {
		t.Fatalf("outage reason = %s, want %s", got, capabilityfit.ReasonDependencyUnavailable)
	}

	tooHeavy := Evaluate(ResolutionFound, surveyedBoxTruck(), Requirement{WeightGrams: 9_000_000})
	if got := tooHeavy.PrimaryReason(); got != capabilityfit.ReasonCapacityExceeded {
		t.Fatalf("mismatch reason = %s, want %s", got, capabilityfit.ReasonCapacityExceeded)
	}
	if outage.PrimaryReason() == tooHeavy.PrimaryReason() {
		t.Fatal("outage and physical mismatch share a reason code")
	}
}

func TestInactiveOrUnverifiedTruckIsRefused(t *testing.T) {
	inactive := surveyedBoxTruck()
	inactive.Active = false
	assertRefused(t, Evaluate(ResolutionFound, inactive, Requirement{}), capabilityfit.ReasonProviderNotEligible)

	unverified := surveyedBoxTruck()
	unverified.Verified = false
	assertRefused(t, Evaluate(ResolutionFound, unverified, Requirement{}), capabilityfit.ReasonProviderNotEligible)
}

// ── Payload ───────────────────────────────────────────────────────────────────

func TestPayloadBoundaryIsInclusive(t *testing.T) {
	truck := surveyedBoxTruck()
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{WeightGrams: 3_000_000}))
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{WeightGrams: 3_000_001}),
		capabilityfit.ReasonCapacityExceeded)
}

// An unmeasured payload is unknown, not unlimited. The old code's
// `if truck.CapacityKg > 0` guard skipped the check entirely here.
func TestUnmeasuredPayloadRefusesRatherThanSkips(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.MaxPayloadGrams, truck.MaxPayloadGramsKnown = 0, false
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{WeightGrams: 2_000_000}),
		capabilityfit.ReasonCapabilityUnknown)
	// ...but a load that states no weight does not depend on the missing fact.
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{}))
}

func TestKilogramConversionIsExplicit(t *testing.T) {
	if got := FromKilograms(3_000); got != 3_000_000 {
		t.Fatalf("FromKilograms(3000) = %d, want 3000000", got)
	}
	if got := FromKilograms(0); got != 0 {
		t.Fatalf("FromKilograms(0) = %d, want 0", got)
	}
	if got := FromKilograms(-5); got != 0 {
		t.Fatalf("FromKilograms(-5) = %d, want 0", got)
	}
}

// ── Dimensions and volume ─────────────────────────────────────────────────────

func TestInternalDimensionBoundariesAndOverflow(t *testing.T) {
	truck := surveyedBoxTruck()
	// Exactly the body's length, and a cross-section that still clears the
	// 1900x2100 doorway -- a load that exactly filled the body would be
	// refused by the opening check, correctly, and would not test this.
	exact := Requirement{LengthMm: 4_000, WidthMm: 1_800, HeightMm: 2_000}
	assertEligible(t, Evaluate(ResolutionFound, truck, exact))

	cases := []struct {
		name string
		req  Requirement
	}{
		{"too long", Requirement{LengthMm: 4_001, WidthMm: 100, HeightMm: 100}},
		{"too wide", Requirement{LengthMm: 100, WidthMm: 2_001, HeightMm: 100}},
		{"too high", Requirement{LengthMm: 100, WidthMm: 100, HeightMm: 2_201}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefused(t, Evaluate(ResolutionFound, truck, tc.req), capabilityfit.ReasonCargoFitFailed)
		})
	}
}

func TestVolumeOverflowRefused(t *testing.T) {
	truck := surveyedBoxTruck()
	req := Requirement{LengthMm: 100, WidthMm: 100, HeightMm: 100, VolumeMl: 20_000_000}
	assertRefused(t, Evaluate(ResolutionFound, truck, req), capabilityfit.ReasonCargoFitFailed)
}

func TestUnmeasuredBodyRefusesADimensionedLoad(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.InternalDimsKnown = false
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{LengthMm: 500, WidthMm: 500, HeightMm: 500}),
		capabilityfit.ReasonCapabilityUnknown)
}

// A legacy direct booking states no dimensions because the customer app has
// never collected them. Refusing every such booking would take the service
// offline, so an unstated dimension only becomes fatal when a product demands
// one.
func TestUnstatedDimensionsStayEligibleUnlessTheProductRequiresThem(t *testing.T) {
	truck := surveyedBoxTruck()
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{WeightGrams: 1_000_000}))

	required := Requirement{WeightGrams: 1_000_000, DimensionsRequired: true}
	assertRefused(t, Evaluate(ResolutionFound, truck, required), capabilityfit.ReasonCapabilityUnknown)
}

// ── Openings ──────────────────────────────────────────────────────────────────

func TestOpeningRefusesALoadThatCannotPassTheDoor(t *testing.T) {
	truck := surveyedBoxTruck()
	// Fits the body (4000x2000x2200) but its smallest cross-section is
	// 1950x1950, which will not pass a 1900-wide door in either orientation.
	req := Requirement{LengthMm: 3_500, WidthMm: 1_950, HeightMm: 1_950}
	assertRefused(t, Evaluate(ResolutionFound, truck, req), capabilityfit.ReasonCargoFitFailed)
}

func TestOpeningAcceptsALoadThatFitsWhenRotated(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.OpeningWidthMm, truck.OpeningHeightMm = 1_200, 2_000
	// 1900x1100 cross-section: fails upright, passes rotated.
	req := Requirement{LengthMm: 3_000, WidthMm: 1_900, HeightMm: 1_100}
	assertEligible(t, Evaluate(ResolutionFound, truck, req))
}

// An open deck has no door. Demanding an opening measurement from a flatbed
// would refuse every flatbed for no physical reason.
func TestOpenDeckSkipsTheOpeningCheck(t *testing.T) {
	flatbed := TruckCapability{
		TruckID: "t", TruckType: "flatbed", Active: true, Verified: true,
		MaxPayloadGrams: 10_000_000, MaxPayloadGramsKnown: true,
		InternalLengthMm: 6_000, InternalWidthMm: 2_400, InternalHeightMm: 3_000,
		InternalDimsKnown: true,
		// Openings deliberately unrecorded.
	}
	req := Requirement{LengthMm: 5_500, WidthMm: 2_300, HeightMm: 2_800, WeightGrams: 8_000_000}
	assertEligible(t, Evaluate(ResolutionFound, flatbed, req))
}

func TestUnmeasuredOpeningOnlyRefusesLoadsBigEnoughToCare(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.OpeningDimsKnown = false

	small := Requirement{LengthMm: 400, WidthMm: 300, HeightMm: 300}
	assertEligible(t, Evaluate(ResolutionFound, truck, small))

	large := Requirement{LengthMm: 2_500, WidthMm: 1_200, HeightMm: 1_200}
	assertRefused(t, Evaluate(ResolutionFound, truck, large), capabilityfit.ReasonCapabilityUnknown)
}

// ── Temperature ───────────────────────────────────────────────────────────────

func TestRefrigerationRequirementRefusesANonRefrigeratedTruck(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.Refrigerated = capabilityfit.No
	req := Requirement{TemperatureClass: "CHILLED"}
	assertRefused(t, Evaluate(ResolutionFound, truck, req), capabilityfit.ReasonRequiredCapabilityUnavailable)
}

// An articulated unit's cooling depends on the trailer coupled to it, so an
// unsurveyed one must fail closed as UNKNOWN rather than be assumed either way.
//
// A box truck is a different case and deliberately answers differently: "box"
// means a dry van by definition, so the catalogue settles Refrigerated=No and
// the refusal is a statement about the vehicle, not a data gap.
func TestUnknownRefrigerationFailsClosed(t *testing.T) {
	trailer := TruckCapability{
		TruckID: "t", TruckType: "trailer", Active: true, Verified: true,
	}
	req := Requirement{TemperatureClass: "FROZEN"}
	assertRefused(t, Evaluate(ResolutionFound, trailer, req), capabilityfit.ReasonCapabilityUnknown)

	dryVan := surveyedBoxTruck()
	assertRefused(t, Evaluate(ResolutionFound, dryVan, req), capabilityfit.ReasonRequiredCapabilityUnavailable)
}

func TestTemperatureBandMustCoverTheRequirement(t *testing.T) {
	reefer := surveyedBoxTruck()
	reefer.TruckType = "refrigerated"
	reefer.Refrigerated = capabilityfit.Yes
	reefer.TemperatureMinC, reefer.TemperatureMaxC = intPtr(-5), intPtr(8)

	within := Requirement{TemperatureClass: "CHILLED", MinTempC: intPtr(0), MaxTempC: intPtr(4)}
	assertEligible(t, Evaluate(ResolutionFound, reefer, within))

	// -18C frozen is colder than this truck can hold.
	frozen := Requirement{TemperatureClass: "FROZEN", MinTempC: intPtr(-20), MaxTempC: intPtr(-18)}
	assertRefused(t, Evaluate(ResolutionFound, reefer, frozen), capabilityfit.ReasonRequiredCapabilityUnavailable)
}

func TestUnrecordedTemperatureRangeOnAReeferIsUnknownNotUnlimited(t *testing.T) {
	reefer := surveyedBoxTruck()
	reefer.TruckType = "refrigerated"
	reefer.Refrigerated = capabilityfit.Yes // range deliberately unrecorded
	req := Requirement{TemperatureClass: "FROZEN", MaxTempC: intPtr(-18)}
	assertRefused(t, Evaluate(ResolutionFound, reefer, req), capabilityfit.ReasonCapabilityUnknown)
}

// A requirement that only says "no warmer than 4C" still needs a reefer, even
// though nobody typed CHILLED.
func TestColdMaximumImpliesRefrigerationWithoutAClassCode(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.Refrigerated = capabilityfit.No
	req := Requirement{MaxTempC: intPtr(4)}
	assertRefused(t, Evaluate(ResolutionFound, truck, req), capabilityfit.ReasonRequiredCapabilityUnavailable)
}

func TestAmbientNeedsNoRefrigeration(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.Refrigerated = capabilityfit.No
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{TemperatureClass: "AMBIENT"}))
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{TemperatureClass: ""}))
}

// ── Loading equipment ─────────────────────────────────────────────────────────

func TestLoadingEquipmentMismatches(t *testing.T) {
	truck := surveyedBoxTruck() // tail lift yes, crane no, ramp yes

	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{TailLiftRequired: true}))
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{CraneRequired: true}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)

	noRamp := surveyedBoxTruck()
	noRamp.LoadingRamp, noRamp.TailLift, noRamp.Hydraulic = capabilityfit.No, capabilityfit.No, capabilityfit.No
	assertRefused(t, Evaluate(ResolutionFound, noRamp, Requirement{ForkliftLoading: true}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
}

// Forklift loading needs a way to reach the bed, and either a ramp or a lift
// provides one. Demanding a specific piece of equipment would refuse trucks
// that can genuinely do the job.
func TestForkliftLoadingAcceptsEitherRampOrLift(t *testing.T) {
	rampOnly := surveyedBoxTruck()
	rampOnly.TailLift, rampOnly.Hydraulic = capabilityfit.No, capabilityfit.No
	assertEligible(t, Evaluate(ResolutionFound, rampOnly, Requirement{ForkliftLoading: true}))

	liftOnly := surveyedBoxTruck()
	liftOnly.LoadingRamp, liftOnly.Hydraulic = capabilityfit.No, capabilityfit.No
	assertEligible(t, Evaluate(ResolutionFound, liftOnly, Requirement{ForkliftLoading: true}))
}

func TestUnrecordedEquipmentFailsClosed(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.Crane = capabilityfit.Unknown
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{CraneRequired: true}),
		capabilityfit.ReasonCapabilityUnknown)
}

// ── Body, class and goods ─────────────────────────────────────────────────────

func TestPreferredTypeIsAHardGateWidenedToFamily(t *testing.T) {
	truck := surveyedBoxTruck() // "box", ENCLOSED_BODY

	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{PreferredTruckType: "van"}))
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{PreferredTruckType: "flatbed"}),
		capabilityfit.ReasonWrongAsset)
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{PreferredTruckType: ""}))
}

func TestVehicleClassIsAHardGate(t *testing.T) {
	truck := surveyedBoxTruck() // box -> TRUCK
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{VehicleClass: "TRUCK"}))
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{VehicleClass: "VAN"}),
		capabilityfit.ReasonWrongAsset)
}

func TestProhibitedGoodsClassWinsOverAPermit(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.SupportedGoodsClasses = []string{"GENERAL", "HAZARDOUS"}
	truck.ProhibitedGoodsClasses = []string{"HAZARDOUS"}
	req := Requirement{GoodsClasses: []string{"HAZARDOUS"}}
	assertRefused(t, Evaluate(ResolutionFound, truck, req), capabilityfit.ReasonRequiredCapabilityUnavailable)
}

func TestUnsupportedGoodsClassRefused(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.SupportedGoodsClasses = []string{"GENERAL"}
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{GoodsClasses: []string{"HAZARDOUS"}}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{GoodsClasses: []string{"GENERAL"}}))
}

// A truck that stated no permit list still carries general cargo; demanding an
// explicit permit from every truck would ground the existing fleet.
func TestEmptyPermitListStillCarriesGeneralCargo(t *testing.T) {
	truck := surveyedBoxTruck()
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{GoodsClasses: []string{"GENERAL"}}))
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{GoodsClasses: []string{"HAZARDOUS"}}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
}

// A tank body carries bulk liquid by construction, and a provider cannot opt
// out of that by leaving their goods lists empty.
func TestTankerRefusesGeneralCargoRegardlessOfGoodsLists(t *testing.T) {
	tanker := surveyedBoxTruck()
	tanker.TruckType = "tanker"
	assertRefused(t, Evaluate(ResolutionFound, tanker, Requirement{GoodsClasses: []string{"GENERAL"}}),
		capabilityfit.ReasonWrongAsset)
	assertEligible(t, Evaluate(ResolutionFound, tanker, Requirement{GoodsClasses: []string{"LIQUID_BULK"}}))
}

func TestContainerRequirements(t *testing.T) {
	carrier := surveyedBoxTruck()
	carrier.TruckType = "container"
	carrier.SupportedContainerSizes = []string{"20FT", "40FT"}

	assertEligible(t, Evaluate(ResolutionFound, carrier, Requirement{ContainerSize: "20ft"}))
	assertRefused(t, Evaluate(ResolutionFound, carrier, Requirement{ContainerSize: "45FT"}),
		capabilityfit.ReasonCargoFitFailed)

	notACarrier := surveyedBoxTruck() // "box"
	assertRefused(t, Evaluate(ResolutionFound, notACarrier, Requirement{ContainerSize: "20FT"}),
		capabilityfit.ReasonWrongAsset)

	unrecorded := surveyedBoxTruck()
	unrecorded.TruckType = "container"
	assertRefused(t, Evaluate(ResolutionFound, unrecorded, Requirement{ContainerSize: "20FT"}),
		capabilityfit.ReasonCapabilityUnknown)
}

// ── Type-derived facts and their precedence ───────────────────────────────────

// A tipper discharges by tilting, so the catalogue's UprightSafe=No must
// refuse an upright load even when the truck's own row is silent.
func TestCatalogueFactsAnswerWhenTheTruckRowIsSilent(t *testing.T) {
	tipper := TruckCapability{
		TruckID: "t", TruckType: "tipper", Active: true, Verified: true,
	}
	assertRefused(t, Evaluate(ResolutionFound, tipper, Requirement{UprightRequired: true}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
}

// An articulated unit's body depends on the trailer coupled to it, so the type
// settles nothing and an unsurveyed one must fail closed on enclosure.
func TestTrailerEnclosureIsUnknownNotAssumed(t *testing.T) {
	trailer := TruckCapability{
		TruckID: "t", TruckType: "trailer", Active: true, Verified: true,
	}
	assertRefused(t, Evaluate(ResolutionFound, trailer, Requirement{EnclosureRequired: true}),
		capabilityfit.ReasonCapabilityUnknown)

	// Once the truck records its own fact, that answers.
	surveyed := trailer
	surveyed.Enclosed = capabilityfit.Yes
	assertEligible(t, Evaluate(ResolutionFound, surveyed, Requirement{EnclosureRequired: true}))
}

func TestInsuranceRecordedAsInvalidRefuses(t *testing.T) {
	truck := surveyedBoxTruck()
	truck.InsuranceValid = capabilityfit.No
	assertRefused(t, Evaluate(ResolutionFound, truck, Requirement{}), capabilityfit.ReasonProviderNotEligible)

	// Unknown insurance is the normal state for older trucks and must not
	// ground the fleet.
	truck.InsuranceValid = capabilityfit.Unknown
	assertEligible(t, Evaluate(ResolutionFound, truck, Requirement{}))
}

// ── Aggregate behaviour ───────────────────────────────────────────────────────

func TestEveryFailureReasonIsReported(t *testing.T) {
	truck := surveyedBoxTruck()
	req := Requirement{
		WeightGrams:        9_000_000,
		LengthMm:           9_000,
		WidthMm:            100,
		HeightMm:           100,
		PreferredTruckType: "flatbed",
	}
	decision := Evaluate(ResolutionFound, truck, req)
	if decision.Eligible {
		t.Fatal("expected refusal")
	}
	joined := decision.ReasonString()
	for _, want := range []string{
		capabilityfit.ReasonWrongAsset,
		capabilityfit.ReasonCapacityExceeded,
		capabilityfit.ReasonCargoFitFailed,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("reasons %q missing %s", joined, want)
		}
	}
}

func TestAnyOfIsThreeValued(t *testing.T) {
	cases := []struct {
		name  string
		facts []capabilityfit.Tri
		want  capabilityfit.Tri
	}{
		{"one yes wins", []capabilityfit.Tri{capabilityfit.No, capabilityfit.Yes}, capabilityfit.Yes},
		{"yes beats unknown", []capabilityfit.Tri{capabilityfit.Unknown, capabilityfit.Yes}, capabilityfit.Yes},
		{"unknown survives", []capabilityfit.Tri{capabilityfit.No, capabilityfit.Unknown}, capabilityfit.Unknown},
		{"all no", []capabilityfit.Tri{capabilityfit.No, capabilityfit.No}, capabilityfit.No},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyOf(tc.facts...); got != tc.want {
				t.Fatalf("anyOf(%v) = %v, want %v", tc.facts, got, tc.want)
			}
		})
	}
}

// A provider who never filled in a goods list must not thereby lose their
// truck's own purpose. This is the case that exposed the blanket
// "empty list means general cargo only" fallback.
func TestUndeclaredGoodsListFallsBackToWhatTheBodyCarries(t *testing.T) {
	tanker := surveyedBoxTruck()
	tanker.TruckType = "tanker" // no SupportedGoodsClasses declared
	assertEligible(t, Evaluate(ResolutionFound, tanker, Requirement{GoodsClasses: []string{"FUEL"}}))

	tipper := surveyedBoxTruck()
	tipper.TruckType = "tipper"
	assertEligible(t, Evaluate(ResolutionFound, tipper, Requirement{GoodsClasses: []string{"AGGREGATE"}}))

	// Nothing dangerous is ever implied by a body family.
	assertRefused(t, Evaluate(ResolutionFound, tipper, Requirement{GoodsClasses: []string{"HAZARDOUS"}}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
}
