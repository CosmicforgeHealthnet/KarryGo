package checkoutquote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxQuoteCommandBody = 8 << 10

type createQuoteRequest struct {
	FulfilmentPlanID string `json:"fulfilment_plan_id"`
	CouponCode       string `json:"coupon_code,omitempty"`
}

type approveQuoteRequest struct {
	QuoteFingerprint string `json:"quote_fingerprint"`
}

type Handler struct {
	service *Service
	limiter RequestLimiter
	limit   RequestLimitPolicy
}

type RequestLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

type RequestLimitPolicy struct {
	Limiter RequestLimiter
	Max     int
	Window  time.Duration
}

func NewHandler(service *Service, protections ...RequestLimitPolicy) *Handler {
	handler := &Handler{service: service}
	if len(protections) > 0 {
		handler.limiter = protections[0].Limiter
		handler.limit = protections[0]
	}
	return handler
}

func strictJSON(c *gin.Context, target any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxQuoteCommandBody+1))
	if err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	if len(body) > maxQuoteCommandBody {
		return apperrors.BadRequest("Request body is too large.", nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest("Request body must contain exactly one JSON object.", err)
	}
	return nil
}

func quoteCustomerSubject(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Type != sharedauth.TokenTypeAccess ||
		claims.Role != "customer" || claims.Service != "customer" ||
		strings.TrimSpace(claims.Subject) == "" {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func quoteHTTPError(err error) error {
	switch {
	case errors.Is(err, ErrQuoteNotFound):
		return apperrors.NotFound("Quote not found.", err)
	case errors.Is(err, ErrQuoteExpired):
		return apperrors.Conflict("This quote expired. Create and approve a new quote.", err)
	case errors.Is(err, ErrSourceNotQuotable):
		return apperrors.Conflict("Checkout or fulfilment changed. Create a new quote.", err)
	case errors.Is(err, ErrQuoteConflict):
		return apperrors.Conflict("The approved commercial inputs changed. Create a new quote.", err)
	case errors.Is(err, ErrFingerprintMismatch):
		return apperrors.Conflict("The quote fingerprint does not match. Reload the quote.", err)
	case errors.Is(err, ErrInvalidAdjustment):
		return apperrors.Conflict("The commercial adjustment is no longer valid.", err)
	case errors.Is(err, ErrDependencyMissing):
		return apperrors.Unavailable("Quote generation is temporarily unavailable.", err)
	default:
		return apperrors.Internal("Quote operation failed.", err)
	}
}

func (h *Handler) create(c *gin.Context) {
	customerID, ok := quoteCustomerSubject(c)
	if !ok {
		return
	}
	if h.limiter != nil && h.limit.Max > 0 {
		if err := h.limiter.Allow(c.Request.Context(), "quote_create", customerID, h.limit.Max, h.limit.Window); err != nil {
			httpx.RespondError(c, err)
			return
		}
	}
	var request createQuoteRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if strings.TrimSpace(request.FulfilmentPlanID) == "" {
		httpx.RespondError(c, apperrors.BadRequest("fulfilment_plan_id is required.", nil))
		return
	}
	quote, err := h.service.CreateWithCoupon(c.Request.Context(), customerID, c.Param("sessionId"), request.FulfilmentPlanID, request.CouponCode)
	if err != nil {
		httpx.RespondError(c, quoteHTTPError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": quote})
}

func (h *Handler) get(c *gin.Context) {
	customerID, ok := quoteCustomerSubject(c)
	if !ok {
		return
	}
	quote, err := h.service.Get(c.Request.Context(), customerID, c.Param("quoteId"))
	if err != nil {
		httpx.RespondError(c, quoteHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": quote})
}

func (h *Handler) approve(c *gin.Context) {
	customerID, ok := quoteCustomerSubject(c)
	if !ok {
		return
	}
	var request approveQuoteRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if strings.TrimSpace(request.QuoteFingerprint) == "" {
		httpx.RespondError(c, apperrors.BadRequest("quote_fingerprint is required.", nil))
		return
	}
	quote, err := h.service.Approve(c.Request.Context(), customerID, c.Param("quoteId"), request.QuoteFingerprint)
	if err != nil {
		httpx.RespondError(c, quoteHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": quote})
}

// RegisterCustomerRoutes exposes only quote creation, read and explicit
// approval. It has no payment or order command and accepts no customer-owned
// prices, fees, discounts, vendor ids, branch ids or funding allocations.
func RegisterCustomerRoutes(root *gin.RouterGroup, service *Service, signer *sharedauth.TokenSigner, idempotencyStore *idempotency.Store, enabled bool, protections ...RequestLimitPolicy) {
	if root == nil || service == nil || signer == nil || !enabled {
		return
	}
	handler := NewHandler(service, protections...)
	commerce := root.Group("")
	commerce.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	commerce.Use(idempotency.CustomerCommandMiddleware(idempotencyStore))
	commerce.POST("/checkout/:sessionId/quotes", handler.create)
	commerce.GET("/quotes/:quoteId", handler.get)
	commerce.POST("/quotes/:quoteId/approve", handler.approve)
}
