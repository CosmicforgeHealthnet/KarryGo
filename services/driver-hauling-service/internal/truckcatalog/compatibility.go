package truckcatalog

import (
	"strings"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

// SatisfiesPreferredType reports whether a truck of type truckType can serve a
// booking that asked for preferred.
//
// The old matcher compared the two strings for exact equality. That is both
// too strict and too brittle: a booking asking for "box" was refused a van
// with an identical enclosed body, while a booking asking for "trailer" was
// happily matched to an articulated unit whose actual trailer nobody had
// checked. This widens the comparison to the body family, which is what a
// customer choosing a type is really choosing, and keeps it a hard gate.
//
// An empty preference means "any truck" and matches everything.
func SatisfiesPreferredType(truckType, preferred string) bool {
	if Normalize(preferred) == "" {
		return true
	}
	wanted, ok := Lookup(preferred)
	if !ok {
		// A preference naming no catalogue type cannot be satisfied by
		// anything. It should never reach here -- booking validation refuses
		// it -- but answering "yes" to an unrecognised requirement is how an
		// unknown becomes a match.
		return false
	}
	actual, ok := Lookup(truckType)
	if !ok {
		return false
	}
	if actual.Code == wanted.Code {
		return true
	}
	// FamilyOther is the absence of a model, so two "other" trucks are not
	// thereby interchangeable and an "other" truck satisfies no specific
	// request. Family widening only means something for modelled families.
	if wanted.Family == FamilyOther || actual.Family == FamilyOther {
		return false
	}
	return actual.Family == wanted.Family
}

// DeclaredFacts is what the catalogue can say about a truck from its type
// alone, expressed in the three-valued form so an unsettled fact stays
// unsettled.
type DeclaredFacts struct {
	Enclosed          capabilityfit.Tri
	WeatherProtected  capabilityfit.Tri
	Refrigerated      capabilityfit.Tri
	UprightSafe       capabilityfit.Tri
	CarriesContainers bool
	BulkLiquidOnly    bool
	Family            BodyFamily
	VehicleClass      string
}

// FactsFor returns the type-derived facts, or an all-Unknown set when the type
// names nothing the catalogue models.
//
// The all-Unknown fallback is the important half. A truck whose type we do not
// recognise must not inherit a neighbouring type's properties, and it must not
// default to "capable"; it must simply be unable to answer, so any hard
// requirement that depends on the answer refuses it.
func FactsFor(truckType string) DeclaredFacts {
	entry, ok := Lookup(truckType)
	if !ok {
		return DeclaredFacts{Family: FamilyOther}
	}
	return DeclaredFacts{
		Enclosed:          entry.Enclosed,
		WeatherProtected:  entry.WeatherProtected,
		Refrigerated:      entry.Refrigerated,
		UprightSafe:       entry.UprightSafe,
		CarriesContainers: entry.CarriesContainers,
		BulkLiquidOnly:    entry.BulkLiquidOnly,
		Family:            entry.Family,
		VehicleClass:      string(entry.VehicleClass),
	}
}

// MatchesVehicleClass reports whether a truck of this type answers a
// cross-service vehicle-class requirement (VAN, TRUCK, ...).
//
// An empty requirement matches everything. An unknown truck type matches
// nothing, for the same reason as above.
func MatchesVehicleClass(truckType, vehicleClass string) bool {
	if vehicleClass == "" {
		return true
	}
	entry, ok := Lookup(truckType)
	if !ok {
		return false
	}
	return equalFold(string(entry.VehicleClass), vehicleClass)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'a' <= x && x <= 'z' {
			x -= 'a' - 'A'
		}
		if 'a' <= y && y <= 'z' {
			y -= 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// BodyCarriesByDefault reports whether a body family carries a goods class
// when the truck itself has declared no permit list.
//
// It exists because the obvious fallback -- "an undeclared list means general
// cargo" -- is wrong for exactly the bodies that carry nothing else. A tanker
// whose provider never filled in a goods list would be refused bulk liquid,
// which is the only thing it can physically carry, while being offered
// pallets, which it cannot. The family already encodes that truth, so the
// fallback reads it rather than assuming.
//
// It is deliberately narrow. Nothing dangerous is ever implied: HAZARDOUS and
// anything else outside the lists below always needs an explicit permit.
func BodyCarriesByDefault(family BodyFamily, goodsClass string) bool {
	class := strings.ToUpper(strings.TrimSpace(goodsClass))
	if class == "" {
		return true
	}
	switch family {
	case FamilyTank:
		// A sealed vessel carries bulk liquid or gas and nothing else.
		switch class {
		case "LIQUID_BULK", "BULK_LIQUID", "FUEL", "GAS":
			return true
		default:
			return false
		}
	case FamilyReefer:
		return class == "GENERAL" || class == "PERISHABLE" || class == "CHILLED" || class == "FROZEN"
	case FamilyTipper:
		// Tippers carry loose bulk solids as readily as general freight.
		return class == "GENERAL" || class == "BULK_SOLID" || class == "AGGREGATE"
	default:
		return class == "GENERAL"
	}
}
