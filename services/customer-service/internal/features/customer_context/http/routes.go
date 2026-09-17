package contexthttp

import (
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterInternalRoutes mounts
// GET /internal/customers/:customerId/context?scopes=a,b&saved_location_id=
// behind HMAC service auth. The caller identity comes only from the verified
// signature. Scope grants come from the configured caller allowlist.
func RegisterInternalRoutes(group *gin.RouterGroup, service *contextusecases.Service, verifier *serviceauth.Verifier) {
	if group == nil || service == nil || verifier == nil {
		return
	}
	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))
	internal.GET("/customers/:customerId/context", func(c *gin.Context) {
		result, err := service.Resolve(c.Request.Context(), contextusecases.Request{
			Caller:          c.GetString("service_name"),
			CustomerID:      c.Param("customerId"),
			RequestedScopes: strings.Split(c.Query("scopes"), ","),
			SavedLocationID: c.Query("saved_location_id"),
		})
		if err != nil {
			httpx.RespondError(c, apperrors.From(err))
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
	})
}
