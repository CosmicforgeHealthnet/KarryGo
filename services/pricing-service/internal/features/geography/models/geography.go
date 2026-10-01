// Package models holds pricing geography: country, state, market and zone.
//
// This is commercial geography, deliberately separate from each service's
// operating zones. An operating zone answers "may we serve here"; a pricing zone
// answers "what does it cost here". Keeping them apart means a market can be
// split for pricing without touching serviceability, and vice versa.
package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Shape types a zone version may carry. A circle is enough for launch; the field
// exists so a polygon can arrive later without a schema change.
const (
	ShapeCircle = "CIRCLE"
)

// Country is a currency and legal context.
type Country struct {
	Code     string
	Name     string
	Currency string
}

// State is a state or region.
type State struct {
	ID          string
	CountryCode string
	Code        string
	Name        string
}

// Market is a coherent commercial operating area.
type Market struct {
	ID          string
	CountryCode string
	StateID     string
	Code        string
	Name        string
	Timezone    string
	Status      string
	CreatedAt   time.Time
}

// Zone refines commercial conditions inside a market.
type Zone struct {
	ID       string
	MarketID string
	Code     string
	Name     string
	Status   string
}

// ZoneVersion is an immutable zone shape. A quote records the version that priced
// it, so redrawing a boundary cannot change what a past quote says about where a
// trip was.
type ZoneVersion struct {
	ID        string
	ZoneID    string
	VersionNo int
	Shape     []byte
	ShapeHash string
	CreatedBy string
	CreatedAt time.Time
}

// CircleShape is a centre and radius, in microdegrees and whole metres so the
// shape hashes identically everywhere.
type CircleShape struct {
	Type        string `json:"type"`
	CenterLatE6 int64  `json:"center_lat_e6"`
	CenterLngE6 int64  `json:"center_lng_e6"`
	RadiusM     int64  `json:"radius_m"`
}

// ErrNotFound covers an unknown country, state, market or zone.
var ErrNotFound = errors.New("geography: not found")

// ValidateMarket checks a market can be created.
func ValidateMarket(market Market) error {
	var problems []string
	if strings.TrimSpace(market.CountryCode) == "" {
		problems = append(problems, "country_code is required")
	}
	if strings.TrimSpace(market.Code) == "" {
		problems = append(problems, "code is required")
	}
	if strings.TrimSpace(market.Name) == "" {
		problems = append(problems, "name is required")
	}
	if len(problems) > 0 {
		return fmt.Errorf("geography: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ValidateCircle checks a circle shape is usable.
func (c CircleShape) Validate() error {
	if c.Type != ShapeCircle {
		return fmt.Errorf("geography: shape type %q is not %s", c.Type, ShapeCircle)
	}
	if c.RadiusM <= 0 {
		return errors.New("geography: a zone radius must be positive")
	}
	if c.CenterLatE6 < -90_000_000 || c.CenterLatE6 > 90_000_000 {
		return errors.New("geography: latitude is out of range")
	}
	if c.CenterLngE6 < -180_000_000 || c.CenterLngE6 > 180_000_000 {
		return errors.New("geography: longitude is out of range")
	}
	return nil
}
