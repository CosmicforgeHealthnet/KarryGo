// Package vehicleclass is the single source of truth for taxi vehicle classes
// and their OPERATIONAL facts only (seats, passenger limits). It carries no
// pricing information -- addendum §1/§37 puts fare rates out of this
// service's scope entirely; see internal/pricing for where a class's rate
// actually lives, as configuration, never as a Go constant.
//
// It deliberately sits outside internal/features so that provider_profile
// (which validates what a driver may register) and booking (which filters
// match candidates, once phase B is built) all read the same definitions.
//
// The hauling service keeps two hand-maintained copies of its equivalent list —
// ValidTruckTypes in provider_profile and ValidPreferredTruckTypes in booking —
// compared by exact string equality at match time. A divergence between them
// produces bookings that silently never match. One package removes that class
// of defect rather than documenting it.
package vehicleclass

import "strings"

const (
	Economy = "economy"
	Comfort = "comfort"
	XL      = "xl"
)

// Spec describes one bookable class's operational facts only. No fare or
// multiplier field belongs here -- see the package doc comment.
type Spec struct {
	// Name is the canonical slug stored on vehicles and bookings.
	Name string
	// Label is the passenger-facing name.
	Label string
	// DefaultSeats is what a newly registered vehicle of this class gets when
	// the driver does not state a seat count.
	DefaultSeats int
	// MaxPassengers caps how many passengers a booking of this class may carry.
	// It is one below DefaultSeats for the classes where the driver occupies a
	// seat in the same cabin.
	MaxPassengers int
}

// specs is ordered in the same cheapest-to-most-premium order the passenger
// app renders the fare tiles in, even though this package no longer knows the
// actual prices -- the ordering itself is a UI/product fact, not a pricing
// one, and internal/pricing's rate config is expected to follow the same
// class order by convention.
var specs = []Spec{
	{Name: Economy, Label: "Economy", DefaultSeats: 4, MaxPassengers: 3},
	{Name: Comfort, Label: "Comfort", DefaultSeats: 4, MaxPassengers: 3},
	{Name: XL, Label: "XL", DefaultSeats: 6, MaxPassengers: 5},
}

// All returns every bookable class, cheapest-to-premium ordered.
func All() []Spec {
	out := make([]Spec, len(specs))
	copy(out, specs)
	return out
}

// Normalize trims and lowercases a caller-supplied class. It does not validate;
// pair it with Valid when the value came from a request body.
func Normalize(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// Get returns the spec for a class. The bool reports whether it is known.
func Get(raw string) (Spec, bool) {
	name := Normalize(raw)
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Valid reports whether raw names a bookable class.
func Valid(raw string) bool {
	_, ok := Get(raw)
	return ok
}

// Names returns the canonical slugs, cheapest first — useful for error messages
// that need to tell a caller what the accepted values are.
func Names() []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Name)
	}
	return out
}
