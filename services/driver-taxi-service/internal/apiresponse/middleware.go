package apiresponse

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providercompat"
)

// ProviderFinancialPrivacy resolves the provider's corporate financial
// visibility policy once per request, before any handler runs.
//
// Fail-closed: if Fleet cannot be reached the request is refused with a
// retryable dependency error rather than served with an unmasked body. A
// Fleet outage must never result in a corporate provider being shown their
// organization's commercial data (Section 8 TAX-018).
//
// A nil reader means the deployment has no Fleet integration at all: every
// provider is independent, and the response contract is byte-for-byte the
// pre-corporate one.
func ProviderFinancialPrivacy(reader providercompat.CorporateContextReader, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := sharedauth.ClaimsFromContext(c)
		if !ok || claims.Subject == "" {
			c.Next()
			return
		}
		c.Set(KeyProviderID, claims.Subject)
		if reader == nil || !enabled {
			c.Set(KeyManaged, false)
			c.Next()
			return
		}
		corporate, err := reader.ReadCorporateContext(c.Request.Context(), "TAXI", claims.Subject)
		if err != nil {
			httpx.Abort(c, apperrors.Unavailable("Provider visibility authority is unavailable.", err))
			return
		}
		c.Set(KeyManaged, corporate.IsManagedProvider)
		if corporate.IsManagedProvider {
			c.Set(KeyPolicy, corporate.FinancialVisibilityPolicy)
		}
		c.Next()
	}
}

// PolicyFrom reads the resolved projection inputs off the request.
func PolicyFrom(c *gin.Context) (providerFacing, managed bool, policy *providercompat.FinancialVisibilityPolicy) {
	if c.GetString(KeyProviderID) == "" {
		return false, false, nil
	}
	managed = c.GetBool(KeyManaged)
	if value, ok := c.Get(KeyPolicy); ok {
		if typed, ok := value.(providercompat.FinancialVisibilityPolicy); ok {
			policy = &typed
		}
	}
	return true, managed, policy
}

// Project applies the resolved policy to a response body. Non-provider
// responses (customer, internal) pass through untouched.
func Project(c *gin.Context, data any) any {
	providerFacing, managed, policy := PolicyFrom(c)
	if !providerFacing {
		return data
	}
	return ProjectResponse(data, managed, policy)
}
