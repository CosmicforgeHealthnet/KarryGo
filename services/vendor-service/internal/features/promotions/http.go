package promotions

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

func NewHandler(service *Service, stores ...*idempotency.Store) *Handler {
	handler := &Handler{service: service}
	if len(stores) != 0 {
		handler.idempotency = stores[0]
	}
	return handler
}

func promotionActor(c *gin.Context) (production.Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return production.Actor{}, false
	}
	return production.Actor{StaffID: staff.StaffID, VendorID: staff.VendorID, SessionID: staff.SessionID, CorrelationID: httpx.GetRequestID(c), CausationID: c.GetHeader("X-Causation-ID"), CommandRef: c.GetHeader("Idempotency-Key")}, true
}
func promotionError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apperrors.NotFound("Promotion not found.", err)
	case errors.Is(err, ErrForbidden):
		return apperrors.Forbidden("You may not manage this promotion.", err)
	case errors.Is(err, ErrConflict), errors.Is(err, ErrInvalidState):
		return apperrors.Conflict("The promotion changed or cannot enter that status.", err)
	case errors.Is(err, ErrInvalidCampaign), errors.Is(err, ErrCouponInvalid):
		return apperrors.Validation("Check the promotion values.", nil)
	default:
		return apperrors.Internal("Promotion operation failed.", err)
	}
}

func (h *Handler) create(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var input CampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.Create(c, actor, input)
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}
func (h *Handler) list(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.List(c, actor)
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"promotions": result}})
}
func (h *Handler) get(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.Get(c, actor, c.Param("promotionId"))
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(result.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) update(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	version, err := strconv.ParseInt(strings.Trim(c.GetHeader("If-Match"), `"`), 10, 64)
	if err != nil || version < 1 {
		httpx.RespondError(c, apperrors.Conflict("If-Match must name the current promotion version.", err))
		return
	}
	var input CampaignInput
	if err = c.ShouldBindJSON(&input); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.Update(c, actor, c.Param("promotionId"), input, version)
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(result.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type statusRequest struct {
	Status Status `json:"status" binding:"required"`
}

func (h *Handler) status(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	version, err := strconv.ParseInt(strings.Trim(c.GetHeader("If-Match"), `"`), 10, 64)
	if err != nil || version < 1 {
		httpx.RespondError(c, apperrors.Conflict("If-Match must name the current promotion version.", err))
		return
	}
	var input statusRequest
	if err = c.ShouldBindJSON(&input); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.SetStatus(c, actor, c.Param("promotionId"), input.Status, version)
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(result.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type couponRequest struct {
	Code                  string `json:"code" binding:"required"`
	UsageLimit            *int64 `json:"usage_limit,omitempty"`
	PerCustomerUsageLimit *int64 `json:"per_customer_usage_limit,omitempty"`
}

func (h *Handler) coupon(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var input couponRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	id, err := h.service.CreateCoupon(c, actor, c.Param("promotionId"), input.Code, input.UsageLimit, input.PerCustomerUsageLimit)
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"coupon_id": id}})
}
func (h *Handler) usage(c *gin.Context) {
	actor, ok := promotionActor(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	count, err := h.service.Usage(c, actor, c.Param("promotionId"))
	if err != nil {
		httpx.RespondError(c, promotionError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"used": count}})
}

func RegisterRoutes(root *gin.RouterGroup, handler *Handler, enabled bool) {
	if root == nil || handler == nil || handler.service == nil || !enabled {
		return
	}
	group := root.Group("/promotions")
	if handler.idempotency != nil {
		group.Use(idempotency.CommandMiddleware(handler.idempotency))
	}
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:promotionId", handler.get)
	group.PATCH("/:promotionId", handler.update)
	group.POST("/:promotionId/status", handler.status)
	group.POST("/:promotionId/coupons", handler.coupon)
	group.GET("/:promotionId/usage", handler.usage)
}
