package fleetprovider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	client  *Client
	enabled bool
}

func NewHandler(client *Client, enabled bool) *Handler {
	return &Handler{client: client, enabled: enabled}
}

func RegisterRoutes(group *gin.RouterGroup, signer *sharedauth.TokenSigner, handler *Handler) {
	if handler == nil {
		return
	}
	routes := group.Group("/provider")
	routes.Use(sharedauth.BearerMiddleware(signer, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))

	routes.GET("/organization-offers", handler.ListOffers)
	routes.GET("/organization-offers/:offerId", handler.GetOffer)
	routes.POST("/organization-offers/:offerId/accept", handler.AcceptOffer)
	routes.POST("/organization-offers/:offerId/decline", handler.DeclineOffer)

	routes.GET("/company-assignments", handler.ListAssignments)
	routes.GET("/company-assignments/:assignmentId", handler.GetAssignment)
	routes.POST("/company-assignments/:assignmentId/accept", handler.AcceptAssignment)
	routes.POST("/company-assignments/:assignmentId/decline", handler.DeclineAssignment)
}

func (h *Handler) ListOffers(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	claims := mustClaims(c)
	offers, err := h.client.ListOffers(c.Request.Context(), metaFrom(c), claims.Subject)
	if err != nil {
		abortFleet(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"offers": offers}})
}

func (h *Handler) GetOffer(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	claims := mustClaims(c)
	offer, err := h.client.GetOffer(c.Request.Context(), metaFrom(c), claims.Subject, c.Param("offerId"))
	if err != nil {
		abortFleet(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": offer})
}

func (h *Handler) ListAssignments(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	claims := mustClaims(c)
	assignments, err := h.client.ListAssignments(c.Request.Context(), metaFrom(c), claims.Subject)
	if err != nil {
		abortFleet(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"assignments": assignments}})
}

func (h *Handler) GetAssignment(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	claims := mustClaims(c)
	assignment, err := h.client.GetAssignment(c.Request.Context(), metaFrom(c), claims.Subject, c.Param("assignmentId"))
	if err != nil {
		abortFleet(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": assignment})
}

func (h *Handler) AcceptOffer(c *gin.Context) {
	h.submit(c, "offerId", h.client.AcceptOffer)
}

func (h *Handler) DeclineOffer(c *gin.Context) {
	h.submit(c, "offerId", h.client.DeclineOffer)
}

func (h *Handler) AcceptAssignment(c *gin.Context) {
	h.submit(c, "assignmentId", h.client.AcceptAssignment)
}

func (h *Handler) DeclineAssignment(c *gin.Context) {
	h.submit(c, "assignmentId", h.client.DeclineAssignment)
}

type decisionFunc func(context.Context, Meta, string, string, string, Decision) (json.RawMessage, error)

func (h *Handler) submit(c *gin.Context, param string, fn decisionFunc) {
	if !h.ready(c) {
		return
	}
	var body Decision
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if body.ClientResponseID == "" {
		httpx.Abort(c, apperrors.BadRequest("clientResponseId is required.", nil))
		return
	}
	meta := metaFrom(c)
	if meta.IdempotencyKey == "" {
		meta.IdempotencyKey = body.ClientResponseID
	}
	claims := mustClaims(c)
	raw, err := fn(c.Request.Context(), meta, claims.Subject, claims.SessionID, c.Param(param), body)
	if err != nil {
		abortFleet(c)
		return
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		httpx.Abort(c, apperrors.Unavailable("Fleet response could not be verified.", err))
		return
	}
	c.JSON(http.StatusOK, payload)
}

func (h *Handler) ready(c *gin.Context) bool {
	if !h.enabled || h.client == nil {
		httpx.Abort(c, apperrors.Unavailable("Taxi Fleet provider work is not enabled.", nil))
		return false
	}
	return true
}

func metaFrom(c *gin.Context) Meta {
	return Meta{
		RequestID:      c.GetHeader(HeaderRequestID),
		CorrelationID:  c.GetHeader(HeaderCorrelationID),
		IdempotencyKey: c.GetHeader(HeaderIdempotencyKey),
	}
}

func mustClaims(c *gin.Context) sharedauth.Claims {
	claims, _ := sharedauth.ClaimsFromContext(c)
	return claims
}

func abortFleet(c *gin.Context) {
	httpx.Abort(c, apperrors.Unavailable("Fleet provider work is temporarily unavailable.", nil))
}
