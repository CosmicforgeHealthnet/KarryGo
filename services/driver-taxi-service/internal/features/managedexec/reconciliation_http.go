package managedexec

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// ReconciliationStore is the slice of the outbox store this surface needs.
type ReconciliationStore interface {
	ListReconciliation(ctx context.Context, target, state string, limit int) ([]outbox.ReconciliationRow, error)
	Requeue(ctx context.Context, id string) error
}

// ReconciliationHandler exposes completed-work delivery state to operators
// (Section 8 TAX-017).
//
// Repairing a stuck completed-work delivery must never mean editing the
// database by hand, so this surface offers both the inspection read and the
// requeue action. The requeue reuses the row's original payload and
// idempotency key, so it is a REDELIVERY -- safe even when Fleet may already
// hold the record.
type ReconciliationHandler struct {
	store   ReconciliationStore
	allowed map[string]bool
}

// NewReconciliationHandler builds the operator surface.
func NewReconciliationHandler(store ReconciliationStore, allowedCallers []string) *ReconciliationHandler {
	allowed := map[string]bool{}
	for _, name := range allowedCallers {
		if name != "" {
			allowed[name] = true
		}
	}
	return &ReconciliationHandler{store: store, allowed: allowed}
}

// RegisterReconciliationRoutes mounts the operator surface. A nil verifier
// means no internal caller is trusted, so nothing is mounted.
func RegisterReconciliationRoutes(group *gin.RouterGroup, handler *ReconciliationHandler, verifier *serviceauth.Verifier) {
	if handler == nil || handler.store == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/fleet/completed-work")
	routes.Use(serviceauth.Middleware(verifier))
	routes.Use(handler.requireAllowedCaller)

	routes.GET("", handler.List)
	routes.POST("/:id/requeue", handler.Requeue)
}

// requireAllowedCaller renders its own refusal (see the execution handler's
// own comment for why a security boundary must not rely on a downstream error
// middleware being mounted).
func (h *ReconciliationHandler) requireAllowedCaller(c *gin.Context) {
	if len(h.allowed) > 0 && !h.allowed[c.GetString("service_name")] {
		httpx.RespondError(c, apperrors.Forbidden("Service is not allowed to inspect completed-work delivery.", nil))
		c.Abort()
		return
	}
	c.Next()
}

// List returns completed-work delivery rows, optionally filtered by
// reconciliation state (PENDING, DELIVERED, RETRY_SCHEDULED, FAILED_TERMINAL,
// RECONCILIATION_REQUIRED).
func (h *ReconciliationHandler) List(c *gin.Context) {
	state := c.Query("state")
	switch state {
	case "", outbox.ReconPending, outbox.ReconDelivered, outbox.ReconRetryScheduled,
		outbox.ReconFailedTerminal, outbox.ReconReconciliationRequired:
	default:
		httpx.Abort(c, apperrors.BadRequest("Unknown reconciliation state.", nil))
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.store.ListReconciliation(c.Request.Context(), outbox.TargetFleet, state, limit)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Completed-work delivery state could not be read.", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows}})
}

// Requeue returns a dead-lettered completed-work row to the delivery queue.
func (h *ReconciliationHandler) Requeue(c *gin.Context) {
	if err := h.store.Requeue(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Abort(c, apperrors.Conflict("This completed-work row cannot be requeued.", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"requeued": true}})
}
