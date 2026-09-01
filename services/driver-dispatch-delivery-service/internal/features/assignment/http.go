package assignment

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func RegisterInternal(group *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if h == nil || h.service == nil {
		return
	}
	g := group.Group("/dispatch/v1/assignments")
	g.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	g.POST("", h.start)
}
func RegisterAdminInternal(group *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if h == nil || h.service == nil {
		return
	}
	g := group.Group("/dispatch/v1/admin-assisted-assignments")
	g.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	g.POST("", h.startAdmin)
}
func RegisterPartnerInternal(group *gin.RouterGroup, h *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if h == nil || h.service == nil {
		return
	}
	g := group.Group("/dispatch/v1/partner/missions")
	g.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	g.POST("/:missionId/queue-options", h.partnerQueueOption)
}
func RegisterProvider(engine *gin.Engine, tokens *authusecases.TokenUsecase, h *Handler) {
	g := engine.Group("/api/v1/provider/assignment-offers")
	g.Use(authhttp.DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	})
	g.GET("", h.list)
	g.POST("/:offerId/accept", h.accept)
	g.POST("/:offerId/decline", h.decline)
}
func RegisterCustomer(engine *gin.Engine, signer *sharedauth.TokenSigner, h *Handler) {
	g := engine.Group("/api/v1/customer/dispatch/missions")
	g.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	g.POST("/:missionId/queue-options", h.queueOption)
}
func (h *Handler) start(c *gin.Context) {
	var in StartRequest
	if c.ShouldBindJSON(&in) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	in.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.Start(c.Request.Context(), in, false)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": out})
}
func (h *Handler) startAdmin(c *gin.Context) {
	var in StartRequest
	if c.ShouldBindJSON(&in) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	in.Strategy = AdminAssisted
	operatorRef := strings.TrimSpace(in.ActorRef)
	if operatorRef == "" {
		fail(c, errors.New("operator identity is required"))
		return
	}
	caller, _ := c.Get("service_name")
	in.ActorRef = strings.TrimSpace(anyString(caller)) + ":" + operatorRef
	in.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.Start(c.Request.Context(), in, true)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": out})
}
func anyString(value any) string { text, _ := value.(string); return text }
func (h *Handler) list(c *gin.Context) {
	out, err := h.service.ListProviderOffers(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) accept(c *gin.Context) {
	var d Decision
	if c.ShouldBindJSON(&d) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	d.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.Accept(c.Request.Context(), c.Param("offerId"), authhttp.DispatchRiderID(c), d)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) decline(c *gin.Context) {
	var d Decision
	if c.ShouldBindJSON(&d) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	d.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.Decline(c.Request.Context(), c.Param("offerId"), authhttp.DispatchRiderID(c), d)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) queueOption(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var in QueueOption
	if c.ShouldBindJSON(&in) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	in.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.ApplyQueueOption(c.Request.Context(), c.Param("missionId"), claims.Subject, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) partnerQueueOption(c *gin.Context) {
	var in QueueOption
	if c.ShouldBindJSON(&in) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	in.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	out, err := h.service.ApplyPartnerQueueOption(c.Request.Context(), c.Param("missionId"), in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func fail(c *gin.Context, err error) {
	status, code := http.StatusBadRequest, "ASSIGNMENT_INVALID"
	switch {
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "ASSIGNMENT_NOT_FOUND"
	case errors.Is(err, ErrConflict):
		status, code = http.StatusConflict, "ASSIGNMENT_CONFLICT"
	case errors.Is(err, ErrExpired):
		status, code = http.StatusConflict, "OFFER_EXPIRED"
	case errors.Is(err, ErrStaleQuote):
		status, code = http.StatusConflict, "REQUOTE_REQUIRED"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}
