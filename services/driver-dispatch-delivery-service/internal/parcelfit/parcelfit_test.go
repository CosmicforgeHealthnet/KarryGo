package parcelfit

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

// A fully-surveyed dispatch bike with a hard top box. Tests start here and
// break one thing at a time, so a refusal always has exactly one cause.
func surveyedBike() Capability {
	return Capability{
		BikeID: "bike-1", ProviderID: "provider-1", BikeType: "dispatch_bike",
		Active: true, Verified: true,
		MaxPayloadGrams: 20_000, MaxPayloadGramsKnown: true,
		BoxLengthMm: 500, BoxWidthMm: 400, BoxHeightMm: 400, BoxDimsKnown: true,
		EnclosedBox: capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		UprightSafe: capabilityfit.Yes, FragileSafe: capabilityfit.Yes,
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

// ── Resolution ────────────────────────────────────────────────────────────────

// The legacy candidate SQL cannot express these three outcomes: an unreachable
// store, an unsurveyed bike and a genuine mismatch all just made the EXISTS
// clause fail. They must now be distinguishable.
func TestUnresolvedBikeCannotReachRanking(t *testing.T) {
	requirement := Requirement{WeightGrams: 5_000}

	missing := Evaluate(ResolutionMissing, Capability{}, requirement)
	assertRefused(t, missing, capabilityfit.ReasonCapabilityUnknown)

	unattempted := Evaluate(ResolutionUnattempted, Capability{}, requirement)
	assertRefused(t, unattempted, capabilityfit.ReasonDependencyUnavailable)

	if missing.PrimaryReason() == unattempted.PrimaryReason() {
		t.Fatal("an unsurveyed bike and a store outage share a reason code")
	}
}

func TestDependencyOutageIsDistinctFromPhysicalMismatch(t *testing.T) {
	outage := Evaluate(ResolutionFailed, Capability{}, Requirement{WeightGrams: 1})
	tooHeavy := Evaluate(ResolutionFound, surveyedBike(), Requirement{WeightGrams: 40_000})

	if outage.PrimaryReason() != capabilityfit.ReasonDependencyUnavailable {
		t.Fatalf("outage reason = %s", outage.PrimaryReason())
	}
	if tooHeavy.PrimaryReason() != capabilityfit.ReasonCapacityExceeded {
		t.Fatalf("mismatch reason = %s", tooHeavy.PrimaryReason())
	}
}

func TestInactiveOrUnverifiedBikeIsRefused(t *testing.T) {
	inactive := surveyedBike()
	inactive.Active = false
	assertRefused(t, Evaluate(ResolutionFound, inactive, Requirement{}),
		capabilityfit.ReasonProviderNotEligible)

	unverified := surveyedBike()
	unverified.Verified = false
	assertRefused(t, Evaluate(ResolutionFound, unverified, Requirement{}),
		capabilityfit.ReasonProviderNotEligible)
}

// ── Payload ───────────────────────────────────────────────────────────────────

func TestPayloadBoundaryIsInclusive(t *testing.T) {
	bike := surveyedBike()
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{WeightGrams: 20_000}))
	assertRefused(t, Evaluate(ResolutionFound, bike, Requirement{WeightGrams: 20_001}),
		capabilityfit.ReasonCapacityExceeded)
}

// An unmeasured payload is unknown, not unlimited. Nothing in the legacy SQL
// checked payload at all, so this is the case where "we simply did not know"
// used to mean "yes".
func TestUnmeasuredPayloadRefusesRatherThanSkips(t *testing.T) {
	bike := surveyedBike()
	bike.MaxPayloadGrams, bike.MaxPayloadGramsKnown = 0, false
	assertRefused(t, Evaluate(ResolutionFound, bike, Requirement{WeightGrams: 5_000}),
		capabilityfit.ReasonCapabilityUnknown)
	// A parcel that states no weight does not depend on the missing fact.
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{}))
}

// ── Box geometry ──────────────────────────────────────────────────────────────

