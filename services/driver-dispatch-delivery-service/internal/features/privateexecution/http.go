package privateexecution

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func Register(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier,
	db *pgxpool.Pool, allowed []string) {
	group.Use(authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), allowed)...)
	group.POST("", handler.create)
	group.GET("/by-operation/:operationId", handler.read)
	group.GET("/by-trip/:tripId", handler.readByTrip)
	group.POST("/:operationId/cancellations", handler.cancel)
	group.GET("/:operationId/cancellation", handler.readCancellation)
}

func success(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"success": true, "data": value})
}

func failure(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "PRIVATE_EXECUTION_UNAVAILABLE"
	switch {
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "PRIVATE_EXECUTION_NOT_FOUND"
	case errors.Is(err, ErrConflict):
		status, code = http.StatusConflict, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, ErrProviderIneligible):
		status, code = http.StatusUnprocessableEntity, "PROVIDER_INELIGIBLE"
	case errors.Is(err, ErrVehicleIneligible):
		status, code = http.StatusUnprocessableEntity, "VEHICLE_INELIGIBLE"
	case errors.Is(err, ErrOutcomeUnknown):
		status, code = http.StatusAccepted, "OUTCOME_UNKNOWN"
	case errors.Is(err, ErrDisabled):
		status, code = http.StatusServiceUnavailable, "PRIVATE_EXECUTION_DISABLED"
	default:
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be") {
			status, code = http.StatusBadRequest, "INVALID_REQUEST"
		}
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}

func (h *Handler) create(c *gin.Context) {
	var body CreateRequest
	if c.ShouldBindJSON(&body) != nil {
		failure(c, errors.New("required request body is invalid"))
		return
	}
	body.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if body.CorrelationID == "" {
		body.CorrelationID = c.GetHeader("X-Correlation-ID")
	}
	view, err := h.service.Create(c.Request.Context(), body)
	if err != nil {
		failure(c, err)
		return
	}
	success(c, http.StatusCreated, view)
}

func (h *Handler) read(c *gin.Context) {
	view, err := h.service.Read(c.Request.Context(), c.Param("operationId"))
	if err != nil {
		failure(c, err)
		return
	}
	success(c, http.StatusOK, view)
}

func (h *Handler) readByTrip(c *gin.Context) {
	record, err := h.service.repo.getByTrip(c.Request.Context(), c.Param("tripId"))
	if err != nil {
		failure(c, err)
		return
	}
	view, err := h.service.reconcile(c.Request.Context(), record)
	if err != nil {
		failure(c, err)
		return
	}
	success(c, http.StatusOK, view)
}

func (h *Handler) cancel(c *gin.Context) {
	var body CancelRequest
	if c.ShouldBindJSON(&body) != nil {
		failure(c, errors.New("required request body is invalid"))
		return
	}
	body.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	view, err := h.service.Cancel(c.Request.Context(), c.Param("operationId"), body)
	if err != nil {
		failure(c, err)
		return
	}
	success(c, http.StatusOK, view)
}

func (h *Handler) readCancellation(c *gin.Context) {
	view, err := h.service.ReadCancellation(c.Request.Context(), c.Param("operationId"))
	if err != nil {
		failure(c, err)
		return
	}
	success(c, http.StatusOK, view)
}
