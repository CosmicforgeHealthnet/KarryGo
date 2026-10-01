// Package engine is pricing-service's calculator: pure, I/O-free code that turns
// a normalized request plus a set of policy bodies into an economic decision.
//
// It has no database, no clock of its own and no network. That is what makes a
// quote replayable: to re-derive a price from months ago, load the exact policy
// versions the quote recorded and evaluate again. An evaluator that could reach
// a database or read time.Now could never be replayed, and reconciliation
// against the original rules would be impossible.
//
// Two rules govern everything here:
//
//  1. Arithmetic is exact. Whole kobo, whole metres, whole seconds. Rates are
//     integers over an integer unit, multipliers exact rationals, and every
//     rounding step names its mode. The architecture test forbids float64.
//  2. An evaluator NEVER changes behaviour for an existing schema version.
//     Frozen golden fixtures per (kind, schema_version) enforce it. A change in
//     behaviour requires a new schema version, because a quote pins the schema
//     version it was priced with.
package engine

import (
	"errors"
	"fmt"
	"time"
)

// Scope is the service being priced.
type Scope string

const (
	ScopeTaxi     Scope = "TAXI"
	ScopeDispatch Scope = "DISPATCH"
	ScopeHaulage  Scope = "HAULAGE"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeTaxi, ScopeDispatch, ScopeHaulage:
		return true
	}
	return false
}

// Channel is how the request reached KarryGo. It matters commercially: a
// commerce delivery and a customer-app delivery can carry different commission
// even at the same price, which is why channel is an input rather than an
// afterthought.
type Channel string

const (
	ChannelCustomerApp Channel = "CUSTOMER_APP"
	ChannelPartnerAPI  Channel = "PARTNER_API"
	ChannelCommerce    Channel = "COMMERCE"
	ChannelOps         Channel = "OPS"
)

// Valid reports whether c is a known channel.
func (c Channel) Valid() bool {
	switch c {
	case ChannelCustomerApp, ChannelPartnerAPI, ChannelCommerce, ChannelOps:
		return true
	}
	return false
}

// Distance bases. STRAIGHT_LINE reproduces the legacy behaviour of several
// surfaces; it is named explicitly so that a policy states which it means rather
// than inheriting whatever the caller happened to send.
const (
	DistanceBasisRoadRoute    = "ROAD_ROUTE"
	DistanceBasisStraightLine = "STRAIGHT_LINE"
)

// Route confidence, as reported by the caller. Pricing does not route; it
// records how good the route it was given was.
const (
	RouteConfidenceHigh   = "HIGH"
	RouteConfidenceMedium = "MEDIUM"
	RouteConfidenceLow    = "LOW"
)

// Flags recorded on a result. They are evidence for reports, not errors: a
// degraded route still gets a price in V1, because the legacy formulas priced
// one silently and the migration must stay price-neutral. A later policy version
// can refuse instead.
const (
	FlagRouteDegraded  = "ROUTE_DEGRADED"
	FlagRouteLowConf   = "ROUTE_LOW_CONFIDENCE"
	FlagMinimumApplied = "MINIMUM_FARE_APPLIED"
)

// Component codes. A result's components always sum exactly to its gross, so the
// arithmetic is auditable line by line.
const (
	ComponentBase          = "BASE"
	ComponentDistance      = "DISTANCE"
	ComponentTime          = "TIME"
	ComponentSizeSurcharge = "SIZE_SURCHARGE"
	ComponentExpress       = "EXPRESS_SURCHARGE"
	ComponentWeight        = "WEIGHT_SURCHARGE"
	ComponentHelpers       = "HELPERS"
	ComponentRounding      = "ROUNDING"
	ComponentMinimumTopUp  = "MINIMUM_TOPUP"
)

