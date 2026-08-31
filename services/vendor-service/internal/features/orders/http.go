package orders

// Section 17E HTTP surface.
//
// The customer surface accepts a quote id and the fingerprint it is committing
// to, and nothing else. It accepts no price, fee, discount, funding split,
// vendor id, branch id or payment reference: every one of those is authority
// the server resolves from the approved quote.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxOrderCommandBody = 8 << 10

type createOrderRequest struct {
	QuoteFingerprint string `json:"quote_fingerprint"`
}

type cancelOrderRequest struct {
	ReasonCode string `json:"reason_code"`
}

type approveCommercialChangeRequest struct {
	QuoteFingerprint string `json:"quote_fingerprint"`
}

type lineAvailabilityRequest struct {
	PickedQuantity        int64            `json:"picked_quantity"`
	UnavailableQuantity   int64            `json:"unavailable_quantity"`
	ActualMeasureQuantity int64            `json:"actual_measure_quantity"`
	Outcome               LineAvailability `json:"outcome"`
}

type substitutionRequest struct {
	ProposedItemRef      string          `json:"proposed_item_ref"`
	ProposedVariantRef   string          `json:"proposed_variant_ref"`
	ProposedCatalogueRef string          `json:"proposed_catalogue_ref"`
	ReplacementSnapshot  json.RawMessage `json:"replacement_snapshot"`
	ProposedQuantity     int64           `json:"proposed_quantity"`
	ProposedPriceMinor   int64           `json:"proposed_price_minor"`
	Reason               string          `json:"reason"`
	ExpiresAt            *time.Time      `json:"expires_at"`
}

// Handler serves both Section 17E surfaces.
type Handler struct {
	service *Service
}

// NewHandler builds the order handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func strictJSON(c *gin.Context, target any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxOrderCommandBody+1))
	if err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	if len(body) > maxOrderCommandBody {
		return apperrors.BadRequest("Request body is too large.", nil)
	}
	// An empty body is a valid no-field command for the transitions that take
	// no input at all.
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
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

func customerSubject(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Type != sharedauth.TokenTypeAccess ||
		claims.Role != "customer" || claims.Service != "customer" ||
		strings.TrimSpace(claims.Subject) == "" {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func vendorActor(c *gin.Context) (Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return Actor{}, false
	}
	return Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
	}, true
}

// expectedVersion reads If-Match. A lifecycle command without it is refused
// rather than applied blind onto a state the caller never saw.
func expectedVersion(c *gin.Context) (int64, error) {
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0, apperrors.BadRequest("If-Match is required for order lifecycle commands.", nil)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, apperrors.BadRequest("If-Match must be the order version.", err)
	}
	return value, nil
}

func orderHTTPError(err error) error {
	var applicationError *apperrors.Error
	if errors.As(err, &applicationError) {
		return applicationError
	}
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return apperrors.NotFound("Order not found.", err)
	case errors.Is(err, ErrQuoteNotOrderable):
		return apperrors.Conflict(
			"This quote can no longer become an order. Create and approve a new quote.", err)
	case errors.Is(err, ErrFingerprintMismatch):
		return apperrors.Conflict("The quote fingerprint does not match. Reload the quote.", err)
	case errors.Is(err, ErrVersionConflict):
		return apperrors.Conflict("This order changed. Reload it and try again.", err)
	case errors.Is(err, ErrOrderConflict):
		return apperrors.Conflict("This order cannot make that transition from its current state.", err)
	case errors.Is(err, ErrNotAttributable):
		return apperrors.Conflict(
			"A commercial benefit on this quote cannot be attributed to a single vendor.", err)
	case errors.Is(err, ErrAllocationImbalance):
		return apperrors.Conflict(
			"This quote does not allocate exactly to the approved total.", err)
	case errors.Is(err, ErrBindingMismatch):
		return apperrors.Conflict(
			"This payment was started for a different agreement. Reload and try again.", err)
	case errors.Is(err, ErrPaymentNotBound):
		return apperrors.Conflict("This order cannot be paid in its current state.", err)
	case errors.Is(err, ErrPaymentConflict):
		return apperrors.Conflict("This payment cannot make that transition.", err)
	case errors.Is(err, ErrPaymentAuthority):
		return apperrors.Unavailable(
			"The payment authority could not complete this request. It will be retried.", err)
	case errors.Is(err, ErrPaymentNotConfigured):
		return apperrors.Unavailable("Payment is temporarily unavailable.", err)
	case errors.Is(err, ErrCommercialChangeNotFound):
		return apperrors.NotFound("Commercial change not found.", err)
	case errors.Is(err, ErrCommercialChangeExpired):
		return apperrors.Conflict("The replacement quote expired. Request a fresh change quote.", err)
	case errors.Is(err, ErrCommercialChangeConflict), errors.Is(err, ErrCommercialChangeRequired), errors.Is(err, ErrAdjustmentNotAvailable):
		return apperrors.Conflict("This commercial change cannot be applied in its current state.", err)
	case errors.Is(err, ErrForbidden):
		return apperrors.Forbidden("You may not act on this order.", err)
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrDependencyMissing):
		return apperrors.Unavailable("Order orchestration is temporarily unavailable.", err)
	default:
		return apperrors.Internal("Order operation failed.", err)
	}
}

