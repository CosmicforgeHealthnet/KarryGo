// Package fuelrelay is the driver-facing fuel price route the driver services
// share. A driver reports what they paid for fuel; the service relays it to
// pricing-service, which builds a daily fuel index per market from it. Drivers
// never call pricing directly, and a report changes no fare.
package fuelrelay

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/pricingclient"
)

// Path is where driver apps send fuel prices, under each service's provider group.
const Path = "/fuel-prices"

// Reporter is the pricing client's fuel method.
type Reporter interface {
	ReportFuel(ctx context.Context, report pricingclient.FuelReport) (pricingclient.FuelReportResult, error)
}

// Request is what the driver app sends.
type Request struct {
	FuelType          string  `json:"fuel_type"`
	PricePerLitreKobo int64   `json:"price_per_litre_kobo"`
	Lat               float64 `json:"lat"`
	Lng               float64 `json:"lng"`
}

// Response tells the app what happened to the report.
type Response struct {
	// Duplicate: the driver already reported this fuel today; the first stands.
	Duplicate bool `json:"duplicate"`
	// Counted: the location is inside a pricing market, so it feeds an index.
	Counted bool `json:"counted"`
}

// Handler relays a signed-in driver's report. providerID returns the driver's
// id from the request's auth context; reporter may be nil when pricing is not
// configured, which answers 503 rather than pretending the report was kept.
func Handler(reporter Reporter, providerID func(*gin.Context) (string, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := providerID(c)
		if !ok || strings.TrimSpace(id) == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
			return
		}
		report := pricingclient.FuelReport{
			ProviderRef:       id,
			FuelType:          pricingclient.NormalizeFuelType(req.FuelType),
			PricePerLitreKobo: req.PricePerLitreKobo,
			LatE6:             int64(math.Round(req.Lat * 1e6)),
			LngE6:             int64(math.Round(req.Lng * 1e6)),
			ReportedAt:        time.Now().UTC(),
		}
		if err := report.Validate(); err != nil {
			httpx.Abort(c, apperrors.Validation(fieldMessage(err), nil))
			return
		}
		if reporter == nil {
			httpx.Abort(c, apperrors.Unavailable("Fuel price reporting is not available yet.", nil))
			return
		}
		result, err := reporter.ReportFuel(c.Request.Context(), report)
		switch {
		case errors.Is(err, pricingclient.ErrNotConfigured):
			httpx.Abort(c, apperrors.Unavailable("Fuel price reporting is not available yet.", err))
			return
		case errors.Is(err, pricingclient.ErrRefused):
			httpx.Abort(c, apperrors.Validation("Check the fuel type and price.", nil))
			return
		case err != nil:
			httpx.Abort(c, apperrors.Unavailable("We couldn't save your fuel price. Please try again.", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": Response{Duplicate: result.Duplicate, Counted: result.MarketCode != ""}})
	}
}

func fieldMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "fuel_type"):
		return "Choose petrol, diesel or gas."
	case strings.Contains(msg, "price"):
		return "Enter a price per litre between ₦100 and ₦10,000."
	case strings.Contains(msg, "location"):
		return "We need your location to know which area this price is for."
	}
	return "Check the fuel price details."
}
