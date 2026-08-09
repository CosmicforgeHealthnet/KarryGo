package booking

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// RiderProfileReader exposes the customer-safe rider profile. Satisfied by
// *profile.Service.
type RiderProfileReader interface {
	GetPublicProfile(ctx context.Context, providerID string) (profile.PublicProfile, error)
}

type Handler struct {
	service *Service
	riders  RiderProfileReader
}

func NewHandler(service *Service, riders RiderProfileReader) *Handler {
	return &Handler{service: service, riders: riders}
}

// RegisterRoutes registers the customer-facing dispatch booking API under
// /api/v1/customer/dispatch. Customer bearer tokens are issued by
// customer-service; the signing secret must match (DISPATCH_CUSTOMER_TOKEN_SECRET).
func RegisterRoutes(engine *gin.Engine, customerSigner *sharedauth.TokenSigner, handler *Handler) {
	group := engine.Group("/api/v1/customer/dispatch")
	group.POST("/bookings/estimate", handler.Estimate)

	authed := group.Group("")
	authed.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	authed.GET("/availability", handler.CheckAvailability)
	authed.POST("/bookings", handler.CreateBooking)
	authed.GET("/bookings", handler.ListBookings)
	authed.GET("/bookings/:id", handler.GetBooking)
	authed.POST("/bookings/:id/cancel", handler.CancelBooking)
	authed.POST("/bookings/:id/review", handler.SubmitReview)
	authed.GET("/bookings/:id/review", handler.GetReview)
	authed.GET("/bookings/:id/location", handler.GetLocation)
	authed.POST("/bookings/:id/verify-driver", handler.VerifyDriverAtPickup)
	authed.GET("/bookings/:id/verification-code", handler.GetVerificationCode)
	authed.POST("/bookings/:id/verification-code/resend", handler.ResendVerificationCode)
	authed.GET("/providers/:id", handler.GetRider)
}

func customerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

func (h *Handler) Estimate(c *gin.Context) {
	var request EstimateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.Estimate(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Fare estimated.", result)
}

func (h *Handler) CheckAvailability(c *gin.Context) {
	if _, ok := customerID(c); !ok {
		return
	}
	lat, latErr := strconv.ParseFloat(c.Query("lat"), 64)
	lng, lngErr := strconv.ParseFloat(c.Query("lng"), 64)
	if latErr != nil || lngErr != nil {
		httpx.Abort(c, apperrors.BadRequest("Query parameters lat and lng are required.", nil))
		return
	}
	result, err := h.service.CheckAvailability(c.Request.Context(), lat, lng)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Rider availability checked.", result)
}

func (h *Handler) CreateBooking(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	var request CreateBookingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.CreateBooking(c.Request.Context(), customer, request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusCreated, "Delivery booking created.", result)
}

func (h *Handler) ListBookings(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	result, err := h.service.ListBookings(c.Request.Context(), customer, ListBookingsOptions{Limit: limit, Offset: offset})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Delivery bookings listed.", gin.H{"bookings": result})
}

func (h *Handler) GetBooking(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	result, err := h.service.GetBooking(c.Request.Context(), customer, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Delivery booking fetched.", result)
}

type cancelBookingRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) CancelBooking(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	var request cancelBookingRequest
	_ = c.ShouldBindJSON(&request) // body is optional
	result, err := h.service.CancelBooking(c.Request.Context(), customer, c.Param("id"), request.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Delivery booking cancelled.", result)
}

func (h *Handler) SubmitReview(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	var request ReviewInput
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.SubmitReview(c.Request.Context(), customer, c.Param("id"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusCreated, "Review submitted.", result)
}

func (h *Handler) GetReview(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	result, err := h.service.GetReview(c.Request.Context(), customer, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Review fetched.", result)
}

func (h *Handler) GetLocation(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	result, err := h.service.GetLiveLocation(c.Request.Context(), customer, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Rider location fetched.", result)
}

type verifyDriverRequest struct {
	Code string `json:"code"`
}

// VerifyDriverAtPickup lets the customer confirm the rider's pickup
// credential (entered manually or scanned as a QR).
func (h *Handler) VerifyDriverAtPickup(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	var request verifyDriverRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Code == "" {
		httpx.Abort(c, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{{Field: "code", Message: "Verification code is required."}}))
		return
	}
	result, err := h.service.VerifyDriverAtPickup(c.Request.Context(), customer, c.Param("id"), request.Code)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Driver verified.", result)
}

// GetVerificationCode returns the customer's own delivery verification code
// for redisplay (?step=delivery — the rider's pickup credential belongs to
// the rider and is not readable here).
func (h *Handler) GetVerificationCode(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	result, err := h.service.GetVerificationCode(c.Request.Context(), customer, c.Param("id"), c.Query("step"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Verification code fetched.", result)
}

// ResendVerificationCode regenerates the customer's own delivery
// verification code (?step=delivery).
func (h *Handler) ResendVerificationCode(c *gin.Context) {
	customer, ok := customerID(c)
	if !ok {
		return
	}
	result, err := h.service.ResendVerificationCode(c.Request.Context(), customer, c.Param("id"), c.Query("step"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "A new verification code was generated.", result)
}

func (h *Handler) GetRider(c *gin.Context) {
	if _, ok := customerID(c); !ok {
		return
	}
	if h.riders == nil {
		httpx.Abort(c, apperrors.Unavailable("Rider profiles are temporarily unavailable.", nil))
		return
	}
	result, err := h.riders.GetPublicProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Rider profile fetched.", result)
}
