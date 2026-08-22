// Package commercequote is the single source of truth for the internal
// commerce delivery-quote contract.
//
// Vendor Service (Section 17C fulfilment planning) asks a logistics authority
// two questions in one call: "can you serve this route with this load", and
// "what would it cost". The authority answers both, persists the answer, and
// returns a reference to it.
//
// The contract lives in shared/go for the same reason
// shared/go/notifications does: the caller and every implementing service must
// agree on it exactly, and three private copies of a wire shape are three
// places for it to drift. It is a SCHEMA and a CLIENT, not a pricing engine —
// each logistics service prices with its own formula
// (shared/go/dispatchpricing for dispatch, hauling's own fare model for
// hauling) and this package never re-derives a fare.
//
// Two rules the implementing services must hold to, because the whole point
// of an authoritative quote collapses without them:
//
//  1. THE QUOTE IS PERSISTED BEFORE IT IS RETURNED. A quote reference that
//     names no stored row is not evidence of anything, and a later section
//     could not verify a fee against it.
//
//  2. THE FEE IS THE AUTHORITY'S OWN. Vendor Service must never compute a
//     platform delivery fee, so a service answering here must price with its
//     real formula rather than echoing anything the caller sent.
package commercequote

import "time"

// SignatureCaller is the service name Vendor Service signs its quote requests
// as. Implementing services allowlist exactly this.
const SignatureCaller = "vendor-service"

// QuotePath is the path every implementing service mounts the quote endpoint
// at, relative to its own internal API base.
const QuotePath = "/internal/commerce/quotes"

// VehicleClass is the carrier class the caller has determined the load needs.
//
// CAR is deliberately absent. driver-taxi-service has no production
// goods-delivery capability, and a class in this contract would be an
// invitation to call one.
type VehicleClass string

const (
	VehicleMotorcycle VehicleClass = "MOTORCYCLE"
	VehicleVan        VehicleClass = "VAN"
	VehicleTruck      VehicleClass = "TRUCK"
)

// Valid reports whether a class is one this contract carries.
func (v VehicleClass) Valid() bool {
	switch v {
	case VehicleMotorcycle, VehicleVan, VehicleTruck:
		return true
	default:
		return false
	}
}

// Urgency is the caller's service urgency for the load.
type Urgency string

const (
	UrgencyStandard  Urgency = "STANDARD"
	UrgencyImmediate Urgency = "IMMEDIATE"
)

// HandlingCapability is something the carrier must actually be able to do.
// The vocabulary matches vendor-service's fulfilment requirements so a
// capability never has to be re-mapped in transit.
const (
	CapabilityHotHold         = "HOT_HOLD"
	CapabilityInsulated       = "INSULATED"
	CapabilityColdChain       = "COLD_CHAIN"
	CapabilityFrozenChain     = "FROZEN_CHAIN"
	CapabilityUprightCarry    = "UPRIGHT_CARRY"
	CapabilityFragileCarry    = "FRAGILE_CARRY"
	CapabilityLeakContainment = "LEAK_CONTAINMENT"
)

// Reason codes. SERVICEABLE is the only one that accompanies a fee.
const (
	ReasonServiceable = "SERVICEABLE"

	// ReasonNoProviderAvailable means nobody is online and reachable for this
	// route right now. It is the honest "we cannot serve you" answer and the
	// one that must never be papered over with a fee.
	ReasonNoProviderAvailable = "NO_PROVIDER_AVAILABLE"

	// ReasonOutOfServiceArea means the route falls outside where this
	// authority operates.
	ReasonOutOfServiceArea = "OUT_OF_SERVICE_AREA"

	// ReasonVehicleClassUnsupported means this authority does not carry the
	// requested class at all — a van asked of dispatch, a motorcycle asked of
	// hauling.
	ReasonVehicleClassUnsupported = "VEHICLE_CLASS_UNSUPPORTED"

	// ReasonLoadExceedsCapacity means no vehicle of the requested class that
	// this authority can offer will carry the load.
	ReasonLoadExceedsCapacity = "LOAD_EXCEEDS_CAPACITY"

	// ReasonHandlingUnsupported means the authority cannot meet a required
	// handling capability — a cold chain being the case that matters.
	ReasonHandlingUnsupported = "HANDLING_UNSUPPORTED"

	// ReasonInvalidRoute means a coordinate is missing, out of range or null
	// island.
	ReasonInvalidRoute = "INVALID_ROUTE"

	// ReasonCurrencyUnsupported means the authority does not price in the
	// requested currency. It refuses rather than converting, because no
	// service in this platform holds exchange-rate authority.
	ReasonCurrencyUnsupported = "CURRENCY_UNSUPPORTED"
)

