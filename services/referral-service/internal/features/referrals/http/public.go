package referralhttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// RateLimiter is the subset of shared/go/ratelimit used here.
type RateLimiter interface {
	Allow(ctx context.Context, bucket, key string, max int, window time.Duration) error
}

// PublicOptions configures the app-facing routes.
type PublicOptions struct {
	Service *referralusecases.Service
	// CustomerTokenSecret verifies customer-app login tokens.
	CustomerTokenSecret []byte
	// ProviderTokenSecrets verify driver-app login tokens, keyed by actor
	// type (hauling, dispatch, taxi).
	ProviderTokenSecrets map[string][]byte
	// Limiter throttles the unauthenticated code check. Nil disables it
	// (local development only).
	Limiter RateLimiter
}

// RegisterPublicRoutes mounts the app-facing routes on the API base group.
func RegisterPublicRoutes(group *gin.RouterGroup, opts PublicOptions) {
	h := &publicHandler{service: opts.Service, limiter: opts.Limiter}
	group.GET("/codes/:code", h.checkCode)

	me := group.Group("/me", accountAuth(opts.CustomerTokenSecret, opts.ProviderTokenSecrets))
	me.GET("", h.myCode)
}

type publicHandler struct {
	service *referralusecases.Service
	limiter RateLimiter
}

const (
	actorTypeKey = "referral_actor_type"
	actorIDKey   = "referral_actor_id"
)

// accountAuth accepts a login token from any of the four apps and records
// which account it belongs to. Each issuer's token is checked with that
// issuer's own secret and must name that issuer's service, so a driver token
// can never pass as a customer's or another service's driver.
func accountAuth(customerSecret []byte, providerSecrets map[string][]byte) gin.HandlerFunc {
	type issuer struct {
		signer    *sharedauth.TokenSigner
		actorType string
		role      string // required role; "" accepts any non-empty role
	}
	var issuers []issuer
	if len(customerSecret) > 0 {
		issuers = append(issuers, issuer{signer: sharedauth.NewTokenSigner(customerSecret), actorType: referralcontract.ActorCustomer, role: "customer"})
	}
	for actorType, secret := range providerSecrets {
		if referralcontract.IsProviderActor(actorType) && len(secret) > 0 {
			issuers = append(issuers, issuer{signer: sharedauth.NewTokenSigner(secret), actorType: actorType})
		}
	}
	return func(c *gin.Context) {
		token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if token == "" || token == c.GetHeader("Authorization") {
			denyRequest(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		for _, candidate := range issuers {
			claims, err := candidate.signer.Verify(token)
			if err != nil {
				continue
			}
			if claims.Type != sharedauth.TokenTypeAccess || claims.Service != candidate.actorType || claims.Subject == "" {
				continue
			}
			if claims.Role == "" || (candidate.role != "" && claims.Role != candidate.role) {
				continue
			}
			c.Set(actorTypeKey, candidate.actorType)
			c.Set(actorIDKey, claims.Subject)
			c.Next()
			return
		}
		denyRequest(c, apperrors.Unauthorized("Your session is not valid. Please sign in again.", nil))
	}
}

func denyRequest(c *gin.Context, err error) {
	httpx.RespondError(c, err)
	c.Abort()
}

func (h *publicHandler) myCode(c *gin.Context) {
	view, err := h.service.MyCode(c.Request.Context(), c.GetString(actorTypeKey), c.GetString(actorIDKey))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// checkCode is unauthenticated (it runs on the sign-up screen), so it is rate
// limited per client to stop anyone enumerating codes.
func (h *publicHandler) checkCode(c *gin.Context) {
	if h.limiter != nil {
		if err := h.limiter.Allow(c.Request.Context(), "code-check", c.ClientIP(), 30, time.Minute); err != nil {
			httpx.Abort(c, err)
			return
		}
	}
	valid, err := h.service.CheckCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"valid": valid}})
}
