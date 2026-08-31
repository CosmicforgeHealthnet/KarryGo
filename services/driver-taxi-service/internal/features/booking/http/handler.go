package bookinghttp

import (
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type BookingHandler struct {
	bookings *bookingusecases.BookingService
}

func NewBookingHandler(bookings *bookingusecases.BookingService) *BookingHandler {
	return &BookingHandler{bookings: bookings}
}

type successResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, successResponse{Success: true, Data: data})
}

func customerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

func providerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

// CreateBooking is wrapped in idempotency.Execute by the router -- this
// method is the `run` callback, so it must read the body itself (the wrapper
// already restored it after computing the fingerprint) and every response
// path must go through httpx.Abort/respond so the wrapper can capture it.
func (h *BookingHandler) CreateBooking(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req createBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	booking, err := h.bookings.CreateBooking(c.Request.Context(), req.toInput(id))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, booking.Public())
}

func (h *BookingHandler) GetBooking(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.GetByIDForCustomer(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) ListBookings(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	bookings, err := h.bookings.ListByCustomer(c.Request.Context(), id, limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	public := make([]bookingmodels.PublicBooking, len(bookings))
	for i, b := range bookings {
		public[i] = b.Public()
	}
	respond(c, nethttp.StatusOK, bookingListResponse{Bookings: public})
}

// EstimateFare is public and unauthenticated: it is the fare-tile screen a
// passenger sees before choosing to log in and book.
func (h *BookingHandler) EstimateFare(c *gin.Context) {
	var req estimateFareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	fares, err := h.bookings.EstimateFare(c.Request.Context(), req.toInput())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, fareEstimateResponse{Fares: fares})
}

func (h *BookingHandler) GetBookingForProvider(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.GetByIDForProvider(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) MarkEnRoutePickup(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.MarkEnRoutePickup(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) MarkArrivedAtPickup(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.MarkArrivedAtPickup(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

// StartTrip is the "arrived_at_pickup -> in_progress" transition. B3 STUB:
// see BookingRepository.MarkInProgress's doc comment -- pickup verification
// is not yet enforced here; phase B5 will tighten this.
func (h *BookingHandler) StartTrip(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.MarkInProgress(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) CompleteTrip(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req completeTripRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.MarkCompleted(c.Request.Context(), req.toInput(c.Param("id"), id))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) CancelByProvider(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req cancelBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.CancelByProvider(c.Request.Context(), c.Param("id"), id, req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) AcceptBooking(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.AcceptBooking(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *BookingHandler) RejectBooking(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	booking, err := h.bookings.RejectBooking(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

// ListActiveBookings is the poll target for a driver app with no push
// channel wired yet: their current offer (if any) and their committed ride
// (if any).
func (h *BookingHandler) ListActiveBookings(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	bookings, err := h.bookings.ListActiveByProvider(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	public := make([]bookingmodels.PublicBooking, len(bookings))
	for i, b := range bookings {
		public[i] = b.Public()
	}
	respond(c, nethttp.StatusOK, bookingListResponse{Bookings: public})
}

// GetPickupCode is the passenger's own display screen: the code (and, on
// the client, a QR rendering of that same code) to show the driver at
// pickup.
func (h *BookingHandler) GetPickupCode(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	credential, err := h.bookings.GetPickupCode(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, credential)
}

func (h *BookingHandler) ResendPickupCode(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	credential, err := h.bookings.ResendPickupCode(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, credential)
}

// VerifyPickup is the driver-initiated check against the code the passenger
// showed them.
func (h *BookingHandler) VerifyPickup(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req verifyPickupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.VerifyPickup(c.Request.Context(), c.Param("id"), id, req.Code)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

// IngestLocationPoints is the batched active-trip GPS upload (phase B6).
// Never a per-batch failure for one bad point -- see LocationPointResult's
// own doc comment -- so a 200 with per-point errors is the normal shape for
// a batch that mixes assigned and unassigned/inactive bookings, not a bug.
func (h *BookingHandler) IngestLocationPoints(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req ingestLocationPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	results, err := h.bookings.IngestLocationPoints(c.Request.Context(), id, req.toInputs())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, locationPointResultResponse{Results: results})
}

// SubmitCustomerReview is the passenger rating their driver over a
// completed ride.
func (h *BookingHandler) SubmitCustomerReview(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req submitReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	review, err := h.bookings.SubmitCustomerReview(c.Request.Context(), c.Param("id"), id, req.toInput())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, review)
}

// GetCustomerReviews is the passenger's own view of both directions'
// reviews -- their own review is always fully visible; the driver's is
// withheld (rating shown, text/tags not) until both sides have submitted or
// the reveal window has elapsed.
func (h *BookingHandler) GetCustomerReviews(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	view, err := h.bookings.GetCustomerReviewsView(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, view)
}

// SubmitProviderReview is the driver rating their passenger over a
// completed ride.
func (h *BookingHandler) SubmitProviderReview(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req submitReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	review, err := h.bookings.SubmitProviderReview(c.Request.Context(), c.Param("id"), id, req.toInput())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, review)
}

// GetProviderReviews mirrors GetCustomerReviews from the driver's side.
func (h *BookingHandler) GetProviderReviews(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	view, err := h.bookings.GetProviderReviewsView(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, view)
}

// GetEarningsInsights is the addendum §19 reporting endpoint. ?from=&until=
// (RFC3339) are both optional; omitting either defaults the WHOLE window to
// the last 30 days ending now, rather than mixing a caller-supplied bound
// with a default for the other -- a half-specified window is more likely a
// client bug than an intentional "open-ended" query.
func (h *BookingHandler) GetEarningsInsights(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	until := time.Now()
	from := until.Add(-30 * 24 * time.Hour)
	if fromParam, untilParam := c.Query("from"), c.Query("until"); fromParam != "" && untilParam != "" {
		parsedFrom, err := time.Parse(time.RFC3339, fromParam)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("'from' must be RFC3339.", err))
			return
		}
		parsedUntil, err := time.Parse(time.RFC3339, untilParam)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("'until' must be RFC3339.", err))
			return
		}
		from, until = parsedFrom, parsedUntil
	}

	insights, err := h.bookings.GetProviderEarningsInsights(c.Request.Context(), id, from, until)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, insights)
}

func (h *BookingHandler) CancelByCustomer(c *gin.Context) {
	id, ok := customerID(c)
	if !ok {
		return
	}
	var req cancelBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.CancelByCustomer(c.Request.Context(), c.Param("id"), id, req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}
