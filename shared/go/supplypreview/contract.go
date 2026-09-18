// Package supplypreview is the one read-only planning contract Vendor uses to
// ask a logistics authority "could you probably do this, right now?" before it
// has committed to anything.
//
// It exists because Vendor's just-in-time orchestration has three distinct
// moments -- Plan, Warm Match, Commit -- and only the last one may create
// canonical execution state. Without a read-only surface, the only way for
// Vendor to learn whether supply exists would be to create a mission and see
// what happens, which is exactly the premature-dispatch problem JIT is meant
// to avoid.
//
// Everything in this package is therefore ADVISORY. A preview:
//
//   - creates no mission, booking, offer, reservation or lock,
//   - alters no provider availability,
//   - creates no payment or settlement state,
//   - and never names a provider, rider, driver or vehicle.
//
// The last point is not politeness. Returning an identity here would make the
// preview a de-facto assignment: the caller would hold a specific provider in
// mind, and Vendor selecting a provider is precisely the authority boundary
// Section 3 forbids crossing. A preview answers about SUPPLY, in aggregate,
// and nothing else.
//
// Pricing is absent for the same reason it is absent from logisticscapability:
// quoting has its own contract in shared/go/commercequote and stays there.
package supplypreview

import (
	"strings"
	"time"
)

// Path is appended to an authority's own internal commerce base, alongside the
// existing quote, mission and capability paths.
const Path = "/internal/commerce/supply-preview"

// SupplyStatus is how much supply the authority believes it has for this
// request, right now.
//
// The three values are deliberately coarse. A precise count would be both
// wrong by the time it arrived and an indirect way of profiling a market's
// provider population, so the vocabulary stops at what a planner can actually
// act on.
type SupplyStatus string

const (
	// StatusAvailable means the authority found eligible supply comfortably.
	StatusAvailable SupplyStatus = "AVAILABLE"

	// StatusLimited means eligible supply exists but is thin enough that a
	// commit may still fail to match. A planner may proceed, but should not
	// treat a match as assured.
	StatusLimited SupplyStatus = "LIMITED"

	// StatusUnknown means the authority could not determine supply -- its own
	// availability index was unreachable, or capability data it needs was
	// missing.
	//
	// UNKNOWN is NOT "none". A planner must not read it as a refusal, because
	// an authority whose index blinked is not an authority that cannot carry
	// the goods. Collapsing the two is how a dependency outage silently turns
	// into "no service in this city".
	StatusUnknown SupplyStatus = "UNKNOWN"
)

// Request is one concrete planning question.
//
// It carries the pickup point, because supply is inherently geographic, plus
// the same physical requirement shape the capability contract already uses so
// an authority can run ONE fit evaluation rather than two divergent ones.
type Request struct {
	// PickupLatitude / PickupLongitude locate the question. A preview without
	// a location can only answer about the authority as a whole, which is what
	// the capability read already does.
	PickupLatitude  float64 `json:"pickupLatitude"`
	PickupLongitude float64 `json:"pickupLongitude"`

	// DropoffLatitude / DropoffLongitude are optional and used only for
	// serviceability of the route, never to reserve anything.
	DropoffLatitude  float64 `json:"dropoffLatitude,omitempty"`
	DropoffLongitude float64 `json:"dropoffLongitude,omitempty"`

	// ProductCode is the authority's own service product, when the caller
	// knows which one it wants. Empty means "any product that fits".
	ProductCode string `json:"productCode,omitempty"`

	// The physical requirement, in the same units as the capability contract.
	WeightGrams       int64      `json:"weightGrams,omitempty"`
	VolumeMl          int64      `json:"volumeMl,omitempty"`
	LengthMm          int64      `json:"lengthMm,omitempty"`
	WidthMm           int64      `json:"widthMm,omitempty"`
	HeightMm          int64      `json:"heightMm,omitempty"`
	PackageCount      int64      `json:"packageCount,omitempty"`
	VehicleClass      string     `json:"vehicleClass,omitempty"`
	Handling          []string   `json:"handling,omitempty"`
	SecurePickup      bool       `json:"securePickup,omitempty"`
	SecureDelivery    bool       `json:"secureDelivery,omitempty"`
	PickupCount       int64      `json:"pickupCount,omitempty"`
	DropoffCount      int64      `json:"dropoffCount,omitempty"`
	RequestedPickupAt *time.Time `json:"requestedPickupAt,omitempty"`
}

