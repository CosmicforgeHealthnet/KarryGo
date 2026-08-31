// Package http exposes the corporate passwordless authentication API.
//
// No endpoint accepts a password field, and no response ever contains an OTP.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/browserauth"
	"cosmicforge/logistics/shared/go/httpx"

	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// Handler serves the authentication routes.
type Handler struct {
	service       *authusecases.Service
	contactSecret []byte
	cookie        browserauth.Policy
}

// NewHandler builds the handler.
func NewHandler(service *authusecases.Service, contactSecret []byte, policies ...browserauth.Policy) *Handler {
	policy := browserauth.Policy{Name: "karrygo_fleet_refresh", Path: "/fleet-partner/v1/auth", Secure: true, SameSite: http.SameSiteLaxMode}
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &Handler{service: service, contactSecret: contactSecret, cookie: policy}
}

// startRequest is the signup/login request. There is deliberately no password
// field: an unknown field would simply be ignored by the JSON decoder, and
// the architecture test asserts no password field name exists in this package.
type startRequest struct {
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	InvitationToken string `json:"invitationToken,omitempty"`
}

type verifyRequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
	DeviceLabel string `json:"deviceLabel,omitempty"`
}

type resendRequest struct {
	ChallengeID string `json:"challengeId"`
}

type selectOrganizationRequest struct {
	MembershipID string `json:"membershipId"`
}

type stepUpStartRequest struct {
	ActionClass string `json:"actionClass"`
}

// RegisterPublicRoutes mounts the unauthenticated authentication endpoints.
func (h *Handler) RegisterPublicRoutes(group *gin.RouterGroup) {
	boundary := h.cookie.Boundary()
	group.OPTIONS("/auth/*path", boundary)
	group.POST("/auth/start", boundary, h.start)
	group.POST("/auth/verify", boundary, h.verify)
	group.POST("/auth/resend", boundary, h.resend)
	group.POST("/auth/refresh", boundary, h.refresh)
	group.POST("/auth/logout", boundary, h.logout)
}

// RegisterAuthenticatedRoutes mounts endpoints requiring a corporate session.
func (h *Handler) RegisterAuthenticatedRoutes(group *gin.RouterGroup) {
	group.POST("/auth/select-organization", h.cookie.Boundary(), h.selectOrganization)
	group.POST("/auth/step-up/start", h.cookie.Boundary(), h.stepUpStart)
	group.POST("/auth/step-up/verify", h.cookie.Boundary(), h.stepUpVerify)
	group.GET("/sessions", h.listSessions)
	group.DELETE("/sessions/:sessionId", h.revokeSession)
}

