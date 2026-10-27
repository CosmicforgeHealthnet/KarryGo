// Package http exposes quotes to the signed calling services, and the shadow
// comparison to operators.
package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/engine"
	"cosmicforge/logistics/services/pricing-service/internal/features/quotes/models"
	"cosmicforge/logistics/services/pricing-service/internal/features/quotes/usecases"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/pricingclient"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Quotes is what the handler needs from the quote service.
type Quotes interface {
	Issue(ctx context.Context, caller string, request pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error)
	FindByRef(ctx context.Context, caller, consumerRef, class string) (pricingclient.Quote, error)
	Bind(ctx context.Context, caller, quoteID, bookingRef string) (pricingclient.Binding, error)
	Reconcile(ctx context.Context, caller, quoteID string, request pricingclient.ReconcileRequest) (pricingclient.Reconciliation, error)
	ShadowSummary(ctx context.Context, since time.Time) ([]models.ShadowSummaryRow, error)
}

// Handler serves the quote routes.
type Handler struct{ quotes Quotes }

// NewHandler builds the handler.
func NewHandler(quotes Quotes) *Handler { return &Handler{quotes: quotes} }

// Options wires the routes.
type Options struct {
	Handler    *Handler
	Verifier   *serviceauth.Verifier
	NonceStore database.Querier
	// QuoteCallers may price, bind and reconcile. They can never change a rate.
	QuoteCallers []string
	// AdminCallers may read the shadow comparison.
	AdminCallers []string
}

// RegisterRoutes mounts the quote and shadow-report routes at the service root,
// behind signing, single-use nonces and per-group caller allowlists.
func RegisterRoutes(router *gin.Engine, opts Options) {
	quotes := router.Group(pricingclient.QuotesPath)
	quotes.Use(serviceauth.Middleware(opts.Verifier))
	quotes.Use(servicetrust.RejectReplayedNonce(opts.NonceStore))
	quotes.Use(servicetrust.RequireCallers(opts.QuoteCallers...))
	quotes.POST("", opts.Handler.Issue)
	quotes.GET("/by-ref", opts.Handler.FindByRef)
	quotes.POST("/:id/bind", opts.Handler.Bind)
	quotes.POST("/:id/reconcile", opts.Handler.Reconcile)

	admin := router.Group("/internal/pricing/v1/admin/shadow")
	admin.Use(serviceauth.Middleware(opts.Verifier))
	admin.Use(servicetrust.RejectReplayedNonce(opts.NonceStore))
	admin.Use(servicetrust.RequireCallers(opts.AdminCallers...))
	admin.GET("/summary", opts.Handler.ShadowSummary)
}

// Issue prices the requested classes.
func (h *Handler) Issue(c *gin.Context) {
	var request pricingclient.QuoteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	response, err := h.quotes.Issue(c.Request.Context(), c.GetString("service_name"), request)
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

// FindByRef returns a quote by the caller's own reference and class.
func (h *Handler) FindByRef(c *gin.Context) {
	quote, err := h.quotes.FindByRef(c.Request.Context(), c.GetString("service_name"), c.Query("consumer_ref"), c.Query("class"))
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": quote})
}

// Bind ties a quote to a booking.
func (h *Handler) Bind(c *gin.Context) {
	var request pricingclient.BindRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	binding, err := h.quotes.Bind(c.Request.Context(), c.GetString("service_name"), c.Param("id"), request.BookingRef)
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": binding})
}

// Reconcile prices a bound quote from the actual route.
func (h *Handler) Reconcile(c *gin.Context) {
	var request pricingclient.ReconcileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	reconciliation, err := h.quotes.Reconcile(c.Request.Context(), c.GetString("service_name"), c.Param("id"), request)
	if err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": reconciliation})
}

// ShadowSummary compares pricing with what callers charged over the last
// `hours` (default a week).
func (h *Handler) ShadowSummary(c *gin.Context) {
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "168"))
	if err != nil || hours <= 0 || hours > 24*90 {
		httpx.Abort(c, apperrors.BadRequest("hours must be between 1 and 2160.", err))
		return
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	rows, err := h.quotes.ShadowSummary(c.Request.Context(), since)
	if err != nil {
		respond(c, err)
		return
	}
	if rows == nil {
		rows = []models.ShadowSummaryRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"since": since, "rows": rows}})
}

func respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecases.ErrInvalid), errors.Is(err, engine.ErrUnsupported):
		httpx.Abort(c, apperrors.Validation(err.Error(), nil))
	case errors.Is(err, models.ErrNoMarket):
		httpx.Abort(c, apperrors.New(http.StatusUnprocessableEntity, apperrors.CodeValidationFailed,
			"No pricing market covers this place.", err))
	case errors.Is(err, models.ErrNoActiveTariff):
		httpx.Abort(c, apperrors.New(http.StatusUnprocessableEntity, apperrors.CodeValidationFailed,
			"There are no active rates for this place yet.", err))
	case errors.Is(err, models.ErrNotFound):
		httpx.Abort(c, apperrors.NotFound("Quote not found.", err))
	case errors.Is(err, models.ErrBindConflict):
		httpx.Abort(c, apperrors.Conflict("This quote is already bound to another booking.", err))
	case errors.Is(err, models.ErrBindExpired):
		httpx.Abort(c, apperrors.Conflict("This quote expired before it was bound.", err))
	case errors.Is(err, models.ErrNotBound):
		httpx.Abort(c, apperrors.Conflict("Bind this quote to its booking before reconciling it.", err))
	default:
		httpx.Abort(c, apperrors.Internal("Pricing could not complete the request.", err))
	}
}
