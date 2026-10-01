package bookinghttp

import (
	nethttp "net/http"
	"strconv"
	"strings"

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
		ReceiverEmail:      req.ReceiverEmail,
		ReceiverCustomerID: req.ReceiverCustomerID,
		PackageContent:     req.PackageContent,
		PackageSize:        req.PackageSize,
		IsFragile:          req.IsFragile,
		PaymentMethod:      req.PaymentMethod,
		ScheduledAt:        req.ScheduledAt,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": booking})
}

// InitiateCardPayment starts the up-front Paystack payment for a card booking and
// returns the authorization URL the customer completes in the checkout WebView.
func (h *BookingHandler) InitiateCardPayment(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req initiateCardPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.svc.InitiateCardPayment(c.Request.Context(), bookingusecases.InitiateCardPaymentInput{
		BookingID:     c.Param("id"),
		CustomerID:    claims.Subject,
		CustomerEmail: req.CustomerEmail,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}

// GetBookingLocation returns the live location of the assigned provider for the
// customer's trip map.
func (h *BookingHandler) GetBookingLocation(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	loc, err := h.svc.GetBookingLocation(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": loc})
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

func (h *BookingHandler) GetCustomerBookingReview(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	review, err := h.svc.GetCustomerBookingReview(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": review})
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

// VerifyDriverAtPickup lets the customer confirm the driver's pickup
// credential (entered manually or scanned as a QR).
func (h *BookingHandler) VerifyDriverAtPickup(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req verifyDriverRequest
	if err := c.ShouldBindJSON(&req); err != nil || credentialValue(req.Code, req.QRToken) == "" {
		httpx.Abort(c, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{{Field: "credential", Message: "An OTP or QR token is required."}}))
		return
	}
	booking, err := h.svc.VerifyDriverAtPickup(c.Request.Context(), claims.Subject, c.Param("id"), credentialValue(req.Code, req.QRToken))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) ReadReceiverCredential(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Receiver authentication is required.", nil))
		return
	}
	view, err := h.svc.ReadReceiverCredential(c.Request.Context(), claims.Subject, c.Param("id"), c.Param("challengeRef"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

// GetVerificationCode returns status for the customer's delivery challenge.
// It never redisplays either bearer credential.
func (h *BookingHandler) GetVerificationCode(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.GetVerificationCode(c.Request.Context(), claims.Subject, c.Param("id"), c.Query("step"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

// ResendVerificationCode regenerates the customer's own delivery
// verification code (?step=delivery).
func (h *BookingHandler) ResendVerificationCode(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.ResendVerificationCode(c.Request.Context(), claims.Subject, c.Param("id"), c.Query("step"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
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
		"created_at":        provider.CreatedAt,
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

// GetProviderEarnings returns the provider's trip-earnings projection
// (balances, today's stats, recent transactions) for the Earnings/Wallet screen.
func (h *BookingHandler) GetProviderEarnings(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	earnings, err := h.svc.GetProviderEarnings(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": earnings})
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

func (h *BookingHandler) GetProviderBookingReview(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	review, err := h.svc.GetProviderBookingReview(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": review})
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

func (h *BookingHandler) MarkEnRoutePickup(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.MarkEnRoutePickup(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) MarkArrivedAtPickup(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.MarkArrivedAtPickup(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func (h *BookingHandler) MarkEnRouteDelivery(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	booking, err := h.svc.MarkEnRouteDelivery(c.Request.Context(), c.Param("id"), claims.Subject)
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
	var req confirmDeliveryRequest
	_ = c.ShouldBindJSON(&req)
	if credentialValue(req.Code, req.QRToken) == "" {
		httpx.Abort(c, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{{Field: "credential", Message: "A delivery OTP or QR token is required."}}))
		return
	}
	booking, err := h.svc.ConfirmDelivery(c.Request.Context(), c.Param("id"), claims.Subject, credentialValue(req.Code, req.QRToken), req.ProofPhotoURL, req.SignatureURL)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": booking})
}

func credentialValue(code, qrToken string) string {
	if value := strings.TrimSpace(qrToken); value != "" {
		return value
	}
	return strings.TrimSpace(code)
}

// GetPickupCredential returns the authenticated driver's pickup challenge
// status. Credentials are returned only by issue/resend commands.
func (h *BookingHandler) GetPickupCredential(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.GetPickupCredential(c.Request.Context(), claims.Subject, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

// ResendPickupCredential regenerates the authenticated driver's own pickup
// credential.
func (h *BookingHandler) ResendPickupCredential(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.ResendPickupCredential(c.Request.Context(), claims.Subject, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
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
