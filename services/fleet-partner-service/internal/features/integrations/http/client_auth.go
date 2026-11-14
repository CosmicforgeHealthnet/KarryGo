package http

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ClientAuthMiddleware resolves the organization exclusively from the API
// credential. Organization identifiers supplied in headers, query or JSON
// bodies are rejected rather than allowed to override that trusted scope.
func ClientAuthMiddleware(s Authenticator, requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID, secret, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(clientID) == "" || secret == "" {
			s.RecordAuthOutcome("malformed_authorization")
			respond.Error(c, apperrors.Unauthorized("Client authentication is required.", nil))
			return
		}
		if c.GetHeader("X-Organization-ID") != "" || c.Query("organizationId") != "" || bodyContainsOrganizationID(c) {
			s.RecordAuthOutcome("organization_override_denied")
			respond.Error(c, apperrors.Forbidden("Organization scope is derived from the client credential.", nil))
			return
		}
		resolved, err := s.Authenticate(c, clientID, secret, c.ClientIP(), requiredScope, httpx.GetRequestID(c), middleware.GetCorrelationID(c))
		if err != nil {
			respond.Error(c, err)
			return
		}
		t := tenant.Context{OrganizationID: resolved.OrganizationID, ActorType: tenant.ActorTypeOrganizationAPIClient,
			ActorID: resolved.APIClientID, RequestID: httpx.GetRequestID(c), CorrelationID: middleware.GetCorrelationID(c)}
		c.Set(middleware.TenantContextKey, t)
		c.Set(middleware.TenantOrganizationIDKey, t.OrganizationID)
		c.Set("organization_api_client_id", resolved.APIClientID)
		c.Set("organization_api_client_scopes", resolved.Scopes)
		c.Request = c.Request.WithContext(tenant.Into(c.Request.Context(), t))
		c.Next()
	}
}

func bodyContainsOrganizationID(c *gin.Context) bool {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return false
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	_, exists := value["organizationId"]
	return exists
}
