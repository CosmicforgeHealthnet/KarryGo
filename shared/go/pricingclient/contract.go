// Package pricingclient is the contract between pricing-service and the
// services that ask it for prices (taxi, dispatch, haulage), and the signed
// client they use.
//
// It carries no fare formula. Pricing decides prices; this package only moves
// the request and the answer, and refuses to send a request no policy could
// sensibly price.
package pricingclient

import (
	"strings"
	"time"
)

// Paths, mounted at pricing-service's root behind HMAC service auth. They are
// never exposed through the public gateway.
const (
	QuotesPath = "/internal/pricing/v1/quotes"
)

// ByRefPath finds a caller's quote by its own reference and class.
const ByRefPath = QuotesPath + "/by-ref"

// BindPath is where a quote is bound to the booking that used it.
func BindPath(quoteID string) string { return QuotesPath + "/" + quoteID + "/bind" }

// ReconcilePath is where a bound quote is priced again from what actually
// happened on the trip, with the quote's ORIGINAL policy versions.
func ReconcilePath(quoteID string) string { return QuotesPath + "/" + quoteID + "/reconcile" }

// Scopes.
const (
	ScopeTaxi     = "TAXI"
	ScopeDispatch = "DISPATCH"
	ScopeHaulage  = "HAULAGE"
)

// Channels.
const (
	ChannelCustomerApp = "CUSTOMER_APP"
	ChannelPartnerAPI  = "PARTNER_API"
	ChannelCommerce    = "COMMERCE"
	ChannelOps         = "OPS"
)

// Modes. A SHADOW quote is evidence only: the caller still charges its own fare,
// and pricing records what it would have charged. CENTRAL is the price charged.
const (
	ModeShadow  = "SHADOW"
	ModeCentral = "CENTRAL"
)

// Route confidence values, as the caller reports them.
const (
	RouteConfidenceHigh   = "HIGH"
	RouteConfidenceMedium = "MEDIUM"
	RouteConfidenceLow    = "LOW"
)

// Point is a coordinate in degrees.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Route is the trusted route snapshot the caller priced. Pricing never routes.
type Route struct {
	DistanceMeters         int64  `json:"distance_meters"`
	DurationSeconds        int64  `json:"duration_seconds"`
	TrafficDurationSeconds int64  `json:"traffic_duration_seconds,omitempty"`
	Confidence             string `json:"confidence,omitempty"`
	// Degraded is true when the caller fell back to a straight line.
	Degraded bool `json:"degraded,omitempty"`
}

// QuoteRequest asks for one quote per class.
type QuoteRequest struct {
	// ConsumerRef is the caller's stable reference for this pricing attempt (a
	// booking id, an estimate id). Asking again with the same reference and
	// class returns the quote already issued instead of a second one.
	ConsumerRef string   `json:"consumer_ref"`
	Scope       string   `json:"scope"`
	Channel     string   `json:"channel"`
	Classes     []string `json:"classes"`

	// Where: the caller's own operating zone when it has one (taxi), else the
	// pickup point. Pricing maps either onto its own geography.
	ConsumerZoneID string `json:"consumer_zone_id,omitempty"`
	Pickup         *Point `json:"pickup,omitempty"`

	Route Route `json:"route"`

	// Dispatch attributes.
	PackageSize string `json:"package_size,omitempty"`
	ServiceTier string `json:"service_tier,omitempty"`

	// Haulage attributes.
	WeightGrams int64 `json:"weight_grams,omitempty"`
	Helpers     int64 `json:"helpers,omitempty"`

	// LegacyFareKobo is, per class, the fare the caller itself computed and
	// will charge. Sent only in shadow mode, so pricing can record how far its
	// answer is from what the customer actually pays.
	LegacyFareKobo map[string]int64 `json:"legacy_fare_kobo,omitempty"`
}

