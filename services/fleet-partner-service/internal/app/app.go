// Package app assembles the HTTP application: middleware chain, route
// classes and readiness policy. cmd/main.go and the HTTP tests both build
// the router through this package so what is tested is what runs.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	platformserviceauth "cosmicforge/logistics/services/fleet-partner-service/internal/platform/serviceauth"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ReadyCheck is one named readiness dependency.
type ReadyCheck struct {
	Name  string
	Check func(context.Context) error
}

// RouteRegistrar mounts a feature's routes onto a route group.
type RouteRegistrar func(*gin.RouterGroup)

// Options wires the application.
type Options struct {
	Config      config.Config
	Metrics     *observability.Metrics
	TokenSigner *auth.TokenSigner
	Verifier    *serviceauth.Verifier
	// ScopeVerifier, when set, upgrades the internal route class's
	// authentication from single-secret HMAC verification (opts.Verifier)
	// to Section 28's version-aware, route/method/event-scoped credential
	// resolution (internal/platform/serviceauth.ScopeVerifier) --
	// middleware.InternalServiceScopeAuth instead of
	// middleware.InternalServiceAuth. Nil (the default for every existing
	// test/deployment that does not set it) falls back to exactly the
	// previous behavior, unchanged -- this field is purely additive so no
	// existing app.Options{...} call site anywhere in this codebase's tests
	// needed to change.
	ScopeVerifier *platformserviceauth.ScopeVerifier
	// NonceStore rejects a replayed internal HMAC nonce even when it carries
	// a validly signed, freshness-window request — see
	// internal/platform/noncestore for why shared/go/serviceauth.Verifier
	// alone does not already do this. Nil disables the check (every request
	// that verifies is accepted), matching every other optional dependency
	// in this Options struct.
	NonceStore *noncestore.Store
	// Limiter backs the Section 28/12 registry-driven rate limiting
	// (middleware.GlobalRateLimit) on every route class. Nil disables the
	// check entirely, matching every other optional dependency in this
	// Options struct; this service's own composition root
	// (internal/app/wiring.go's Components.Limiter) always constructs one
	// (Redis-backed when configured, in-process fallback otherwise), so
	// production never actually passes nil.
	Limiter  ratelimit.Limiter
	Resolver tenant.MembershipResolver
	// SessionStatus enforces mid-lifetime session revocation on every
	// tenant-scoped request — see middleware.SessionStatusChecker and
	// middleware.ResolveTenant. Nil disables the check, matching every
	// other optional dependency in this Options struct.
	SessionStatus middleware.SessionStatusChecker
	ReadyChecks   []ReadyCheck
	// WorkerReady reports in-process worker readiness (nil when the worker
	// runs as a separate process).
	WorkerReady func() bool

	// PublicAuthRoutes are mounted without authentication (start, verify,
	// resend, refresh). They are the only unauthenticated corporate routes.
	PublicAuthRoutes RouteRegistrar
	// IdentityRoutes require a valid corporate session but no organization
	// scope (logout, organization selection, sessions, draft creation).
	IdentityRoutes RouteRegistrar
	// TenantRoutes require resolved organization scope.
	TenantRoutes RouteRegistrar
	// OrganizationAPIRoutes are authenticated with a scoped organization API
	// client credential, never with a corporate session or internal-service
	// credential. OrganizationAPIAuth must resolve the tenant context.
	OrganizationAPIAuth   gin.HandlerFunc
	OrganizationAPIRoutes RouteRegistrar
	// InternalRoutes are service-to-service routes mounted behind HMAC
	// service authentication. They are never reachable with a corporate,
	// provider or customer token.
	InternalRoutes RouteRegistrar
	// CustomerRoutes are the unauthenticated secure-link routes a CRM customer
	// reaches from an emailed quotation. They carry no session and no tenant
	// scope: the hashed, expiring, single-quotation token in the request is the
	// entire authorization decision, and the handler rate-limits it.
	CustomerRoutes RouteRegistrar
}

