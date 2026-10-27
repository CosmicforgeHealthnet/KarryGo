package pricingclient

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FuelReportsPath is where driver services relay what drivers paid for fuel.
// Pricing turns the reports into a daily fuel index per market; it never
// changes a fare from them without an approved policy change.
const FuelReportsPath = "/internal/pricing/v1/fuel-reports"

// Fuel types a driver can report.
const (
	FuelPetrol = "PETROL"
	FuelDiesel = "DIESEL"
	FuelGas    = "GAS"
)

// Sane bounds for a reported price, ₦100 to ₦10,000 a litre. Anything outside
// is a typo, not a market price.
const (
	MinFuelPriceKobo = 10_000
	MaxFuelPriceKobo = 1_000_000
)

// FuelReport is one driver's fuel price for a day. ProviderRef is the driver
// service's own provider id; together with the calling service it allows one
// report per driver, fuel type and day.
type FuelReport struct {
	ProviderRef       string    `json:"provider_ref"`
	FuelType          string    `json:"fuel_type"`
	PricePerLitreKobo int64     `json:"price_per_litre_kobo"`
	LatE6             int64     `json:"lat_e6"`
	LngE6             int64     `json:"lng_e6"`
	ReportedAt        time.Time `json:"reported_at"`
}

// FuelReportResult says whether the report was counted.
type FuelReportResult struct {
	ID string `json:"id"`
	// Duplicate is true when the driver already reported this fuel today; the
	// first report stands.
	Duplicate bool `json:"duplicate"`
	// MarketCode is empty when the location is outside every pricing market;
	// the report is kept but feeds no index.
	MarketCode string `json:"market_code,omitempty"`
}

// NormalizeFuelType returns the canonical fuel type, or "" when unknown.
func NormalizeFuelType(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case FuelPetrol, "PMS":
		return FuelPetrol
	case FuelDiesel, "AGO":
		return FuelDiesel
	case FuelGas, "CNG", "LPG":
		return FuelGas
	}
	return ""
}

// Validate refuses a report no index should count.
func (r FuelReport) Validate() error {
	switch {
	case strings.TrimSpace(r.ProviderRef) == "":
		return fmt.Errorf("%w: provider_ref is required", ErrInvalidRequest)
	case r.FuelType != FuelPetrol && r.FuelType != FuelDiesel && r.FuelType != FuelGas:
		return fmt.Errorf("%w: fuel_type must be PETROL, DIESEL or GAS", ErrInvalidRequest)
	case r.PricePerLitreKobo < MinFuelPriceKobo || r.PricePerLitreKobo > MaxFuelPriceKobo:
		return fmt.Errorf("%w: price_per_litre_kobo is out of range", ErrInvalidRequest)
	case r.LatE6 < -90_000_000 || r.LatE6 > 90_000_000 || r.LngE6 < -180_000_000 || r.LngE6 > 180_000_000:
		return fmt.Errorf("%w: location is out of range", ErrInvalidRequest)
	case r.LatE6 == 0 && r.LngE6 == 0:
		return fmt.Errorf("%w: location is required", ErrInvalidRequest)
	case r.ReportedAt.IsZero():
		return fmt.Errorf("%w: reported_at is required", ErrInvalidRequest)
	}
	return nil
}

// ReportFuel sends a fuel price report.
func (c *Client) ReportFuel(ctx context.Context, report FuelReport) (FuelReportResult, error) {
	report.FuelType = NormalizeFuelType(report.FuelType)
	if err := report.Validate(); err != nil {
		return FuelReportResult{}, err
	}
	var result FuelReportResult
	err := c.post(ctx, FuelReportsPath, report, &result)
	return result, err
}
