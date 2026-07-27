package delivery

import (
	"fmt"
	"strconv"
	"strings"

	"cosmicforge/logistics/shared/go/dispatchpricing"
)

// Fare math lives in shared/go/dispatchpricing — the single source of truth
// shared with dispatch-delivery-service's customer booking API, so the partner
// and customer surfaces can never quote different fares for the same route.
// No external maps API is called.

// haversineKM returns the great-circle distance between two lat/lng points in km.
func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	return dispatchpricing.HaversineKm(lat1, lng1, lat2, lng2)
}

// calculateFareKobo returns the fare in kobo for a distance, package size, and tier.
func calculateFareKobo(distanceKM float64, packageSize, serviceTier string) int64 {
	return dispatchpricing.FareKobo(distanceKM, packageSize, serviceTier)
}

// estimateETA returns a human ETA string like "25 mins" for a distance.
func estimateETA(distanceKM float64) string {
	return dispatchpricing.ETALabel(distanceKM)
}

// roundKM rounds a distance to one decimal place for display.
func roundKM(distanceKM float64) float64 {
	return dispatchpricing.RoundKM(distanceKM)
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
