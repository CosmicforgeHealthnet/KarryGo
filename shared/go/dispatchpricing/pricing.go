// Package dispatchpricing is the single source of truth for the dispatch
// (bike delivery) fare formula. It is shared by the api-gateway partner
// delivery API and the dispatch-delivery-service customer booking API so the
// two surfaces can never quote different fares for the same route.
//
// Deterministic and pure: no external maps API, no config, no I/O.
package dispatchpricing

import (
	"fmt"
	"math"
	"strings"
)

const (
	BaseFareKobo    int64 = 80000  // NGN 800
	PerKmKobo       int64 = 18000  // NGN 180 / km
	ExpressMult           = 1.35   // express multiplier
	MediumAddonKobo int64 = 30000  // NGN 300
	LargeAddonKobo  int64 = 70000  // NGN 700
	MinFareKobo     int64 = 100000 // NGN 1000
	FareRoundKobo   int64 = 50     // round to nearest 50 kobo

	AvgCitySpeedKMH  = 18.0 // average city speed for the ETA estimate
	PickupBufferMins = 10   // fixed pickup buffer
	MinETAMins       = 15
)

const (
	SizeSmall  = "small"
	SizeMedium = "medium"
	SizeLarge  = "large"

	TierStandard = "standard"
	TierExpress  = "express"
)

// FareBreakdown is the display decomposition of a fare. Components are
// indicative; the authoritative total is FareKobo (rounded and min-clamped).
type FareBreakdown struct {
	BaseFareKobo         int64 `json:"base_fare_kobo"`
	DistanceFareKobo     int64 `json:"distance_fare_kobo"`
	SizeSurchargeKobo    int64 `json:"size_surcharge_kobo"`
	ExpressSurchargeKobo int64 `json:"express_surcharge_kobo"`
}

// NormalizeSize maps free-form input onto small|medium|large (default small).
func NormalizeSize(size string) string {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case SizeMedium:
		return SizeMedium
	case SizeLarge:
		return SizeLarge
	default:
		return SizeSmall
	}
}

// NormalizeTier maps free-form input onto standard|express (default standard).
func NormalizeTier(tier string) string {
	if strings.EqualFold(strings.TrimSpace(tier), TierExpress) {
		return TierExpress
	}
	return TierStandard
}

// FareKobo returns the rounded, min-clamped fare for a route.
func FareKobo(distanceKM float64, packageSize, serviceTier string) int64 {
	fare := float64(BaseFareKobo) + float64(PerKmKobo)*distanceKM
	switch NormalizeSize(packageSize) {
	case SizeMedium:
		fare += float64(MediumAddonKobo)
	case SizeLarge:
		fare += float64(LargeAddonKobo)
	}
	if NormalizeTier(serviceTier) == TierExpress {
		fare *= ExpressMult
	}
	rounded := int64(math.Round(fare/float64(FareRoundKobo))) * FareRoundKobo
	if rounded < MinFareKobo {
		rounded = MinFareKobo
	}
	return rounded
}

// Breakdown returns the display breakdown for a route.
func Breakdown(distanceKM float64, packageSize, serviceTier string) FareBreakdown {
	distanceFare := int64(math.Round(float64(PerKmKobo) * distanceKM))
	var sizeSurcharge int64
	switch NormalizeSize(packageSize) {
	case SizeMedium:
		sizeSurcharge = MediumAddonKobo
	case SizeLarge:
		sizeSurcharge = LargeAddonKobo
	}
	var expressSurcharge int64
	if NormalizeTier(serviceTier) == TierExpress {
		subtotal := float64(BaseFareKobo) + float64(PerKmKobo)*distanceKM + float64(sizeSurcharge)
		expressSurcharge = int64(math.Round(subtotal * (ExpressMult - 1)))
	}
	return FareBreakdown{
		BaseFareKobo:         BaseFareKobo,
		DistanceFareKobo:     distanceFare,
		SizeSurchargeKobo:    sizeSurcharge,
		ExpressSurchargeKobo: expressSurcharge,
	}
}

// ETAMinutes returns the estimated pickup-to-delivery minutes for a distance.
func ETAMinutes(distanceKM float64) int {
	mins := int(math.Ceil(distanceKM/AvgCitySpeedKMH*60)) + PickupBufferMins
	if mins < MinETAMins {
		mins = MinETAMins
	}
	return mins
}

// ETALabel returns a human ETA string like "25 mins" for a distance.
func ETALabel(distanceKM float64) string {
	return fmt.Sprintf("%d mins", ETAMinutes(distanceKM))
}

// RoundKM rounds a distance to one decimal place for display.
func RoundKM(distanceKM float64) float64 {
	return math.Round(distanceKM*10) / 10
}

// HaversineKm returns the great-circle distance in kilometres between two
// latitude/longitude points (WGS84).
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
