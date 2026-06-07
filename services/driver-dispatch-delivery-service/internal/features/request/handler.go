package request

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	authhttp "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/http"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	group := engine.Group("/api/v1/provider/requests")
	group.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	group.GET("", handler.ListInbox)
	group.GET("/:id", handler.GetRequest)
	group.POST("/:id/accept", handler.AcceptRequest)
	group.POST("/:id/reject", handler.RejectRequest)
}

func (h *Handler) ListInbox(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	result, err := h.service.ListInbox(c.Request.Context(), authhttp.DispatchRiderID(c), ListInboxOptions{
		Status: InboxStatus(c.Query("status")), Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	httpx.RespondSuccess(c, http.StatusOK, "Request inbox loaded.", result)
}

// GetRequest — Phase 6F: full booking detail including receiver_phone.
func (h *Handler) GetRequest(c *gin.Context) {
	detail, err := h.service.GetRequestDetail(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	httpx.RespondSuccess(c, http.StatusOK, "Request loaded.", detail)
}

// AcceptRequest — Phase 6G: atomic Redis lock, DB transaction, event publish.
func (h *Handler) AcceptRequest(c *gin.Context) {
	result, err := h.service.Accept(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	httpx.RespondSuccess(c, http.StatusOK, "Request accepted.", result)
}

// RejectRequest — Phase 6H: optional reason body, event publish.
func (h *Handler) RejectRequest(c *gin.Context) {
	var body RejectRequest
	// Body is optional — ignore bind error and use empty reason.
	_ = c.ShouldBindJSON(&body)

	// Validate reason if provided.
	if body.Reason != "" {
		if _, ok := ValidRejectReasons[body.Reason]; !ok {
			httpx.Abort(c, apperrors.New(http.StatusBadRequest, apperrors.CodeValidationFailed, "Check your details.", nil))
			return
		}
	}

	result, err := h.service.Reject(c.Request.Context(), c.Param("id"), authhttp.DispatchRiderID(c), body.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	httpx.RespondSuccess(c, http.StatusOK, "Request rejected.", result)
}

func requireDispatchProviderRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	}
}