// Validate reports the first problem that makes the request unsendable.
func (r QuoteRequest) Validate() (string, bool) {
	switch {
	case strings.TrimSpace(r.ConsumerRef) == "":
		return "consumer_ref is required", false
	case r.Scope != ScopeTaxi && r.Scope != ScopeDispatch && r.Scope != ScopeHaulage:
		return "unknown scope", false
	case strings.TrimSpace(r.Channel) == "":
		return "channel is required", false
	case len(r.Classes) == 0:
		return "at least one class is required", false
	case strings.TrimSpace(r.ConsumerZoneID) == "" && r.Pickup == nil:
		return "a consumer zone or a pickup point is required", false
	case r.Route.DistanceMeters < 0 || r.Route.DurationSeconds < 0 || r.Route.TrafficDurationSeconds < 0:
		return "route figures cannot be negative", false
	case r.WeightGrams < 0 || r.Helpers < 0:
		return "weight and helpers cannot be negative", false
	}
	for _, class := range r.Classes {
		if strings.TrimSpace(class) == "" {
			return "classes cannot be blank", false
		}
	}
	if r.Pickup != nil && (r.Pickup.Lat < -90 || r.Pickup.Lat > 90 || r.Pickup.Lng < -180 || r.Pickup.Lng > 180) {
		return "pickup is not a coordinate", false
	}
	return "", true
}

// Component is one line of a price.
type Component struct {
	Code       string `json:"code"`
	AmountKobo int64  `json:"amount_kobo"`
	Basis      string `json:"basis"`
}

// Quote is one class's price.
type Quote struct {
	ID              string      `json:"id"`
	Class           string      `json:"class"`
	Mode            string      `json:"mode"`
	Currency        string      `json:"currency"`
	GrossKobo       int64       `json:"gross_kobo"`
	ProviderKobo    int64       `json:"provider_kobo"`
	PlatformFeeKobo int64       `json:"platform_fee_kobo"`
	TaxKobo         int64       `json:"tax_kobo"`
	Components      []Component `json:"components"`
	Flags           []string    `json:"flags,omitempty"`
	// LegacyFareKobo echoes what the caller said it charges; DifferenceKobo is
	// GrossKobo minus it (positive: pricing would charge more).
	LegacyFareKobo      *int64    `json:"legacy_fare_kobo,omitempty"`
	DifferenceKobo      *int64    `json:"difference_kobo,omitempty"`
	TariffVersionID     string    `json:"tariff_version_id"`
	CommissionVersionID string    `json:"commission_version_id,omitempty"`
	IssuedAt            time.Time `json:"issued_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// Unpriced explains a requested class pricing could not price.
type Unpriced struct {
	Class  string `json:"class"`
	Reason string `json:"reason"`
}

// QuoteResponse is the answer to a QuoteRequest.
type QuoteResponse struct {
	Quotes   []Quote    `json:"quotes"`
	Unpriced []Unpriced `json:"unpriced,omitempty"`
}

// BindRequest ties a quote to the booking that used it.
type BindRequest struct {
	BookingRef string `json:"booking_ref"`
}

// Binding is the recorded bind.
type Binding struct {
	QuoteID    string    `json:"quote_id"`
	BookingRef string    `json:"booking_ref"`
	BoundAt    time.Time `json:"bound_at"`
}

// ReconcileRequest prices a bound quote again from the actual route.
type ReconcileRequest struct {
	Route Route `json:"route"`
	// LegacyFinalKobo is the final fare the caller charged, in shadow mode.
	LegacyFinalKobo *int64 `json:"legacy_final_kobo,omitempty"`
}

// Reconciliation is the reconciled price.
type Reconciliation struct {
	QuoteID         string      `json:"quote_id"`
	GrossKobo       int64       `json:"gross_kobo"`
	ProviderKobo    int64       `json:"provider_kobo"`
	PlatformFeeKobo int64       `json:"platform_fee_kobo"`
	Components      []Component `json:"components"`
	LegacyFinalKobo *int64      `json:"legacy_final_kobo,omitempty"`
	DifferenceKobo  *int64      `json:"difference_kobo,omitempty"`
	ReconciledAt    time.Time   `json:"reconciled_at"`
}