// Point is one end of a route.
type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`

	// Address is display evidence only. No authority may price from it.
	Address string `json:"address,omitempty"`
}

// Load is what is actually being carried.
//
// Every measurement is a caller statement of what it knows. WeightEvidence
// travels with it so an authority can tell a measured 4 kg from an unmeasured
// one and refuse to promise more than the evidence supports.
type Load struct {
	PackageCount int64 `json:"package_count"`

	TotalWeightGrams int64  `json:"total_weight_grams"`
	WeightEvidence   string `json:"weight_evidence,omitempty"`

	TotalVolumeMl int64 `json:"total_volume_ml,omitempty"`

	LongestDimensionMm int `json:"longest_dimension_mm,omitempty"`

	LargeLoad bool `json:"large_load,omitempty"`

	Capabilities []string `json:"capabilities,omitempty"`
}

// Request is one quote request.
type Request struct {
	// QuoteReference is the caller-owned idempotency key. Repeating it with
	// an identical request returns the same quote; repeating it with changed
	// inputs produces a new revision of the same reference.
	QuoteReference string `json:"quote_reference"`

	VehicleClass VehicleClass `json:"vehicle_class"`

	Pickup  Point `json:"pickup"`
	Dropoff Point `json:"dropoff"`

	Load Load `json:"load"`

	Urgency Urgency `json:"urgency,omitempty"`

	// Currency is the caller's expected currency. An authority that cannot
	// quote in it refuses rather than converting: no service in this platform
	// holds exchange-rate authority.
	Currency string `json:"currency"`

	// CorrelationID ties the quote to the checkout it was requested for, for
	// operator tracing only. It carries no authority.
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Breakdown is the display decomposition of a fee. Components are indicative;
// FeeMinor is the authoritative total.
type Breakdown struct {
	BaseMinor      int64 `json:"base_minor"`
	DistanceMinor  int64 `json:"distance_minor"`
	SurchargeMinor int64 `json:"surcharge_minor"`
	HandlingMinor  int64 `json:"handling_minor"`
}

// Quote is the authority's persisted answer.
type Quote struct {
	// QuoteID is the authority's own identity for the stored quote. It is the
	// handle a later section presents when it wants the quote verified.
	QuoteID string `json:"quote_id"`

	QuoteReference string `json:"quote_reference"`

	// Revision increments when the same reference is re-quoted with changed
	// inputs. It is what lets a caller notice that the quote it holds is not
	// the quote the authority now has.
	Revision int64 `json:"revision"`

	// Service names the answering authority, so a plan records which one
	// spoke rather than inferring it from the class.
	Service string `json:"service"`

	Serviceable bool   `json:"serviceable"`
	ReasonCode  string `json:"reason_code"`

	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	// PickupRef and DropoffRef are the authority's own handles for the two
	// route endpoints this quote is bound to. A mission created later must
	// present them, which is what stops a quote for one route being spent on
	// another.
	PickupRef  string `json:"pickup_ref,omitempty"`
	DropoffRef string `json:"dropoff_ref,omitempty"`

	DistanceKm       float64 `json:"distance_km"`
	EstimatedMinutes int     `json:"estimated_minutes"`

	FeeMinor  int64     `json:"fee_minor"`
	Currency  string    `json:"currency,omitempty"`
	Breakdown Breakdown `json:"breakdown"`

	ExpiresAt time.Time `json:"expires_at"`

	// AvailableProviders is the serviceability evidence behind the answer: how
	// many carriers the authority could actually see for this route. Zero with
	// Serviceable true would be a contradiction and implementing services must
	// not produce it.
	AvailableProviders int `json:"available_providers"`

	QuotedAt time.Time `json:"quoted_at"`
}

// Expired reports whether the quote has lapsed at the given instant.
func (q Quote) Expired(now time.Time) bool {
	return !q.ExpiresAt.After(now)
}

// Envelope is the platform success envelope the quote endpoint answers in.
type Envelope struct {
	Success bool  `json:"success"`
	Data    Quote `json:"data"`
}

// Validate checks a request is coherent before it is signed or answered.
//
// It is exported so the caller and the server run the SAME check: a request
// the client would refuse to send must not be one the server would happily
// answer.
func (r Request) Validate() (string, bool) {
	if trimmed := trim(r.QuoteReference); trimmed == "" {
		return "quote_reference is required", false
	}

	if !r.VehicleClass.Valid() {
		return "vehicle_class is not a supported class", false
	}

	if !validCoordinate(r.Pickup.Latitude, r.Pickup.Longitude) {
		return "pickup coordinates are missing or invalid", false
	}

	if !validCoordinate(r.Dropoff.Latitude, r.Dropoff.Longitude) {
		return "dropoff coordinates are missing or invalid", false
	}

	if len(trim(r.Currency)) != 3 {
		return "currency must be a three-letter code", false
	}

	if r.Load.PackageCount <= 0 {
		return "load.package_count must be positive", false
	}

	if r.Load.TotalWeightGrams < 0 || r.Load.TotalVolumeMl < 0 ||
		r.Load.LongestDimensionMm < 0 {
		return "load measurements must not be negative", false
	}

	return "", true
}