func (h *Handler) requestCommercialChange(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	var request ChangeRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	change, err := h.service.RequestCommercialChange(c.Request.Context(), customerID, c.Param("orderId"), request)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": change})
}

func (h *Handler) getCommercialChange(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	change, err := h.service.GetCommercialChange(c.Request.Context(), customerID, c.Param("changeId"))
	if err != nil || change.ParentOrderID != c.Param("orderId") {
		if err == nil {
			err = ErrCommercialChangeNotFound
		}
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(change.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": change})
}

func (h *Handler) approveCommercialChange(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	var request approveCommercialChangeRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if strings.TrimSpace(request.QuoteFingerprint) == "" {
		httpx.RespondError(c, apperrors.BadRequest("quote_fingerprint is required.", nil))
		return
	}
	change, err := h.service.ApproveCommercialChange(c.Request.Context(), customerID, c.Param("changeId"), request.QuoteFingerprint)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(change.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": change})
}

func (h *Handler) rejectCommercialChange(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	if err := strictJSON(c, &struct{}{}); err != nil {
		httpx.RespondError(c, err)
		return
	}
	change, err := h.service.RejectCommercialChange(c.Request.Context(), customerID, c.Param("changeId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": change})
}

// -----------------------------------------------------------------------------
// Customer handlers
// -----------------------------------------------------------------------------

func (h *Handler) createOrder(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	var request createOrderRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if strings.TrimSpace(request.QuoteFingerprint) == "" {
		httpx.RespondError(c, apperrors.BadRequest("quote_fingerprint is required.", nil))
		return
	}

	order, created, err := h.service.CreateFromQuote(
		c.Request.Context(), customerID, c.Param("quoteId"), request.QuoteFingerprint,
	)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}

	// A retry of the same approved quote is answered with the order that
	// already exists, never with a second one.
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.Header("ETag", strconv.FormatInt(order.Version, 10))
	c.JSON(status, gin.H{"success": true, "data": order})
}

func (h *Handler) getOrder(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	order, err := h.service.GetForCustomer(c.Request.Context(), customerID, c.Param("orderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(order.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

func (h *Handler) listOrders(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	orders, err := h.service.ListForCustomer(c.Request.Context(), customerID, limit)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	if orders == nil {
		orders = []ParentOrder{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"orders": orders}})
}

func (h *Handler) getOrderETA(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	eta, err := h.service.store.ListCustomerETA(c.Request.Context(), customerID, c.Param("orderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	if eta == nil {
		eta = []ProductionReadiness{}
	}
	missions, err := h.service.store.ListCurrentDeliveryETAForCustomer(c.Request.Context(), customerID, c.Param("orderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	if missions == nil {
		missions = []DeliveryETA{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"order_id": c.Param("orderId"), "allocations": eta, "missions": missions}})
}

func (h *Handler) cancelVendorOrderAsCustomer(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request cancelOrderRequest
	if err = strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}

	vendorOrder, parent, err := h.service.CancelByCustomer(
		c.Request.Context(), customerID, c.Param("vendorOrderId"), request.ReasonCode, version,
	)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	respondTransition(c, vendorOrder, parent)
}

// -----------------------------------------------------------------------------
// Payment handlers
// -----------------------------------------------------------------------------

// startPayment binds a payment operation to an approved quote.
//
// The body carries only the fingerprint the client is committing to. It cannot
// name a total, a currency, an allocation, a vendor or a payment reference:
// every one of those is read from the stored order.
func (h *Handler) startPayment(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	var request createOrderRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if strings.TrimSpace(request.QuoteFingerprint) == "" {
		httpx.RespondError(c, apperrors.BadRequest("quote_fingerprint is required.", nil))
		return
	}

	order, operation, err := h.service.StartPayment(
		c.Request.Context(), customerID, c.Param("quoteId"), request.QuoteFingerprint,
	)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}

	c.Header("ETag", strconv.FormatInt(order.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"order":   order,
		"payment": operation,
	}})
}

func (h *Handler) getPayment(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	operation, err := h.service.GetPaymentForCustomer(
		c.Request.Context(), customerID, c.Param("orderId"),
	)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(operation.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": operation})
}

// -----------------------------------------------------------------------------
// Vendor handlers
// -----------------------------------------------------------------------------

func (h *Handler) listVendorOrders(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	orders, err := h.service.ListForVendor(c.Request.Context(), actor, VendorQueueFilter{
		BranchID: strings.TrimSpace(c.Query("branch_id")),
		Status:   VendorStatus(strings.TrimSpace(c.Query("status"))),
		Limit:    limit,
	})
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	if orders == nil {
		orders = []VendorOrder{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"vendor_orders": orders}})
}

func (h *Handler) getVendorOrder(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	order, err := h.service.GetForVendor(c.Request.Context(), actor, c.Param("vendorOrderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(order.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

// advance is one handler per transition, so each command is its own route and
// its own OpenAPI operation rather than a switch on a client-supplied verb.
func (h *Handler) advance(transition Transition) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := vendorActor(c)
		if !ok {
			return
		}
		version, err := expectedVersion(c)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		var request cancelOrderRequest
		if err = strictJSON(c, &request); err != nil {
			httpx.RespondError(c, err)
			return
		}

		vendorOrder, parent, err := h.service.Advance(
			c.Request.Context(), actor, c.Param("vendorOrderId"),
			transition, request.ReasonCode, version,
		)
		if err != nil {
			httpx.RespondError(c, orderHTTPError(err))
			return
		}
		respondTransition(c, vendorOrder, parent)
	}
}

func (h *Handler) reportAvailability(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request lineAvailabilityRequest
	if err = strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	outcome, err := h.service.ReportLineAvailability(c.Request.Context(), actor, c.Param("vendorOrderId"), c.Param("lineId"), version, request.PickedQuantity, request.UnavailableQuantity, request.ActualMeasureQuantity, request.Outcome)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(outcome.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": outcome})
}

func (h *Handler) proposeSubstitution(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request substitutionRequest
	if err := strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", nil))
		return
	}
	proposal, err := h.service.ProposeSubstitution(c.Request.Context(), actor, c.Param("vendorOrderId"), c.Param("lineId"), SubstitutionProposal{Version: version, ProposedItemRef: request.ProposedItemRef, ProposedVariantRef: request.ProposedVariantRef, ProposedCatalogueRef: request.ProposedCatalogueRef, ReplacementSnapshot: request.ReplacementSnapshot, ProposedQuantity: request.ProposedQuantity, ProposedPriceMinor: request.ProposedPriceMinor, Reason: request.Reason, ExpiresAt: request.ExpiresAt}, key)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": proposal})
}

func (h *Handler) getSubstitution(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	p, err := h.service.store.GetSubstitutionForCustomer(c.Request.Context(), customerID, c.Param("proposalId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(p.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

func (h *Handler) respondSubstitution(approve bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		customerID, ok := customerSubject(c)
		if !ok {
			return
		}
		version, err := expectedVersion(c)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		p, err := h.service.RespondToSubstitution(c.Request.Context(), customerID, c.Param("proposalId"), approve, version)
		if err != nil {
			httpx.RespondError(c, orderHTTPError(err))
			return
		}
		c.Header("ETag", strconv.FormatInt(p.Version, 10))
		c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
	}
}

// respondTransition always returns the ParentOrder's re-derived aggregate state
// alongside the VendorOrder that moved, so a caller never has to guess whether
// a sibling allocation changed the purchase's overall outcome.
func respondTransition(c *gin.Context, vendorOrder VendorOrder, parent ParentOrder) {
	c.Header("ETag", strconv.FormatInt(vendorOrder.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"vendor_order": vendorOrder,
		"parent_order": gin.H{
			"parent_order_id":           parent.ID,
			"state":                     parent.State,
			"version":                   parent.Version,
			"vendor_order_count":        parent.VendorOrderCount,
			"refund_prepared_minor":     parent.RefundPreparedMinor,
			"settlement_prepared_minor": parent.SettlementPreparedMinor,
		},
	}})
}

// -----------------------------------------------------------------------------
// Registration
// -----------------------------------------------------------------------------

// RegisterCustomerRoutes mounts the customer order surface.
//
// There is deliberately no route that sets a payment reference, an allocation,
// a price or a vendor order status other than the customer's own withdrawal.
func RegisterCustomerRoutes(
	root *gin.RouterGroup,
	service *Service,
	signer *sharedauth.TokenSigner,
	idempotencyStore *idempotency.Store,
	enabled bool,
	paymentEnabled bool,
) {
	if root == nil || service == nil || signer == nil || !enabled {
		return
	}
	handler := NewHandler(service)
	group := root.Group("")
	group.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	group.Use(idempotency.CustomerCommandMiddleware(idempotencyStore))
	group.POST("/quotes/:quoteId/orders", handler.createOrder)
	group.GET("/orders", handler.listOrders)
	group.GET("/orders/:orderId", handler.getOrder)
	group.GET("/orders/:orderId/eta", handler.getOrderETA)
	group.POST("/orders/:orderId/support-cases", handler.createCustomerSupportCase)
	group.GET("/orders/:orderId/support-cases", handler.listCustomerSupportCases)
	group.GET("/orders/:orderId/support-cases/:caseRef", handler.getCustomerSupportCase)
	// A payment-capable order deployment is also the only deployment in which
	// a commercial change can complete. The handler still fails closed with a
	// dependency error if the quote authority was not wired, while the route
	// shape remains parity-testable against the fully enabled production surface.
	if paymentEnabled {
		group.POST("/orders/:orderId/commercial-changes", handler.requestCommercialChange)
		group.GET("/orders/:orderId/commercial-changes/:changeId", handler.getCommercialChange)
		group.POST("/commercial-changes/:changeId/approve", handler.approveCommercialChange)
		group.POST("/commercial-changes/:changeId/reject", handler.rejectCommercialChange)
	}
	group.POST("/vendor-orders/:vendorOrderId/cancel", handler.cancelVendorOrderAsCustomer)
	group.GET("/substitutions/:proposalId", handler.getSubstitution)
	group.POST("/substitutions/:proposalId/approve", handler.respondSubstitution(true))
	group.POST("/substitutions/:proposalId/reject", handler.respondSubstitution(false))
	group.GET("/substitution-cases", handler.pendingSubstitutionCases)
	group.GET("/substitution-cases/:caseId", handler.getSubstitutionCase)
	group.POST("/substitution-cases/:caseId/accept", handler.respondSubstitutionCase(true))
	group.POST("/substitution-cases/:caseId/reject", handler.respondSubstitutionCase(false))

	// Section 17F. The payment surface only exists when a real money
	// authority is wired, so a deployment without Wallet credentials exposes
	// no route to probe rather than one that can only answer 503.
	if paymentEnabled {
		group.POST("/quotes/:quoteId/payment", handler.startPayment)
		group.GET("/orders/:orderId/payment", handler.getPayment)
	}
}

// RegisterVendorRoutes mounts the vendor staff order surface.
func RegisterVendorRoutes(root *gin.RouterGroup, service *Service, enabled bool, idempotencyStores ...*idempotency.Store) {
	if root == nil || service == nil || !enabled {
		return
	}
	handler := NewHandler(service)
	group := root.Group("/commerce/orders")
	if len(idempotencyStores) > 0 {
		group.Use(idempotency.CommandMiddleware(idempotencyStores[0]))
	}
	group.GET("", handler.listVendorOrders)
	group.GET("/:vendorOrderId", handler.getVendorOrder)
	group.POST("/:vendorOrderId/support-cases", handler.createVendorSupportCase)
	group.GET("/:vendorOrderId/support-cases", handler.listVendorSupportCases)
	group.POST("/:vendorOrderId/accept", handler.advance(TransitionAccept))
	group.POST("/:vendorOrderId/ready", handler.advance(TransitionReady))
	group.POST("/:vendorOrderId/handover", handler.advance(TransitionHandover))
	group.POST("/:vendorOrderId/deliver", handler.advance(TransitionDeliver))
	group.POST("/:vendorOrderId/complete", handler.advance(TransitionComplete))
	group.POST("/:vendorOrderId/cancel", handler.advance(TransitionCancel))
	group.POST("/:vendorOrderId/reject", handler.advance(TransitionReject))
	group.POST("/:vendorOrderId/preparing", handler.advance(TransitionPreparing))
	group.POST("/:vendorOrderId/start_preparing", handler.advance(TransitionPreparing))
	group.POST("/:vendorOrderId/picking", handler.advance(TransitionPicking))
	group.POST("/:vendorOrderId/start_picking", handler.advance(TransitionPicking))
	group.POST("/:vendorOrderId/pack", handler.advance(TransitionPack))
	group.POST("/:vendorOrderId/ready-for-pickup", handler.advance(TransitionReadyForPickup))
	group.POST("/:vendorOrderId/ready_for_pickup", handler.advance(TransitionReadyForPickup))
	group.POST("/:vendorOrderId/pickup", handler.advance(TransitionPickup))
	group.POST("/:vendorOrderId/delivering", handler.advance(TransitionDelivering))
	group.POST("/:vendorOrderId/delivered", handler.advance(TransitionDelivered))
	group.POST("/:vendorOrderId/lines/:lineId/availability", handler.reportAvailability)
	group.POST("/:vendorOrderId/lines/:lineId/substitutions", handler.proposeSubstitution)
	group.GET("/:vendorOrderId/picking-queue", handler.pickingQueue)
	group.GET("/:vendorOrderId/picking", handler.pickingQueue)
	group.POST("/:vendorOrderId/lines/:lineId/quality", handler.recordQualityResult)
	group.POST("/:vendorOrderId/lines/:lineId/actual-quantity", handler.recordActualQuantity)
}
