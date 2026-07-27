package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
)

// providerServiceTokens maps the asserted provider service onto the source
// service that owns it. An execution service may only submit decisions for
// providers it actually authenticates: dispatch cannot answer for a taxi
// provider, and vice versa.
var providerServiceTokens = map[string]string{
	"DISPATCH": models.SourceDispatch,
	"TAXI":     models.SourceTaxi,
	"HAULING":  models.SourceHauling,
}

// callerOwnsProviderService pairs an authenticated caller with the provider
// service it is allowed to speak for.
var callerOwnsProviderService = map[string]string{
	"driver-dispatch-delivery-service": "DISPATCH",
	"driver-taxi-service":              "TAXI",
	"driver-hauling-service":           "HAULING",
}

// providerResponseBody is a provider decision relayed by its owning service.
//
// providerId is present but is NOT self-asserted trust: the owning service has
// already authenticated the provider's session and is vouching for the
// identity. Fleet still checks that the caller is allowed to speak for that
// provider service, and that the provider actually owns the assignment.
type providerResponseBody struct {
	ProviderID        string     `json:"providerId"`
	ProviderService   string     `json:"providerService"`
	ProviderSessionID string     `json:"providerSessionId"`
	ClientResponseID  string     `json:"clientResponseId"`
	ExpectedVersion   int64      `json:"expectedVersion"`
	ReasonCode        string     `json:"reasonCode"`
	Note              string     `json:"note"`
	EvidenceFileIDs   []string   `json:"evidenceFileIds"`
	DeviceTime        *time.Time `json:"deviceRecordedAt"`
}

// ProviderAssignmentView is the provider-safe assignment DTO.
//
// It is built field by field from an explicit allowlist. There is no path here
// that serializes an internal assignment or projection and strips fields
// afterwards, because a stripping filter fails open the moment an upstream
// model gains a commercial field.
type ProviderAssignmentView struct {
	AssignmentID     string    `json:"assignmentId"`
	Status           string    `json:"status"`
	AllowedActions   []string  `json:"allowedActions"`
	ResponseDeadline time.Time `json:"responseDeadline"`
	SourceService    string    `json:"sourceService"`
	Version          int64     `json:"version"`
	Replayed         bool      `json:"replayed"`
}

// newProviderAssignmentView builds the provider-facing response.
func newProviderAssignmentView(result *usecases.AssignmentResult, now time.Time) ProviderAssignmentView {
	view := ProviderAssignmentView{
		AllowedActions: []string{},
		Replayed:       result.Replayed,
	}
	if result.Assignment != nil {
		view.AssignmentID = result.Assignment.ID
		view.Status = result.Assignment.Status
		view.ResponseDeadline = result.Assignment.ResponseDeadline
		view.SourceService = result.Assignment.SourceService
		view.Version = result.Assignment.Version
		// Always server-computed from state, never taken from the caller and
		// never narrowed by policy.
		view.AllowedActions = result.Assignment.AllowedActions(now)
	}
	if result.AllowedActions != nil {
		view.AllowedActions = result.AllowedActions
	}
	return view
}

// RegisterProviderResponseRoutes mounts the internal provider-decision routes.
func RegisterProviderResponseRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	group.POST("/assignments/:assignmentId/provider-accept",
		handler.requireAllowedCaller, handler.ProviderAcceptAssignment)
	group.POST("/assignments/:assignmentId/provider-decline",
		handler.requireAllowedCaller, handler.ProviderDeclineAssignment)

	group.POST("/marketplace-offers/:offerId/provider-accept",
		handler.requireAllowedCaller, handler.ProviderAcceptOffer)
	group.POST("/marketplace-offers/:offerId/provider-decline",
		handler.requireAllowedCaller, handler.ProviderDeclineOffer)
}

// offerResponseBody extends the provider decision with the source references
// the delegated path needs.
type offerResponseBody struct {
	providerResponseBody
	DispatchReservationID  string `json:"dispatchReservationId"`
	DispatchRequestVersion int64  `json:"dispatchRequestVersion"`
}

