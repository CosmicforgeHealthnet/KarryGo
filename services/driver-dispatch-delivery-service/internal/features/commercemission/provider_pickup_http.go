package commercemission

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
)

func RegisterProviderPickupRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *MultiStopMissionHandler) {
	if engine == nil || tokens == nil || handler == nil || handler.service == nil {
		return
	}
	group := engine.Group("/api/v1/provider/commerce-missions")
	group.Use(authhttp.DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	})
	group.GET("/by-trip/:tripId", handler.readProviderTrip)
	group.POST("/:missionRef/stops/:stopRef/arrive", handler.arrivePickup)
	group.POST("/:missionRef/stops/:stopRef/complete", handler.completePickup)
	group.POST("/:missionRef/receiver-unavailable", handler.receiverUnavailable)
}

func (h *MultiStopMissionHandler) receiverUnavailable(c *gin.Context) {
	var request dispatchcommerce.CommandRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if !validProviderOperationHeader(c, request.OperationRef) {
		return
	}
	mission, err := h.service.ReceiverUnavailable(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("missionRef"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func (h *MultiStopMissionHandler) arrivePickup(c *gin.Context) {
	var request dispatchcommerce.StopActionRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if !validProviderOperationHeader(c, request.OperationRef) {
		return
	}
	credential, err := h.service.ArrivePickup(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("missionRef"), c.Param("stopRef"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.PickupCredential]{Success: true, Data: credential})
}

func (h *MultiStopMissionHandler) completePickup(c *gin.Context) {
	var request dispatchcommerce.StopActionRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if !validProviderOperationHeader(c, request.OperationRef) {
		return
	}
	mission, err := h.service.CompletePickup(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("missionRef"), c.Param("stopRef"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func validProviderOperationHeader(c *gin.Context, operation string) bool {
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(c.GetHeader("Idempotency-Key")) != strings.TrimSpace(operation) {
		httpx.Abort(c, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must equal operation_ref.", nil))
		return false
	}
	return true
}
