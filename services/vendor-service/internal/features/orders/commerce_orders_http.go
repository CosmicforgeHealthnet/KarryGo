package orders

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

type section21Handler struct{ service *Section21Service }

func section21Error(err error) error {
	switch {
	case errors.Is(err, ErrSection21NotFound):
		return apperrors.NotFound("Resource not found.", err)
	case errors.Is(err, ErrSection21Forbidden):
		return apperrors.Forbidden("You may not act on this resource.", err)
	case errors.Is(err, ErrSection21Expired):
		return apperrors.Conflict("This resource is closed or expired.", err)
	case errors.Is(err, ErrSection21InvalidState), errors.Is(err, ErrSection21Conflict):
		return apperrors.Conflict("This operation conflicts with the current state.", err)
	case errors.Is(err, ErrSection21Capacity):
		return apperrors.Conflict("Required production capacity is unavailable.", err)
	case errors.Is(err, ErrSection21CheckoutNeeded):
		return apperrors.Conflict("An approved CheckoutQuote is required.", err)
	case errors.Is(err, ErrSection21Payment), errors.Is(err, ErrSection21PaymentPolicy):
		return apperrors.Conflict("The required Wallet payment policy could not be completed.", err)
	case errors.Is(err, ErrSection21Schedule):
		return apperrors.Conflict("The recurring feeding schedule is invalid or no longer writable.", err)
	case errors.Is(err, ErrRestrictedGoods):
		return apperrors.Validation("The requested goods are not eligible for Vendor Commerce.", nil)
	case errors.Is(err, ErrSection21Disabled):
		return apperrors.Unavailable("Section 21 commerce is unavailable.", err)
	default:
		return apperrors.Internal("Section 21 commerce operation failed.", err)
	}
}

func respondSection21(c *gin.Context, err error)      { httpx.RespondError(c, section21Error(err)) }
func section21Customer(c *gin.Context) (string, bool) { return customerSubject(c) }
func section21Vendor(c *gin.Context) (string, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(staff.VendorID), true
}
func section21Staff(c *gin.Context) (Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return Actor{}, false
	}
	return Actor{StaffID: staff.StaffID, VendorID: staff.VendorID, SessionID: staff.SessionID, CorrelationID: httpx.GetRequestID(c)}, true
}

type scheduledOrderRequest struct{ ScheduledOrderInput }
type groupRequest struct {
	BusinessID  string    `json:"business_id"`
	BranchID    string    `json:"branch_id"`
	ClosingAt   time.Time `json:"closing_at"`
	BudgetMinor *int64    `json:"budget_minor"`
	Currency    string    `json:"currency"`
}
type joinGroupRequest struct {
	InviteToken string `json:"invite_token"`
	DisplayName string `json:"display_name"`
}
type selectionRequest struct{ GroupSelection }
type convertRequest struct {
	QuoteID          string `json:"quote_id"`
	QuoteFingerprint string `json:"quote_fingerprint"`
}
type rfqRequest struct{ RFQInput }
type rfqQuoteRequest struct {
	VendorID       string              `json:"vendor_id"`
	BranchID       string              `json:"branch_id"`
	PriceMinor     int64               `json:"price_minor"`
	Currency       string              `json:"currency"`
	Snapshot       map[string]any      `json:"snapshot"`
	Capacity       CapacityRequirement `json:"capacity"`
	ExpiresAt      time.Time           `json:"expires_at"`
	IdempotencyKey string              `json:"idempotency_key"`
}
type rfqCounterOfferRequest struct {
	PriceMinor     int64               `json:"price_minor"`
	Currency       string              `json:"currency"`
	Snapshot       map[string]any      `json:"snapshot"`
	Capacity       CapacityRequirement `json:"capacity"`
	ExpiresAt      time.Time           `json:"expires_at"`
	IdempotencyKey string              `json:"idempotency_key"`
}
type rfqAcceptRequest struct {
	QuoteID         string `json:"quote_id"`
	Revision        int64  `json:"revision"`
	CheckoutQuoteID string `json:"checkout_quote_id"`
}
type recurringScheduleRequest struct{ RecurringScheduleInput }
type scheduleStatusRequest struct {
	Status          string `json:"status"`
	ExpectedVersion int64  `json:"expected_version"`
}

