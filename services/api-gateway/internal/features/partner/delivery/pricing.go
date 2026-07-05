package delivery

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"cosmicforge/logistics/services/api-gateway/internal/pkg/geo"
)

// Deterministic local pricing. No external maps API is called.
const (
	baseFareKobo    int64 = 80000  // NGN 800
	perKmKobo       int64 = 18000  // NGN 180 / km
	expressMult           = 1.35   // express multiplier
	mediumAddonKobo int64 = 30000  // NGN 300
	largeAddonKobo  int64 = 70000  // NGN 700
	minFareKobo     int64 = 100000 // NGN 1000
	fareRoundKobo   int64 = 50     // round to nearest 50 kobo

	avgCitySpeedKMH  = 18.0 // average city speed
	pickupBufferMins = 10   // fixed pickup buffer
	minETAMins       = 15
)

// haversineKM returns the great-circle distance between two lat/lng points in km.
// It delegates to the shared pricing-engine geo helper.
func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	return geo.HaversineKm(lat1, lng1, lat2, lng2)
}

// calculateFareKobo returns the fare in kobo for a distance, package size, and tier.
func calculateFareKobo(distanceKM float64, packageSize, serviceTier string) int64 {
	fare := float64(baseFareKobo) + float64(perKmKobo)*distanceKM

	switch strings.ToLower(strings.TrimSpace(packageSize)) {
	case "medium":
		fare += float64(mediumAddonKobo)
	case "large":
		fare += float64(largeAddonKobo)
	}

	if strings.EqualFold(strings.TrimSpace(serviceTier), ServiceTierExpress) {
		fare *= expressMult
	}

	rounded := int64(math.Round(fare/float64(fareRoundKobo))) * fareRoundKobo
	if rounded < minFareKobo {
		rounded = minFareKobo
	}
	return rounded
}

// estimateETA returns a human ETA string like "25 mins" for a distance.
func estimateETA(distanceKM float64) string {
	mins := int(math.Ceil(distanceKM/avgCitySpeedKMH*60)) + pickupBufferMins
	if mins < minETAMins {
		mins = minETAMins
	}
	return fmt.Sprintf("%d mins", mins)
}

// roundKM rounds a distance to one decimal place for display.
func roundKM(distanceKM float64) float64 {
	return math.Round(distanceKM*10) / 10
}

// nairaSign is the Naira currency symbol (U+20A6), required in fare display
// strings by the Partner API response contract.
const nairaSign = "₦"

// formatNaira renders a kobo amount as a naira currency string,
// e.g. 250000 -> "<naira>2,500.00".
func formatNaira(kobo int64) string {
	naira := kobo / 100
	frac := kobo % 100
	if frac < 0 {
		frac = -frac
	}
	return nairaSign + withThousands(naira) + "." + fmt.Sprintf("%02d", frac)
}

func withThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
