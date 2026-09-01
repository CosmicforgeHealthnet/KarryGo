package routeintegrity

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func RegisterProvider(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	group := engine.Group("/api/v1/provider")
	group.Use(authhttp.DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	})
	group.POST("/trips/:id/reroute", handler.reroute)
	group.POST("/offline-commands", handler.command)
}
func RegisterCustomer(engine *gin.Engine, signer *sharedauth.TokenSigner, handler *Handler) {
	group := engine.Group("/api/v1/customer/dispatch/bookings")
	group.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	group.GET("/:id/route-status", handler.status)
}
func (h *Handler) reroute(c *gin.Context) {
	var body struct {
		ReasonCode string `json:"reason_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	reason := strings.ToUpper(strings.TrimSpace(body.ReasonCode))
	if reason != "ROAD_CLOSURE" && reason != "SAFETY_REROUTE" && reason != "ROUTE_PROVIDER_UPDATE" {
		httpx.Abort(c, apperrors.BadRequest("Reroute reason is invalid.", nil))
		return
	}
	result, err := h.service.EnsureRoute(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c), reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Route updated.", result)
}
func (h *Handler) command(c *gin.Context) {
	var body DeviceCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	body.IdempotencyKey = c.GetHeader("Idempotency-Key")
	result, err := h.service.ReplayCommand(c.Request.Context(), authhttp.DispatchRiderID(c), body)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Offline command reconciled.", result)
}
func (h *Handler) status(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.CustomerStatus(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Route status loaded.", result)
}