func TestParcelMustFitTheBoxInSomeOrientation(t *testing.T) {
	bike := surveyedBike() // 500 x 400 x 400

	// Exactly the box.
	assertEligible(t, Evaluate(ResolutionFound, bike,
		Requirement{LengthMm: 500, WidthMm: 400, HeightMm: 400}))

	// Same volume, wrong orientation as presented -- but it rotates in.
	assertEligible(t, Evaluate(ResolutionFound, bike,
		Requirement{LengthMm: 400, WidthMm: 500, HeightMm: 400}))

	// One millimetre too long on the longest axis: nothing can be rotated to
	// fix that.
	assertRefused(t, Evaluate(ResolutionFound, bike,
		Requirement{LengthMm: 501, WidthMm: 100, HeightMm: 100}),
		capabilityfit.ReasonCargoFitFailed)

	// Fits by total volume but not by shape.
	assertRefused(t, Evaluate(ResolutionFound, bike,
		Requirement{LengthMm: 450, WidthMm: 450, HeightMm: 100}),
		capabilityfit.ReasonCargoFitFailed)
}

// An upright-only parcel cannot be rotated to fit, so it has to clear the box
// on its own axes. Rotating it is exactly the handling the requirement forbids.
func TestUprightParcelCannotBeRotatedIntoTheBox(t *testing.T) {
	bike := surveyedBike() // 500 x 400 x 400
	rotatable := Requirement{LengthMm: 400, WidthMm: 500, HeightMm: 300}
	assertEligible(t, Evaluate(ResolutionFound, bike, rotatable))

	upright := rotatable
	upright.UprightRequired = true
	assertRefused(t, Evaluate(ResolutionFound, bike, upright), capabilityfit.ReasonCargoFitFailed)
}

func TestUnmeasuredBoxRefusesADimensionedParcel(t *testing.T) {
	bike := surveyedBike()
	bike.BoxDimsKnown = false
	assertRefused(t, Evaluate(ResolutionFound, bike,
		Requirement{LengthMm: 200, WidthMm: 200, HeightMm: 200}),
		capabilityfit.ReasonCapabilityUnknown)
}

// A parcel with no stated dimensions must keep matching, or every legacy
// booking would stop dispatching the day this shipped.
func TestUnstatedDimensionsStayEligibleUnlessTheProductRequiresThem(t *testing.T) {
	bike := surveyedBike()
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{WeightGrams: 3_000}))

	required := Requirement{WeightGrams: 3_000, DimensionsRequired: true}
	assertRefused(t, Evaluate(ResolutionFound, bike, required), capabilityfit.ReasonCapabilityUnknown)
}

// ── Handling ──────────────────────────────────────────────────────────────────

func TestHandlingMismatches(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Capability)
		request Requirement
		reason  string
	}{
		{
			"fragile on a bike with no fragile handling",
			func(c *Capability) { c.FragileSafe = capabilityfit.No },
			Requirement{Fragile: true},
			capabilityfit.ReasonRequiredCapabilityUnavailable,
		},
		{
			"fragile on an unsurveyed bike",
			func(c *Capability) { c.FragileSafe = capabilityfit.Unknown },
			Requirement{Fragile: true},
			capabilityfit.ReasonCapabilityUnknown,
		},
		{
			"upright on a bike that cannot promise it",
			func(c *Capability) { c.UprightSafe = capabilityfit.No },
			Requirement{UprightRequired: true},
			capabilityfit.ReasonRequiredCapabilityUnavailable,
		},
		{
			"enclosure on an open rack",
			func(c *Capability) { c.EnclosedBox = capabilityfit.No },
			Requirement{EnclosureRequired: true},
			capabilityfit.ReasonRequiredCapabilityUnavailable,
		},
		{
			"weather protection with neither cover nor box",
			func(c *Capability) {
				c.WeatherProtected = capabilityfit.No
				c.EnclosedBox = capabilityfit.No
			},
			Requirement{WeatherProtectionRequired: true},
			capabilityfit.ReasonRequiredCapabilityUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bike := surveyedBike()
			tc.mutate(&bike)
			assertRefused(t, Evaluate(ResolutionFound, bike, tc.request), tc.reason)
		})
	}
}

// An enclosed box is weatherproof by construction, so it satisfies the weaker
// requirement even when nobody ticked the weather box separately.
func TestEnclosedBoxSatisfiesWeatherProtection(t *testing.T) {
	bike := surveyedBike()
	bike.WeatherProtected = capabilityfit.Unknown
	bike.EnclosedBox = capabilityfit.Yes
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{WeatherProtectionRequired: true}))
}

// ── Temperature ───────────────────────────────────────────────────────────────

