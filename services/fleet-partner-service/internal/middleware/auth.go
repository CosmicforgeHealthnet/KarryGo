package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
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

// nonceReplayTTL bounds how long a consumed nonce is remembered. It matches
// the freshness window InternalServiceAuth's verifier already enforces
// (serviceauth.NewVerifier's maxSkew, 5 minutes at this service's one
// construction site in cmd/main.go): a request cannot be replayed once its
// own signature has expired on that basis alone, so nothing is lost letting
// a nonce record expire once the window it could matter in has passed.
const nonceReplayTTL = 5 * time.Minute

// RejectReplayedNonce closes the gap serviceauth.Verifier leaves open: it
// requires and signs a nonce but never records that one was already used,
// so a captured, validly signed request can be replayed verbatim within its
// freshness window. This must run after InternalServiceAuth — it trusts
// ServiceCallerKey and the nonce header exactly because the signature
// covering both was already verified.
//
// A nil store disables the check: the request is accepted exactly as it was
// before this middleware existed, matching every other optional dependency
// in this service's Options struct that "unconfigured" degrades safely
// rather than fails closed.
func RejectReplayedNonce(store *noncestore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil {
			c.Next()
			return
		}
		nonce := c.GetHeader(serviceauth.HeaderNonce)
		caller := c.GetString(ServiceCallerKey)
		if nonce == "" || caller == "" {
			// InternalServiceAuth already refuses a missing nonce or unresolved
			// caller before this middleware runs; reaching here without either
			// means the auth chain was not applied in order, which is itself a
			// configuration bug worth failing closed on rather than skipping.
			httpx.Abort(c, apperrors.Unauthorized("Service authentication is required.", nil))
			return
		}
		if err := store.Consume(c.Request.Context(), caller, nonce, nonceReplayTTL); err != nil {
			if errors.Is(err, noncestore.ErrNonceReplayed) {
				httpx.Abort(c, apperrors.Conflict("This request has already been processed.", nil))
				return
			}
			httpx.Abort(c, apperrors.Unavailable("Service authentication is temporarily unavailable.", nil))
			return
		}
		c.Next()
	}
}

// SessionStatusChecker resolves whether the session an access token was
// minted for is still usable. Access tokens carry no revocation state of
// their own (they are locally verified, short-lived HMAC tokens, not
// introspected against an external authority), so a session revoked mid
// lifetime — DELETE /sessions/:sessionId, credential reset, reuse-detected
// family revocation — would otherwise stay honored by every route until the
// token's own expiry (up to AccessTokenTTL, capped at 1h) even though the
// session itself is already gone. This check closes that gap without
// changing the token format: it runs the same per-request DB round trip
// ResolveTenant already makes for membership, so wiring it in adds one more
// indexed point lookup, not a second request cycle.
type SessionStatusChecker interface {
	Status(ctx context.Context, sessionID string) (string, error)
}

// sessionStatusActive must match auth/models.SessionStatusActive's value.
// Duplicated as a local string constant, rather than importing the auth
// feature's models package here, so this platform-level middleware package
// depends only on the narrow SessionStatusChecker port it defines, not on
// any one feature's internal types.
const sessionStatusActive = "ACTIVE"

// ResolveTenant resolves the authenticated user's active organization
// membership into a typed tenant context. It runs after CorporateBearer.
// Client-supplied organization IDs (body, query, header) are never consulted.
// Missing membership fails closed.
//
// sessions may be nil, matching every other optional dependency in this
// service's composition root (RejectReplayedNonce, MetricsAuth): when unset,
// per-request session-revocation enforcement is simply off and behavior is
// unchanged from before this check existed. In this service's own wiring
// (internal/app.Options.SessionStatus, set in cmd/main.go) it is always
// non-nil in practice, because Components.Sessions is unconditionally
// constructed. A lookup error fails closed (Unavailable) rather than
// treating an unprovable session as valid — a missing gate must never read
// as an open one, same rule the authorizer nil-check already follows.
func ResolveTenant(resolver tenant.MembershipResolver, sessions SessionStatusChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}

		if sessions != nil {
			status, err := sessions.Status(c.Request.Context(), claims.SessionID)
			if err != nil {
				httpx.Abort(c, apperrors.Unavailable("Session status could not be verified.", err))
				return
			}
			if status != sessionStatusActive {
				httpx.Abort(c, apperrors.Unauthorized("This session has been revoked. Please sign in again.", nil))
				return
			}
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
