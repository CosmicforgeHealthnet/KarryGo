package orders

import (
	"errors"
	"net/http"
	"strconv"
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
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		return typed
	}
	switch {
	case errors.Is(err, errScheduleView):
		return apperrors.Validation("Choose a valid schedule filter.", nil)
	case errors.Is(err, ErrSection21NotFound):
		return apperrors.NotFound("We could not find that schedule or quote. Refresh and try again.", err)
	case errors.Is(err, ErrSection21Forbidden), errors.Is(err, ErrForbidden):
		return apperrors.Forbidden("You do not have permission to do this for this branch.", err)
	case errors.Is(err, ErrSection21Expired):
		return apperrors.Conflict("This has closed or expired. Refresh to see the latest.", err)
	case errors.Is(err, ErrSection21InvalidState), errors.Is(err, ErrSection21Conflict):
		return apperrors.Conflict("This has changed since you opened it. Refresh and try again.", err)
	case errors.Is(err, ErrSection21Capacity):
		return apperrors.Conflict("There is not enough kitchen capacity for that time. Choose another time or skip reserving capacity.", err)
	case errors.Is(err, ErrSection21CheckoutNeeded):
		return apperrors.Conflict("The customer needs to approve their order before it can be scheduled.", err)
	case errors.Is(err, ErrSection21Payment), errors.Is(err, ErrSection21PaymentPolicy):
		return apperrors.Conflict("The payment could not be completed. Please try again shortly.", err)
	case errors.Is(err, ErrSection21Schedule):
		return apperrors.Conflict("This meal schedule cannot be changed now. Check the days, time and dates, or refresh.", err)
	case errors.Is(err, ErrRestrictedGoods):
		return apperrors.Validation("The requested goods are not eligible for Vendor Commerce.", nil)
	case errors.Is(err, ErrSection21Disabled):
		return apperrors.Unavailable("Scheduled orders are temporarily unavailable. Please try again shortly.", err)
	default:
		return apperrors.Internal("Something went wrong on our side. Please try again.", err)
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
	out, created, err := h.service.CreateRecurringByStaff(c.Request.Context(), actor, request.RecurringScheduleInput)
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

func scheduledReadFilter(c *gin.Context) ScheduledReadFilter {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return normalizeScheduledFilter(ScheduledReadFilter{BranchID: c.Query("branch_id"), View: c.Query("view"), Limit: limit, Offset: offset})
}
func (h *section21Handler) listScheduled(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	f := scheduledReadFilter(c)
	rows, err := h.service.ListScheduledByStaff(c.Request.Context(), actor, f)
	if err != nil {
		respondSection21(c, err)
		return
	}
	more := len(rows) > f.Limit
	if more {
		rows = rows[:f.Limit]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "pagination": gin.H{"limit": f.Limit, "offset": f.Offset, "has_more": more}}})
}
func (h *section21Handler) getScheduled(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.GetScheduledByStaff(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) listRecurringSchedules(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	f := scheduledReadFilter(c)
	rows, err := h.service.ListRecurringByStaff(c.Request.Context(), actor, f)
	if err != nil {
		respondSection21(c, err)
		return
	}
	more := len(rows) > f.Limit
	if more {
		rows = rows[:f.Limit]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "pagination": gin.H{"limit": f.Limit, "offset": f.Offset, "has_more": more}}})
}
func (h *section21Handler) getRecurringSchedule(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.GetRecurringByStaff(c.Request.Context(), actor, c.Param("scheduleId"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

func (h *section21Handler) listVendorRFQs(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	f := scheduledReadFilter(c)
	rows, err := h.service.ListVendorRFQs(c.Request.Context(), actor, f)
	if err != nil {
		respondSection21(c, err)
		return
	}
	more := len(rows) > f.Limit
	if more {
		rows = rows[:f.Limit]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "pagination": gin.H{"limit": f.Limit, "offset": f.Offset, "has_more": more}}})
}
func (h *section21Handler) getVendorRFQ(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.GetVendorRFQ(c.Request.Context(), actor, c.Param("rfqId"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *section21Handler) vendorRFQQuotes(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.ListVendorRFQQuotes(c.Request.Context(), actor, c.Param("rfqId"))
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

func (h *section21Handler) listSchedulingQuotes(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	f := scheduledReadFilter(c)
	out, err := h.service.ListSchedulingQuotes(c.Request.Context(), actor, f)
	if err != nil {
		respondSection21(c, err)
		return
	}
	more := len(out) > f.Limit
	if more {
		out = out[:f.Limit]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": out, "pagination": gin.H{"limit": f.Limit, "offset": f.Offset, "has_more": more}}})
}
func (h *section21Handler) getSchedulingQuote(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	out, err := h.service.GetSchedulingQuote(c.Request.Context(), actor, c.Param("quoteId"), c.Query("branch_id"))
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
	g.GET("/recurring-setup-contexts", h.listRecurringSetupContexts)
	g.GET("/scheduling-quotes", h.listSchedulingQuotes)
	g.GET("/scheduling-quotes/:quoteId", h.getSchedulingQuote)
	g.GET("/scheduled-orders", h.listScheduled)
	g.GET("/scheduled-orders/:id", h.getScheduled)
	g.GET("/recurring-schedules", h.listRecurringSchedules)
	g.GET("/recurring-schedules/:scheduleId", h.getRecurringSchedule)
	g.POST("/scheduled-orders", h.createScheduled)
	g.POST("/scheduled-orders/:id/cancel", h.cancelScheduled)
	g.GET("/rfqs", h.listVendorRFQs)
	g.GET("/rfqs/:rfqId", h.getVendorRFQ)
	g.GET("/rfqs/:rfqId/quotes", h.vendorRFQQuotes)
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

func (h *section21Handler) listRecurringSetupContexts(c *gin.Context) {
	actor, ok := section21Staff(c)
	if !ok {
		return
	}
	f := scheduledReadFilter(c)
	out, more, err := h.service.ListRecurringSetupContexts(c.Request.Context(), actor, f)
	if err != nil {
		respondSection21(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": out, "pagination": gin.H{"limit": f.Limit, "offset": f.Offset, "has_more": more, "next_offset": f.Offset + f.Limit}}})
}
