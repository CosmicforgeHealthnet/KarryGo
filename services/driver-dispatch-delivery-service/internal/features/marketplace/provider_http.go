package marketplace

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

// ProviderHandler serves the provider-facing organization marketplace routes.
type ProviderHandler struct {
	service *Service
	flags   Flags
}

// NewProviderHandler builds the handler.
func NewProviderHandler(service *Service, flags Flags) *ProviderHandler {
	return &ProviderHandler{service: service, flags: flags}
}

// RegisterProviderRoutes mounts the provider-facing routes.
//
// Exact paths:
//
//	GET  /api/v1/provider/organization-offers
//	GET  /api/v1/provider/organization-offers/:offerId
//	POST /api/v1/provider/organization-offers/:offerId/accept
//	POST /api/v1/provider/organization-offers/:offerId/decline
//
//	GET  /api/v1/provider/company-assignments
//	GET  /api/v1/provider/company-assignments/:assignmentId
//	POST /api/v1/provider/company-assignments/:assignmentId/accept
//	POST /api/v1/provider/company-assignments/:assignmentId/decline
//
// Both groups sit behind the same provider-session middleware and role guard
// as the existing trip routes. There is no unauthenticated variant, and a
// customer or corporate token cannot satisfy the dispatch-provider role.
func RegisterProviderRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase,
	handler *ProviderHandler) {
	offers := engine.Group("/api/v1/provider/organization-offers")
	offers.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProvider())
	offers.GET("", handler.ListOffers)
	offers.GET("/:offerId", handler.GetOffer)
	offers.POST("/:offerId/accept", handler.AcceptOffer)
	offers.POST("/:offerId/decline", handler.DeclineOffer)

	assignments := engine.Group("/api/v1/provider/company-assignments")
	assignments.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProvider())
	assignments.GET("", handler.ListAssignments)
	assignments.GET("/:assignmentId", handler.GetAssignment)
	assignments.POST("/:assignmentId/accept", handler.AcceptAssignment)
	assignments.POST("/:assignmentId/decline", handler.DeclineAssignment)
}

// requireDispatchProvider rejects any session that is not a dispatch provider.
//
// Mirrors the guard on the existing trip routes: a customer or corporate
// token may authenticate but must never authorize a provider action.
func requireDispatchProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden(
				"This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	}
}

// providerIdentity extracts the provider from the VERIFIED session.
//
// This is the only place a provider id enters a provider action. It is never
// read from the body, the query string, a path parameter or a client header,
// so a provider cannot act as somebody else by editing their request.
func providerIdentity(c *gin.Context) (providerID string, sessionID string) {
	return authhttp.DispatchRiderID(c), authhttp.SessionID(c)
}

// decisionBody is what a provider app may send.
//
// It deliberately has NO providerId field: supplying one is silently
// impossible rather than merely ignored, so there is no field for a client to
// hope might be honoured.
type decisionBody struct {
	ClientResponseID string   `json:"clientResponseId"`
	ExpectedVersion  int64    `json:"expectedVersion"`
	ReasonCode       string   `json:"reasonCode"`
	Note             string   `json:"note"`
	EvidenceFileIDs  []string `json:"evidenceFileIds"`
	// Delegated acceptance needs the canonical request and the organization
	// the provider is accepting on behalf of. providerId is still absent —
	// identity comes only from the session.
	RequestID      string `json:"requestId"`
	OrganizationID string `json:"organizationId"`
	MembershipID   string `json:"organizationMembershipId"`
}

func (h *ProviderHandler) decision(c *gin.Context) (ProviderDecision, bool) {
	var body decisionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return ProviderDecision{}, false
	}
	return ProviderDecision{
		OfferID:          c.Param("offerId"),
		AssignmentID:     c.Param("assignmentId"),
		ClientResponseID: body.ClientResponseID,
		ExpectedVersion:  body.ExpectedVersion,
		ReasonCode:       body.ReasonCode,
		Note:             body.Note,
		EvidenceFileIDs:  body.EvidenceFileIDs,
		RequestID:        c.GetHeader(HeaderRequestID),
		CorrelationID:    c.GetHeader(HeaderCorrelationID),
	}, true
}

