// Package marketmodels defines markets and operating zones (addendum §29):
// country -> market -> operating_zone. Every booking resolves to one of each
// at creation, and the geo index is partitioned by market so candidate search
// is scoped before eligibility filtering runs.
package marketmodels

import "time"

type Market struct {
	ID        string
	Country   string
	Name      string
	Code      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Zone's coverage is a simple circle (Center + RadiusKm), not a polygon — no
// PostGIS is installed and none is needed at this platform's scale. RadiusKm
// serves double duty deliberately: it is both "how far this zone's coverage
// extends" (used to resolve which zone a pickup point belongs to) and "how
// far the matcher searches within it" (used by matching, phase B). One column
// for both avoids two radii that could be configured to disagree with no
// clear meaning for the gap between them.
type Zone struct {
	ID                string
	MarketID          string
	Name              string
	Code              string
	CenterLat         float64
	CenterLng         float64
	RadiusKm          float64
	SupportedClasses  []string
	IsAirportZone     bool
	ComplianceProfile string
	Version           int
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SupportsClass reports whether vehicleClass is bookable in this zone.
func (z Zone) SupportsClass(vehicleClass string) bool {
	for _, c := range z.SupportedClasses {
		if c == vehicleClass {
			return true
		}
	}
	return false
}
