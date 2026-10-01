package managedexec

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Handler exposes Taxi's half of the Fleet execution contract.
//
// Every route here is internal and HMAC-authenticated: these are Fleet's
// calls INTO Taxi, never a provider's or a passenger's. The paths and JSON
// field names deliberately mirror Dispatch's existing marketplace surface,
// because Fleet's TaxiExecution adapter is literally the Dispatch adapter
// pointed at Taxi's base path -- inventing a different shape here would mean
// inventing a second protocol, which Section 8 TAX-012 forbids.
type Handler struct {
	service *Service
	enabled bool
	// allowed is the set of service callers permitted to drive execution.
	allowed map[string]bool
}

// NewHandler builds the Fleet-facing execution handler.
func NewHandler(service *Service, enabled bool, allowedCallers []string) *Handler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		for _, part := range strings.Split(caller, ",") {
			if name := strings.TrimSpace(part); name != "" {
				allowed[name] = true
			}
		}
	}
	return &Handler{service: service, enabled: enabled, allowed: allowed}
}

// RegisterRoutes mounts the internal marketplace surface.
//
// A nil verifier means no internal caller is trusted yet, so nothing is
// mounted at all -- the same "disabled integration, no half-open surface"
// convention the rest of this service follows.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier) {
	if handler == nil || handler.service == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/marketplace")
	routes.Use(serviceauth.Middleware(verifier))
	routes.Use(handler.requireAllowedCaller)

	routes.POST("/reservations", handler.Reserve)
	routes.GET("/reservations/:sourceRequestId", handler.ReadReservation)
	routes.POST("/reservations/:sourceRequestId/release", handler.ReleaseReservation)
	routes.GET("/eligibility", handler.Eligibility)
	routes.POST("/executions", handler.ConfirmExecution)
	routes.GET("/executions/by-idempotency/:idempotencyKey", handler.ReadExecution)
	routes.GET("/requests/:sourceRequestId", handler.ReadRequest)
	routes.GET("/trips/:sourceTripId", handler.ReadTrip)
	routes.POST("/trips/:sourceTripId/cancel", handler.CancelExecution)
}

// requireAllowedCaller restricts execution to the services allowed to drive
// it. HMAC proves WHO is calling; this decides whether that caller may.
//
// It renders its own failure with httpx.RespondError rather than httpx.Abort,
// for the same reason serviceauth.Middleware does: this is a security
// boundary, and httpx.Abort only attaches the error for a downstream error
// middleware to render. A caller mounting this group without that middleware
// would otherwise get a silent 200 instead of a refusal.
func (h *Handler) requireAllowedCaller(c *gin.Context) {
	if !h.enabled {
		httpx.RespondError(c, apperrors.Unavailable("Taxi Fleet managed execution is not enabled.", nil))
		c.Abort()
		return
	}
	caller := c.GetString("service_name")
	if len(h.allowed) > 0 && !h.allowed[caller] {
		httpx.RespondError(c, apperrors.Forbidden("Service is not allowed to drive taxi execution.", nil))
		c.Abort()
		return
	}
	c.Next()
}

// ok writes the shared success envelope Fleet's adapter expects.
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

type reserveBody struct {
	SourceRequestID          string `json:"source_request_id"`
	ExpectedSourceVersion    int64  `json:"expected_source_version"`
	OrganizationID           string `json:"organization_id"`
	AcceptanceSource         string `json:"acceptance_source"`
	AcceptingProviderID      string `json:"accepting_provider_id"`
	OrganizationMembershipID string `json:"organization_membership_id"`
	IdempotencyKey           string `json:"idempotency_key"`
}

