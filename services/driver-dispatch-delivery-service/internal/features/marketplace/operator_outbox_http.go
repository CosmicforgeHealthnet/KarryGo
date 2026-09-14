package marketplace

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// OperatorOutboxHandler exposes delivery state and repair to operators.
type OperatorOutboxHandler struct {
	store   *OperatorOutboxStore
	allowed map[string]bool
}

// NewOperatorOutboxHandler builds the surface.
func NewOperatorOutboxHandler(store *OperatorOutboxStore, allowedCallers []string) *OperatorOutboxHandler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		for _, part := range strings.Split(caller, ",") {
			if name := strings.TrimSpace(part); name != "" {
				allowed[name] = true
			}
		}
	}
	return &OperatorOutboxHandler{store: store, allowed: allowed}
}

// RegisterOperatorOutboxRoutes mounts the surface on the internal group.
//
// A nil verifier mounts nothing rather than a half-open surface that answers
// 401 and thereby confirms the route exists.
func RegisterOperatorOutboxRoutes(
	group *gin.RouterGroup, handler *OperatorOutboxHandler, verifier *serviceauth.Verifier,
) {
	if handler == nil || handler.store == nil || handler.store.DB == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/dispatch/v1/operations/outbox")
	routes.Use(serviceauth.Middleware(verifier))
	routes.Use(handler.requireAllowedCaller)

	routes.GET("", handler.List)
	routes.POST("/:id/requeue", handler.Requeue)
}

// requireAllowedCaller renders its own refusal rather than deferring to an
// error middleware. This is a security boundary, and httpx.Abort only
// attaches the error for a downstream renderer: a group mounted without that
// middleware would answer 200 instead of refusing.
func (h *OperatorOutboxHandler) requireAllowedCaller(c *gin.Context) {
	if len(h.allowed) > 0 && !h.allowed[c.GetString("service_name")] {
		httpx.RespondError(c, apperrors.Forbidden(
			"Service is not allowed to inspect event delivery.", nil))
		c.Abort()
		return
	}
	c.Next()
}

// List returns queued events, optionally filtered by delivery state.
func (h *OperatorOutboxHandler) List(c *gin.Context) {
	status := c.Query("status")
	if !ValidOutboxStatus(status) {
		httpx.RespondError(c, apperrors.BadRequest("Unknown delivery status.", nil))
		c.Abort()
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.store.List(c.Request.Context(), status, c.Query("target"), limit)
	if err != nil {
		httpx.RespondError(c, apperrors.Unavailable("Event delivery state could not be read.", err))
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows}})
}

// Requeue returns one stuck event to the delivery queue.
func (h *OperatorOutboxHandler) Requeue(c *gin.Context) {
	err := h.store.Requeue(c.Request.Context(), c.Param("id"), c.GetString("service_name"))
	switch {
	case errors.Is(err, ErrNotRequeueable):
		httpx.RespondError(c, apperrors.Conflict(
			"This event cannot be requeued. It was already delivered, or it does not exist.", nil))
		c.Abort()
	case err != nil:
		httpx.RespondError(c, apperrors.Unavailable("This event could not be requeued.", err))
		c.Abort()
	default:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"requeued": true}})
	}
}