func (h *section21Handler) createScheduled(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	var in scheduledOrderRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	if err := h.service.AuthorizeStaff(c.Request.Context(), actor, in.BranchID); err != nil {
		respondSection21(c, err)
		return
	}
	in.BusinessID = actor.VendorID
	out, created, err := h.service.AcceptScheduled(c.Request.Context(), in.ScheduledOrderInput)
	if err != nil {
		respondSection21(c, err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	c.JSON(code, gin.H{"success": true, "data": out, "created": created})
}
func (h *section21Handler) cancelScheduled(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	if err := h.service.CancelScheduledByStaff(c.Request.Context(), actor, c.Param("id")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) createGroup(c *gin.Context) {
	organizer, ok := section21Customer(c)
	if !ok {
		return
	}
	var in groupRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	g, p, err := h.service.CreateGroup(c.Request.Context(), organizer, in.BusinessID, in.BranchID, in.ClosingAt, in.BudgetMinor, in.Currency)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"group": g, "organizer_participant": p}})
}
func (h *section21Handler) closeGroup(c *gin.Context) {
	organizer, ok := section21Customer(c)
	if !ok {
		return
	}
	if err := h.service.CloseGroup(c.Request.Context(), organizer, c.Param("groupId")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) convertGroup(c *gin.Context) {
	organizer, ok := section21Customer(c)
	if !ok {
		return
	}
	var in convertRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	o, created, err := h.service.ConvertGroup(c.Request.Context(), organizer, c.Param("groupId"), in.QuoteID, in.QuoteFingerprint)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": o, "created": created, "payment_model": "ORGANIZER_PAID"})
}
func (h *section21Handler) joinGroup(c *gin.Context) {
	var in joinGroupRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	p, err := h.service.JoinGroup(c.Request.Context(), c.Param("groupId"), in.InviteToken, in.DisplayName)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}
func (h *section21Handler) selection(c *gin.Context) {
	var in selectionRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	in.ID = c.Param("selectionId")
	p := c.GetHeader("X-Group-Participant-Token")
	if strings.TrimSpace(p) == "" {
		httpx.RespondError(c, apperrors.Unauthorized("A group participant token is required.", nil))
		return
	}
	out, err := h.service.UpsertGroupSelection(c.Request.Context(), c.Param("groupId"), p, in.GroupSelection)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) deleteSelection(c *gin.Context) {
	token := c.GetHeader("X-Group-Participant-Token")
	if strings.TrimSpace(token) == "" {
		httpx.RespondError(c, apperrors.Unauthorized("A group participant token is required.", nil))
		return
	}
	if err := h.service.DeleteGroupSelection(c.Request.Context(), c.Param("groupId"), token, c.Param("selectionId")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) createRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	var in rfqRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	in.RequesterID = requester
	out, err := h.service.CreateRFQ(c.Request.Context(), in.RFQInput)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": out})
}
func (h *section21Handler) openRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	if err := h.service.OpenRFQ(c.Request.Context(), requester, c.Param("rfqId")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) quoteRFQ(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	var in rfqQuoteRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	if err := h.service.AuthorizeStaff(c.Request.Context(), actor, in.BranchID); err != nil {
		respondSection21(c, err)
		return
	}
	in.VendorID = actor.VendorID
	out, err := h.service.SubmitRFQQuote(c.Request.Context(), c.Param("rfqId"), in.VendorID, in.BranchID, in.PriceMinor, in.Currency, in.Snapshot, in.Capacity, in.ExpiresAt, in.IdempotencyKey)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": out})
}
func (h *section21Handler) listRFQQuotes(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	out, err := h.service.ListRFQQuotes(c.Request.Context(), requester, c.Param("rfqId"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) counterRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	var in rfqCounterOfferRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	out, err := h.service.SubmitRFQCounterOffer(c.Request.Context(), requester, c.Param("rfqId"), c.Param("quoteId"), CounterOfferInput{PriceMinor: in.PriceMinor, Currency: in.Currency, Snapshot: in.Snapshot, Capacity: in.Capacity, ExpiresAt: in.ExpiresAt, IdempotencyKey: in.IdempotencyKey})
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": out})
}
func (h *section21Handler) acceptRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	var in rfqAcceptRequest
	if err := strictJSON(c, &in); err != nil {
		respondSection21(c, err)
		return
	}
	out, err := h.service.AcceptRFQQuote(c.Request.Context(), requester, c.Param("rfqId"), in.QuoteID, in.Revision, in.CheckoutQuoteID)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) convertRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	if err := h.service.ConvertRFQ(c.Request.Context(), requester, c.Param("rfqId")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) cancelRFQ(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	if err := h.service.CancelRFQ(c.Request.Context(), requester, c.Param("rfqId")); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) rfqConversionStatus(c *gin.Context) {
	requester, ok := section21Customer(c)
	if !ok {
		return
	}
	out, err := h.service.GetRFQConversionStatus(c.Request.Context(), requester, c.Param("rfqId"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) createRecurringSchedule(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	var request recurringScheduleRequest
	if err := strictJSON(c, &request); err != nil {
		respondSection21(c, err)
		return
	}
	request.BusinessID = actor.VendorID
	if err := h.service.AuthorizeStaff(c.Request.Context(), actor, request.BranchID); err != nil {
		respondSection21(c, err)
		return
	}
	out, created, err := h.service.CreateRecurringSchedule(c.Request.Context(), request.RecurringScheduleInput)
	if err != nil {
		respondSection21(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"success": true, "data": out, "created": created})
}
func (h *section21Handler) setRecurringScheduleStatus(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	var request scheduleStatusRequest
	if err := strictJSON(c, &request); err != nil {
		respondSection21(c, err)
		return
	}
	if request.ExpectedVersion <= 0 {
		request.ExpectedVersion = 1
	}
	if err := h.service.SetRecurringScheduleStatusByStaff(c.Request.Context(), actor, c.Param("scheduleId"), request.Status, request.ExpectedVersion); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) updateRecurringSchedule(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		respondSection21(c, err)
		return
	}
	var request recurringScheduleRequest
	if err = strictJSON(c, &request); err != nil {
		respondSection21(c, err)
		return
	}
	if err = h.service.UpdateRecurringSchedule(c.Request.Context(), actor, c.Param("scheduleId"), request.RecurringScheduleInput, version); err != nil {
		respondSection21(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *section21Handler) listRecurringOccurrences(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.ListRecurringOccurrencesByStaff(c.Request.Context(), actor, c.Param("scheduleId"), 100)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// RegisterSection21VendorRoutes mounts staff-only scheduled/bulk and vendor
// RFQ quote operations beneath the existing protected Vendor router.
func RegisterSection21VendorRoutes(root *gin.RouterGroup, service *Section21Service, enabled bool) {
	if root == nil || service == nil || !enabled {
		return
	}
	h := &section21Handler{service: service}
	g := root.Group("/commerce/section21")
	g.POST("/scheduled-orders", h.createScheduled)
	g.POST("/scheduled-orders/:id/cancel", h.cancelScheduled)
	g.POST("/rfqs/:rfqId/quotes", h.quoteRFQ)
	g.POST("/recurring-schedules", h.createRecurringSchedule)
	g.PUT("/recurring-schedules/:scheduleId", h.updateRecurringSchedule)
	g.POST("/recurring-schedules/:scheduleId/status", h.setRecurringScheduleStatus)
	g.GET("/recurring-schedules/:scheduleId/occurrences", h.listRecurringOccurrences)
}

// RegisterSection21CustomerRoutes mounts customer-owned group and requester
// RFQ commands. Participant mutation is token-scoped and intentionally has no
// broad staff or customer bearer requirement.
func RegisterSection21CustomerRoutes(router *gin.Engine, service *Section21Service, signer *sharedauth.TokenSigner, enabled bool) {
	if router == nil || service == nil || signer == nil || !enabled {
		return
	}
	h := &section21Handler{service: service}
	g := router.Group("/api/v1/commerce/section21")
	customer := g.Group("")
	customer.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	customer.POST("/groups", h.createGroup)
	customer.POST("/groups/:groupId/close", h.closeGroup)
	customer.POST("/groups/:groupId/convert", h.convertGroup)
	customer.POST("/rfqs", h.createRFQ)
	customer.POST("/rfqs/:rfqId/open", h.openRFQ)
	customer.GET("/rfqs/:rfqId/quotes", h.listRFQQuotes)
	customer.POST("/rfqs/:rfqId/quotes/:quoteId/counter", h.counterRFQ)
	customer.POST("/rfqs/:rfqId/accept", h.acceptRFQ)
	customer.POST("/rfqs/:rfqId/convert", h.convertRFQ)
	customer.GET("/rfqs/:rfqId/conversion", h.rfqConversionStatus)
	customer.POST("/rfqs/:rfqId/cancel", h.cancelRFQ)
	g.POST("/groups/:groupId/join", h.joinGroup)
	g.PUT("/groups/:groupId/selections/:selectionId", h.selection)
	g.DELETE("/groups/:groupId/selections/:selectionId", h.deleteSelection)
}
