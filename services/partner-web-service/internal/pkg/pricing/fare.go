// Package pricing implements the Partner API fare calculation engine. Fares are
// config-driven (no hardcoded values in handlers) and expressed in kobo.
package pricing

// Config holds the fare parameters, all in kobo except the multiplier.
type Config struct {
	BaseKobo          int64
	StandardPerKmKobo int64
	ExpressPerKmKobo  int64
	LargeMultiplier   float64
}

// CalculateFare returns the fare in kobo for a distance, tier, and size.
// Express uses a higher per-km rate than standard, a large package applies the
// multiplier, and the fare is never below the base fare.
func CalculateFare(cfg Config, distanceKm float64, tier, size string) int64 {
	perKm := cfg.StandardPerKmKobo

	if tier == "express" {
		perKm = cfg.ExpressPerKmKobo
	}

	fare := cfg.BaseKobo + int64(distanceKm*float64(perKm))

	if size == "large" {
		fare = int64(float64(fare) * cfg.LargeMultiplier)
	}

	if fare < cfg.BaseKobo {
		fare = cfg.BaseKobo
	}

	return fare
}
