package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// RequireStepUp attaches step-up evidence to every tenant-scoped request and
// refuses a route in the step-up registry (internal/platform/stepup) unless
// the request proves a fresh step-up grant for that route's action class.
//
// It must run after CorporateBearer and ResolveTenant: the identity, session
// and organization a grant is checked against come from the verified token
// and the resolved tenant, never from the request. The only request input it
// reads is the X-Step-Up-Reference header.
//
// A nil verifier does not disable enforcement: protected routes then answer
// "unavailable" rather than running without the check.
func RequireStepUp(verifier stepup.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, _ := auth.ClaimsFromContext(c)
		organizationID := ""
		if tenantCtx, ok := TenantFromGin(c); ok {
			organizationID = tenantCtx.OrganizationID
		}
		proof := stepup.NewProof(verifier, strings.TrimSpace(c.GetHeader(stepup.HeaderReference)),
			claims.Subject, claims.SessionID, organizationID)
		c.Set(stepup.ContextKey, proof)
		c.Request = c.Request.WithContext(stepup.WithProof(c.Request.Context(), proof))

		if actionClass, protected := stepup.RouteActionClass(c.Request.Method, c.FullPath()); protected {
			if err := proof.Require(c.Request.Context(), actionClass); err != nil {
				httpx.Abort(c, err)
				return
			}
		}
		c.Next()
	}
}
