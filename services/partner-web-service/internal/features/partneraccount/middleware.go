package partneraccount

import (
	"crypto/hmac"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
	"cosmicforge/logistics/services/partner-web-service/internal/features/partnerauth"
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

// RequirePartnerSessionOrAPIKey authenticates a dashboard-facing READ route
// using EITHER a session JWT (normal dashboard usage after login) OR a partner
// API key (so any existing third-party integration hitting the same read
// routes with X-API-Key/Bearer keeps working unchanged).
//
// Session is tried first since it's the expected path for dashboard traffic.
// If the Authorization header is present but is not a valid session token, it
// is NOT discarded - it's retried as a raw API key, since a partner API key can
// also be sent as "Authorization: Bearer <key>" (see partnerauth.ExtractAPIKey).
//
// On success it sets the same partner_id context key that
// partnerauth.PartnerID(c) reads, via partnerauth.ApplyAPIKeyContext for the
// key path, so ListDeliveries/GetDelivery/ListWebhooks need no changes at all.
// When authenticated via API key, this also sets the key's permission scopes
// in context, so partnerauth.RequirePermission (chained after this on the
// route) can enforce them; session-authenticated requests are never scope
// gated by RequirePermission (see that function's doc comment).
//
// This is intentionally NOT a replacement for RequirePartnerSession: routes
// that mutate state (create/cancel delivery, register/delete/test webhook)
// stay API-key-only and are not wired through this function.
func RequirePartnerSessionOrAPIKey(svc *Service, keyRepo partnerauth.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw := bearerToken(c); raw != "" {
			pid, tokenHash, account, err := svc.AuthenticateSession(c.Request.Context(), raw)
			if err == nil {
				c.Set(ctxPartnerID, pid)
				c.Set(ctxSessionTokenHash, tokenHash)
				c.Set(ctxPartnerPlan, account.Plan)
				c.Set(ctxPartnerStatus, account.Status)
				c.Next()
				return
			}
			// Not a valid session token - fall through and try the same or a
			// separately-provided value as an API key below.
		}

		raw := partnerauth.ExtractAPIKey(c)
		if raw == "" {
			respondAPIError(c, delivery.NewAPIError(http.StatusUnauthorized, "missing_credentials",
				"Provide a session token or an API key.", nil))
			return
		}

		record, err := partnerauth.VerifyAPIKey(c.Request.Context(), keyRepo, raw, time.Now())
		if err != nil {
			status, code, message, ok := partnerauth.AsAPIKeyError(err)
			if !ok {
				status, code, message = http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid."
			}
			respondAPIError(c, delivery.NewAPIError(status, code, message, nil))
			return
		}

		partnerauth.ApplyAPIKeyContext(c, record)
		partnerauth.RecordAPIKeyUsage(keyRepo, record.ID)
		c.Next()
	}
}

// RequireActiveAccount ensures the API-key-authenticated partner is active. It
// runs AFTER partnerauth.RequirePartnerAPIKey (which sets partner_id). It is
// also safe to chain after RequirePartnerSessionOrAPIKey: when the request
// authenticated via session, AuthenticateSession already checked the account
// is active, so this becomes a cheap redundant confirmation rather than a
// behavior change.
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