// Request is the normalized, already-validated input to a price calculation.
//
// Distances are whole metres and durations whole seconds. The caller converts
// from whatever it holds; the engine never sees a fractional kilometre.
type Request struct {
	Scope   Scope
	Channel Channel
	// Class is the vehicle or service class: economy/comfort/xl for taxi,
	// MOTORCYCLE/VAN/TRUCK for dispatch and haulage.
	Class string

	DistanceMeters  int64
	DurationSeconds int64
	// TrafficDurationSeconds is 0 when unknown. A policy uses it only if its
	// time basis says so, and a policy that charges traffic-aware time must not
	// also carry a traffic market-pressure signal, or traffic is paid for twice.
	TrafficDurationSeconds int64

	RouteConfidence string
	RouteDegraded   bool

	// Dispatch attributes.
	PackageSize string // small | medium | large
	ServiceTier string // standard | express

	// Haulage attributes.
	WeightGrams int64
	Helpers     int64

	// AsOf is the moment being priced. The engine uses it only for explanation
	// text; version resolution happens before the engine is called.
	AsOf time.Time
}

// Component is one line of the calculation.
type Component struct {
	Code string `json:"code"`
	// AmountKobo may be negative for ROUNDING, which can round a total down.
	AmountKobo int64 `json:"amount_kobo"`
	// Basis explains in words how this line was derived, so an operator reading
	// a quote does not have to re-derive the arithmetic.
	Basis string `json:"basis"`
}

// Funding is who pays. In V1 the customer funds everything: merchant funding and
// KarryGo subsidy arrive with the promotions work.
type Funding struct {
	CustomerKobo        int64 `json:"customer_kobo"`
	MerchantKobo        int64 `json:"merchant_kobo"`
	PlatformSubsidyKobo int64 `json:"platform_subsidy_kobo"`
}

// Total returns the funded amount.
func (f Funding) Total() int64 { return f.CustomerKobo + f.MerchantKobo + f.PlatformSubsidyKobo }

// Allocation is who receives.
type Allocation struct {
	ProviderKobo    int64 `json:"provider_kobo"`
	PlatformFeeKobo int64 `json:"platform_fee_kobo"`
	TaxKobo         int64 `json:"tax_kobo"`
}

// Total returns the allocated amount.
func (a Allocation) Total() int64 { return a.ProviderKobo + a.PlatformFeeKobo + a.TaxKobo }

// Result is the engine's decision.
type Result struct {
	Currency   string      `json:"currency"`
	Components []Component `json:"components"`
	GrossKobo  int64       `json:"gross_kobo"`
	Funding    Funding     `json:"funding"`
	Allocation Allocation  `json:"allocation"`
	Flags      []string    `json:"flags,omitempty"`
	// Explanation is the human-readable derivation, stored with the quote so an
	// operator or a dispute reviewer can read what happened without a developer.
	Explanation []string `json:"explanation"`
}

// ErrUnsupported is returned when a policy cannot price the request, for
// example a taxi class the policy has no rates for. Pricing refuses rather than
// guessing: a missing class must surface as an unpriced class, not a wrong fare.
var ErrUnsupported = errors.New("engine: policy does not support this request")

// Validate checks the request is internally coherent. The engine assumes the
// caller has already resolved geography and policy; this catches inputs that no
// policy could sensibly price.
func (r Request) Validate() error {
	var problems []string
	if !r.Scope.Valid() {
		problems = append(problems, fmt.Sprintf("unknown scope %q", r.Scope))
	}
	if !r.Channel.Valid() {
		problems = append(problems, fmt.Sprintf("unknown channel %q", r.Channel))
	}
	if r.Class == "" {
		problems = append(problems, "class is required")
	}
	if r.DistanceMeters < 0 {
		problems = append(problems, "distance cannot be negative")
	}
	if r.DurationSeconds < 0 {
		problems = append(problems, "duration cannot be negative")
	}
	if r.TrafficDurationSeconds < 0 {
		problems = append(problems, "traffic duration cannot be negative")
	}
	if r.WeightGrams < 0 {
		problems = append(problems, "weight cannot be negative")
	}
	if r.Helpers < 0 {
		problems = append(problems, "helper count cannot be negative")
	}
	if len(problems) > 0 {
		return fmt.Errorf("engine: invalid request: %s", join(problems))
	}
	return nil
}

func join(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += "; "
		}
		out += part
	}
	return out
}
