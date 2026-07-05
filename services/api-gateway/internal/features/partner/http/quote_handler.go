package http

import (
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/geo"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
)

const quoteCurrency = "NGN"

// latLng is a nested coordinate pair in the quote request.
type latLng struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

// quoteRequest is the POST /quotes body (pricing engine contract).
type quoteRequest struct {
	Pickup  *latLng `json:"pickup"`
	Dropoff *latLng `json:"dropoff"`
	Tier    string  `json:"tier"`
	Size    string  `json:"size"`
}

var validTiers = map[string]struct{}{"standard": {}, "express": {}}
var validSizes = map[string]struct{}{"small": {}, "medium": {}, "large": {}}

// GetQuote handles POST /api/v1/partner/v1/quotes. It computes distance with
// geo.HaversineKm and fare with pricing.CalculateFare (config-driven). It performs
// no DB write, payment, or event publish. Auth is API key + active account.
func (h *Handler) GetQuote(c *gin.Context) {
	var req quoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondInvalidJSON(c)
		return
	}

	fields := []delivery.FieldError{}
	validateCoord(&fields, "pickup", req.Pickup)
	validateCoord(&fields, "dropoff", req.Dropoff)

	tier, ok := normalizeChoice(req.Tier, "standard", validTiers)
	if !ok {
		fields = append(fields, delivery.FieldError{Field: "tier", Message: "Tier must be one of: standard, express."})
	}
	size, ok := normalizeChoice(req.Size, "small", validSizes)
	if !ok {
		fields = append(fields, delivery.FieldError{Field: "size", Message: "Size must be one of: small, medium, large."})
	}

	if len(fields) > 0 {
		respondError(c, delivery.ValidationError(fields))
		return
	}

	distanceKm := geo.HaversineKm(*req.Pickup.Lat, *req.Pickup.Lng, *req.Dropoff.Lat, *req.Dropoff.Lng)
	amountKobo := pricing.CalculateFare(h.pricing, distanceKm, tier, size)

	respondData(c, http.StatusOK, gin.H{
		"distance_km": round2(distanceKm),
		"currency":    quoteCurrency,
		"amount_kobo": amountKobo,
		"tier":        tier,
		"size":        size,
	})
}

// validateCoord appends field errors for a required nested coordinate.
func validateCoord(fields *[]delivery.FieldError, name string, p *latLng) {
	if p == nil || p.Lat == nil {
		*fields = append(*fields, delivery.FieldError{Field: name + ".lat", Message: "This field is required."})
	} else if *p.Lat < -90 || *p.Lat > 90 {
		*fields = append(*fields, delivery.FieldError{Field: name + ".lat", Message: "Latitude must be between -90 and 90."})
	}
	if p == nil || p.Lng == nil {
		*fields = append(*fields, delivery.FieldError{Field: name + ".lng", Message: "This field is required."})
	} else if *p.Lng < -180 || *p.Lng > 180 {
		*fields = append(*fields, delivery.FieldError{Field: name + ".lng", Message: "Longitude must be between -180 and 180."})
	}
}

// normalizeChoice lowercases/trims value, applies the default when empty, and
// reports whether the result is in the allowed set.
func normalizeChoice(value, def string, allowed map[string]struct{}) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		v = def
	}
	_, ok := allowed[v]
	return v, ok
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
