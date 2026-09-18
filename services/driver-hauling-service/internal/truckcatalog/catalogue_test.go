package truckcatalog

import (
	"testing"

	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/commercequote"
)

// The catalogue must still cover every value the two old lists carried, or the
// consolidation would silently orphan persisted rows.
func TestCatalogueCoversEveryHistoricalTruckType(t *testing.T) {
	historical := []string{
		"flatbed", "container", "tipper", "van", "refrigerated", "pickup",
		"box", "tanker", "trailer", "dump", "lowbed", "crane", "other",
	}
	for _, code := range historical {
		if _, ok := Lookup(code); !ok {
			t.Fatalf("historical truck type %q is not in the catalogue", code)
		}
		if !ValidForProvider(code) {
			t.Fatalf("historical truck type %q is no longer provider-selectable", code)
		}
		if !ValidForBooking(code) {
			t.Fatalf("historical truck type %q is no longer bookable", code)
		}
	}
	if len(All()) != len(historical) {
		t.Fatalf("catalogue has %d entries, historical lists had %d", len(All()), len(historical))
	}
}

func TestLookupNormalizesCaseAndWhitespace(t *testing.T) {
	entry, ok := Lookup("  ReFrIgErAtEd ")
	if !ok {
		t.Fatal("mixed-case value did not resolve")
	}
	if entry.Code != TypeRefrigerated {
		t.Fatalf("code = %s, want %s", entry.Code, TypeRefrigerated)
	}
	if got := Canonical(" BOX "); got != "box" {
		t.Fatalf("canonical = %q, want box", got)
	}
	if got := Canonical("hovercraft"); got != "" {
		t.Fatalf("unknown canonical = %q, want empty", got)
	}
}

// An empty preferred type means "any truck" and has always been valid. A
// booking that does not name a type must not start failing validation.
func TestEmptyPreferredTypeStaysValidForBookings(t *testing.T) {
	if !ValidForBooking("") {
		t.Fatal("empty preferred type was rejected")
	}
	if !ValidForBooking("   ") {
		t.Fatal("whitespace preferred type was rejected")
	}
	// A provider, by contrast, must name a real type: a truck with no type
	// cannot be matched against anything.
	if ValidForProvider("") {
		t.Fatal("empty type was accepted for provider registration")
	}
}

func TestUnknownTypeIsRejectedEverywhere(t *testing.T) {
	for _, value := range []string{"hovercraft", "lorry", "18-wheeler"} {
		if ValidForProvider(value) || ValidForBooking(value) || Known(value) {
			t.Fatalf("unmodelled type %q was accepted", value)
		}
	}
}

// Facts the type name genuinely settles are declared; everything else stays
// Unknown so it fails closed rather than passing silently.
func TestTypeDerivedFactsOnlyClaimWhatTheNameSettles(t *testing.T) {
	reefer := FactsFor("refrigerated")
	if reefer.Refrigerated != capabilityfit.Yes {
		t.Fatalf("refrigerated truck: Refrigerated = %v, want Yes", reefer.Refrigerated)
	}
	if reefer.Enclosed != capabilityfit.Yes {
		t.Fatalf("refrigerated truck: Enclosed = %v, want Yes", reefer.Enclosed)
	}

	flatbed := FactsFor("flatbed")
	if flatbed.Enclosed != capabilityfit.No || flatbed.WeatherProtected != capabilityfit.No {
		t.Fatalf("flatbed claims enclosure: %+v", flatbed)
	}

	// A tipper discharges by tilting, so it cannot promise orientation.
	tipper := FactsFor("tipper")
	if tipper.UprightSafe != capabilityfit.No {
		t.Fatalf("tipper: UprightSafe = %v, want No", tipper.UprightSafe)
	}

	// An articulated unit's body is whatever trailer is coupled to it, so the
	// name settles nothing. This is the case a naive mapping gets wrong.
	trailer := FactsFor("trailer")
	if trailer.Enclosed != capabilityfit.Unknown {
		t.Fatalf("trailer: Enclosed = %v, want Unknown", trailer.Enclosed)
	}
	if trailer.Refrigerated != capabilityfit.Unknown {
		t.Fatalf("trailer: Refrigerated = %v, want Unknown", trailer.Refrigerated)
	}

	// "other" is the absence of a model and must claim nothing at all.
	other := FactsFor("other")
	if other.Enclosed != capabilityfit.Unknown || other.WeatherProtected != capabilityfit.Unknown ||
		other.Refrigerated != capabilityfit.Unknown || other.UprightSafe != capabilityfit.Unknown {
		t.Fatalf("other claims a fact: %+v", other)
	}
}