// LongestDimensionMm is the largest single edge of the requested cargo.
//
// The capability contract bounds one dimension rather than three, because a
// carrier's binding constraint is almost always its longest usable edge. This
// derives that single number so a caller never has to pick one by hand and get
// it subtly wrong.
func (r Request) LongestDimensionMm() int64 {
	longest := r.LengthMm
	if r.WidthMm > longest {
		longest = r.WidthMm
	}
	if r.HeightMm > longest {
		longest = r.HeightMm
	}
	return longest
}

// Response is the authority's advisory answer.
//
// Note what is absent: no provider id, no vehicle id, no distance to a named
// provider, no count of candidates. EstimatedPickupETASeconds is the only
// supply-derived number, and it is an aggregate the caller cannot invert into
// an identity.
type Response struct {
	// Authority names the answering service.
	Authority string `json:"authority"`

	// Serviceable is the combined answer: the authority can physically carry
	// this AND believes supply exists. A false here with a StatusUnknown means
	// "ask again", not "never".
	Serviceable bool `json:"serviceable"`

	// ReasonCode is a stable code from the shared catalogue. It is
	// SERVICEABLE when Serviceable is true.
	ReasonCode string `json:"reasonCode"`

	// ProductCode is the product the authority would use, when it resolved one.
	ProductCode string `json:"productCode,omitempty"`

	// SupplyStatus is the coarse supply signal.
	SupplyStatus SupplyStatus `json:"supplyStatus"`

	// EstimatedPickupETASeconds is an aggregate estimate, omitted when supply
	// is UNKNOWN or the authority cannot estimate.
	EstimatedPickupETASeconds int64 `json:"estimatedPickupEtaSeconds,omitempty"`

	// CheckedAt is when the authority computed this answer. A preview is a
	// point-in-time observation, and the caller needs to know how stale it is.
	CheckedAt time.Time `json:"checkedAt"`

	// CapabilityVersion and PolicyVersion pin what the answer was computed
	// against, so a later commit can tell whether the ground moved.
	CapabilityVersion int64  `json:"capabilityVersion"`
	PolicyVersion     string `json:"policyVersion,omitempty"`
}

// Stable reason codes specific to supply, layered on top of the capability
// contract's physical-fit codes (which a preview returns verbatim when the
// refusal is a capability mismatch rather than a supply one).
const (
	// ReasonServiceable mirrors logisticscapability.ReasonServiceable so one
	// caller-side switch handles both contracts.
	ReasonServiceable = "SERVICEABLE"

	// ReasonNotServiceable means the location itself is outside any market
	// this authority operates.
	ReasonNotServiceable = "SERVICE_NOT_SERVICEABLE"

	// ReasonSupplyUnavailable means the authority operates here and can carry
	// the goods, but has no eligible supply right now.
	ReasonSupplyUnavailable = "SUPPLY_CURRENTLY_UNAVAILABLE"

	// ReasonNoEligibleProvider means every candidate failed hard eligibility.
	// It is distinct from ReasonSupplyUnavailable: there WERE providers, none
	// of them fit.
	ReasonNoEligibleProvider = "NO_ELIGIBLE_PROVIDER"

	// ReasonDependencyUnavailable means the authority could not answer because
	// something it depends on did not respond. It is never a refusal.
	ReasonDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"

	// ReasonProductUnavailable means no active product covers this request.
	ReasonProductUnavailable = "NETWORK_PRODUCT_UNAVAILABLE"
)

// Unknown builds the answer an authority gives when a dependency it needs did
// not respond.
//
// It is a constructor rather than a literal because the mistake it prevents is
// specific and expensive: returning Serviceable=false with a physical-mismatch
// reason when the real problem was an outage. A caller that sees
// DEPENDENCY_UNAVAILABLE knows to retry or fall back; one that sees
// WEIGHT_EXCEEDED concludes the goods will never fit and stops asking.
func Unknown(authority string, now time.Time, capabilityVersion int64) Response {
	return Response{
		Authority:         authority,
		Serviceable:       false,
		ReasonCode:        ReasonDependencyUnavailable,
		SupplyStatus:      StatusUnknown,
		CheckedAt:         now.UTC(),
		CapabilityVersion: capabilityVersion,
	}
}

// NormalizeHandling upper-cases and de-blanks handling codes so an authority
// compares the same vocabulary the capability contract declares.
func NormalizeHandling(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		trimmed := strings.ToUpper(strings.TrimSpace(code))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
