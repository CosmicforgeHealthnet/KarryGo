// Package http exposes the corporate passwordless authentication API.
//
// No endpoint accepts a password field, and no response ever contains an OTP.
package http

import (
	"context"
	"net/http"
	"os"

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
	sessionMarker browserauth.Policy
	// organizationScope derives the organization a step-up grant binds to
	// on these identity-scoped routes, where no tenant context exists. See
	// WithOrganizationScope.
	organizationScope func(ctx context.Context, identityID, sessionID string) (string, error)
}

// WithOrganizationScope sets how step-up start and phone change find the
// caller's organization. It must resolve the same organization the tenant
// middleware resolves for the session's protected requests (the composition
// root passes app.StepUpOrganizationScope); otherwise a grant can never match
// an organization-scoped check. Without it the organization comes only from
// a tenant context, which these identity routes never have.
func (h *Handler) WithOrganizationScope(scope func(ctx context.Context, identityID, sessionID string) (string, error)) *Handler {
	h.organizationScope = scope
	return h
}

// stepUpOrganization returns the trusted organization for step-up binding:
// the resolved tenant when present, else the configured session scope. It
// never reads an organization from the request.
func (h *Handler) stepUpOrganization(c *gin.Context, claims auth.Claims) (string, error) {
	if value, ok := c.Get(middleware.TenantOrganizationIDKey); ok {
		if organizationID, _ := value.(string); organizationID != "" {
			return organizationID, nil
		}
	}
	if h.organizationScope == nil {
		return "", nil
	}
	return h.organizationScope(c.Request.Context(), claims.Subject, claims.SessionID)
}

// NewHandler builds the handler.
//
// cookie is the sensitive refresh-token cookie, scoped narrowly to the auth
// API path so it's never sent to endpoints that don't need it. sessionMarker
// is a separate, non-sensitive cookie set alongside it with Path=/ — it
// carries no credential, it exists purely so edge middleware (which can't
// see the narrowly-scoped refresh cookie on other paths) can tell whether a
// browser plausibly has a session, for routing purposes only. Real
// authorization always happens server-side via the access/refresh tokens.
func NewHandler(service *authusecases.Service, contactSecret []byte, policies ...browserauth.Policy) *Handler {
	refreshPolicy, _ := browserauth.Load("FLEET_PARTNER_BROWSER", os.Getenv("APP_ENV"), "karrygo_fleet_refresh", "/fleet-partner/v1/auth")
	markerPolicy, _ := browserauth.Load("FLEET_PARTNER_BROWSER", os.Getenv("APP_ENV"), "karrygo_has_session", "/")

	if len(policies) > 0 {
		refreshPolicy = policies[0]
		// The marker shares the configured policy (Secure, SameSite, trusted
		// origins) and differs only in name and path. Loading it from a
		// separate env prefix let the two cookies drift apart.
		markerPolicy = refreshPolicy
		markerPolicy.Name = "karrygo_has_session"
		markerPolicy.Path = "/"
	}
	if len(policies) > 1 {
		markerPolicy = policies[1]
	}

	return &Handler{
		service:       service,
		contactSecret: contactSecret,
		cookie:        refreshPolicy,
		sessionMarker: markerPolicy,
	}
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
	// Deprecated alias is signup-only; it cannot authenticate an existing identity.
	group.POST("/auth/start", boundary, h.start)
	group.POST("/auth/sign-up/start", boundary, h.start)
	group.POST("/auth/sign-in/start", boundary, h.signIn)
	group.POST("/auth/verify", boundary, h.verify)
	group.POST("/auth/resend", boundary, h.resend)
	group.POST("/auth/refresh", boundary, h.refresh)
	group.POST("/auth/logout", boundary, h.logout)
}

// RegisterAuthenticatedRoutes mounts endpoints requiring a corporate session.
func (h *Handler) RegisterAuthenticatedRoutes(group *gin.RouterGroup) {
	group.POST("/auth/phone/change", h.cookie.Boundary(), h.changePhone)
	group.POST("/auth/select-organization", h.cookie.Boundary(), h.selectOrganization)
	group.POST("/auth/step-up/start", h.cookie.Boundary(), h.stepUpStart)
	group.POST("/auth/step-up/verify", h.cookie.Boundary(), h.stepUpVerify)
	group.GET("/identity/profile", h.currentProfile)
	group.GET("/sessions", h.listSessions)
	group.DELETE("/sessions/:sessionId", h.revokeSession)
}