func (h *Handler) start(c *gin.Context) {
	var request startRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	result, err := h.service.Start(c.Request.Context(), authusecases.StartInput{
		Email:           request.Email,
		Phone:           request.Phone,
		InvitationToken: request.InvitationToken,
		IPHash:          h.ipHash(c),
		UserAgentHash:   h.userAgentHash(c),
		RequestID:       httpx.GetRequestID(c),
		CorrelationID:   middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	// The response is identical whether or not the account exists.
	respond.JSON(c, http.StatusAccepted, gin.H{
		"challengeId":        result.ChallengeID,
		"deliveryChannel":    result.DeliveryChannel,
		"maskedDestination":  result.MaskedDestination,
		"expiresInSeconds":   result.ExpiresInSeconds,
		"resendAfterSeconds": result.ResendAfterSeconds,
	})
}

func (h *Handler) verify(c *gin.Context) {
	var request verifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	result, err := h.service.Verify(c.Request.Context(), authusecases.VerifyInput{
		ChallengeID:   request.ChallengeID,
		Code:          request.Code,
		DeviceLabel:   request.DeviceLabel,
		IPHash:        h.ipHash(c),
		UserAgentHash: h.userAgentHash(c),
		RequestID:     httpx.GetRequestID(c),
		CorrelationID: middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	if result.Tokens != nil {
		h.cookie.Set(c, result.Tokens.RefreshToken, result.Tokens.RefreshExpiresAt)
	}
	respond.OK(c, verifyResponse(result))
}

func verifyResponse(result authusecases.VerifyResult) gin.H {
	body := gin.H{"nextStep": result.NextStep, "identityId": result.IdentityID}
	if result.Tokens != nil {
		body["tokens"] = gin.H{
			"accessToken":      result.Tokens.AccessToken,
			"accessExpiresAt":  result.Tokens.AccessExpiresAt,
			"refreshExpiresAt": result.Tokens.RefreshExpiresAt,
			"sessionId":        result.Tokens.SessionID,
		}
	}
	if len(result.Organizations) > 0 {
		organizations := make([]gin.H, 0, len(result.Organizations))
		for _, membership := range result.Organizations {
			organizations = append(organizations, gin.H{
				"membershipId":     membership.ID,
				"organizationId":   membership.OrganizationID,
				"organizationName": membership.OrganizationName,
				"status":           membership.Status,
				"isOwner":          membership.IsOwner,
			})
		}
		body["organizations"] = organizations
	}
	if result.StepUpReference != "" {
		body["stepUp"] = gin.H{
			"reference": result.StepUpReference,
			"expiresAt": result.StepUpExpiresAt,
		}
	}
	return body
}

func (h *Handler) resend(c *gin.Context) {
	var request resendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	result, err := h.service.Resend(c.Request.Context(), authusecases.ResendInput{
		ChallengeID:   request.ChallengeID,
		IPHash:        h.ipHash(c),
		RequestID:     httpx.GetRequestID(c),
		CorrelationID: middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, gin.H{
		"challengeId":        result.ChallengeID,
		"deliveryChannel":    result.DeliveryChannel,
		"maskedDestination":  result.MaskedDestination,
		"expiresInSeconds":   result.ExpiresInSeconds,
		"resendAfterSeconds": result.ResendAfterSeconds,
	})
}

func (h *Handler) refresh(c *gin.Context) {
	token, _ := c.Cookie(h.cookie.Name)
	tokens, err := h.service.Refresh(c.Request.Context(), authusecases.RefreshInput{
		RefreshToken:  token,
		IPHash:        h.ipHash(c),
		UserAgentHash: h.userAgentHash(c),
		RequestID:     httpx.GetRequestID(c),
		CorrelationID: middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	h.cookie.Set(c, tokens.RefreshToken, tokens.RefreshExpiresAt)
	respond.OK(c, gin.H{
		"accessToken":      tokens.AccessToken,
		"accessExpiresAt":  tokens.AccessExpiresAt,
		"refreshExpiresAt": tokens.RefreshExpiresAt,
		"sessionId":        tokens.SessionID,
	})
}

func (h *Handler) logout(c *gin.Context) {
	token, _ := c.Cookie(h.cookie.Name)
	if err := h.service.LogoutRefresh(c.Request.Context(), token,
		httpx.GetRequestID(c), middleware.GetCorrelationID(c)); err != nil {
		respond.Error(c, err)
		return
	}
	h.cookie.Clear(c)
	respond.OK(c, gin.H{"loggedOut": true})
}

func (h *Handler) selectOrganization(c *gin.Context) {
	var request selectOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	claims, _ := auth.ClaimsFromContext(c)

	tokens, err := h.service.SelectOrganization(c.Request.Context(), authusecases.SelectOrganizationInput{
		IdentityID:    claims.Subject,
		SessionID:     claims.SessionID,
		MembershipID:  request.MembershipID,
		IPHash:        h.ipHash(c),
		UserAgentHash: h.userAgentHash(c),
		RequestID:     httpx.GetRequestID(c),
		CorrelationID: middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	h.cookie.Set(c, tokens.RefreshToken, tokens.RefreshExpiresAt)
	respond.OK(c, gin.H{
		"accessToken":      tokens.AccessToken,
		"accessExpiresAt":  tokens.AccessExpiresAt,
		"refreshExpiresAt": tokens.RefreshExpiresAt,
		"sessionId":        tokens.SessionID,
	})
}

func (h *Handler) stepUpStart(c *gin.Context) {
	var request stepUpStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	claims, _ := auth.ClaimsFromContext(c)
	organizationID, _ := c.Get(middleware.TenantOrganizationIDKey)
	organization, _ := organizationID.(string)

	identity, err := h.service.IdentityByID(c.Request.Context(), claims.Subject)
	if err != nil {
		respond.Error(c, err)
		return
	}

	result, err := h.service.Start(c.Request.Context(), authusecases.StartInput{
		Email:          identity.EmailNormalized,
		Phone:          identity.PhoneE164,
		Purpose:        "STEP_UP",
		IdentityID:     claims.Subject,
		SessionID:      claims.SessionID,
		OrganizationID: organization,
		ActionClass:    request.ActionClass,
		IPHash:         h.ipHash(c),
		UserAgentHash:  h.userAgentHash(c),
		RequestID:      httpx.GetRequestID(c),
		CorrelationID:  middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, gin.H{
		"challengeId":       result.ChallengeID,
		"deliveryChannel":   result.DeliveryChannel,
		"maskedDestination": result.MaskedDestination,
		"expiresInSeconds":  result.ExpiresInSeconds,
	})
}

func (h *Handler) stepUpVerify(c *gin.Context) {
	var request verifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	result, err := h.service.Verify(c.Request.Context(), authusecases.VerifyInput{
		ChallengeID:   request.ChallengeID,
		Code:          request.Code,
		IPHash:        h.ipHash(c),
		RequestID:     httpx.GetRequestID(c),
		CorrelationID: middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{
		"stepUpReference": result.StepUpReference,
		"expiresAt":       result.StepUpExpiresAt,
	})
}

func (h *Handler) listSessions(c *gin.Context) {
	claims, _ := auth.ClaimsFromContext(c)
	sessions, err := h.service.ListSessions(c.Request.Context(), claims.Subject)
	if err != nil {
		respond.Error(c, err)
		return
	}
	// Refresh-token hashes are never serialized.
	body := make([]gin.H, 0, len(sessions))
	for _, session := range sessions {
		body = append(body, gin.H{
			"sessionId":      session.ID,
			"status":         session.Status,
			"organizationId": session.OrganizationID,
			"deviceLabel":    session.DeviceLabel,
			"lastSeenAt":     session.LastSeenAt,
			"createdAt":      session.CreatedAt,
			"current":        session.ID == claims.SessionID,
		})
	}
	respond.List(c, body, "")
}

func (h *Handler) revokeSession(c *gin.Context) {
	claims, _ := auth.ClaimsFromContext(c)
	if err := h.service.RevokeSession(c.Request.Context(), claims.Subject, c.Param("sessionId"),
		httpx.GetRequestID(c), middleware.GetCorrelationID(c)); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"revoked": true})
}

// ipHash and userAgentHash keep raw client identifiers out of the database.
func (h *Handler) ipHash(c *gin.Context) string {
	return contact.Hash(h.contactSecret, c.ClientIP())
}

func (h *Handler) userAgentHash(c *gin.Context) string {
	return contact.Hash(h.contactSecret, c.Request.UserAgent())
}
