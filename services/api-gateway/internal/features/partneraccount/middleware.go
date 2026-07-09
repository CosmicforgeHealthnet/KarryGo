package partneraccount

import (
	"crypto/hmac"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

// Gin context keys set by the dashboard session middleware.
const (
	ctxPartnerID        = "partner_id"
	ctxPartnerPlan      = "partner_plan"
	ctxPartnerStatus    = "partner_status"
	ctxSessionTokenHash = "session_token_hash"
)

// SessionPartnerID returns the dashboard-authenticated partner id.
func SessionPartnerID(c *gin.Context) string { return ctxString(c, ctxPartnerID) }

// SessionTokenHash returns the current session's token hash.
func SessionTokenHash(c *gin.Context) string { return ctxString(c, ctxSessionTokenHash) }

// PartnerPlan returns the authenticated partner's plan.
func PartnerPlan(c *gin.Context) string { return ctxString(c, ctxPartnerPlan) }

func ctxString(c *gin.Context, k string) string {
	v, ok := c.Get(k)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// RequirePartnerSession authenticates a dashboard request using a session JWT.
// It never accepts an API key. On success it sets partner_id, partner_plan,
// partner_status, and session_token_hash in the context.
func RequirePartnerSession(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c)
		if raw == "" {
			respondAPIError(c, delivery.NewAPIError(http.StatusUnauthorized, "missing_token",
				"Provide your session token in the Authorization header.", nil))
			return
		}
		pid, tokenHash, account, err := svc.AuthenticateSession(c.Request.Context(), raw)
		if err != nil {
			respondAPIError(c, err)
			return
		}
		c.Set(ctxPartnerID, pid)
		c.Set(ctxSessionTokenHash, tokenHash)
		c.Set(ctxPartnerPlan, account.Plan)
		c.Set(ctxPartnerStatus, account.Status)
		c.Next()
	}
}

// RequireActiveAccount ensures the API-key-authenticated partner is active. It
// runs AFTER partnerauth.RequirePartnerAPIKey (which sets partner_id).
func RequireActiveAccount(accounts AccountRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := partnerauth.PartnerID(c)
		if pid == "" {
			partnerauth.WriteError(c, http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid.")
			return
		}
		account, err := accounts.FindByID(c.Request.Context(), pid)
		if err != nil {
			partnerauth.WriteError(c, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		if account == nil {
			partnerauth.WriteError(c, http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid.")
			return
		}
		if !isActive(account.Status) {
			partnerauth.WriteError(c, http.StatusForbidden, "partner_account_inactive", "This partner account is not active.")
			return
		}
		c.Set(ctxPartnerPlan, account.Plan)
		c.Next()
	}
}

// RequireServiceKey guards internal service-to-service routes with a shared key.
func RequireServiceKey(configured string) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := strings.TrimSpace(c.GetHeader("X-Internal-Service-Key"))
		if configured == "" || provided == "" || !hmac.Equal([]byte(provided), []byte(configured)) {
			partnerauth.WriteError(c, http.StatusUnauthorized, "unauthorized_service",
				"A valid internal service key is required.")
			return
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	const bearer = "Bearer "
	if len(authz) > len(bearer) && strings.EqualFold(authz[:len(bearer)], bearer) {
		return strings.TrimSpace(authz[len(bearer):])
	}
	return ""
}
