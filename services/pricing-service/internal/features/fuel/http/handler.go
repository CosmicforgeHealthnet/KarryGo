// Package http accepts fuel price reports from the driver services.
package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/fuel/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/pricingclient"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Reporter records a fuel report.
type Reporter interface {
	Report(ctx context.Context, caller string, in pricingclient.FuelReport) (pricingclient.FuelReportResult, error)
}

// Options wires the route.
type Options struct {
	Reporter   Reporter
	Verifier   *serviceauth.Verifier
	NonceStore database.Querier
	// Callers are the driver services. Drivers never call pricing directly.
	Callers []string
}

// RegisterRoutes mounts the fuel report route behind signing, single-use
// nonces and the caller allowlist.
func RegisterRoutes(router *gin.Engine, opts Options) {
	group := router.Group(pricingclient.FuelReportsPath)
	group.Use(serviceauth.Middleware(opts.Verifier))
	group.Use(servicetrust.RejectReplayedNonce(opts.NonceStore))
	group.Use(servicetrust.RequireCallers(opts.Callers...))
	group.POST("", func(c *gin.Context) {
		var in pricingclient.FuelReport
		if err := c.ShouldBindJSON(&in); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
			return
		}
		result, err := opts.Reporter.Report(c.Request.Context(), c.GetString("service_name"), in)
		if errors.Is(err, models.ErrInvalid) {
			httpx.Abort(c, apperrors.Validation(err.Error(), nil))
			return
		}
		if err != nil {
			httpx.Abort(c, apperrors.Internal("Pricing could not record the fuel price.", err))
			return
		}
		status := http.StatusCreated
		if result.Duplicate {
			status = http.StatusOK
		}
		c.JSON(status, gin.H{"success": true, "data": result})
	})
}
