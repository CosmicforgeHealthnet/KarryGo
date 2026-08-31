package commercemission

import (
	"net/http"

	"github.com/gin-gonic/gin"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type OperationalHandler struct{ service *OperationalService }

func NewOperationalHandler(service *OperationalService) *OperationalHandler {
	return &OperationalHandler{service: service}
}

func RegisterProviderOperationalRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *OperationalHandler) {
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
	group.POST("/:missionRef/evidence", handler.submitEvidence)
	group.POST("/:missionRef/exceptions", handler.reportException)
	group.POST("/:missionRef/returns", handler.startReturn)
	group.POST("/:missionRef/returns/:returnId/complete", handler.completeReturn)
	group.POST("/:missionRef/provider-transfers/start", handler.startTransfer)
	group.POST("/:missionRef/provider-transfers/:transferRef/complete", handler.completeTransfer)
}

func RegisterInternalOperationalRoutes(group *gin.RouterGroup, handler *OperationalHandler,
	verifier *serviceauth.Verifier, nonces authority.NonceClaimer, allowed []string) {
	if group == nil || handler == nil || handler.service == nil || verifier == nil {
		return
	}
	routes := group.Group("/dispatch/v1/commerce/missions")
	routes.Use(authority.ServiceBoundary(verifier, nonces, allowed)...)
	routes.GET("/:missionRef/evidence", handler.listEvidence)
}

func (h *OperationalHandler) submitEvidence(c *gin.Context) {
	var command EvidenceCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	result, err := h.service.SubmitProviderEvidence(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), c.Query("stop_ref"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[EvidenceRecord]{Success: true, Data: result})
}

func (h *OperationalHandler) reportException(c *gin.Context) {
	var command ExceptionCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	result, err := h.service.ReportException(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[ExceptionRecord]{Success: true, Data: result})
}

func (h *OperationalHandler) startReturn(c *gin.Context) {
	var command ReturnCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	result, err := h.service.StartReturn(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[ReturnRecord]{Success: true, Data: result})
}

func (h *OperationalHandler) completeReturn(c *gin.Context) {
	var command CompleteReturnCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	if command.ReturnID == "" {
		command.ReturnID = c.Param("returnId")
	}
	if command.ReturnID != c.Param("returnId") {
		httpx.Abort(c, apperrors.New(409, "RETURN_SCOPE_MISMATCH", "Return id does not match the route.", nil))
		return
	}
	result, err := h.service.CompleteReturn(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[ReturnRecord]{Success: true, Data: result})
}

func (h *OperationalHandler) startTransfer(c *gin.Context) {
	var command StartTransferCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	result, err := h.service.StartProviderTransfer(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[TransferCredential]{Success: true, Data: result})
}

func (h *OperationalHandler) completeTransfer(c *gin.Context) {
	var command CompleteTransferCommand
	if err := decodeMissionBody(c, &command); err != nil || !validProviderOperationHeader(c, command.OperationRef) {
		if err != nil {
			httpx.Abort(c, err)
		}
		return
	}
	result, err := h.service.CompleteProviderTransfer(c, authhttp.DispatchRiderID(c), c.Param("missionRef"), c.Param("transferRef"), command)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: result})
}

func (h *OperationalHandler) listEvidence(c *gin.Context) {
	result, err := h.service.repo.ListOperationalEvidence(c, missionCaller(c), c.Param("missionRef"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[[]EvidenceRecord]{Success: true, Data: result})
}
