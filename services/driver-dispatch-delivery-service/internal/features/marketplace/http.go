package marketplace

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Header names for the cross-service contract.
const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderCorrelationID  = "X-Correlation-ID"
	HeaderRequestID      = "X-Request-ID"
)

// AllowCallers restricts an authenticated internal route to named services.
//
// serviceauth proves *who* the caller is; it does not decide what they may
// reach. Holding a valid signature for some other integration must not grant
// access to organization reservations, so authorization is a separate gate.
func AllowCallers(allowed ...string) gin.HandlerFunc {
	permitted := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		permitted[name] = true
	}
	return func(c *gin.Context) {
		// The name comes from the verified signature, never from the body.
		name, _ := c.Get("service_name")
		serviceName, _ := name.(string)
		if serviceName == "" || !permitted[serviceName] {
			httpx.Abort(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
			return
		}
		c.Next()
	}
}

// Handler exposes the internal marketplace API.
type Handler struct {
	service *Service
	flags   Flags
}

// NewHandler builds the handler.
func NewHandler(service *Service, flags Flags) *Handler {
	return &Handler{service: service, flags: flags}
}

// RegisterInternalRoutes mounts the internal marketplace API.
//
// Every route sits behind service authentication AND the caller allowlist.
// There is no unauthenticated variant of any of these endpoints.
func RegisterInternalRoutes(group *gin.RouterGroup, handler *Handler,
	verifier *serviceauth.Verifier, allowedCallers []string) {
	group.Use(serviceauth.Middleware(verifier))
	group.Use(AllowCallers(allowedCallers...))

	group.POST("/reservations", handler.CreateReservation)
	group.GET("/reservations/:reservationId", handler.GetReservation)
	group.GET("/reservations/by-idempotency/:idempotencyKey", handler.GetReservationByIdempotency)
	group.POST("/reservations/:reservationId/release", handler.ReleaseReservation)

	group.GET("/requests/:requestId", handler.GetCanonicalRequest)
	group.GET("/requests/:requestId/reservation", handler.GetRequestReservation)

	group.POST("/eligibility/providers/validate", handler.ValidateProvider)
	group.POST("/eligibility/vehicles/validate", handler.ValidateVehicle)

	group.POST("/executions", handler.CreateExecution)
	group.GET("/executions/by-idempotency/:idempotencyKey", handler.GetExecutionByIdempotency)
	group.GET("/executions/by-assignment/:fleetAssignmentId", handler.GetExecutionByAssignment)
	group.GET("/trips/:tripId", handler.GetTrip)
}

type providerEligibilityBody struct {
	RequestID                string `json:"requestId"`
	OrganizationID           string `json:"organizationId"`
	ProviderID               string `json:"providerId"`
	OrganizationMembershipID string `json:"organizationMembershipId"`
	ExpectedRequestVersion   int64  `json:"expectedRequestVersion"`
}

