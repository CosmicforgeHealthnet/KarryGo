package securehandover

import (
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

func RegisterProvider(engine *gin.Engine, tokens *authusecases.TokenUsecase, handler *Handler) {
	if handler == nil || handler.service == nil || !handler.service.Enabled() {
		return
	}
	group := engine.Group("/api/v1/provider/trips")
	group.Use(authhttp.DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	})
	group.GET("/:id/stops/:stopId/pickup-credential", handler.pickupCredential)
	group.POST("/:id/stops/:stopId/parcels/scan", handler.scanParcel)
	group.POST("/:id/stops/:stopId/evidence-session", handler.captureSession)
	group.POST("/:id/stops/:stopId/evidence", handler.submitEvidence)
	group.POST("/:id/delivery/verify", handler.verifyDelivery)
	group.POST("/:id/return/verify", handler.verifyReturn)
}

func RegisterCustomer(engine *gin.Engine, signer *sharedauth.TokenSigner, handler *Handler) {
	if handler == nil || handler.service == nil || !handler.service.Enabled() {
		return
	}
	group := engine.Group("/api/v1/customer/dispatch/bookings")
	group.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	group.POST("/:id/pickup/verify", handler.verifyCustomerPickup)
	group.POST("/:id/handover/resend", handler.resendCustomer)
	group.POST("/:id/return-credential", handler.returnCredentialCustomer)
}

func RegisterInternal(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if handler == nil || handler.service == nil || !handler.service.Enabled() {
		return
	}
	routes := group.Group("/commerce/dispatch/missions")
	routes.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	routes.POST("/:id/stops/:stopId/pickup/verify", handler.verifySourcePickup)
	routes.GET("/:id/stops/:stopId/handover/:stage", handler.readSourceHandover)
	routes.GET("/:id/handover/:stage", handler.readSourceHandover)
	routes.POST("/:id/handover/resend", handler.resendSource)
	routes.POST("/:id/stops/:stopId/return-credential", handler.returnCredentialSource)
}

func (h *Handler) readSourceHandover(c *gin.Context) {
	caller, _ := c.Get("service_name")
	stage := Stage(strings.ToUpper(strings.TrimSpace(c.Param("stage"))))
	result, err := h.service.ReadBySource(c.Request.Context(), stringValue(caller), c.Param("id"), c.Param("stopId"), stage)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) pickupCredential(c *gin.Context) {
	operation := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if operation == "" {
		operation = "pickup-credential:" + c.Param("id") + ":" + c.Param("stopId")
	}
	result, err := h.service.IssuePickup(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), c.Param("stopId"), operation)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) verifyCustomerPickup(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var command VerifyCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.VerifyPickupByCustomer(c.Request.Context(), claims.Subject, c.Param("id"), command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) verifySourcePickup(c *gin.Context) {
	var command VerifyCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	caller, _ := c.Get("service_name")
	result, err := h.service.VerifyPickupBySource(c.Request.Context(), stringValue(caller), c.Param("id"), c.Param("stopId"), command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) verifyDelivery(c *gin.Context) {
	var command VerifyCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.VerifyDelivery(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) verifyReturn(c *gin.Context) {
	var command VerifyCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.VerifyReturn(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) returnCredentialCustomer(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.IssueReturnByCustomer(c.Request.Context(), claims.Subject, c.Param("id"), strings.TrimSpace(c.GetHeader("Idempotency-Key")))
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) returnCredentialSource(c *gin.Context) {
	caller, _ := c.Get("service_name")
	result, err := h.service.IssueReturnBySource(c.Request.Context(), stringValue(caller), c.Param("id"), c.Param("stopId"), strings.TrimSpace(c.GetHeader("Idempotency-Key")))
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) resendCustomer(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var command ResendCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.Resend(c.Request.Context(), Actor{Type: "CUSTOMER", Ref: claims.Subject}, command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) resendSource(c *gin.Context) {
	var command ResendCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	caller, _ := c.Get("service_name")
	result, err := h.service.Resend(c.Request.Context(), Actor{Type: sourceActor(stringValue(caller)), Ref: stringValue(caller), SourceRef: c.Param("id")}, command)
	respond(c, http.StatusOK, result, err)
}

func (h *Handler) scanParcel(c *gin.Context) {
	var command ScanCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	err := h.service.ScanParcel(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), c.Param("stopId"), command)
	respond(c, http.StatusOK, gin.H{"scanned": err == nil}, err)
}

func (h *Handler) captureSession(c *gin.Context) {
	var command CaptureSessionCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.CreateCaptureSession(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), c.Param("stopId"), command)
	respond(c, http.StatusCreated, result, err)
}

func (h *Handler) submitEvidence(c *gin.Context) {
	var command EvidenceCommand
	if c.ShouldBindJSON(&command) != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	command.OperationRef = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := h.service.SubmitEvidence(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("id"), c.Param("stopId"), command)
	respond(c, http.StatusCreated, result, err)
}

func respond(c *gin.Context, status int, result any, err error) {
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(status, gin.H{"success": true, "data": result})
}

func stringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}
