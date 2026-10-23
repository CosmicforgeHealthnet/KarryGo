package contexthttp

import (
	nethttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterInternalRoutes mounts
// GET /internal/customers/:customerId/context?scopes=a,b&saved_location_id=&guest_traveller_id=
// behind HMAC service auth. The caller identity comes only from the verified
// signature. Scope grants come from the configured caller allowlist.
func RegisterInternalRoutes(group *gin.RouterGroup, service *contextusecases.Service, verifier *serviceauth.Verifier, vendorStandingSecrets ...[]byte) {
	if group == nil || service == nil || verifier == nil {
		return
	}
	internal := group.Group("/internal")
	var standingVerifier *serviceauth.Verifier
	if len(vendorStandingSecrets) == 1 && len(vendorStandingSecrets[0]) >= 32 {
		standingVerifier = serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": vendorStandingSecrets[0]}, 30*time.Second)
	}
	internal.Use(func(c *gin.Context) {
		selected := verifier
		if c.GetHeader(serviceauth.HeaderServiceName) == "vendor-service" {
			scopes := strings.Split(c.Query("scopes"), ",")
			asksStanding := false
			for _, scope := range scopes {
				if strings.TrimSpace(scope) == "account_standing" {
					asksStanding = true
				}
			}
			if asksStanding {
				if len(scopes) != 1 || len(c.Request.URL.Query()["scopes"]) != 1 {
					c.AbortWithStatus(nethttp.StatusForbidden)
					return
				}
				if standingVerifier == nil {
					c.AbortWithStatus(nethttp.StatusServiceUnavailable)
					return
				}
				selected = standingVerifier
			}
		}
		serviceauth.Middleware(selected)(c)
	})
	internal.GET("/customers/:customerId/context", func(c *gin.Context) {
		result, err := service.Resolve(c.Request.Context(), contextusecases.Request{
			Caller:           c.GetString("service_name"),
			CustomerID:       c.Param("customerId"),
			RequestedScopes:  strings.Split(c.Query("scopes"), ","),
			SavedLocationID:  c.Query("saved_location_id"),
			GuestTravellerID: c.Query("guest_traveller_id"),
		})
		if err != nil {
			httpx.RespondError(c, apperrors.From(err))
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
	})
}
