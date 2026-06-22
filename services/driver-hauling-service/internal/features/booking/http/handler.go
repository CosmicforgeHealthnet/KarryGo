package bookinghttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type BookingHandler struct {
	svc          *bookingusecases.BookingService
	providerRepo providerauthrepositories.ProviderRepository
	truckRepo    providerprofilerepositories.TruckRepository
}

func NewBookingHandler(
	svc *bookingusecases.BookingService,
	providerRepo providerauthrepositories.ProviderRepository,
	truckRepo providerprofilerepositories.TruckRepository,
) *BookingHandler {
	return &BookingHandler{svc: svc, providerRepo: providerRepo, truckRepo: truckRepo}
}

// ─── Customer handlers ────────────────────────────────────────────────────────

func (h *BookingHandler) EstimateFare(c *gin.Context) {
	var req estimateFareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	estimate := h.svc.EstimateFare(bookingusecases.EstimateFareInput{
		PickupLat:     req.PickupLat,
		PickupLng:     req.PickupLng,
		DropoffLat:    req.DropoffLat,
		DropoffLng:    req.DropoffLng,
		CargoWeightKg: req.CargoWeightKg,
		HelperCount:   req.HelperCount,
	})
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": estimate})
}

func (h *BookingHandler) CreateBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req createBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	booking, err := h.svc.CreateBooking(c.Request.Context(), bookingusecases.CreateBookingInput{
		CustomerID:         claims.Subject,
		PickupAddress:      req.PickupAddress,
		PickupLat:          req.PickupLat,
		PickupLng:          req.PickupLng,
		DropoffAddress:     req.DropoffAddress,
		DropoffLat:         req.DropoffLat,
		DropoffLng:         req.DropoffLng,
		PreferredTruckType: req.PreferredTruckType,
		CargoWeightKg:      req.CargoWeightKg,
		CargoDescription:   req.CargoDescription,
		RequiresHelpers:    req.RequiresHelpers,
		HelperCount:        req.HelperCount,
		WeightCategory:     req.WeightCategory,
		ReceiverName:       req.ReceiverName,
		ReceiverPhone:      req.ReceiverPhone,
		PackageContent:     req.PackageContent,
		PackageSize:        req.PackageSize,
		IsFragile:          req.IsFragile,
		ScheduledAt:        req.ScheduledAt,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) GetCustomerBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.GetBooking(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) ListCustomerBookings(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)

	bookings, err := h.svc.ListCustomerBookings(c.Request.Context(), claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": bookings})
}

func (h *BookingHandler) SubmitReview(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req submitReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	review, err := h.svc.SubmitReview(c.Request.Context(), bookingusecases.SubmitReviewInput{
		BookingID:        c.Param("id"),
		CustomerID:       claims.Subject,
		Rating:           req.Rating,
		ReviewText:       req.ReviewText,
		RecommendsDriver: req.RecommendsDriver,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": review})
}

func (h *BookingHandler) CancelBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req cancelBookingRequest
	_ = c.ShouldBindJSON(&req)

	booking, err := h.svc.CancelBooking(c.Request.Context(), c.Param("id"), claims.Subject, req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

// GetPublicProvider returns a safe public view of a provider for display in the customer app.
func (h *BookingHandler) GetPublicProvider(c *gin.Context) {
	_, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	provider, err := h.providerRepo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":                provider.ID,
		"first_name":        provider.FirstName,
		"last_name":         provider.LastName,
		"profile_photo_url": provider.ProfilePhotoURL,
		"phone":             provider.Phone,
		"rating":            provider.Rating,
		"total_trips":       provider.TotalTrips,
	}})
}

// GetPublicTruck returns a safe public view of a truck for display in the customer app.
func (h *BookingHandler) GetPublicTruck(c *gin.Context) {
	_, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	truck, err := h.truckRepo.GetByIDAnywhere(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":           truck.ID,
		"make":         truck.Make,
		"model":        truck.Model,
		"color":        truck.Color,
		"plate_number": truck.PlateNumber,
		"truck_type":   truck.TruckType,
	}})
}

// ─── Provider handlers ────────────────────────────────────────────────────────

func (h *BookingHandler) ListProviderBookings(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)

	bookings, err := h.svc.ListProviderBookings(c.Request.Context(), claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": bookings})
}

func (h *BookingHandler) GetProviderBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.GetProviderBooking(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) AcceptBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.AcceptBooking(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) RejectBooking(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.RejectBooking(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) ConfirmPickup(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.ConfirmPickup(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) ConfirmDelivery(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.ConfirmDelivery(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) CancelByProvider(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req cancelBookingRequest
	_ = c.ShouldBindJSON(&req)
	booking, err := h.svc.CancelByProvider(c.Request.Context(), c.Param("id"), claims.Subject, req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	v := c.Query(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
