package fleetproviderwork

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providercompat"
)

// Fleet operating modes, as Fleet persists them.
const (
	ModeCentralized = "CENTRALIZED"
	ModeDelegated   = "DELEGATED"
	ModeHybrid      = "HYBRID"
)

// Handler serves one execution service's provider-facing corporate surface.
type Handler struct {
	client    *Client
	corporate providercompat.CorporateContextReader
	service   string
	enabled   bool
}

// HandlerConfig builds the facade.
type HandlerConfig struct {
	Client *Client
	// Corporate reads Fleet's canonical corporate context. Nil means the
	// deployment has no Fleet integration at all, and every managed route
	// below then fails closed rather than serving corporate work with no
	// authority behind it.
	Corporate providercompat.CorporateContextReader
	// Service is the Fleet service category ("TAXI", "HAULING", "DISPATCH").
	Service string
	Enabled bool
}

// NewHandler builds the provider-facing Fleet facade.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{client: cfg.Client, corporate: cfg.Corporate,
		service: strings.TrimSpace(cfg.Service), enabled: cfg.Enabled}
}

// RegisterRoutes mounts the canonical provider surface on an already
// bearer-authenticated provider group.
//
// The route shapes are identical across execution services on purpose: one
// provider app screen can drive Taxi, Hauling or Dispatch without per-service
// branching, and a reconnect recovers state from the LIST reads rather than
// depending on a notification having arrived (HLG-005).
func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	if handler == nil {
		return
	}
	group.GET("/organization-offers", handler.ListOffers)
	group.GET("/organization-offers/:offerId", handler.GetOffer)
	group.POST("/organization-offers/:offerId/accept", handler.AcceptOffer)
	group.POST("/organization-offers/:offerId/decline", handler.DeclineOffer)

	group.GET("/company-assignments", handler.ListAssignments)
	group.GET("/company-assignments/:assignmentId", handler.GetAssignment)
	group.POST("/company-assignments/:assignmentId/accept", handler.AcceptAssignment)
	group.POST("/company-assignments/:assignmentId/decline", handler.DeclineAssignment)
}

// authorizeManaged is the fail-closed gate in front of every corporate route.
//
// Fleet must authoritatively confirm a live membership that covers THIS
// service before any corporate-sensitive operation runs. A Fleet outage
// yields a retryable 503; an inactive membership, an ineligible service, a
// provider mismatch or an organization mismatch each yield a definite
// rejection. There is deliberately no branch that proceeds as independent.
func (h *Handler) authorizeManaged(c *gin.Context) (providercompat.CorporateContext, bool) {
	claims := mustClaims(c)
	corporate, err := providercompat.AuthorizeManagedOperation(c.Request.Context(), h.corporate,
		providercompat.ManagedRequest{
			Service:        h.service,
			ProviderID:     claims.Subject,
			RequireManaged: true,
		})
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return providercompat.CorporateContext{}, false
	}
	return corporate, true
}

// requireOfferAcceptance enforces Fleet's PERSISTED operating mode for the
// delegated (provider-direct) acquisition path.
//
// CENTRALIZED means the organization dispatches work itself, so a provider
// taking an offer directly would bypass the organization's own control. The
// mode is read from the corporate context Fleet returned on THIS request --
// never a value this package defaults or caches.
func requireOfferAcceptance(corporate providercompat.CorporateContext) error {
	switch strings.ToUpper(strings.TrimSpace(corporate.OperatingMode)) {
	case ModeDelegated, ModeHybrid:
		return nil
	case ModeCentralized:
		return providercompat.NewContextError(providercompat.FailureOperatingMode, "",
			"operating mode CENTRALIZED does not permit direct offer acceptance", nil)
	default:
		// An unknown or missing mode is not a licence to proceed: the
		// organization's policy is exactly what we failed to establish.
		return providercompat.NewContextError(providercompat.FailureOperatingMode, "",
			"organization operating mode could not be established", nil)
	}
}

func (h *Handler) ready(c *gin.Context) bool {
	if !h.enabled || h.client == nil {
		httpx.Abort(c, apperrors.Unavailable("Fleet provider work is not enabled.", nil))
		return false
	}
	return true
}

// ListOffers returns the provider's organization marketplace offers.
func (h *Handler) ListOffers(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if _, ok := h.authorizeManaged(c); !ok {
		return
	}
	claims := mustClaims(c)
	page, err := h.client.ListOffers(c.Request.Context(), metaFrom(c), claims.Subject, queryFrom(c))
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true,
		"data": gin.H{"offers": page.Items, "nextCursor": page.NextCursor}})
}

