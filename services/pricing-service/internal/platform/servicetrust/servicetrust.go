// Package servicetrust guards pricing-service's internal API.
//
// Three checks apply to every signed request, in order:
//
//  1. serviceauth.Middleware verifies the HMAC signature and names the caller.
//  2. RejectReplayedNonce makes the request single-use.
//  3. RequireCallers checks that this particular caller is allowed on this
//     particular route group.
//
// The third check exists because holding a secret is not the same as holding
// permission. A booking service must be able to ask for a quote; it must not be
// able to approve a rate change. Pricing decides money, so authority is granted
// per route group rather than per service.
package servicetrust

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// CallerFrom returns the verified caller name that serviceauth.Middleware put on
// the context. Handlers must use this rather than any service name in the
// request body: the body is not signed evidence of who is calling.
func CallerFrom(c *gin.Context) string { return c.GetString("service_name") }

// RequireCallers rejects signed requests from services outside allowed. It must
// run after serviceauth.Middleware.
func RequireCallers(allowed ...string) gin.HandlerFunc {
	set := make(map[string]bool, len(allowed))
	for _, caller := range allowed {
		set[caller] = true
	}
	return func(c *gin.Context) {
		if !set[CallerFrom(c)] {
			deny(c, apperrors.Forbidden("This service may not call this endpoint.", nil))
			return
		}
		c.Next()
	}
}

// RejectReplayedNonce makes every signed request single-use by recording its
// nonce until well past the signature's validity window.
//
// For pricing this is not merely hygiene: replaying a quote request would mint a
// second sealed quote for one intent, and replaying a bind or a reconcile would
// attempt a second economic decision on the same booking.
func RejectReplayedNonce(db database.Querier) gin.HandlerFunc {
	return func(c *gin.Context) {
		nonce := c.GetHeader(serviceauth.HeaderNonce)
		caller := CallerFrom(c)
		if nonce == "" || caller == "" {
			deny(c, apperrors.Unauthorized("Signed requests must carry a nonce.", nil))
			return
		}
		tag, err := db.Exec(c.Request.Context(), `
			INSERT INTO pricing_service_nonces (caller, nonce, expires_at)
			VALUES ($1, $2, now() + interval '15 minutes')
			ON CONFLICT DO NOTHING`, caller, nonce)
		if err != nil {
			deny(c, apperrors.Unavailable("Request could not be verified.", err))
			return
		}
		if tag.RowsAffected() != 1 {
			deny(c, apperrors.Unauthorized("This request was already used.", nil))
			return
		}
		c.Next()
	}
}

// PruneNonces deletes expired nonces. Safe to run from any replica.
func PruneNonces(ctx context.Context, db database.Querier) error {
	_, err := db.Exec(ctx, `DELETE FROM pricing_service_nonces WHERE expires_at < $1`, time.Now())
	return err
}

func deny(c *gin.Context, err error) {
	httpx.RespondError(c, err)
	c.Abort()
}