// An unrecognised type must inherit nothing from its neighbours.
func TestUnknownTypeDeclaresNothing(t *testing.T) {
	facts := FactsFor("hovercraft")
	if facts.Enclosed != capabilityfit.Unknown || facts.Refrigerated != capabilityfit.Unknown ||
		facts.WeatherProtected != capabilityfit.Unknown || facts.UprightSafe != capabilityfit.Unknown {
		t.Fatalf("unknown type declared facts: %+v", facts)
	}
	if facts.CarriesContainers || facts.BulkLiquidOnly {
		t.Fatalf("unknown type claimed a capability: %+v", facts)
	}
}

func TestSatisfiesPreferredTypeWidensToFamily(t *testing.T) {
	cases := []struct {
		name      string
		truck     string
		preferred string
		want      bool
	}{
		{"any truck when no preference", "tanker", "", true},
		{"exact match", "flatbed", "flatbed", true},
		{"same enclosed family", "van", "box", true},
		{"same tipper family", "dump", "tipper", true},
		{"different family refused", "flatbed", "refrigerated", false},
		{"open deck cannot serve enclosed", "flatbed", "box", false},
		{"unknown truck refused", "hovercraft", "box", false},
		{"unknown preference refused", "box", "hovercraft", false},
		{"other never widens to a modelled family", "other", "box", false},
		{"modelled type never satisfies other", "box", "other", false},
		{"other matches itself exactly", "other", "other", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SatisfiesPreferredType(tc.truck, tc.preferred); got != tc.want {
				t.Fatalf("SatisfiesPreferredType(%q,%q) = %v, want %v",
					tc.truck, tc.preferred, got, tc.want)
			}
		})
	}
}

func TestVehicleClassMappingIsTotalAndStable(t *testing.T) {
	for _, entry := range All() {
		switch entry.VehicleClass {
		case commercequote.VehicleVan, commercequote.VehicleTruck:
		default:
			t.Fatalf("%s maps to %q, which Hauling does not declare",
				entry.Code, entry.VehicleClass)
		}
	}
	if !MatchesVehicleClass("van", "VAN") {
		t.Fatal("van did not match VAN")
	}
	if MatchesVehicleClass("van", "TRUCK") {
		t.Fatal("van matched TRUCK")
	}
	if !MatchesVehicleClass("van", "") {
		t.Fatal("empty class requirement did not match")
	}
	if MatchesVehicleClass("hovercraft", "TRUCK") {
		t.Fatal("unknown type matched a class requirement")
	}
}

func TestEveryEntryHasAValidFamilyAndUniqueCode(t *testing.T) {
	seen := map[Type]bool{}
	for _, entry := range All() {
		if seen[entry.Code] {
			t.Fatalf("duplicate code %s", entry.Code)
		}
		seen[entry.Code] = true
		if !entry.Family.Valid() {
			t.Fatalf("%s has unmodelled family %s", entry.Code, entry.Family)
		}
		if entry.Label == "" {
			t.Fatalf("%s has no customer-facing label", entry.Code)
		}
	}
}

func TestInFamilyGroupsCorrectly(t *testing.T) {
	tippers := Codes(InFamily(FamilyTipper))
	if len(tippers) != 2 {
		t.Fatalf("tipper family = %v, want dump and tipper", tippers)
	}
	if tippers[0] != "dump" || tippers[1] != "tipper" {
		t.Fatalf("tipper family not sorted: %v", tippers)
	}
}