// setSessionCookies sets both the real (narrowly-scoped) refresh cookie and
// the Path=/ marker cookie that lets frontend routing middleware see that a
// session exists. Called everywhere a token pair is issued or rotated.
func (h *Handler) setSessionCookies(c *gin.Context, tokens *authusecases.TokenPair) {
	h.cookie.Set(c, tokens.RefreshToken, tokens.RefreshExpiresAt)
	h.sessionMarker.Set(c, "1", tokens.RefreshExpiresAt)
}

// clearSessionCookies clears both cookies on logout.
func (h *Handler) clearSessionCookies(c *gin.Context) {
	h.cookie.Clear(c)
	h.sessionMarker.Clear(c)
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
	// Eligible registration only; conflicts return before OTP generation.
	respond.JSON(c, http.StatusAccepted, gin.H{
		"challengeId":        result.ChallengeID,
		"deliveryChannel":    result.DeliveryChannel,
		"deliveryStatus":     "ACCEPTED",
		"maskedDestination":  result.MaskedDestination,
		"expiresInSeconds":   result.ExpiresInSeconds,
		"resendAfterSeconds": result.ResendAfterSeconds,
		"expiresAt":          result.ExpiresAt,
		"resendAvailableAt":  result.ResendAvailableAt,
	})
}

func (h *Handler) signIn(c *gin.Context) {
	var request struct {
		Identifier string `json:"identifier"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	result, err := h.service.SignIn(c.Request.Context(), authusecases.StartInput{Identifier: request.Identifier, IPHash: h.ipHash(c), UserAgentHash: h.userAgentHash(c), RequestID: httpx.GetRequestID(c), CorrelationID: middleware.GetCorrelationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, gin.H{"challengeId": result.ChallengeID, "deliveryChannel": result.DeliveryChannel, "maskedDestination": result.MaskedDestination, "expiresInSeconds": result.ExpiresInSeconds, "resendAfterSeconds": result.ResendAfterSeconds, "expiresAt": result.ExpiresAt, "resendAvailableAt": result.ResendAvailableAt, "deliveryStatus": "ACCEPTED"})
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
		h.setSessionCookies(c, result.Tokens)
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
				"membershipId":       membership.ID,
				"organizationId":     membership.OrganizationID,
				"organizationName":   membership.OrganizationName,
				"status":             membership.Status,
				"organizationStatus": membership.OrganizationStatus,
				"isOwner":            membership.IsOwner,
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
		"deliveryStatus":     "ACCEPTED",
		"maskedDestination":  result.MaskedDestination,
		"expiresInSeconds":   result.ExpiresInSeconds,
		"resendAfterSeconds": result.ResendAfterSeconds,
		"expiresAt":          result.ExpiresAt,
		"resendAvailableAt":  result.ResendAvailableAt,
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
	h.setSessionCookies(c, tokens)
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
	h.clearSessionCookies(c)
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
	h.setSessionCookies(c, tokens)
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
	organization, err := h.stepUpOrganization(c, claims)
	if err != nil {
		respond.Error(c, err)
		return
	}

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

func (h *Handler) changePhone(c *gin.Context) {
	var request struct {
		Phone           string `json:"phone"`
		StepUpReference string `json:"stepUpReference"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	claims, _ := auth.ClaimsFromContext(c)
	organizationID, err := h.stepUpOrganization(c, claims)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.service.ChangePhone(c.Request.Context(), claims.Subject, claims.SessionID, organizationID, request.Phone, request.StepUpReference); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"phoneAssociated": true, "phoneVerified": false})
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

func (h *Handler) currentProfile(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		respond.Error(c, apperrors.Unauthorized("Sign in to view your account details.", nil))
		return
	}
	result, err := h.service.CurrentProfile(c.Request.Context(), claims.Subject, claims.SessionID, httpx.GetRequestID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	respond.OK(c, result)
}
