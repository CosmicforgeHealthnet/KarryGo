// Package models holds the quote domain: what pricing said, for whom, and from
// which exact policy versions.
package models

import (
	"errors"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
)

// Modes.
const (
	ModeShadow  = "SHADOW"
	ModeCentral = "CENTRAL"
)

// Unpriced reasons.
const (
	ReasonClassNotPriced = "class_not_priced"
)

var (
	// ErrNotFound covers an unknown quote, or one that belongs to another caller.
	ErrNotFound = errors.New("quotes: not found")
	// ErrNoMarket means no pricing market covers the requested place.
	ErrNoMarket = errors.New("quotes: no pricing market covers this place")
	// ErrNoActiveTariff means the place has no ACTIVE rate book for the scope.
	ErrNoActiveTariff = errors.New("quotes: no active rates for this place")
	// ErrBindConflict means the quote is already bound to a different booking.
	ErrBindConflict = errors.New("quotes: quote is already bound to another booking")
	// ErrBindExpired means the quote expired before it was bound.
	ErrBindExpired = errors.New("quotes: quote expired before it was bound")
	// ErrNotBound means a quote must be bound before it can be reconciled.
	ErrNotBound = errors.New("quotes: quote must be bound before it is reconciled")
)

// Place is where a quote was priced, resolved onto pricing's own geography.
type Place struct {
	MarketID      string
	MarketCode    string
	StateID       string
	CountryCode   string
	Currency      string
	ZoneID        string
	ZoneVersionID string
}

// ZoneCandidate is an active zone with its current circle, used to find the
// zone containing a pickup point.
type ZoneCandidate struct {
	Place
	CenterLatE6 int64
	CenterLngE6 int64
	RadiusM     int64
}

// PricedRequest is the normalized request stored with a quote: integers only,
// so it hashes and replays identically.
type PricedRequest struct {
	Scope                  string `json:"scope"`
	Channel                string `json:"channel"`
	Class                  string `json:"class"`
	DistanceMeters         int64  `json:"distance_meters"`
	DurationSeconds        int64  `json:"duration_seconds"`
	TrafficDurationSeconds int64  `json:"traffic_duration_seconds"`
	RouteConfidence        string `json:"route_confidence"`
	RouteDegraded          bool   `json:"route_degraded"`
	PackageSize            string `json:"package_size"`
	ServiceTier            string `json:"service_tier"`
	WeightGrams            int64  `json:"weight_grams"`
	Helpers                int64  `json:"helpers"`
	ConsumerZoneID         string `json:"consumer_zone_id"`
	PickupLatE6            int64  `json:"pickup_lat_e6"`
	PickupLngE6            int64  `json:"pickup_lng_e6"`
	HasPickup              bool   `json:"has_pickup"`
}

// EngineRequest converts the stored request back into the engine's input.
func (r PricedRequest) EngineRequest(asOf time.Time) engine.Request {
	return engine.Request{
		Scope:                  engine.Scope(r.Scope),
		Channel:                engine.Channel(r.Channel),
		Class:                  r.Class,
		DistanceMeters:         r.DistanceMeters,
		DurationSeconds:        r.DurationSeconds,
		TrafficDurationSeconds: r.TrafficDurationSeconds,
		RouteConfidence:        r.RouteConfidence,
		RouteDegraded:          r.RouteDegraded,
		PackageSize:            r.PackageSize,
		ServiceTier:            r.ServiceTier,
		WeightGrams:            r.WeightGrams,
		Helpers:                r.Helpers,
		AsOf:                   asOf,
	}
}

// Quote is one class's issued price.
type Quote struct {
	ID                  string
	ConsumerService     string
	ConsumerRef         string
	Scope               string
	Channel             string
	Class               string
	Mode                string
	Place               Place
	TariffVersionID     string
	CommissionVersionID string
	Request             PricedRequest
	Result              engine.Result
	ResultHash          string
	LegacyFareKobo      *int64
	IssuedAt            time.Time
	ExpiresAt           time.Time
}

// Binding ties a quote to the booking that used it.
type Binding struct {
	ID              string
	QuoteID         string
	ConsumerService string
	BookingRef      string
	BoundAt         time.Time
}

// Reconciliation is a bound quote priced again from the actual route.
type Reconciliation struct {
	ID              string
	QuoteID         string
	Request         PricedRequest
	Result          engine.Result
	ResultHash      string
	LegacyFinalKobo *int64
	ReconciledAt    time.Time
}

// ShadowSummaryRow is how often, and by how much, pricing disagreed with what a
// caller actually charged, for one scope and class.
type ShadowSummaryRow struct {
	Scope              string `json:"scope"`
	Class              string `json:"class"`
	Quotes             int64  `json:"quotes"`
	Compared           int64  `json:"compared"`
	Matching           int64  `json:"matching"`
	TotalDifference    int64  `json:"total_difference_kobo"`
	MaxAbsDifference   int64  `json:"max_abs_difference_kobo"`
	Reconciled         int64  `json:"reconciled"`
	ReconciledMatching int64  `json:"reconciled_matching"`
}
