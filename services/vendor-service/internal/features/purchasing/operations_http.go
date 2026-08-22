package purchasing

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 15D operational HTTP surface: supplier performance, purchasing
// dashboards, outstanding deliveries and operational alerts.
//
// These are reads over the whole business, so they authorize on their own
// permissions rather than on vendor.purchasing.read: a dashboard aggregating
// every branch is a wider disclosure than reading one purchase order. The
// resolved scope is passed to the service, which intersects any requested
// branch filter with it, so a branch-scoped operator can never widen the view.

const (
	PermPurchasingPerformanceRead = "vendor.purchasing.performance.read"
	PermPurchasingDashboardRead   = "vendor.purchasing.dashboard.read"
)

// operationsScopeResolver is the purchasing Service's own scope resolver.
// It is declared here rather than reusing ScopeResolver because the Service
// resolves from a full Actor, not a bare staff id, and 15D must not change a
// 15A signature to suit itself.
type operationsScopeResolver interface {
	ResolveScope(ctx context.Context, actor Actor, permission string) (Scope, error)
}

// OperationsHandler serves the 15D operational surface.
type OperationsHandler struct {
	service *GoodsReceivingService
	scopes  operationsScopeResolver
}

func NewOperationsHandler(
	service *GoodsReceivingService,
	scopes operationsScopeResolver,
) *OperationsHandler {
	return &OperationsHandler{
		service: service,
		scopes:  scopes,
	}
}

// authorized resolves the caller and their scope for one permission.
func (h *OperationsHandler) authorized(
	c *gin.Context,
	permission string,
) (Actor, Scope, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized(
			"Authentication is required.", nil))
		return Actor{}, Scope{}, false
	}

	actor := Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}

	if h.scopes == nil {
		httpx.RespondError(c, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil))
		return Actor{}, Scope{}, false
	}

	scope, err := h.scopes.ResolveScope(c, actor, permission)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}

	if !scope.HasAny() {
		httpx.RespondError(c, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil))
		return Actor{}, Scope{}, false
	}

	return actor, scope, true
}

// branchFilter reads a repeated or comma-separated branch_id filter.
func branchFilter(c *gin.Context) []string {
	values := c.QueryArray("branch_id")
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func optionalTime(c *gin.Context, key string) (*time.Time, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperrors.BadRequest(
			key+" must be an RFC3339 timestamp.", nil)
	}
	return &parsed, nil
}

func (h *OperationsHandler) supplierPerformance(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermPurchasingPerformanceRead)
	if !ok {
		return
	}

	from, err := optionalTime(c, "from")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	to, err := optionalTime(c, "to")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	out, err := h.service.SupplierPerformance(
		c, actor, scope, c.Param("supplierId"),
		SupplierPerformanceQuery{
			BranchIDs: branchFilter(c),
			From:      from,
			To:        to,
		})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	respondPurchasingOK(c, gin.H{"performance": out})
}

func (h *OperationsHandler) dashboard(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermPurchasingDashboardRead)
	if !ok {
		return
	}

	out, err := h.service.PurchasingDashboard(c, actor, scope, branchFilter(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	respondPurchasingOK(c, gin.H{"dashboard": out})
}

func (h *OperationsHandler) outstandingDeliveries(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermPurchasingDashboardRead)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	out, err := h.service.ListOutstandingDeliveries(
		c, actor, scope,
		normalizeDeliveryWindow(c.Query("window")),
		branchFilter(c), limit)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	respondPurchasingOK(c, gin.H{"deliveries": out})
}

func (h *OperationsHandler) listAlerts(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermPurchasingDashboardRead)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	out, err := h.service.ListPurchasingAlerts(
		c, actor, scope, c.Query("status"), branchFilter(c), limit)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	respondPurchasingOK(c, gin.H{"alerts": out})
}

// alertTransition acknowledges or resolves an alert. Acting on an alert is a
// management action rather than a read, so it takes the manage permission.
func (h *OperationsHandler) alertTransition(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, scope, ok := h.authorized(c, PermPurchasingBaseManage)
		if !ok {
			return
		}

		out, err := h.service.TransitionPurchasingAlert(
			c, actor, scope, c.Param("alertId"), target)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		respondPurchasingOK(c, gin.H{"alert": out})
	}
}

func respondPurchasingOK(c *gin.Context, data gin.H) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// RegisterOperationsRoutes mounts the Section 15D operational surface under the
// existing /purchasing base, alongside 15A/15B/15C.
func RegisterOperationsRoutes(
	root *gin.RouterGroup,
	h *OperationsHandler,
	enabled bool,
) {
	if root == nil || h == nil || !enabled {
		return
	}

	group := root.Group("/purchasing")

	group.GET("/dashboard", h.dashboard)
	group.GET("/outstanding-deliveries", h.outstandingDeliveries)

	group.GET("/alerts", h.listAlerts)
	group.POST("/alerts/:alertId/acknowledge",
		h.alertTransition(AlertStatusAcknowledged))
	group.POST("/alerts/:alertId/resolve",
		h.alertTransition(AlertStatusResolved))

	group.GET("/suppliers/:supplierId/performance", h.supplierPerformance)
}
