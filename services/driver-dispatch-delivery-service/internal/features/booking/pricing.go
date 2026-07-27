package booking

import (
	"cosmicforge/logistics/shared/go/dispatchpricing"
)

// Fare math lives in shared/go/dispatchpricing — the single source of truth
// shared with the api-gateway partner delivery API, so the customer and
// partner surfaces can never quote different fares for the same route.

// haversineKM returns the great-circle distance between two points in km.
func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	return dispatchpricing.HaversineKm(lat1, lng1, lat2, lng2)
}

// calculateFareKobo returns the rounded, min-clamped fare for a route.
func calculateFareKobo(distanceKM float64, packageSize, serviceTier string) int64 {
	return dispatchpricing.FareKobo(distanceKM, packageSize, serviceTier)
}

// fareBreakdown returns the display breakdown for a route. Components are
// indicative; the authoritative total is calculateFareKobo.
func fareBreakdown(distanceKM float64, packageSize, serviceTier string) FareBreakdown {
	b := dispatchpricing.Breakdown(distanceKM, packageSize, serviceTier)
	return FareBreakdown{
		BaseFareKobo:         b.BaseFareKobo,
		DistanceFareKobo:     b.DistanceFareKobo,
		SizeSurchargeKobo:    b.SizeSurchargeKobo,
		ExpressSurchargeKobo: b.ExpressSurchargeKobo,
	}
}

// estimateETA returns a human ETA string like "25 mins" for a distance.
func estimateETA(distanceKM float64) string {
	return dispatchpricing.ETALabel(distanceKM)
}

// roundKM rounds a distance to one decimal place for display.
func roundKM(distanceKM float64) float64 {
	return dispatchpricing.RoundKM(distanceKM)
}
