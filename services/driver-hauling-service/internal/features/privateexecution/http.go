package privateexecution

import (
	"errors"
	"strings"

	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func Register(root *gin.RouterGroup, h *Handler, boundary *fleetauthority.Handler, verifier *serviceauth.Verifier, allowed []string) {
	g := root.Group("/internal/private-executions")
	g.Use(fleetauthority.Boundary(boundary, verifier, allowed)...)
	g.POST("", h.create)
	g.GET("/by-operation/:operationId", h.read)
	g.GET("/by-trip/:tripId", h.readTrip)
	g.POST("/:operationId/cancellations", h.cancel)
	g.GET("/:operationId/cancellation", h.readCancellation)
}
func ok(c *gin.Context, status int, v any) { c.JSON(status, gin.H{"success": true, "data": v}) }
func fail(c *gin.Context, err error) {
	status, code := 500, "PRIVATE_EXECUTION_UNAVAILABLE"
	switch {
	case errors.Is(err, ErrNotFound):
		status, code = 404, "PRIVATE_EXECUTION_NOT_FOUND"
	case errors.Is(err, ErrConflict):
		status, code = 409, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, ErrIneligible):
		status, code = 422, "PROVIDER_OR_TRUCK_INELIGIBLE"
	case errors.Is(err, ErrNotCancellable):
		status, code = 409, "NOT_CANCELLABLE"
	case errors.Is(err, ErrDisabled):
		status, code = 503, "PRIVATE_EXECUTION_DISABLED"
	default:
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be") || strings.Contains(err.Error(), "does not match") {
			status, code = 400, "INVALID_REQUEST"
		}
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}
func (h *Handler) create(c *gin.Context) {
	var b CreateRequest
	if c.ShouldBindJSON(&b) != nil {
		fail(c, errors.New("required request body is invalid"))
		return
	}
	b.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if b.CorrelationID == "" {
		b.CorrelationID = c.GetHeader("X-Correlation-ID")
	}
	v, e := h.service.Create(c, b)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, v)
}
func (h *Handler) read(c *gin.Context) {
	v, e := h.service.Read(c, c.Param("operationId"))
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, v)
}
func (h *Handler) readTrip(c *gin.Context) {
	v, e := h.service.ReadByTrip(c, c.Param("tripId"))
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, v)
}
func (h *Handler) cancel(c *gin.Context) {
	var b CancelRequest
	if c.ShouldBindJSON(&b) != nil {
		fail(c, errors.New("required request body is invalid"))
		return
	}
	b.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	v, e := h.service.Cancel(c, c.Param("operationId"), b)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, v)
}
func (h *Handler) readCancellation(c *gin.Context) {
	v, e := h.service.ReadCancellation(c, c.Param("operationId"))
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, v)
}