// AcceptOffer records a provider's acceptance of an organization offer.
func (h *ProviderHandler) AcceptOffer(c *gin.Context) {
	if !h.flags.ProviderOffersEnabled {
		httpx.Abort(c, apperrors.Unavailable("Organization offers are not enabled.", nil))
		return
	}
	providerID, sessionID := providerIdentity(c)
	var body decisionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.service.AcceptOfferDelegated(c.Request.Context(), providerID, sessionID,
		DelegatedOfferInput{
			OfferID:         c.Param("offerId"),
			RequestID:       body.RequestID,
			OrganizationID:  body.OrganizationID,
			MembershipID:    body.MembershipID,
			ExpectedVersion: body.ExpectedVersion,
			Decision: ProviderDecision{
				OfferID:          c.Param("offerId"),
				ClientResponseID: body.ClientResponseID,
				ExpectedVersion:  body.ExpectedVersion,
				RequestID:        c.GetHeader(HeaderRequestID),
				CorrelationID:    c.GetHeader(HeaderCorrelationID),
			},
		})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, result)
}

// DeclineOffer records a provider's refusal of an organization offer.
//
// Deliberately NOT gated on ProviderOffersEnabled. Accepting new work can be
// switched off; refusing work already offered cannot, or a provider could be
// left holding an offer they have no way to decline.
func (h *ProviderHandler) DeclineOffer(c *gin.Context) {
	h.submit(c, RelayProviderOfferDecline)
}

// AcceptAssignment records a provider's acceptance of a company assignment.
func (h *ProviderHandler) AcceptAssignment(c *gin.Context) {
	if !h.flags.CompanyAssignEnabled {
		httpx.Abort(c, apperrors.Unavailable("Company assignments are not enabled.", nil))
		return
	}
	h.submit(c, RelayProviderAssignmentAccept)
}

// DeclineAssignment records a provider's refusal of a company assignment.
//
// Also ungated, for the same reason as DeclineOffer.
func (h *ProviderHandler) DeclineAssignment(c *gin.Context) {
	h.submit(c, RelayProviderAssignmentDecline)
}

// submit runs one provider decision.
func (h *ProviderHandler) submit(c *gin.Context, operationType string) {
	providerID, sessionID := providerIdentity(c)
	decision, ok := h.decision(c)
	if !ok {
		return
	}

	result, err := h.service.SubmitProviderDecision(c.Request.Context(), operationType,
		providerID, sessionID, decision)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, result)
}

// ProviderOfferSummary is the provider-safe offer list item.
//
// Built field by field. There is no path here that serializes a broadcast, a
// trip or a booking payload — all of which carry fare_amount — and strips
// fields afterwards.
type ProviderOfferSummary struct {
	OfferID        string   `json:"offerId"`
	RequestID      string   `json:"requestId"`
	ServiceType    string   `json:"serviceType"`
	PickupAddress  string   `json:"pickupAddress"`
	DropoffAddress string   `json:"dropoffAddress"`
	ExpiresAt      string   `json:"expiresAt"`
	AllowedActions []string `json:"allowedActions"`
}

// ProviderAssignmentSummary is the provider-safe assignment list item.
type ProviderAssignmentSummary struct {
	AssignmentID     string   `json:"assignmentId"`
	RequestID        string   `json:"requestId"`
	Status           string   `json:"status"`
	ResponseDeadline string   `json:"responseDeadline"`
	AllowedActions   []string `json:"allowedActions"`
}

// ListOffers returns the organization offers addressed to this provider.
//
// Scoped by the session's provider id, so a provider can only ever enumerate
// their own offers.
func (h *ProviderHandler) ListOffers(c *gin.Context) {
	providerID, _ := providerIdentity(c)
	offers, err := h.service.ListProviderOffers(c.Request.Context(), providerID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, gin.H{"offers": offers})
}

// GetOffer returns one organization offer belonging to this provider.
func (h *ProviderHandler) GetOffer(c *gin.Context) {
	providerID, _ := providerIdentity(c)
	offer, err := h.service.GetProviderOffer(c.Request.Context(), providerID, c.Param("offerId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, offer)
}

// ListAssignments returns this provider's company assignments.
func (h *ProviderHandler) ListAssignments(c *gin.Context) {
	providerID, _ := providerIdentity(c)
	assignments, err := h.service.ListProviderAssignments(c.Request.Context(), providerID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, gin.H{"assignments": assignments})
}

// GetAssignment returns one company assignment belonging to this provider.
func (h *ProviderHandler) GetAssignment(c *gin.Context) {
	providerID, _ := providerIdentity(c)
	assignment, err := h.service.GetProviderAssignment(c.Request.Context(), providerID,
		c.Param("assignmentId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondProvider(c, http.StatusOK, assignment)
}

// respondProvider writes the canonical envelope for provider-facing data.
func respondProvider(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{
		"requestId": c.GetHeader(HeaderRequestID),
	}})
}
