package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
)

// GlobalRateLimit is the registry-driven counterpart to RateLimit: instead
// of a risk class fixed at route-registration time, it looks up
// c.FullPath()+method in riskregistry on every request. This is what lets
// one middleware instance, mounted once per route class (public,
// tenant-scoped, internal), cover every route that class serves without
// hand-wrapping each of the ~250 individual routes -- see
// internal/app/app.go's three call sites and
// docs/section28-security-privacy-audit.md's "Rate limiting" section for
// why three, not one: c.FullPath() is resolved before any middleware runs
// regardless of where in the chain it's mounted, but tenant/service-caller
// context is only populated by the auth middleware specific to each route
// class, so each class's own group is where its own identity dimensions
// first become available.
//
// An unclassified route (registry miss) falls back to the current STANDARD
// rule (read live from ratelimit.DefaultRiskRules on every request, not a
// value snapshotted at package-init time — so a runtime change to the
// STANDARD rule is reflected immediately) rather than either failing the
// request or skipping the check silently.
// TestRouteRiskRegistry_CoversEveryMountedRoute (internal/section28contract)
// makes this path unreachable in a passing build; it exists only so a route
// that somehow slips through in production degrades to a safe,
// conservative bound rather than an unbounded one.
func GlobalRateLimit(limiter ratelimit.Limiter, metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		riskClass, ok := riskregistry.Lookup(c.Request.Method, c.FullPath())
		rule := ratelimit.DefaultRiskRules[ratelimit.RiskStandard]
		if ok {
			rule = ratelimit.DefaultRiskRules[riskClass]
		} else {
			riskClass = "UNCLASSIFIED"
			if metrics != nil {
				metrics.IncCounter("fleet_partner_rate_limit_unclassified_route_total", nil, 1)
			}
		}
		enforceRateLimit(c, limiter, metrics, riskClass, rule)
	}
}

// RateLimit enforces a risk class fixed at route-registration time. Kept
// for callers that want to pin a class explicitly rather than rely on the
// registry (the section28 feature handlers this pass added originally used
// it this way; GlobalRateLimit is the broader, registry-driven mechanism
// that now covers the rest of the API surface).
//
// Both RateLimit and GlobalRateLimit funnel into enforceRateLimit, which is
// layered across every identity dimension the request actually carries —
// organization, organization user, service caller, and always IP as the
// floor. Each dimension is checked independently against the SAME rule
// under its own key, so a single organization spreading load across many
// users is still bounded by the organization key even though each user
// individually stays under limit, and one user cannot exhaust capacity by
// hitting many organizations either — this is what Section 12's "Do not
// allow one organization to exhaust all service capacity" and "Separate
// keys for IP/user/org/service" both require, using this service's
// existing distributed ratelimit.Limiter (Redis-backed when configured,
// in-process fallback otherwise — see internal/app/wiring.go's
// Components.Limiter) rather than a second, parallel rate-limit store. A
// nil limiter disables the check entirely, matching every other optional
// dependency in this composition root. A Redis/infrastructure error during
// Allow fails OPEN for that one dimension (never locks every caller out
// because a cache is briefly unreachable) — the same posture
// ratelimit.RedisLimiter.Allow already documents for the auth-flow scopes
// this middleware's rules coexist with.
func RateLimit(limiter ratelimit.Limiter, metrics *observability.Metrics, riskClass string, rule ratelimit.Rule) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		enforceRateLimit(c, limiter, metrics, riskClass, rule)
	}
}

func enforceRateLimit(c *gin.Context, limiter ratelimit.Limiter, metrics *observability.Metrics, riskClass string, rule ratelimit.Rule) {
	scope := "risk:" + riskClass
	for _, key := range rateLimitKeys(c) {
		allowed, err := limiter.Allow(c.Request.Context(), scope, key, rule)
		if err != nil {
			continue
		}
		if !allowed {
			if metrics != nil {
				metrics.IncCounter("fleet_partner_rate_limit_denied_total",
					map[string]string{"risk_class": riskClass}, 1)
			}
			c.Header("Retry-After", strconv.Itoa(int(rule.Window.Seconds())))
			httpx.Abort(c, apperrors.RateLimited("Too many requests. Please slow down and try again.", nil))
			return
		}
	}
	c.Next()
}

// rateLimitKeys returns every identity dimension present on this request,
// most specific first. IP is always included as the floor -- an
// unauthenticated or misconfigured caller must still be bounded by
// something.
//
// ClientIP() is gin's own trusted-proxy-aware resolution
// (app.BuildRouter always calls SetTrustedProxies explicitly, defaulting
// to trusting none — see config.Config.TrustedProxies): a caller cannot
// pick its own rate-limit bucket by setting an arbitrary
// X-Forwarded-For header unless an operator has explicitly named that
// caller's proxy as trusted.
func rateLimitKeys(c *gin.Context) []string {
	var keys []string
	if tenantCtx, ok := TenantFromGin(c); ok {
		if tenantCtx.OrganizationID != "" {
			keys = append(keys, "org:"+tenantCtx.OrganizationID)
		}
		if tenantCtx.OrganizationUserID != "" {
			keys = append(keys, "user:"+tenantCtx.OrganizationUserID)
		}
	}
	if caller := c.GetString(ServiceCallerKey); caller != "" {
		keys = append(keys, "svc:"+caller)
	}
	keys = append(keys, "ip:"+c.ClientIP())
	return keys
}
