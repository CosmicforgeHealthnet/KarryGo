package completedwork

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Handler exposes completed-work delivery state to operators (HLG-020).
//
// Repairing a stuck completed-work delivery must never mean editing the
// database by hand, so this surface offers both the inspection read and the
// requeue action. The requeue reuses the row's original payload and
// idempotency key, so it is a REDELIVERY -- safe even when Fleet may already
// hold the record.
type Handler struct {
	store   *Store
	allowed map[string]bool
}

// NewHandler builds the operator surface.
func NewHandler(store *Store, allowedCallers []string) *Handler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		for _, part := range strings.Split(caller, ",") {
			if name := strings.TrimSpace(part); name != "" {
				allowed[name] = true
			}
		}
	}
	return &Handler{store: store, allowed: allowed}
}

// RegisterRoutes mounts the operator surface. A nil verifier means no
// internal caller is trusted, so nothing is mounted rather than a half-open
// surface that answers 401 and confirms it exists.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier) {
	if handler == nil || handler.store == nil || handler.store.DB == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/fleet/completed-work")
	routes.Use(serviceauth.Middleware(verifier))
	routes.Use(handler.requireAllowedCaller)

	routes.GET("", handler.List)
	routes.POST("/:id/requeue", handler.Requeue)
}

// requireAllowedCaller restricts inspection to allowlisted services.
//
// It renders its own failure with httpx.RespondError rather than httpx.Abort:
// this is a security boundary, and httpx.Abort only attaches the error for a
// downstream error middleware to render. A group mounted without that
// middleware would otherwise answer 200 instead of refusing.
func (h *Handler) requireAllowedCaller(c *gin.Context) {
	if len(h.allowed) > 0 && !h.allowed[c.GetString("service_name")] {
		httpx.RespondError(c, apperrors.Forbidden("Service is not allowed to inspect completed-work delivery.", nil))
		c.Abort()
		return
	}
	c.Next()
}

// List returns completed-work delivery rows, optionally filtered by
// reconciliation state.
func (h *Handler) List(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	switch state {
	case "", ReconPending, ReconDelivered, ReconRetryScheduled,
		ReconFailedTerminal, ReconReconciliationRequired:
	default:
		httpx.RespondError(c, apperrors.BadRequest("Unknown reconciliation state.", nil))
		c.Abort()
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.store.List(c.Request.Context(), state, limit)
	if err != nil {
		httpx.RespondError(c, apperrors.Unavailable("Completed-work delivery state could not be read.", err))
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows}})
}

// Requeue returns a dead-lettered completed-work row to the delivery queue.
func (h *Handler) Requeue(c *gin.Context) {
	if err := h.store.Requeue(c.Request.Context(), c.Param("id")); err != nil {
		httpx.RespondError(c, apperrors.Conflict("This completed-work row cannot be requeued.", err))
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"requeued": true}})
}
