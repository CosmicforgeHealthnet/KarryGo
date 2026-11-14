// Package http serves the Section 16 route-planning routes (/routes...).
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/section16http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	PermissionDispatchOverride = "dispatch.override_warning"
	PermissionRoutesRead       = "routes.read"
	PermissionRoutesManage     = "routes.manage"
	PermissionRoutesOptimize   = "routes.optimize"
	PermissionRoutesApprove    = "routes.approve"
	PermissionRoutesPublish    = "routes.publish"
	PermissionRoutesReplan     = "routes.replan"
)

// Route is one route manifest entry.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/routes", "listRoutePlans", PermissionRoutesRead, "ROUTE_PLANNING_ENABLED", "corporate", false, false},
		{"POST", "/routes", "createRoutePlan", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", true, false},
		{"GET", "/routes/:routeId", "getRoutePlan", PermissionRoutesRead, "ROUTE_PLANNING_ENABLED", "corporate", false, false},
		{"PATCH", "/routes/:routeId", "updateRoutePlan", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", false, true},
		{"POST", "/routes/:routeId/optimize", "optimizeRoute", PermissionRoutesOptimize, "ROUTE_OPTIMIZATION_ENABLED", "corporate", true, true},
		{"POST", "/routes/:routeId/approve", "approveRoute", PermissionRoutesApprove, "ROUTE_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/routes/:routeId/publish", "publishRoute", PermissionRoutesPublish, "ROUTE_PUBLICATION_ENABLED", "corporate", true, true},
		{"POST", "/routes/:routeId/replan", "replanRoute", PermissionRoutesReplan, "ROUTE_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/routes/:routeId/cancel", "cancelRoute", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", true, true},
		{"GET", "/routes/:routeId/stops", "listRouteStops", PermissionRoutesRead, "ROUTE_PLANNING_ENABLED", "corporate", false, false},
		{"POST", "/routes/:routeId/stops", "createRouteStop", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", false, false},
		{"PATCH", "/routes/:routeId/stops/:stopId", "updateRouteStop", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", false, true},
		{"DELETE", "/routes/:routeId/stops/:stopId", "deleteRouteStop", PermissionRoutesManage, "ROUTE_PLANNING_ENABLED", "corporate", false, false},
	}
}

// Service is the use-case surface the route handlers call.
type Service interface {
	CreateRoute(ctx context.Context, t tenant.Context, in models.CreateRouteInput) (models.RoutePlan, error)
	ListRoutes(ctx context.Context, t tenant.Context, f dispatchmodels.PageFilter) ([]models.RoutePlan, string, error)
	GetRoute(ctx context.Context, t tenant.Context, id string) (models.RoutePlan, error)
	EditRoute(ctx context.Context, t tenant.Context, id string, in models.EditRouteInput) (models.RoutePlan, error)
	Optimize(ctx context.Context, t tenant.Context, id string, expected int64, key string) (models.OptimizationRequest, error)
	ApproveRoute(ctx context.Context, t tenant.Context, id string, v int64) (models.RoutePlan, error)
	PublishRoute(ctx context.Context, t tenant.Context, id string, in models.PublishRouteInput) (models.RoutePlan, []string, error)
	Replan(ctx context.Context, t tenant.Context, id string, expected int64, key, reason string) (models.RoutePlan, error)
	CancelRoute(ctx context.Context, t tenant.Context, id string, v int64) (models.RoutePlan, error)
	ListStops(ctx context.Context, t tenant.Context, routeID string) ([]models.RouteStop, error)
	AddStop(ctx context.Context, t tenant.Context, routeID string, v models.RouteStop) (models.RouteStop, error)
	EditStop(ctx context.Context, t tenant.Context, routeID, id string, v models.RouteStop) (models.RouteStop, error)
	DeleteStop(ctx context.Context, t tenant.Context, routeID, id string) error
}

type Handler struct {
	s Service
	k section16http.Kit
}

func NewHandler(s Service, a *accesscontrol.Authorizer, idem *idempotency.Store) *Handler {
	k := section16http.Kit{Idempotency: idem, Conflict: dispatchmodels.ErrConflict}
	if a != nil {
		k.Authorizer = a
	}
	return &Handler{s: s, k: k}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{"listRoutePlans": h.listRoutes, "createRoutePlan": h.createRoute, "getRoutePlan": h.getRoute, "updateRoutePlan": h.updateRoute, "optimizeRoute": h.optimize, "approveRoute": h.approveRoute, "publishRoute": h.publishRoute, "replanRoute": h.replan, "cancelRoute": h.cancelRoute, "listRouteStops": h.listStops, "createRouteStop": h.createStop, "updateRouteStop": h.updateStop, "deleteRouteStop": h.deleteStop}
	for _, r := range TenantRoutes() {
		g.Handle(r.Method, r.Path, handlers[r.OperationID])
	}
}

type versionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
	OverrideReason  string `json:"overrideReason,omitempty"`
}

type createRouteBody struct {
	BranchID                string          `json:"branchId"`
	DispatchPlanID          string          `json:"dispatchPlanId"`
	RouteReference          string          `json:"routeReference"`
	RouteType               string          `json:"routeType"`
	ProviderMembershipID    string          `json:"providerMembershipId"`
	ProviderID              string          `json:"providerId"`
	VehicleOrTruckReference string          `json:"vehicleOrTruckReference"`
	ExecutionService        string          `json:"executionService"`
	ConstraintSnapshot      json.RawMessage `json:"constraintSnapshot"`
}

func (h *Handler) createRoute(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b createRouteBody
	if !section16http.Bind(c, &b) {
		return
	}
	in := models.CreateRouteInput{BranchID: b.BranchID, DispatchPlanID: b.DispatchPlanID, RouteReference: b.RouteReference, RouteType: b.RouteType, ProviderMembershipID: b.ProviderMembershipID, ProviderID: b.ProviderID, VehicleOrTruckReference: b.VehicleOrTruckReference, ExecutionService: b.ExecutionService, ConstraintSnapshot: b.ConstraintSnapshot, IdempotencyKey: c.GetHeader("Idempotency-Key")}
	h.k.Command(c, t, "POST /routes", b, func() (int, any, error) { v, e := h.s.CreateRoute(c, t, in); return 201, v, e })
}
func (h *Handler) listRoutes(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListRoutes(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) getRoute(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	v, e := h.s.GetRoute(c, t, c.Param("routeId"))
	h.k.Write(c, v, e)
}
func (h *Handler) updateRoute(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b struct {
		ProviderMembershipID    string          `json:"providerMembershipId"`
		ProviderID              string          `json:"providerId"`
		VehicleOrTruckReference string          `json:"vehicleOrTruckReference"`
		ConstraintSnapshot      json.RawMessage `json:"constraintSnapshot"`
		ExpectedVersion         int64           `json:"expectedVersion"`
	}
	if !section16http.Bind(c, &b) {
		return
	}
	v, e := h.s.EditRoute(c, t, c.Param("routeId"), models.EditRouteInput{ProviderMembershipID: b.ProviderMembershipID, ProviderID: b.ProviderID, VehicleOrTruckReference: b.VehicleOrTruckReference, ConstraintSnapshot: b.ConstraintSnapshot, ExpectedVersion: section16http.Version(c, b.ExpectedVersion)})
	h.k.Write(c, v, e)
}
func (h *Handler) optimize(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesOptimize)
	if !ok {
		return
	}
	var b versionBody
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.Optimize(c, t, c.Param("routeId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
		return 202, v, e
	})
}
func (h *Handler) approveRoute(c *gin.Context) {
	h.routeAction(c, PermissionRoutesApprove, func(t tenant.Context, b versionBody) (any, error) {
		return h.s.ApproveRoute(c, t, c.Param("routeId"), b.ExpectedVersion)
	})
}
func (h *Handler) publishRoute(c *gin.Context) {
	h.routeAction(c, PermissionRoutesPublish, func(t tenant.Context, b versionBody) (any, error) {
		if b.OverrideReason != "" {
			if _, ok := h.k.Gate(c, PermissionDispatchOverride); !ok {
				return nil, apperrors.Forbidden("Override permission required.", nil)
			}
		}
		r, a, e := h.s.PublishRoute(c, t, c.Param("routeId"), models.PublishRouteInput{ExpectedVersion: b.ExpectedVersion, OverrideReason: b.OverrideReason, IdempotencyKey: c.GetHeader("Idempotency-Key")})
		return gin.H{"route": r, "assignmentIds": a}, e
	})
}
func (h *Handler) replan(c *gin.Context) {
	h.routeAction(c, PermissionRoutesReplan, func(t tenant.Context, b versionBody) (any, error) {
		return h.s.Replan(c, t, c.Param("routeId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"), b.Reason)
	})
}
func (h *Handler) cancelRoute(c *gin.Context) {
	h.routeAction(c, PermissionRoutesManage, func(t tenant.Context, b versionBody) (any, error) {
		return h.s.CancelRoute(c, t, c.Param("routeId"), b.ExpectedVersion)
	})
}
func (h *Handler) routeAction(c *gin.Context, p string, run func(tenant.Context, versionBody) (any, error)) {
	t, ok := h.k.Gate(c, p)
	if !ok {
		return
	}
	var b versionBody
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) { v, e := run(t, b); return 200, v, e })
}
func (h *Handler) listStops(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	v, e := h.s.ListStops(c, t, c.Param("routeId"))
	h.k.Write(c, v, e)
}
func (h *Handler) createStop(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b models.RouteStop
	if !section16http.Bind(c, &b) {
		return
	}
	v, e := h.s.AddStop(c, t, c.Param("routeId"), b)
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.Created(c, v)
}
func (h *Handler) updateStop(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b models.RouteStop
	if !section16http.Bind(c, &b) {
		return
	}
	b.Version = section16http.Version(c, b.Version)
	v, e := h.s.EditStop(c, t, c.Param("routeId"), c.Param("stopId"), b)
	h.k.Write(c, v, e)
}
func (h *Handler) deleteStop(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	e := h.s.DeleteStop(c, t, c.Param("routeId"), c.Param("stopId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	c.Status(http.StatusNoContent)
}

// parseFilter reads the shared list filter. Malformed, negative or oversized
// limit/offset values, a malformed cursor and a cursor sent with an offset
// are a validation error rather than a SQL failure.
func parseFilter(c *gin.Context) (dispatchmodels.PageFilter, error) {
	page, e := section16http.ParsePage(c)
	if e != nil {
		return dispatchmodels.PageFilter{}, e
	}
	return dispatchmodels.PageFilter{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Service: strings.ToUpper(c.Query("service")), ProviderID: c.Query("providerId"), VehicleReference: c.Query("vehicleOrTruckReference"), AlertType: strings.ToUpper(c.Query("alertType")), Limit: page.Limit, Offset: page.Offset, After: page.After, AfterID: page.AfterID}, nil
}