// BuildRouter constructs the full gin engine.
func BuildRouter(opts Options) *gin.Engine {
	if opts.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	// Explicit proxy trust, always set (never left at gin's own default,
	// which trusts every proxy and only warns about it): empty means trust
	// none, so ClientIP() reads the raw connection's RemoteAddr rather than
	// an X-Forwarded-For value any caller could set to any value it likes —
	// see config.Config.TrustedProxies.
	_ = router.SetTrustedProxies(opts.Config.TrustedProxies)

	// Documented middleware ordering — see internal/middleware/middleware.go.
	// middleware.Recovery (not shared httpx.Recovery) so a panic is logged as
	// structured JSON like every other log line this service emits — see
	// middleware.Recovery's own doc comment.
	router.Use(middleware.Recovery(opts.Config))
	router.Use(httpx.RequestID())
	router.Use(middleware.CorrelationID())
	router.Use(middleware.Tracing(opts.Config.TraceSampleRate))
	router.Use(middleware.SecureHeaders())
	router.Use(middleware.BodySizeLimit(opts.Config.MaxRequestBodyBytes))
	router.Use(middleware.RequestLogger(opts.Config, opts.Metrics))
	router.Use(respond.ErrorHandler())

	// Health/liveness: process liveness only — no dependency checks. /live is
	// the Section 29 canonical name; /health is preserved unchanged for any
	// existing caller (load balancer, orchestrator) already pointed at it.
	// Neither ever touches PostgreSQL, Redis or any external service — a
	// downstream outage must never cause this process to be killed and
	// restarted, which is exactly what a dependency-coupled liveness probe
	// would risk under an orchestrator.
	liveness := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": config.ServiceName, "status": "ok"})
	}
	router.GET("/health", liveness)
	router.GET("/live", liveness)

	// Readiness: validated config is implied by startup; checks the database
	// (and other mandatory dependencies) plus worker readiness.
	router.GET("/ready", func(c *gin.Context) {
		type failure struct {
			Dependency string `json:"dependency"`
			Status     string `json:"status"`
		}
		var failures []failure
		for _, check := range opts.ReadyChecks {
			checkCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			err := check.Check(checkCtx)
			cancel()
			if err != nil {
				// Never expose credentials or internal error detail.
				failures = append(failures, failure{Dependency: check.Name, Status: "unavailable"})
			}
		}
		if opts.WorkerReady != nil && !opts.WorkerReady() {
			failures = append(failures, failure{Dependency: "worker", Status: "not_ready"})
		}
		if len(failures) > 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"service": config.ServiceName, "status": "not_ready", "failures": failures,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"service": config.ServiceName, "status": "ready"})
	})

	// Metrics: protected; never public.
	if opts.Config.MetricsEnabled {
		router.GET("/metrics", middleware.MetricsAuth(opts.Config.MetricsAuthToken), func(c *gin.Context) {
			c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(opts.Metrics.Render()))
		})
	}

	// Corporate authentication: the only unauthenticated corporate routes.
	// They are passwordless — email + phone + emailed OTP — and never accept
	// a password field or return an OTP.
	if opts.PublicAuthRoutes != nil {
		public := router.Group("/fleet-partner/v1")
		public.Use(middleware.RequireJSON())
		// Pre-auth: no tenant, no service caller yet, so every key this
		// resolves to is IP-only -- still real protection (a caller cannot
		// evade it by omitting a bearer token) and layers with the
		// auth feature's own existing per-identifier scope limiter
		// (ratelimit.ScopeChallengeCreate etc, applied inside the usecase,
		// not here) rather than replacing it.
		public.Use(middleware.GlobalRateLimit(opts.Limiter, opts.Metrics))
		opts.PublicAuthRoutes(public)
	}

	// Customer secure-link API: no session, no tenant resolution. A CRM
	// customer following a quotation link presents a hashed, expiring token
	// scoped to one quotation and one issued version; everything else about the
	// request is untrusted, and the handler rate-limits validation attempts.
	if opts.CustomerRoutes != nil {
		customer := router.Group("/fleet-partner/public/v1")
		opts.CustomerRoutes(customer)
	}

	// Organization integration API: a distinct authentication and route
	// namespace prevents an API client from reaching the corporate management
	// endpoints that create, rotate, or revoke credentials. The first limiter
	// bounds invalid credentials by trusted-proxy-aware IP; the second adds the
	// resolved organization/client dimensions after authentication.
	if opts.OrganizationAPIAuth != nil && opts.OrganizationAPIRoutes != nil {
		organizationAPI := router.Group("/fleet-partner/integrations/v1")
		organizationAPI.Use(middleware.RequireJSON())
		organizationAPI.Use(middleware.RateLimit(opts.Limiter, opts.Metrics,
			ratelimit.RiskSensitive, ratelimit.DefaultRiskRules[ratelimit.RiskSensitive]))
		organizationAPI.Use(opts.OrganizationAPIAuth)
		organizationAPI.Use(middleware.GlobalRateLimit(opts.Limiter, opts.Metrics))
		opts.OrganizationAPIRoutes(organizationAPI)
	}

	// Corporate web API: Bearer token + tenant resolution.
	corporate := router.Group("/fleet-partner/v1")
	corporate.Use(middleware.RequireJSON("/fleet-partner/v1/organizations/:organizationId/documents/upload-content"))
	corporate.Use(middleware.CorporateBearer(opts.TokenSigner))
	{
		// Identity-scoped routes: a verified session, organization scope not
		// yet required (a new owner has no organization until they create one).
		if opts.IdentityRoutes != nil {
			opts.IdentityRoutes(corporate)
		}
		// Foundation meta endpoint: authenticated; returns resolved request
		// context. Tenant scope is attached when membership resolves; this
		// endpoint itself is not tenant-scoped (membership infrastructure is
		// a later work package) and never trusts client-supplied org IDs.
		corporate.GET("/meta", func(c *gin.Context) {
			claims, _ := auth.ClaimsFromContext(c)
			data := gin.H{
				"service":       config.ServiceName,
				"routeClass":    "corporate",
				"actorId":       claims.Subject,
				"correlationId": middleware.GetCorrelationID(c),
				"tenant":        nil,
			}
			membership, err := opts.Resolver.ResolveMembership(c.Request.Context(), claims.Subject)
			if err == nil {
				data["tenant"] = gin.H{
					"organizationId":     membership.OrganizationID,
					"organizationUserId": membership.OrganizationUserID,
				}
			}
			respond.OK(c, data)
		})

		// Foundation validation endpoint: POST with a JSON body, echoing only
		// safe request context. Exercises content-type enforcement and the
		// success envelope; not a substitute for future business APIs.
		corporate.POST("/meta/echo", func(c *gin.Context) {
			var body map[string]any
			if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength != 0 {
				respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
				return
			}
			respond.OK(c, gin.H{
				"receivedKeys":  len(body),
				"correlationId": middleware.GetCorrelationID(c),
			})
		})

		// Tenant-scoped foundation probe: proves fail-closed tenant
		// resolution end to end.
		tenantScoped := corporate.Group("")
		tenantScoped.Use(middleware.ResolveTenant(opts.Resolver, opts.SessionStatus))
		// Registered after ResolveTenant so organization/user identity is
		// already resolved and available for keying -- see
		// middleware.GlobalRateLimit's own doc comment for why this can't
		// simply be mounted once on corporate/router instead.
		tenantScoped.Use(middleware.GlobalRateLimit(opts.Limiter, opts.Metrics))
		if opts.TenantRoutes != nil {
			opts.TenantRoutes(tenantScoped)
		}
		tenantScoped.GET("/meta/tenant", func(c *gin.Context) {
			tenantCtx, ok := middleware.TenantFromGin(c)
			if !ok {
				respond.Error(c, tenant.ErrMembershipNotFound)
				return
			}
			respond.OK(c, gin.H{
				"organizationId":     tenantCtx.OrganizationID,
				"organizationUserId": tenantCtx.OrganizationUserID,
				"actorType":          tenantCtx.ActorType,
				"permittedBranchIds": tenantCtx.PermittedBranchIDs,
			})
		})
	}

	// Internal service API: HMAC service authentication only.
	internal := router.Group("/internal/fleet-partner/v1")
	internal.Use(middleware.RequireJSON())
	if opts.ScopeVerifier != nil {
		internal.Use(middleware.InternalServiceScopeAuth(opts.ScopeVerifier))
	} else {
		internal.Use(middleware.InternalServiceAuth(opts.Verifier))
	}
	internal.Use(middleware.RejectReplayedNonce(opts.NonceStore))
	// Registered after InternalServiceAuth so ServiceCallerKey is already
	// set, giving every internal route a service-caller key rather than
	// falling back to IP only.
	internal.Use(middleware.GlobalRateLimit(opts.Limiter, opts.Metrics))
	{
		internal.GET("/meta", func(c *gin.Context) {
			respond.OK(c, gin.H{
				"service":       config.ServiceName,
				"routeClass":    "internal",
				"caller":        c.GetString(middleware.ServiceCallerKey),
				"correlationId": middleware.GetCorrelationID(c),
			})
		})
		if opts.InternalRoutes != nil {
			opts.InternalRoutes(internal)
		}
	}

	return router
}
