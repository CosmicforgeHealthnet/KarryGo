package privateexecution

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Handler exposes Taxi's half of Fleet's private execution contract.
type Handler struct {
	service *Service
	allowed map[string]bool
}

// NewHandler builds the handler over an allowlist of permitted callers.
func NewHandler(service *Service, allowedCallers []string) *Handler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		for _, part := range strings.Split(caller, ",") {
			if name := strings.TrimSpace(part); name != "" {
				allowed[name] = true
			}
		}
	}
	return &Handler{service: service, allowed: allowed}
}

// Register mounts the surface.
//
// A nil verifier mounts nothing at all. A half-open internal surface that
// answers 401 still confirms it exists and still has to be reasoned about; a
// route that was never registered does not.
func Register(root *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier) {
	if root == nil || handler == nil || handler.service == nil || verifier == nil {
		return
	}
	group := root.Group("/internal/private-executions")
	group.Use(serviceauth.Middleware(verifier), handler.requireAllowedCaller)

	group.POST("", handler.create)
	group.GET("/by-operation/:operationId", handler.read)
	group.GET("/by-booking/:bookingId", handler.readBooking)
	group.POST("/:operationId/cancellations", handler.cancel)
	group.GET("/:operationId/cancellation", handler.readCancellation)
}

func (h *Handler) requireAllowedCaller(c *gin.Context) {
	if len(h.allowed) > 0 && !h.allowed[c.GetString("service_name")] {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false,
			"error": gin.H{"code": "CALLER_NOT_ALLOWED"}})
		return
	}
	c.Next()
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

// fail maps a domain error onto a status Fleet's worker can act on.
//
// The distinction that matters is retryable versus not. A conflict or an
// ineligibility will never succeed on retry, so Fleet must dead-letter rather
// than loop; a disabled surface is a deployment state Fleet should keep
// probing. Collapsing them into 500 would make Fleet retry a permanent refusal
// until it exhausted its attempts, hours later.
func fail(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "PRIVATE_EXECUTION_UNAVAILABLE"
	switch {
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "PRIVATE_EXECUTION_NOT_FOUND"
	case errors.Is(err, ErrConflict):
		status, code = http.StatusConflict, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, ErrIneligible):
		status, code = http.StatusUnprocessableEntity, "PROVIDER_OR_VEHICLE_INELIGIBLE"
	case errors.Is(err, ErrNotCancellable):
		status, code = http.StatusConflict, "NOT_CANCELLABLE"
	case errors.Is(err, ErrUnserviceable):
		status, code = http.StatusUnprocessableEntity, "PICKUP_NOT_SERVICEABLE"
	case errors.Is(err, ErrDisabled):
		status, code = http.StatusServiceUnavailable, "PRIVATE_EXECUTION_DISABLED"
	default:
		// Validation failures are plain errors from validate(); they are the
		// caller's to fix and must not be reported as this service faulting.
		message := err.Error()
		if strings.Contains(message, "required") || strings.Contains(message, "must ") ||
			strings.Contains(message, "does not match") {
			status, code = http.StatusBadRequest, "INVALID_REQUEST"
		}
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}

func (h *Handler) create(c *gin.Context) {
	var body CreateRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, errors.New("required request body is invalid"))
		return
	}
	body.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if body.CorrelationID == "" {
		body.CorrelationID = strings.TrimSpace(c.GetHeader("X-Correlation-ID"))
	}
	view, err := h.service.Create(c, body)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, view)
}

func (h *Handler) read(c *gin.Context) {
	view, err := h.service.Read(c, c.Param("operationId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, view)
}

func (h *Handler) readBooking(c *gin.Context) {
	view, err := h.service.ReadByBooking(c, c.Param("bookingId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, view)
}

func (h *Handler) cancel(c *gin.Context) {
	var body CancelRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, errors.New("required request body is invalid"))
		return
	}
	body.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	view, err := h.service.Cancel(c, c.Param("operationId"), body)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, view)
}

func (h *Handler) readCancellation(c *gin.Context) {
	view, err := h.service.ReadCancellation(c, c.Param("operationId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, view)
}