// GetOffer returns one offer.
func (h *Handler) GetOffer(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if _, ok := h.authorizeManaged(c); !ok {
		return
	}
	claims := mustClaims(c)
	offer, err := h.client.GetOffer(c.Request.Context(), metaFrom(c), claims.Subject, c.Param("offerId"))
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": offer})
}

// ListAssignments returns the provider's company assignments.
func (h *Handler) ListAssignments(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if _, ok := h.authorizeManaged(c); !ok {
		return
	}
	claims := mustClaims(c)
	page, err := h.client.ListAssignments(c.Request.Context(), metaFrom(c), claims.Subject, queryFrom(c))
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true,
		"data": gin.H{"assignments": page.Items, "nextCursor": page.NextCursor}})
}

// GetAssignment returns one company assignment.
func (h *Handler) GetAssignment(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if _, ok := h.authorizeManaged(c); !ok {
		return
	}
	claims := mustClaims(c)
	assignment, err := h.client.GetAssignment(c.Request.Context(), metaFrom(c), claims.Subject, c.Param("assignmentId"))
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assignment})
}

// AcceptOffer relays a provider's offer acceptance, subject to operating mode.
func (h *Handler) AcceptOffer(c *gin.Context) {
	h.submit(c, "offerId", h.client.AcceptOffer, requireOfferAcceptance)
}

// DeclineOffer relays a provider's offer decline.
//
// Declining is always permitted regardless of operating mode: refusing work
// is a legitimate provider outcome, and a mode that restricts ACCEPTING must
// not trap a provider into an offer they cannot refuse.
func (h *Handler) DeclineOffer(c *gin.Context) {
	h.submit(c, "offerId", h.client.DeclineOffer, nil)
}

// AcceptAssignment relays a provider's company-assignment acceptance.
func (h *Handler) AcceptAssignment(c *gin.Context) {
	h.submit(c, "assignmentId", h.client.AcceptAssignment, nil)
}

// DeclineAssignment relays a provider's company-assignment decline.
func (h *Handler) DeclineAssignment(c *gin.Context) {
	h.submit(c, "assignmentId", h.client.DeclineAssignment, nil)
}

type decisionFunc func(context.Context, Meta, string, string, string, Decision) (json.RawMessage, error)

type modeGate func(providercompat.CorporateContext) error

func (h *Handler) submit(c *gin.Context, param string, fn decisionFunc, gate modeGate) {
	if !h.ready(c) {
		return
	}
	var body Decision
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if strings.TrimSpace(body.ClientResponseID) == "" {
		httpx.Abort(c, apperrors.BadRequest("clientResponseId is required.", nil))
		return
	}
	// Authorize BEFORE relaying. An accept or decline is a corporate-sensitive
	// write: it must never reach Fleet on behalf of a provider whose
	// membership this service could not authoritatively verify.
	corporate, ok := h.authorizeManaged(c)
	if !ok {
		return
	}
	if gate != nil {
		if err := gate(corporate); err != nil {
			httpx.Abort(c, AsAPIError(err))
			return
		}
	}
	meta := metaFrom(c)
	// clientResponseId is the provider's own idempotency identity for this
	// decision. Reusing it as the Fleet idempotency key is what makes a
	// retried accept resolve to the same Fleet response instead of a second
	// decision.
	if meta.IdempotencyKey == "" {
		meta.IdempotencyKey = body.ClientResponseID
	}
	claims := mustClaims(c)
	raw, err := fn(c.Request.Context(), meta, claims.Subject, claims.SessionID, c.Param(param), body)
	if err != nil {
		httpx.Abort(c, AsAPIError(err))
		return
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		httpx.Abort(c, apperrors.Unavailable("Fleet response could not be verified.", err))
		return
	}
	c.JSON(http.StatusOK, payload)
}

func metaFrom(c *gin.Context) Meta {
	return Meta{
		RequestID:      c.GetHeader(HeaderRequestID),
		CorrelationID:  c.GetHeader(HeaderCorrelationID),
		IdempotencyKey: c.GetHeader(HeaderIdempotencyKey),
	}
}

func queryFrom(c *gin.Context) ListQuery {
	limit, _ := strconv.Atoi(c.Query("limit"))
	return ListQuery{Status: c.Query("status"), Limit: limit, Cursor: c.Query("cursor")}
}

func mustClaims(c *gin.Context) sharedauth.Claims {
	claims, _ := sharedauth.ClaimsFromContext(c)
	return claims
}