// ProviderAcceptOffer records a provider's direct acceptance of an
// organization marketplace offer (the delegated path).
func (h *InternalHandler) ProviderAcceptOffer(c *gin.Context) {
	var body offerResponseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	actor, err := h.resolveProviderActor(c, body.providerResponseBody)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	result, err := h.service.AcceptOffer(c.Request.Context(), usecases.OfferResponseInput{
		Actor:                  actor,
		OfferID:                c.Param("offerId"),
		ClientResponseID:       body.ClientResponseID,
		ExpectedVersion:        body.ExpectedVersion,
		DispatchReservationID:  body.DispatchReservationID,
		DispatchRequestVersion: body.DispatchRequestVersion,
		RequestID:              c.GetHeader("X-Request-ID"),
		CorrelationID:          c.GetHeader("X-Correlation-ID"),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOffer(c, http.StatusOK, result)
}

// ProviderDeclineOffer records a provider's refusal of an organization offer.
//
// Consults no feature flag: refusing work already offered can never be
// switched off.
func (h *InternalHandler) ProviderDeclineOffer(c *gin.Context) {
	var body offerResponseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	actor, err := h.resolveProviderActor(c, body.providerResponseBody)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	result, err := h.service.DeclineOffer(c.Request.Context(), usecases.OfferResponseInput{
		Actor:            actor,
		OfferID:          c.Param("offerId"),
		ClientResponseID: body.ClientResponseID,
		ExpectedVersion:  body.ExpectedVersion,
		ReasonCode:       body.ReasonCode,
		Note:             body.Note,
		EvidenceFileIDs:  body.EvidenceFileIDs,
		RequestID:        c.GetHeader("X-Request-ID"),
		CorrelationID:    c.GetHeader("X-Correlation-ID"),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOffer(c, http.StatusOK, result)
}

// ProviderOfferView is the provider-safe offer-response DTO, built field by
// field from an allowlist.
type ProviderOfferView struct {
	OfferID        string   `json:"offerId"`
	RecipientID    string   `json:"recipientId"`
	Status         string   `json:"status"`
	AssignmentID   string   `json:"assignmentId,omitempty"`
	AllowedActions []string `json:"allowedActions"`
	Replayed       bool     `json:"replayed"`
}

func respondOffer(c *gin.Context, status int, result *usecases.OfferResponseResult) {
	view := ProviderOfferView{
		OfferID:        result.OfferID,
		RecipientID:    result.RecipientID,
		Status:         result.Status,
		AllowedActions: result.AllowedActions,
		Replayed:       result.Replayed,
	}
	if view.AllowedActions == nil {
		view.AllowedActions = []string{}
	}
	if result.Assignment != nil {
		view.AssignmentID = result.Assignment.ID
	}
	c.JSON(status, gin.H{"data": view, "meta": gin.H{
		"requestId": c.GetHeader("X-Request-ID"),
	}})
}

// resolveProviderActor validates the asserted provider identity.
//
// This is the boundary that stops a corporate user, a customer, or one
// execution service from recording a decision as if it were the provider.
func (h *InternalHandler) resolveProviderActor(c *gin.Context, body providerResponseBody) (usecases.ProviderActor, error) {
	raw, _ := c.Get("service_name")
	callerService, _ := raw.(string)

	// The caller may only speak for the provider service it owns.
	owned, known := callerOwnsProviderService[callerService]
	if !known {
		return usecases.ProviderActor{},
			apperrors.Forbidden("This service may not submit provider decisions.", nil)
	}
	if body.ProviderService == "" || body.ProviderService != owned {
		return usecases.ProviderActor{},
			apperrors.Forbidden("This service may not submit decisions for that provider service.", nil)
	}
	sourceService, ok := providerServiceTokens[body.ProviderService]
	if !ok {
		return usecases.ProviderActor{},
			apperrors.BadRequest("providerService is not recognised.", nil)
	}
	if body.ProviderID == "" {
		return usecases.ProviderActor{},
			apperrors.BadRequest("providerId is required.", nil)
	}
	// A decision must be traceable to an authenticated provider session in the
	// owning service; without it we cannot tell a real provider action from a
	// server-side replay of someone else's data.
	if body.ProviderSessionID == "" {
		return usecases.ProviderActor{},
			apperrors.BadRequest("providerSessionId is required.", nil)
	}
	if body.ClientResponseID == "" {
		return usecases.ProviderActor{},
			apperrors.BadRequest("clientResponseId is required.", nil)
	}

	return usecases.ProviderActor{
		ExecutingProviderID: body.ProviderID,
		SourceService:       sourceService,
	}, nil
}

// ProviderAcceptAssignment records a provider's acceptance.
func (h *InternalHandler) ProviderAcceptAssignment(c *gin.Context) {
	var body providerResponseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	actor, err := h.resolveProviderActor(c, body)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	result, err := h.service.AcceptAssignmentForProvider(c.Request.Context(),
		usecases.AcceptAssignmentInput{
			Actor:            actor,
			AssignmentID:     c.Param("assignmentId"),
			ClientResponseID: body.ClientResponseID,
			DeviceTime:       body.DeviceTime,
			ExpectedVersion:  body.ExpectedVersion,
			RequestID:        c.GetHeader("X-Request-ID"),
			CorrelationID:    c.GetHeader("X-Correlation-ID"),
		})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, newProviderAssignmentView(result, time.Now().UTC()))
}

// ProviderDeclineAssignment records a provider's refusal.
//
// Declining consults no feature flag. Accepting work can be disabled while a
// source service is degraded, but the ability to refuse work already offered
// can never be switched off — that would strip a provider of the only exit
// from an assignment they cannot safely perform.
func (h *InternalHandler) ProviderDeclineAssignment(c *gin.Context) {
	var body providerResponseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	actor, err := h.resolveProviderActor(c, body)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	result, err := h.service.DeclineAssignmentForProvider(c.Request.Context(),
		usecases.DeclineAssignmentInput{
			Actor:            actor,
			AssignmentID:     c.Param("assignmentId"),
			ClientResponseID: body.ClientResponseID,
			ReasonCode:       body.ReasonCode,
			Note:             body.Note,
			EvidenceFileIDs:  body.EvidenceFileIDs,
			ExpectedVersion:  body.ExpectedVersion,
			RequestID:        c.GetHeader("X-Request-ID"),
			CorrelationID:    c.GetHeader("X-Correlation-ID"),
		})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, newProviderAssignmentView(result, time.Now().UTC()))
}

// respondProvider writes the canonical envelope for a provider-safe view.
func respondProvider(c *gin.Context, status int, view ProviderAssignmentView) {
	c.JSON(status, gin.H{"data": view, "meta": gin.H{
		"requestId": c.GetHeader("X-Request-ID"),
	}})
}