func TestThermalRequirements(t *testing.T) {
	noBag := surveyedBike()
	noBag.ThermalBag = capabilityfit.No
	assertRefused(t, Evaluate(ResolutionFound, noBag, Requirement{TemperatureClass: "CHILLED"}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)

	unsurveyed := surveyedBike() // ThermalBag left Unknown
	assertRefused(t, Evaluate(ResolutionFound, unsurveyed, Requirement{TemperatureClass: "CHILLED"}),
		capabilityfit.ReasonCapabilityUnknown)

	// A bag with no declared class cannot promise a class.
	unclassified := surveyedBike()
	unclassified.ThermalBag = capabilityfit.Yes
	assertRefused(t, Evaluate(ResolutionFound, unclassified, Requirement{TemperatureClass: "FROZEN"}),
		capabilityfit.ReasonCapabilityUnknown)

	// A chilled bag does not keep ice cream frozen.
	chilled := surveyedBike()
	chilled.ThermalBag, chilled.TemperatureClass = capabilityfit.Yes, "CHILLED"
	assertRefused(t, Evaluate(ResolutionFound, chilled, Requirement{TemperatureClass: "FROZEN"}),
		capabilityfit.ReasonRequiredCapabilityUnavailable)
	assertEligible(t, Evaluate(ResolutionFound, chilled, Requirement{TemperatureClass: "CHILLED"}))

	// A frozen bag does serve chilled.
	frozen := surveyedBike()
	frozen.ThermalBag, frozen.TemperatureClass = capabilityfit.Yes, "FROZEN"
	assertEligible(t, Evaluate(ResolutionFound, frozen, Requirement{TemperatureClass: "CHILLED"}))
}

func TestAmbientNeedsNoThermalEquipment(t *testing.T) {
	bike := surveyedBike()
	bike.ThermalBag = capabilityfit.No
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{TemperatureClass: "AMBIENT"}))
	assertEligible(t, Evaluate(ResolutionFound, bike, Requirement{}))
}

// ── Certification ─────────────────────────────────────────────────────────────

// A bike_type is not a certificate, and an unrecorded admission is not an
// admission. Both Unknown and No must refuse.
func TestCertificationRequiredRefusesAnUnadmittedBike(t *testing.T) {
	requirement := Requirement{
		ProductCode: "DISPATCH_COLD_CHAIN", CertificationRequired: true,
	}

	unadmitted := surveyedBike() // ProductEligibility left Unknown
	assertRefused(t, Evaluate(ResolutionFound, unadmitted, requirement),
		capabilityfit.ReasonAssetNotCertified)

	refused := surveyedBike()
	refused.ProductEligibility = capabilityfit.No
	refused.ProductEligibilityNote = "eligibility for DISPATCH_COLD_CHAIN is SUSPENDED"
	decision := Evaluate(ResolutionFound, refused, requirement)
	assertRefused(t, decision, capabilityfit.ReasonAssetNotCertified)
	if !strings.Contains(decision.Details[capabilityfit.ReasonAssetNotCertified], "SUSPENDED") {
		t.Fatalf("refusal lost the reason: %v", decision.Details)
	}

	admitted := surveyedBike()
	admitted.ProductEligibility = capabilityfit.Yes
	assertEligible(t, Evaluate(ResolutionFound, admitted, requirement))
}

// A product that needs no certificate must not require one.
func TestUncertifiedProductNeedsNoAdmission(t *testing.T) {
	bike := surveyedBike() // ProductEligibility Unknown
	assertEligible(t, Evaluate(ResolutionFound, bike,
		Requirement{ProductCode: "DISPATCH_STANDARD", CertificationRequired: false}))
}

// ── Aggregate ─────────────────────────────────────────────────────────────────

func TestEveryFailureReasonIsReported(t *testing.T) {
	bike := surveyedBike()
	bike.FragileSafe = capabilityfit.No
	decision := Evaluate(ResolutionFound, bike, Requirement{
		WeightGrams: 90_000, LengthMm: 2_000, WidthMm: 100, HeightMm: 100, Fragile: true,
	})
	joined := decision.ReasonString()
	for _, want := range []string{
		capabilityfit.ReasonCapacityExceeded,
		capabilityfit.ReasonCargoFitFailed,
		capabilityfit.ReasonRequiredCapabilityUnavailable,
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
