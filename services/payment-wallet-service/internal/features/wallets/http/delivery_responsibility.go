package wallethttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type deliveryResponsibilityRequest struct {
	OperationRef               string `json:"operation_ref" binding:"required"`
	IdempotencyKey             string `json:"idempotency_key" binding:"required"`
	ReasonCode                 string `json:"reason_code" binding:"required"`
	PlatformExecutionCostMinor int64  `json:"platform_execution_cost_minor"`
}

func (h *Handler) TransitionDeliveryResponsibility(c *gin.Context) {
	if c.GetString("service_name") != "vendor-service" {
		httpx.Abort(c, apperrors.Forbidden("Caller may not transition commerce delivery responsibility.", nil))
		return
	}
	var request deliveryResponsibilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.TransitionDeliveryResponsibility(c, walletusecases.TransitionDeliveryResponsibilityInput{
		VendorOrderID: c.Param("vendorOrderId"), OperationRef: request.OperationRef,
		IdempotencyKey: request.IdempotencyKey, ReasonCode: request.ReasonCode,
		PlatformExecutionCostMinor: request.PlatformExecutionCostMinor,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) GetDeliveryResponsibilityTransition(c *gin.Context) {
	if c.GetString("service_name") != "vendor-service" {
		httpx.Abort(c, apperrors.Forbidden("Caller may not read commerce delivery responsibility transitions.", nil))
		return
	}
	result, err := h.service.DeliveryResponsibilityTransition(c, c.Param("operationRef"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}