// Reserve claims a taxi booking for one organization.
func (h *Handler) Reserve(c *gin.Context) {
	var body reserveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if body.IdempotencyKey == "" {
		body.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	outcome, err := h.service.Reserve(c.Request.Context(), ReserveInput{
		SourceRequestID: body.SourceRequestID, ExpectedSourceVersion: body.ExpectedSourceVersion,
		OrganizationID: body.OrganizationID, AcceptanceSource: body.AcceptanceSource,
		AcceptingProviderID:      body.AcceptingProviderID,
		OrganizationMembershipID: body.OrganizationMembershipID,
		IdempotencyKey:           body.IdempotencyKey,
		CorrelationID:            c.GetHeader("X-Correlation-ID"),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, outcome)
}

// ReadReservation resolves an ambiguous reservation by read-back.
func (h *Handler) ReadReservation(c *gin.Context) {
	outcome, err := h.service.ReadReservation(c.Request.Context(),
		c.Param("sourceRequestId"), c.Query("idempotency_key"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, outcome)
}

type releaseBody struct {
	OrganizationID string `json:"organization_id"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ReleaseReservation returns a reserved booking to the open pool.
func (h *Handler) ReleaseReservation(c *gin.Context) {
	var body releaseBody
	// A release with no body is still a valid release: the path already names
	// the request, so an unparseable body must not strand the reservation.
	_ = c.ShouldBindJSON(&body)
	if err := h.service.ReleaseReservation(c.Request.Context(),
		c.Param("sourceRequestId"), body.OrganizationID); err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, gin.H{"released": true})
}

// Eligibility answers Taxi's own readiness question for a provider.
func (h *Handler) Eligibility(c *gin.Context) {
	answer, err := h.service.Eligibility(c.Request.Context(),
		c.Query("source_request_id"), c.Query("provider_id"), c.Query("vehicle_reference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, answer)
}

type confirmBody struct {
	RequestID                string `json:"requestId"`
	ExpectedRequestVersion   int64  `json:"expectedRequestVersion"`
	ReservationID            string `json:"reservationId"`
	OrganizationID           string `json:"organizationId"`
	SettlementOrganizationID string `json:"settlementOrganizationId"`
	ExecutingProviderID      string `json:"executingProviderId"`
	OrganizationMembershipID string `json:"organizationMembershipId"`
	VehicleID                string `json:"vehicleId"`
	FleetAssignmentID        string `json:"fleetAssignmentId"`
	FleetAssignmentVersion   int64  `json:"fleetAssignmentVersion"`
	ProviderResponseID       string `json:"providerResponseId"`
	AcceptanceSource         string `json:"acceptanceSource"`
	ExecutionSource          string `json:"executionSource"`
}

// ConfirmExecution binds the managed provider and creates the canonical taxi
// execution.
func (h *Handler) ConfirmExecution(c *gin.Context) {
	var body confirmBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	outcome, err := h.service.ConfirmProviderAndCreateExecution(c.Request.Context(), ConfirmInput{
		SourceRequestID: body.RequestID, ExpectedSourceVersion: body.ExpectedRequestVersion,
		ReservationID: body.ReservationID, OrganizationID: body.OrganizationID,
		SettlementOrganizationID: body.SettlementOrganizationID,
		ExecutingProviderID:      body.ExecutingProviderID,
		OrganizationMembershipID: body.OrganizationMembershipID,
		VehicleID:                body.VehicleID, FleetAssignmentID: body.FleetAssignmentID,
		FleetAssignmentVersion: body.FleetAssignmentVersion,
		ProviderResponseID:     body.ProviderResponseID, AcceptanceSource: body.AcceptanceSource,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		CorrelationID:  c.GetHeader("X-Correlation-ID"),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, outcome)
}

// ReadExecution resolves an ambiguous execution creation by read-back.
func (h *Handler) ReadExecution(c *gin.Context) {
	outcome, err := h.service.ReadExecutionOutcome(c.Request.Context(), c.Param("idempotencyKey"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, outcome)
}

// ReadRequest returns the authoritative booking state for reconciliation.
func (h *Handler) ReadRequest(c *gin.Context) {
	view, err := h.service.ReadRequest(c.Request.Context(), c.Param("sourceRequestId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, view)
}

type cancelBody struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CancelExecution cancels a canonical taxi execution on Fleet's instruction.
func (h *Handler) CancelExecution(c *gin.Context) {
	var body cancelBody
	_ = c.ShouldBindJSON(&body)
	if err := h.service.CancelExecution(c.Request.Context(), c.Param("sourceTripId"), body.Reason); err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, gin.H{"cancelled": true})
}

// ReadTrip returns the authoritative trip state for reconciliation.
func (h *Handler) ReadTrip(c *gin.Context) {
	view, err := h.service.ReadTrip(c.Request.Context(), c.Param("sourceTripId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	ok(c, view)
}
