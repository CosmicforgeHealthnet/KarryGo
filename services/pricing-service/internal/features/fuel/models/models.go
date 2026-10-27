// Package models holds the fuel index's records and its pure rules.
package models

import (
	"errors"
	"sort"
	"time"
)

// Location is the timezone a report's day is counted in. Nigeria has one.
var Location = mustLoad("Africa/Lagos")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("WAT", 3600)
	}
	return loc
}

// Day is the calendar day t falls on in Location, at midnight UTC so it
// compares cleanly with a Postgres date.
func Day(t time.Time) time.Time {
	local := t.In(Location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// ErrInvalid is a report the index must not count.
var ErrInvalid = errors.New("fuel: invalid report")

// Report is one driver's fuel price for a day.
type Report struct {
	ID                string
	ConsumerService   string
	ProviderRef       string
	FuelType          string
	PricePerLitreKobo int64
	LatE6, LngE6      int64
	MarketID          string
	MarketCode        string
	ZoneID            string
	ReportDay         time.Time
	ReportedAt        time.Time
}

// Index is the fuel price for a market, fuel and day.
type Index struct {
	MarketID     string
	FuelType     string
	Day          time.Time
	MedianKobo   int64
	ReportCount  int
	DroppedCount int
	WindowDays   int
}

// Proposal is a tariff change the index proposed.
type Proposal struct {
	ID                string
	MarketID          string
	TariffKind        string
	FuelType          string
	IndexDay          time.Time
	FuelPriceKobo     int64
	BaselineKobo      int64
	ChangeBPS         int64
	ShareBPS          int64
	ReportCount       int
	RulesVersionID    string
	BaseVersionID     string
	ProposedVersionID string
	ApprovalRequestID string
	CreatedAt         time.Time
}

// Median returns the median of prices after dropping outliers: anything below
// half or above one and a half times the first median. A median survives a few
// wild reports, and dropping the wildest stops a handful of typos from moving
// it at all. It returns the median, how many reports it used and how many it
// dropped.
func Median(prices []int64) (median int64, used, dropped int) {
	if len(prices) == 0 {
		return 0, 0, 0
	}
	first := median64(prices)
	kept := make([]int64, 0, len(prices))
	for _, p := range prices {
		if p*2 >= first && p*2 <= first*3 {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return first, len(prices), 0
	}
	return median64(kept), len(kept), len(prices) - len(kept)
}

func median64(values []int64) int64 {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
