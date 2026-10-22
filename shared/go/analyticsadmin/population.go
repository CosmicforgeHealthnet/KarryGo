package analyticsadmin

// Unattested fact populations.
//
// A scoped coverage manifest attests every fact an owner published for one
// scope, and a fact belongs to a scope only through the owner's own signed
// authorization context on the resource the fact is anchored to. Some facts an
// owner publishes are about records that carry no owner-recorded market, zone,
// organization or tenant and have no enrolled administrative resource kind, so
// no context can ever place them in a scope. Such a fact appears in no scoped
// manifest. Left implicit, that would let a report over the population read
// as COMPLETE while silently omitting all of it.
//
// This is the closed, owner-reviewed declaration of those populations. It is
// consumed on both sides of the contract: an owner's producer counts the facts
// as unattested instead of anchoring them, and Analytics refuses a COMPLETE
// release for any (authority, product, fact kind) combination listed here,
// falling back to the bounded PARTIAL path. Nothing is inferred from
// aggregates and no scope is assigned to these records. A population leaves
// this list only when its records have an enrolled resource kind whose binding
// is drawn from the owner's own rows, so that an owner-signed context (never
// an assumption) proves each record's scope.

import (
	"strings"

	"cosmicforge/logistics/shared/go/networkanalytics"
)

// UnattestedPopulation is one population whose scope membership the owner
// cannot attest. Kinds empty means every fact kind of the product.
type UnattestedPopulation struct {
	Owner, Authority, ProductCode string
	// EntityTypes are the delivered aggregate types (compared
	// case-insensitively) carrying the population's facts. An owner producer
	// must never anchor them.
	EntityTypes []string
	Kinds       []networkanalytics.Kind
	Reason      string
}

func kindsOf(names ...string) []networkanalytics.Kind {
	out := []networkanalytics.Kind{}
	for _, name := range names {
		if _, kind, ok := networkanalytics.Classify(name, ""); ok {
			out = append(out, kind)
		}
	}
	return out
}

// declaredPopulations is the closed declaration. It is empty: every
// population an owner publishes is now anchored to an enrolled resource kind
// whose owner-signed context alone places a record in a scope (Dispatch
// bookings, delivery missions and commerce missions; Taxi goods missions and
// corporate rides). A population returns here only when its records lose
// that binding. Unexported so only this package's tests can substitute it.
var declaredPopulations = func() []UnattestedPopulation { return nil }

// UnattestedPopulations returns the closed declaration (a fresh copy).
func UnattestedPopulations() []UnattestedPopulation {
	return append([]UnattestedPopulation(nil), declaredPopulations()...)
}

// UnattestedEntityType reports whether an owner's delivered aggregate type
// carries a declared unattested population. Producers must not anchor it.
func UnattestedEntityType(owner, entityType string) bool {
	for _, p := range UnattestedPopulations() {
		if p.Owner != owner {
			continue
		}
		for _, t := range p.EntityTypes {
			if strings.EqualFold(t, strings.TrimSpace(entityType)) {
				return true
			}
		}
	}
	return false
}

// PopulationAttested is false when any declared unattested population of the
// authority and product contributes a fact kind the report reads. A scoped
// report for such a population can never be COMPLETE.
func PopulationAttested(authority, productCode string, kinds []networkanalytics.Kind) bool {
	return PopulationAttestedUnder(UnattestedPopulations(), authority, productCode, kinds)
}

// PopulationAttestedUnder evaluates the rule against an explicit declaration.
func PopulationAttestedUnder(declared []UnattestedPopulation, authority, productCode string, kinds []networkanalytics.Kind) bool {
	for _, p := range declared {
		if p.Authority != authority || p.ProductCode != productCode {
			continue
		}
		if len(p.Kinds) == 0 {
			return false
		}
		for _, k := range p.Kinds {
			for _, read := range kinds {
				if k == read {
					return false
				}
			}
		}
	}
	return true
}