// ValidateProvider answers a fleet provider-eligibility question.
func (h *Handler) ValidateProvider(c *gin.Context) {
	var body providerEligibilityBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.ValidateProvider(c.Request.Context(), ProviderEligibilityInput{
		RequestID:                body.RequestID,
		OrganizationID:           body.OrganizationID,
		ProviderID:               body.ProviderID,
		OrganizationMembershipID: body.OrganizationMembershipID,
		ExpectedRequestVersion:   body.ExpectedRequestVersion,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

type vehicleEligibilityBody struct {
	RequestID              string `json:"requestId"`
	OrganizationID         string `json:"organizationId"`
	ProviderID             string `json:"providerId"`
	VehicleID              string `json:"vehicleId"`
	ExpectedRequestVersion int64  `json:"expectedRequestVersion"`
}

// ValidateVehicle answers a fleet vehicle-eligibility question.
func (h *Handler) ValidateVehicle(c *gin.Context) {
	var body vehicleEligibilityBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.ValidateVehicle(c.Request.Context(), VehicleEligibilityInput{
		RequestID:              body.RequestID,
		OrganizationID:         body.OrganizationID,
		ProviderID:             body.ProviderID,
		VehicleID:              body.VehicleID,
		ExpectedRequestVersion: body.ExpectedRequestVersion,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

type executionBody struct {
	RequestID                string `json:"requestId"`
	ExpectedRequestVersion   int64  `json:"expectedRequestVersion"`
	ReservationID            string `json:"reservationId"`
	OrganizationID           string `json:"organizationId"`
	SettlementOrganizationID string `json:"settlementOrganizationId"`
	ExecutingProviderID      string `json:"executingProviderId"`
	OrganizationMembershipID string `json:"organizationMembershipId"`
	FleetAssignmentID        string `json:"fleetAssignmentId"`
	ProviderResponseID       string `json:"providerResponseId"`
	AcceptanceSource         string `json:"acceptanceSource"`
	VehicleID                string `json:"vehicleId"`
	ExecutionSource          string `json:"executionSource"`
}

// CreateExecution creates the canonical organization-managed trip.
func (h *Handler) CreateExecution(c *gin.Context) {
	if !h.flags.InternalAPIEnabled {
		httpx.Abort(c, apperrors.Unavailable("The internal marketplace API is not enabled.", nil))
		return
	}
	var body executionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.CreateExecution(c.Request.Context(), ExecutionInput{
		RequestID:                body.RequestID,
		ExpectedRequestVersion:   body.ExpectedRequestVersion,
		ReservationID:            body.ReservationID,
		OrganizationID:           body.OrganizationID,
		SettlementOrganizationID: body.SettlementOrganizationID,
		ExecutingProviderID:      body.ExecutingProviderID,
		OrganizationMembershipID: body.OrganizationMembershipID,
		FleetAssignmentID:        body.FleetAssignmentID,
		ProviderResponseID:       body.ProviderResponseID,
		AcceptanceSource:         body.AcceptanceSource,
		VehicleID:                body.VehicleID,
		ExecutionSource:          body.ExecutionSource,
		IdempotencyKey:           c.GetHeader(HeaderIdempotencyKey),
		CorrelationID:            c.GetHeader(HeaderCorrelationID),
		RequestIDHeader:          c.GetHeader(HeaderRequestID),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusCreated, result)
}

// GetExecutionByIdempotency resolves an unknown execution outcome.
func (h *Handler) GetExecutionByIdempotency(c *gin.Context) {
	result, err := h.service.ReadExecutionByIdempotencyKey(c.Request.Context(),
		c.Param("idempotencyKey"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// GetExecutionByAssignment resolves an unknown outcome by fleet assignment.
func (h *Handler) GetExecutionByAssignment(c *gin.Context) {
	result, err := h.service.ReadExecutionByAssignment(c.Request.Context(),
		c.Param("fleetAssignmentId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// GetTrip returns the canonical trip read model.
func (h *Handler) GetTrip(c *gin.Context) {
	result, err := h.service.ReadTrip(c.Request.Context(), c.Param("tripId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

type createReservationBody struct {
	RequestID                string `json:"requestId"`
	ExpectedRequestVersion   int64  `json:"expectedRequestVersion"`
	OrganizationID           string `json:"organizationId"`
	AcceptanceSource         string `json:"acceptanceSource"`
	AcceptingProviderID      string `json:"acceptingProviderId"`
	OrganizationMembershipID string `json:"organizationMembershipId"`
}

// CreateReservation reserves a canonical request for an organization.
func (h *Handler) CreateReservation(c *gin.Context) {
	if !h.flags.InternalAPIEnabled {
		httpx.Abort(c, apperrors.Unavailable("The internal marketplace API is not enabled.", nil))
		return
	}
	var body createReservationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.service.Reserve(c.Request.Context(), ReserveInput{
		RequestID:                body.RequestID,
		ExpectedRequestVersion:   body.ExpectedRequestVersion,
		OrganizationID:           body.OrganizationID,
		AcceptanceSource:         body.AcceptanceSource,
		AcceptingProviderID:      body.AcceptingProviderID,
		OrganizationMembershipID: body.OrganizationMembershipID,
		// Idempotency and correlation come from headers, never the body, so a
		// caller cannot disguise a different command as a replay.
		IdempotencyKey: c.GetHeader(HeaderIdempotencyKey),
		CorrelationID:  c.GetHeader(HeaderCorrelationID),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusCreated, result)
}

// GetReservation reads a reservation by id.
func (h *Handler) GetReservation(c *gin.Context) {
	result, err := h.service.ReadReservation(c.Request.Context(), c.Param("reservationId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// GetReservationByIdempotency is the read-back after an unknown outcome.
func (h *Handler) GetReservationByIdempotency(c *gin.Context) {
	result, err := h.service.ReadReservationByIdempotencyKey(c.Request.Context(),
		c.Param("idempotencyKey"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// GetRequestReservation returns the live reservation on a request.
func (h *Handler) GetRequestReservation(c *gin.Context) {
	result, err := h.service.ReadReservationForRequest(c.Request.Context(), c.Param("requestId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

type releaseBody struct {
	ReasonCode                 string `json:"reasonCode"`
	ExpectedReservationVersion int64  `json:"expectedReservationVersion"`
}

// ReleaseReservation gives a request back to the marketplace.
func (h *Handler) ReleaseReservation(c *gin.Context) {
	if !h.flags.InternalAPIEnabled {
		httpx.Abort(c, apperrors.Unavailable("The internal marketplace API is not enabled.", nil))
		return
	}
	var body releaseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.service.Release(c.Request.Context(), ReleaseInput{
		ReservationID:              c.Param("reservationId"),
		ReasonCode:                 body.ReasonCode,
		ExpectedReservationVersion: body.ExpectedReservationVersion,
		IdempotencyKey:             c.GetHeader(HeaderIdempotencyKey),
		CorrelationID:              c.GetHeader(HeaderCorrelationID),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// GetCanonicalRequest returns reconciliation state for a request.
func (h *Handler) GetCanonicalRequest(c *gin.Context) {
	result, err := h.service.ReadCanonicalRequest(c.Request.Context(), c.Param("requestId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// respond writes the canonical {data} envelope.
func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{
		"requestId": c.GetHeader(HeaderRequestID),
	}})
}
