package billing

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// PartnerIDResolver extracts the authenticated partner id from the request
// context (set by the dashboard session middleware).
type PartnerIDResolver func(c *gin.Context) string

// RegisterBillingRoutes mounts a compact partner billing route set. The main
// gateway currently registers dashboard billing through partneraccount routes;
// this helper is kept compile-safe for tests or alternate wiring.
func RegisterBillingRoutes(r *gin.Engine, sessionMiddleware gin.HandlerFunc, partnerID PartnerIDResolver, svc *Service) {
	grp := r.Group("/api/v1/partner/v1/billing")
	grp.Use(sessionMiddleware)

	grp.GET("/invoices", func(c *gin.Context) {
		views, total, err := svc.ListInvoices(c.Request.Context(), partnerID(c), 1, 100)
		if err != nil {
			respondError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"invoices": views, "meta": gin.H{"total": total}}})
	})

	grp.GET("/invoices/:id", func(c *gin.Context) {
		view, err := svc.GetInvoice(c.Request.Context(), partnerID(c), c.Param("id"))
		if err != nil {
			respondError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
	})
}

func respondError(c *gin.Context, err error) {
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) {
		apiErr = delivery.ErrInternal()
	}
	fields := apiErr.Fields
	if fields == nil {
		fields = []delivery.FieldError{}
	}
	c.AbortWithStatusJSON(apiErr.Status, gin.H{
		"success": false,
		"error":   gin.H{"code": apiErr.Code, "message": apiErr.Message, "fields": fields},
	})
}
