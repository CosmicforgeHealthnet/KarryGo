package customertracking

import (
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"time"
)

func RegisterCustomer(g *gin.RouterGroup, r Reader, signer *auth.TokenSigner, enabled bool) {
	g.GET("/customer/bookings/:id/location", auth.BearerMiddleware(signer, "customer", "customer"), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !enabled {
			httpx.RespondError(c, apperrors.Unavailable("Ride tracking is unavailable.", nil))
			return
		}
		claims, _ := auth.ClaimsFromContext(c)
		p, err := r.Read(c, c.Param("id"), claims.Subject)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		c.JSON(200, gin.H{"success": true, "data": gin.H{"booking_ref": p.SourceRef, "booking_status": p.Status, "location_available": p.Location.Available, "location_state": p.Location.State, "location": p.Location, "provider": p.Provider, "eta": p.ETA, "pickup_eta": p.PickupETA, "dropoff_eta": p.DropoffETA, "eta_seconds": etaSeconds(p.PickupETA, p.DropoffETA)}})
	})
}
func RegisterInternal(root *gin.Engine, r Reader, verifier *serviceauth.Verifier, redis redis.UniversalClient, enabled bool) {
	if verifier == nil {
		return
	}
	g := root.Group("/internal/taxi/v1/customer-share", journeyshare.RedisBoundary(verifier, redis, "customer-service")...)
	g.GET("/bookings/:sourceRef", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !enabled {
			httpx.RespondError(c, apperrors.Unavailable("Ride tracking is unavailable.", nil))
			return
		}
		if c.Query("customer_id") == "" {
			httpx.RespondError(c, apperrors.BadRequest("Customer assertion is required.", nil))
			return
		}
		p, err := r.Read(c, c.Param("sourceRef"), c.Query("customer_id"))
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
	})
}

// etaSeconds is the seconds until each ETA, so an app with a wrong clock
// still shows the right countdown.
func etaSeconds(pickup, dropoff *time.Time) gin.H {
	out := gin.H{}
	now := time.Now()
	for key, at := range map[string]*time.Time{"pickup": pickup, "dropoff": dropoff} {
		if at != nil {
			secs := int64(at.Sub(now).Seconds())
			if secs < 0 {
				secs = 0
			}
			out[key] = secs
		}
	}
	return out
}
