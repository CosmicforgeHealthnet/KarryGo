package deliveryrecovery

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
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
	routes := engine.Group("/api/v1/provider/trips")
	routes.Use(authhttp.DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	})
	routes.POST("/:id/delivery-attempts", handler.startAttempt)
	routes.POST("/:id/delivery-attempts/:attemptId/contact", handler.recordContact)
	routes.POST("/:id/delivery-attempts/:attemptId/fail", handler.failAttempt)
	routes.POST("/:id/breakdown", handler.reportIncident)
	routes.GET("/:id/hub-hold", handler.readHub)
	routes.POST("/:id/hub-hold/evidence-content", handler.uploadHubEvidence)
	routes.POST("/:id/hub-hold/checkpoints", handler.hubCheckpoint)
}

func RegisterCustomer(engine *gin.Engine, signer *sharedauth.TokenSigner, handler *Handler) {
	route := engine.Group("/api/v1/customer/dispatch/bookings")
	route.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	route.POST("/:id/delivery-option", handler.selectRecovery)
}

func RegisterEmergencyInternal(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier, db *pgxpool.Pool, allowed []string) {
	if handler == nil || handler.service == nil {
		return
	}
	routes := group.Group("/dispatch/v1/trips")
	routes.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	routes.POST("/:id/emergency-recovery", handler.emergencyRecovery)
}

func (h *Handler) startAttempt(c *gin.Context) {
	result, err := h.service.StartAttempt(c.Request.Context(), StartAttemptCommand{TripID: c.Param("id"), ProviderID: authhttp.DispatchRiderID(c), IdempotencyKey: c.GetHeader("Idempotency-Key")})
	respond(c, http.StatusCreated, "Delivery attempt started.", result, err)
}
func (h *Handler) recordContact(c *gin.Context) {
	var body struct {
		Method  string `json:"method"`
		Outcome string `json:"outcome"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.RecordContact(c.Request.Context(), ContactCommand{AttemptID: c.Param("attemptId"), ProviderID: authhttp.DispatchRiderID(c), Method: body.Method, Outcome: body.Outcome, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	respond(c, http.StatusCreated, "Contact attempt recorded.", result, err)
}
func (h *Handler) failAttempt(c *gin.Context) {
	var body struct {
		ReasonCode string `json:"reason_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.FailAttempt(c.Request.Context(), FailAttemptCommand{AttemptID: c.Param("attemptId"), ProviderID: authhttp.DispatchRiderID(c), ReasonCode: body.ReasonCode, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	respond(c, http.StatusOK, "Receiver-unavailable recovery options loaded.", result, err)
}
func (h *Handler) selectRecovery(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Subject == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var body SelectRecoveryCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	body.BookingID, body.CustomerID, body.IdempotencyKey = c.Param("id"), claims.Subject, c.GetHeader("Idempotency-Key")
	result, err := h.service.SelectRecovery(c.Request.Context(), body)
	respond(c, http.StatusOK, "Delivery recovery selected.", result, err)
}
func (h *Handler) reportIncident(c *gin.Context) {
	var body IncidentCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	body.TripID, body.ProviderID, body.IdempotencyKey = c.Param("id"), authhttp.DispatchRiderID(c), c.GetHeader("Idempotency-Key")
	result, err := h.service.ReportIncident(c.Request.Context(), body)
	respond(c, http.StatusAccepted, "Incident recorded and safe replacement requested.", result, err)
}
func (h *Handler) hubCheckpoint(c *gin.Context) {
	var body HubCheckpointCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	body.TripID, body.ProviderID, body.IdempotencyKey = c.Param("id"), authhttp.DispatchRiderID(c), c.GetHeader("Idempotency-Key")
	result, err := h.service.RecordHubCheckpoint(c.Request.Context(), body)
	respond(c, http.StatusOK, "Hub custody checkpoint recorded.", result, err)
}
func (h *Handler) emergencyRecovery(c *gin.Context) {
	var body EmergencyRecoveryCommand
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	caller, _ := c.Get("service_name")
	body.TripID, body.IdempotencyKey = c.Param("id"), c.GetHeader("Idempotency-Key")
	operatorRef := strings.TrimSpace(body.ActorRef)
	if operatorRef == "" {
		httpx.Abort(c, apperrors.BadRequest("Operator reference is required.", nil))
		return
	}
	body.ActorRef = strings.TrimSpace(anyString(caller)) + ":" + operatorRef
	result, err := h.service.RecordEmergencyRecovery(c.Request.Context(), body)
	respond(c, http.StatusAccepted, "Emergency recovery recorded.", result, err)
}
func anyString(value any) string { text, _ := value.(string); return text }
func respond(c *gin.Context, status int, message string, data any, err error) {
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, status, message, data)
}
