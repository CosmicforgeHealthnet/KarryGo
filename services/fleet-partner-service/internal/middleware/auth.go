package middleware

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Gin context keys for resolved identity and tenant scope.
const (
	TenantOrganizationIDKey = "tenant_organization_id"
	TenantContextKey        = "tenant_context"
	TenantRoleVersionKey    = "tenant_role_version"
	ServiceCallerKey        = "service_name"
)

// CorporateRoleOrganizationUser is the required token role on corporate routes.
const CorporateRoleOrganizationUser = config.CorporateRole

// CorporateBearer authenticates organization users on /fleet-partner/v1
// routes: it requires a Bearer access token issued for this service with the
// organization_user role, and stores the verified claims.
func CorporateBearer(signer *auth.TokenSigner) gin.HandlerFunc {
	return auth.BearerMiddleware(signer, CorporateRoleOrganizationUser, config.ServiceName)
}

// InternalServiceAuth authenticates platform services on
// /internal/fleet-partner/v1 routes via shared HMAC service signatures.
// Bearer user tokens are explicitly rejected: only signed service requests
// pass. Request/correlation IDs are already resolved by outer middleware.
func InternalServiceAuth(verifier *serviceauth.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader(serviceauth.HeaderServiceName) == "" {
			httpx.Abort(c, apperrors.Unauthorized("Service authentication is required.", nil))
			return
		}
		serviceName, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.Set(ServiceCallerKey, serviceName)
		c.Next()
	}
}

// ResolveTenant resolves the authenticated user's active organization
// membership into a typed tenant context. It runs after CorporateBearer.
// Client-supplied organization IDs (body, query, header) are never consulted.
// Missing membership fails closed.
func ResolveTenant(resolver tenant.MembershipResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}

		membership, err := resolver.ResolveMembership(c.Request.Context(), claims.Subject)
		if err != nil {
			switch err {
			case tenant.ErrResolverUnconfigured:
				httpx.Abort(c, apperrors.Unavailable("Organization membership resolution is not configured.", err))
			case tenant.ErrMembershipNotFound, tenant.ErrMembershipSuspended:
				httpx.Abort(c, apperrors.Forbidden("You do not have an active organization membership.", nil))
			default:
				httpx.Abort(c, apperrors.Internal("", err))
			}
			return
		}
		if membership.Status != "" && membership.Status != tenant.MembershipStatusActive {
			httpx.Abort(c, apperrors.Forbidden("You do not have an active organization membership.", nil))
			return
		}

		tenantCtx := tenant.Context{
			OrganizationID:     membership.OrganizationID,
			OrganizationUserID: membership.OrganizationUserID,
			ActorType:          tenant.ActorTypeOrganizationUser,
			ActorID:            claims.Subject,
			PermittedBranchIDs: membership.PermittedBranchIDs,
			RequestID:          httpx.GetRequestID(c),
			CorrelationID:      GetCorrelationID(c),
		}
		if err := tenantCtx.Validate(); err != nil {
			httpx.Abort(c, apperrors.Forbidden("Organization scope is required.", err))
			return
		}

		c.Set(TenantContextKey, tenantCtx)
		c.Set(TenantOrganizationIDKey, tenantCtx.OrganizationID)
		c.Request = c.Request.WithContext(tenant.Into(c.Request.Context(), tenantCtx))
		c.Next()
	}
}

// TenantFromGin returns the resolved tenant context for handlers.
func TenantFromGin(c *gin.Context) (tenant.Context, bool) {
	value, ok := c.Get(TenantContextKey)
	if !ok {
		return tenant.Context{}, false
	}
	tenantCtx, ok := value.(tenant.Context)
	return tenantCtx, ok
}

// MetricsAuth protects /metrics with a static bearer token compared in
// constant time.
func MetricsAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			httpx.Abort(c, apperrors.Unavailable("Metrics are not configured.", nil))
			return
		}
		header := c.GetHeader("Authorization")
		if subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+token)) != 1 {
			httpx.Abort(c, apperrors.Unauthorized("Metrics authentication is required.", nil))
			return
		}
		c.Next()
	}
}
