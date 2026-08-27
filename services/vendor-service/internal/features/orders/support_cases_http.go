package orders

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"
)

type commerceIssueRequest struct {
	Category       supportcase.Category `json:"category"`
	Description    string               `json:"description"`
	VendorOrderRef string               `json:"vendor_order_ref"`
	LineRefs       []string             `json:"line_refs"`
}

func (h *Handler) createCustomerSupportCase(c *gin.Context) {
	customer, ok := customerSubject(c)
	if !ok {
		return
	}
	var req commerceIssueRequest
	if err := strictJSON(c, &req); err != nil {
		httpx.RespondError(c, err)
		return
	}
	operation := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if operation == "" {
		httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", nil))
		return
	}
	value, created, err := h.service.CreateCustomerCommerceIssue(c.Request.Context(), customer, c.Param("orderId"), CommerceIssueInput{OperationRef: operation, Category: req.Category, Description: req.Description, VendorOrderRef: req.VendorOrderRef, LineRefs: req.LineRefs})
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"success": true, "data": value})
}
func (h *Handler) listCustomerSupportCases(c *gin.Context) {
	customer, ok := customerSubject(c)
	if !ok {
		return
	}
	values, err := h.service.ListCustomerSupportCases(c.Request.Context(), customer, c.Param("orderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"cases": values}})
}
func (h *Handler) getCustomerSupportCase(c *gin.Context) {
	customer, ok := customerSubject(c)
	if !ok {
		return
	}
	value, err := h.service.GetCustomerSupportCase(c.Request.Context(), customer, c.Param("orderId"), c.Param("caseRef"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
}
func (h *Handler) createVendorSupportCase(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	var req commerceIssueRequest
	if err := strictJSON(c, &req); err != nil {
		httpx.RespondError(c, err)
		return
	}
	operation := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if operation == "" {
		httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", nil))
		return
	}
	value, created, err := h.service.CreateVendorCommerceIssue(c.Request.Context(), actor, c.Param("vendorOrderId"), CommerceIssueInput{OperationRef: operation, Category: req.Category, Description: req.Description, LineRefs: req.LineRefs})
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"success": true, "data": value})
}
func (h *Handler) listVendorSupportCases(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	values, err := h.service.ListVendorSupportCases(c.Request.Context(), actor, c.Param("vendorOrderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"cases": values}})
}

func RegisterSupportResolutionRoutes(router *gin.Engine, service *Service, verifier *serviceauth.Verifier) {
	if router == nil || service == nil || verifier == nil {
		return
	}
	handler := func(c *gin.Context) {
		caller, _ := c.Get("service_name")
		if caller != "support-dispute-service" {
			httpx.RespondError(c, apperrors.Forbidden("Service is not allowed to publish support resolutions.", nil))
			return
		}
		var event supportcase.ResolutionEvent
		if err := c.ShouldBindJSON(&event); err != nil {
			httpx.RespondError(c, apperrors.BadRequest("Request body must be valid JSON.", err))
			return
		}
		if err := service.ApplySupportResolution(c.Request.Context(), event); err != nil {
			httpx.RespondError(c, orderHTTPError(err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
	router.POST("/api/v1/internal/vendor/commerce/support-case-resolutions", serviceauth.Middleware(verifier), handler)
}

var _ = time.Time{}
