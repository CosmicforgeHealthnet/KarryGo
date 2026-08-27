package supporthttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"
)

func RegisterCommerceCaseRoutes(group *gin.RouterGroup, service *supportusecases.SupportService, secrets serviceauth.Secrets) {
	if group == nil || service == nil {
		return
	}
	verify := serviceauth.Middleware(serviceauth.NewVerifier(secrets, 5*time.Minute))
	internal := group.Group("/internal/commerce-cases", verify, requireService("vendor-service"))
	internal.POST("", createCommerceCase(service))
	internal.GET("/operations/:operationRef", getCommerceCaseByOperation(service))
	internal.GET("/:caseRef", getCommerceCaseByRef(service))
	resolution := group.Group("/admin/commerce-cases", verify, requireService("admin-backoffice-service"))
	resolution.POST("/:caseRef/resolution", resolveCommerceCase(service))
}

func requireService(allowed ...string) gin.HandlerFunc {
	set := map[string]bool{}
	for _, v := range allowed {
		set[v] = true
	}
	return func(c *gin.Context) {
		caller, _ := c.Get("service_name")
		if !set[caller.(string)] {
			httpx.RespondError(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
			c.Abort()
			return
		}
		c.Next()
	}
}

func createCommerceCase(service *supportusecases.SupportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req supportcase.CreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body must be valid JSON.", err))
			return
		}
		value, created, err := service.CreateCommerceCase(c.Request.Context(), req)
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.JSON(status, gin.H{"success": true, "data": value})
	}
}

func getCommerceCaseByOperation(service *supportusecases.SupportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, err := service.GetCommerceCaseByOperation(c.Request.Context(), c.Query("vendor_ref"), c.Param("operationRef"))
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
	}
}
func getCommerceCaseByRef(service *supportusecases.SupportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, err := service.GetCommerceCaseByRef(c.Request.Context(), c.Query("vendor_ref"), c.Param("caseRef"))
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
	}
}
func resolveCommerceCase(service *supportusecases.SupportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Resolution         string `json:"resolution"`
			FinancialActionRef string `json:"financial_action_ref"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body must be valid JSON.", err))
			return
		}
		actor := strings.TrimSpace(c.GetHeader("X-Actor-ID"))
		if actor == "" {
			actor = "admin-backoffice-service"
		}
		value, err := service.ResolveCommerceCase(c.Request.Context(), c.Param("caseRef"), req.Resolution, req.FinancialActionRef, actor, time.Now())
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
	}
}
