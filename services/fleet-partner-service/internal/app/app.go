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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
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
	Resolver    tenant.MembershipResolver
	ReadyChecks []ReadyCheck
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
	// InternalRoutes are service-to-service routes mounted behind HMAC
	// service authentication. They are never reachable with a corporate,
	// provider or customer token.
	InternalRoutes RouteRegistrar
}

// BuildRouter constructs the full gin engine.
func BuildRouter(opts Options) *gin.Engine {
	if opts.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()

	// Documented middleware ordering — see internal/middleware/middleware.go.
	router.Use(httpx.Recovery())
	router.Use(httpx.RequestID())
	router.Use(middleware.CorrelationID())
	router.Use(middleware.SecureHeaders())
	router.Use(middleware.BodySizeLimit(opts.Config.MaxRequestBodyBytes))
	router.Use(middleware.RequestLogger(opts.Config, opts.Metrics))
	router.Use(respond.ErrorHandler())

	// Health: process liveness only — no dependency checks.
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": config.ServiceName, "status": "ok"})
	})

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
		opts.PublicAuthRoutes(public)
	}

	// Corporate web API: Bearer token + tenant resolution.
	corporate := router.Group("/fleet-partner/v1")
	corporate.Use(middleware.RequireJSON())
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
		tenantScoped.Use(middleware.ResolveTenant(opts.Resolver))
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
	internal.Use(middleware.InternalServiceAuth(opts.Verifier))
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
