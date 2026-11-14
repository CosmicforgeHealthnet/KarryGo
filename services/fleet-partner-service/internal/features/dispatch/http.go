package dispatch

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	PermissionDispatchRead     = "dispatch.read"
	PermissionDispatchPlan     = "dispatch.plan"
	PermissionDispatchApprove  = "dispatch.approve"
	PermissionDispatchPublish  = "dispatch.publish"
	PermissionDispatchOverride = "dispatch.override_warning"
	PermissionRoutesRead       = "routes.read"
	PermissionRoutesManage     = "routes.manage"
	PermissionRoutesOptimize   = "routes.optimize"
	PermissionRoutesApprove    = "routes.approve"
	PermissionRoutesPublish    = "routes.publish"
	PermissionRoutesReplan     = "routes.replan"
	PermissionControlTowerRead = "control_tower.read"
	PermissionTrackingRead     = "tracking.read"
	PermissionGeofencesRead    = "geofences.read"
	PermissionGeofencesManage  = "geofences.manage"
	PermissionAlertsRead       = "alerts.read"
	PermissionAlertsAck        = "alerts.acknowledge"
	PermissionAlertsResolve    = "alerts.resolve"
	PermissionAlertsReplay     = "alerts.replay"
	PermissionRescueRead       = "rescue.read"
	PermissionRescueManage     = "rescue.manage"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/dispatch/board", "getDispatchBoard", PermissionDispatchRead, "CONTROL_TOWER_ENABLED", "corporate", false, false},
		{"GET", "/dispatch/unassigned", "getUnassignedWork", PermissionDispatchRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"GET", "/dispatch/candidates", "getAssignmentCandidates", PermissionDispatchRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"POST", "/dispatch/plans", "createDispatchPlan", PermissionDispatchPlan, "DISPATCH_PLANNING_ENABLED", "corporate", true, false},
		{"GET", "/dispatch/plans", "listDispatchPlans", PermissionDispatchRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"GET", "/dispatch/plans/:planId", "getDispatchPlan", PermissionDispatchRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"PATCH", "/dispatch/plans/:planId", "updateDispatchPlan", PermissionDispatchPlan, "DISPATCH_PLANNING_ENABLED", "corporate", false, true},
		{"POST", "/dispatch/plans/:planId/approve", "approveDispatchPlan", PermissionDispatchApprove, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/dispatch/plans/:planId/publish", "publishDispatchPlan", PermissionDispatchPublish, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/dispatch/plans/:planId/cancel", "cancelDispatchPlan", PermissionDispatchPlan, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
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
		{"GET", "/control-tower", "getControlTower", PermissionControlTowerRead, "CONTROL_TOWER_ENABLED", "corporate", false, false},
		{"GET", "/tracking", "listTrackingProjections", PermissionTrackingRead, "TRACKING_PROJECTIONS_ENABLED", "corporate", false, false},
		{"GET", "/tracking/:canonicalTripReference", "getTrackingProjection", PermissionTrackingRead, "TRACKING_PROJECTIONS_ENABLED", "corporate", false, false},
		{"GET", "/geofences", "listGeofences", PermissionGeofencesRead, "GEOFENCES_ENABLED", "corporate", false, false},
		{"POST", "/geofences", "createGeofence", PermissionGeofencesManage, "GEOFENCES_ENABLED", "corporate", true, false},
		{"GET", "/geofences/:geofenceId", "getGeofence", PermissionGeofencesRead, "GEOFENCES_ENABLED", "corporate", false, false},
		{"PATCH", "/geofences/:geofenceId", "updateGeofence", PermissionGeofencesManage, "GEOFENCES_ENABLED", "corporate", false, true},
		{"POST", "/geofences/:geofenceId/activate", "activateGeofence", PermissionGeofencesManage, "GEOFENCES_ENABLED", "corporate", false, true},
		{"POST", "/geofences/:geofenceId/deactivate", "deactivateGeofence", PermissionGeofencesManage, "GEOFENCES_ENABLED", "corporate", false, true},
		{"GET", "/geofence-events", "listGeofenceEvents", PermissionGeofencesRead, "GEOFENCES_ENABLED", "corporate", false, false},
		{"GET", "/alerts", "listOperationalAlerts", PermissionAlertsRead, "OPERATIONAL_ALERTS_ENABLED", "corporate", false, false},
		{"GET", "/alerts/:alertId", "getOperationalAlert", PermissionAlertsRead, "OPERATIONAL_ALERTS_ENABLED", "corporate", false, false},
		{"POST", "/alerts/:alertId/acknowledge", "acknowledgeOperationalAlert", PermissionAlertsAck, "OPERATIONAL_ALERTS_ENABLED", "corporate", true, true},
		{"POST", "/alerts/:alertId/resolve", "resolveOperationalAlert", PermissionAlertsResolve, "OPERATIONAL_ALERTS_ENABLED", "corporate", true, true},
		{"POST", "/alerts/jobs/:jobId/replay", "replaySection16Job", PermissionAlertsReplay, "OPERATIONAL_ALERTS_ENABLED", "corporate", true, false},
		{"GET", "/rescue-cases", "listRescueCases", PermissionRescueRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"POST", "/rescue-cases", "openRescueCase", PermissionRescueManage, "DISPATCH_PLANNING_ENABLED", "corporate", true, false},
		{"GET", "/rescue-cases/:rescueCaseId", "getRescueCase", PermissionRescueRead, "DISPATCH_PLANNING_ENABLED", "corporate", false, false},
		{"POST", "/rescue-cases/:rescueCaseId/replacement", "assignRescueReplacement", PermissionRescueManage, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/rescue-cases/:rescueCaseId/resolve", "resolveRescueCase", PermissionRescueManage, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
		{"POST", "/rescue-cases/:rescueCaseId/cancel", "cancelRescueCase", PermissionRescueManage, "DISPATCH_PLANNING_ENABLED", "corporate", true, true},
	}
}
func InternalRoutes() []Route {
	return []Route{{"POST", "/tracking/events", "ingestTrackingEvent", "tracking.events.ingest", "TRACKING_PROJECTIONS_ENABLED", "internal-hmac-v2", true, false}}
}

type Handler struct {
	s    *Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s *Service, a *accesscontrol.Authorizer, idem *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: idem}
}
func (h *Handler) TenantRoutes() []Route   { return TenantRoutes() }
func (h *Handler) InternalRoutes() []Route { return InternalRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{"getDispatchBoard": h.board, "getUnassignedWork": h.unassigned, "getAssignmentCandidates": h.candidates, "createDispatchPlan": h.createPlan, "listDispatchPlans": h.listPlans, "getDispatchPlan": h.getPlan, "updateDispatchPlan": h.updatePlan, "approveDispatchPlan": h.approvePlan, "publishDispatchPlan": h.publishPlan, "cancelDispatchPlan": h.cancelPlan, "listRoutePlans": h.listRoutes, "createRoutePlan": h.createRoute, "getRoutePlan": h.getRoute, "updateRoutePlan": h.updateRoute, "optimizeRoute": h.optimize, "approveRoute": h.approveRoute, "publishRoute": h.publishRoute, "replanRoute": h.replan, "cancelRoute": h.cancelRoute, "listRouteStops": h.listStops, "createRouteStop": h.createStop, "updateRouteStop": h.updateStop, "deleteRouteStop": h.deleteStop, "getControlTower": h.controlTower, "listTrackingProjections": h.listTracking, "getTrackingProjection": h.getTracking, "listGeofences": h.listGeofences, "createGeofence": h.createGeofence, "getGeofence": h.getGeofence, "updateGeofence": h.updateGeofence, "activateGeofence": h.activateGeofence, "deactivateGeofence": h.deactivateGeofence, "listGeofenceEvents": h.listGeofenceEvents, "listOperationalAlerts": h.listAlerts, "getOperationalAlert": h.getAlert, "acknowledgeOperationalAlert": h.ackAlert, "resolveOperationalAlert": h.resolveAlert, "replaySection16Job": h.replay, "listRescueCases": h.listRescueCases, "openRescueCase": h.openRescueCase, "getRescueCase": h.getRescueCase, "assignRescueReplacement": h.assignRescueReplacement, "resolveRescueCase": h.resolveRescueCase, "cancelRescueCase": h.cancelRescueCase}
	for _, r := range TenantRoutes() {
		g.Handle(r.Method, r.Path, handlers[r.OperationID])
	}
}
func (h *Handler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/tracking/events", h.ingestTracking)
}
func (h *Handler) gate(c *gin.Context, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	if h.a == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return t, false
	}
	rv := int64(1)
	if raw, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = raw.(int64)
	}
	// Narrowed to the branches where the permission is granted; a
	// caller-chosen ?branchId never widens it.
	scoped, e := h.a.RequireScoped(c, t, rv, permission)
	if e != nil {
		respond.Error(c, e)
		return t, false
	}
	middleware.SetTenant(c, scoped)
	return scoped, true
}

// parseFilter reads the shared list filter. Malformed, negative or oversized
// limit/offset values are a validation error rather than a SQL failure.
func parseFilter(c *gin.Context) (PageFilter, error) {
	page, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		return PageFilter{}, e
	}
	return PageFilter{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Service: strings.ToUpper(c.Query("service")), ProviderID: c.Query("providerId"), VehicleReference: c.Query("vehicleOrTruckReference"), AlertType: strings.ToUpper(c.Query("alertType")), Limit: page.Limit, Offset: page.Offset}, nil
}
func version(c *gin.Context, body int64) int64 {
	if body > 0 {
		return body
	}
	v, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return v
}
func bind(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return false
	}
	return true
}
func (h *Handler) command(c *gin.Context, t tenant.Context, endpoint string, body any, run func() (int, any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, _ := json.Marshal(body)
	fp := idempotency.Fingerprint(c.Request.Method, endpoint, raw)
	rec, e := h.idem.Begin(c, t.OrganizationID, endpoint, key, fp)
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	status, data, e := run()
	var payload []byte
	if e != nil {
		if errors.Is(e, ErrConflict) {
			e = apperrors.Conflict("Expected version or idempotency fingerprint conflicts with current state.", e)
		}
		appErr := apperrors.From(e)
		details := appErr.Fields
		if details == nil {
			details = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: appErr.Code, Message: appErr.Message, Details: details, RequestID: httpx.GetRequestID(c)}})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(gin.H{"data": data, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if e = h.idem.Complete(c, t.OrganizationID, endpoint, key, status, payload); e != nil {
		respond.Error(c, e)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

func (h *Handler) board(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.Board(c, t, f)
	write(c, v, e)
}
func (h *Handler) controlTower(c *gin.Context) {
	t, ok := h.gate(c, PermissionControlTowerRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ControlTower(c, t, f)
	write(c, v, e)
}
func (h *Handler) unassigned(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.Unassigned(c, t, f)
	write(c, v, e)
}
func (h *Handler) candidates(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	sourceType, sourceID := "", ""
	for _, x := range []struct{ k, v string }{{"ORDER", c.Query("orderId")}, {"SHIPMENT", c.Query("shipmentId")}, {"ROUTE", c.Query("routeId")}, {"ROUTE_STOP", c.Query("routeStopId")}} {
		if x.v != "" {
			sourceType, sourceID = x.k, x.v
			break
		}
	}
	if sourceID == "" {
		respond.Error(c, apperrors.BadRequest("one source identifier is required.", nil))
		return
	}
	v, e := h.s.GenerateCandidates(c, t, c.Query("branchId"), sourceType, sourceID)
	write(c, v, e)
}
func (h *Handler) createPlan(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchPlan)
	if !ok {
		return
	}
	var b CreatePlanInput
	if !bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.command(c, t, "POST /dispatch/plans", b, func() (int, any, error) { v, e := h.s.CreatePlan(c, t, b); return http.StatusCreated, v, e })
}
func (h *Handler) listPlans(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListPlans(c, t, f)
	write(c, v, e)
}
func (h *Handler) getPlan(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	v, e := h.s.GetPlan(c, t, c.Param("planId"))
	write(c, v, e)
}
func (h *Handler) updatePlan(c *gin.Context) {
	t, ok := h.gate(c, PermissionDispatchPlan)
	if !ok {
		return
	}
	var b EditPlanInput
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	v, e := h.s.EditPlan(c, t, c.Param("planId"), b)
	write(c, v, e)
}

type versionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
	OverrideReason  string `json:"overrideReason,omitempty"`
}

func (h *Handler) planAction(c *gin.Context, p, to string, run func(tenant.Context, versionBody) (DispatchPlan, error)) {
	t, ok := h.gate(c, p)
	if !ok {
		return
	}
	var b versionBody
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) { v, e := run(t, b); return 200, v, e })
}
func (h *Handler) approvePlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchApprove, PlanApproved, func(t tenant.Context, b versionBody) (DispatchPlan, error) {
		return h.s.ApprovePlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
}
func (h *Handler) publishPlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchPublish, PlanPublished, func(t tenant.Context, b versionBody) (DispatchPlan, error) {
		return h.s.PublishPlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
}
func (h *Handler) cancelPlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchPlan, PlanCancelled, func(t tenant.Context, b versionBody) (DispatchPlan, error) {
		return h.s.CancelPlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
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
	t, ok := h.gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b createRouteBody
	if !bind(c, &b) {
		return
	}
	in := CreateRouteInput{b.BranchID, b.DispatchPlanID, b.RouteReference, b.RouteType, b.ProviderMembershipID, b.ProviderID, b.VehicleOrTruckReference, b.ExecutionService, b.ConstraintSnapshot, c.GetHeader("Idempotency-Key")}
	h.command(c, t, "POST /routes", b, func() (int, any, error) { v, e := h.s.CreateRoute(c, t, in); return 201, v, e })
}
func (h *Handler) listRoutes(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListRoutes(c, t, f)
	write(c, v, e)
}
func (h *Handler) getRoute(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	v, e := h.s.GetRoute(c, t, c.Param("routeId"))
	write(c, v, e)
}
func (h *Handler) updateRoute(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesManage)
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
	if !bind(c, &b) {
		return
	}
	v, e := h.s.EditRoute(c, t, c.Param("routeId"), EditRouteInput{b.ProviderMembershipID, b.ProviderID, b.VehicleOrTruckReference, b.ConstraintSnapshot, version(c, b.ExpectedVersion)})
	write(c, v, e)
}
func (h *Handler) optimize(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesOptimize)
	if !ok {
		return
	}
	var b versionBody
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
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
			if _, ok := h.gate(c, PermissionDispatchOverride); !ok {
				return nil, apperrors.Forbidden("Override permission required.", nil)
			}
		}
		r, a, e := h.s.PublishRoute(c, t, c.Param("routeId"), PublishRouteInput{b.ExpectedVersion, b.OverrideReason, c.GetHeader("Idempotency-Key")})
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
	t, ok := h.gate(c, p)
	if !ok {
		return
	}
	var b versionBody
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) { v, e := run(t, b); return 200, v, e })
}

func (h *Handler) listStops(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesRead)
	if !ok {
		return
	}
	v, e := h.s.ListStops(c, t, c.Param("routeId"))
	write(c, v, e)
}
func (h *Handler) createStop(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b RouteStop
	if !bind(c, &b) {
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
	t, ok := h.gate(c, PermissionRoutesManage)
	if !ok {
		return
	}
	var b RouteStop
	if !bind(c, &b) {
		return
	}
	b.Version = version(c, b.Version)
	v, e := h.s.EditStop(c, t, c.Param("routeId"), c.Param("stopId"), b)
	write(c, v, e)
}
func (h *Handler) deleteStop(c *gin.Context) {
	t, ok := h.gate(c, PermissionRoutesManage)
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
func (h *Handler) listTracking(c *gin.Context) {
	t, ok := h.gate(c, PermissionTrackingRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListTracking(c, t, f)
	write(c, v, e)
}
func (h *Handler) getTracking(c *gin.Context) {
	t, ok := h.gate(c, PermissionTrackingRead)
	if !ok {
		return
	}
	v, e := h.s.GetTracking(c, t, c.Param("canonicalTripReference"))
	write(c, v, e)
}
func (h *Handler) listGeofences(c *gin.Context) {
	t, ok := h.gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListGeofences(c, t, f)
	write(c, v, e)
}
func (h *Handler) createGeofence(c *gin.Context) {
	t, ok := h.gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b CreateGeofenceInput
	if !bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.command(c, t, "POST /geofences", b, func() (int, any, error) { v, e := h.s.CreateGeofence(c, t, b); return 201, v, e })
}
func (h *Handler) getGeofence(c *gin.Context) {
	t, ok := h.gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	v, e := h.s.GetGeofence(c, t, c.Param("geofenceId"))
	write(c, v, e)
}
func (h *Handler) updateGeofence(c *gin.Context) {
	t, ok := h.gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b struct {
		CreateGeofenceInput
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !bind(c, &b) {
		return
	}
	v, e := h.s.UpdateGeofence(c, t, c.Param("geofenceId"), b.CreateGeofenceInput, version(c, b.ExpectedVersion))
	write(c, v, e)
}
func (h *Handler) activateGeofence(c *gin.Context)   { h.geofenceState(c, "ACTIVE") }
func (h *Handler) deactivateGeofence(c *gin.Context) { h.geofenceState(c, "INACTIVE") }
func (h *Handler) geofenceState(c *gin.Context, status string) {
	t, ok := h.gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b versionBody
	if !bind(c, &b) {
		return
	}
	v, e := h.s.SetGeofenceActive(c, t, c.Param("geofenceId"), status, version(c, b.ExpectedVersion))
	write(c, v, e)
}
func (h *Handler) listAlerts(c *gin.Context) {
	t, ok := h.gate(c, PermissionAlertsRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListAlerts(c, t, f)
	write(c, v, e)
}
func (h *Handler) getAlert(c *gin.Context) {
	t, ok := h.gate(c, PermissionAlertsRead)
	if !ok {
		return
	}
	v, e := h.s.GetAlert(c, t, c.Param("alertId"))
	write(c, v, e)
}
func (h *Handler) ackAlert(c *gin.Context) {
	t, ok := h.gate(c, PermissionAlertsAck)
	if !ok {
		return
	}
	var b versionBody
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.AcknowledgeAlert(c, t, c.Param("alertId"), b.ExpectedVersion)
		return 200, v, e
	})
}
func (h *Handler) resolveAlert(c *gin.Context) {
	t, ok := h.gate(c, PermissionAlertsResolve)
	if !ok {
		return
	}
	var b struct {
		ExpectedVersion  int64  `json:"expectedVersion"`
		ResolutionCode   string `json:"resolutionCode"`
		ResolutionReason string `json:"resolutionReason"`
	}
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.ResolveAlert(c, t, c.Param("alertId"), b.ExpectedVersion, b.ResolutionCode, b.ResolutionReason)
		return 200, v, e
	})
}
func (h *Handler) replay(c *gin.Context) {
	t, ok := h.gate(c, PermissionAlertsReplay)
	if !ok {
		return
	}
	b := struct{}{}
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		e := h.s.ReplayJob(c, t, c.Param("jobId"))
		return 202, gin.H{"jobId": c.Param("jobId"), "status": "PENDING"}, e
	})
}
func (h *Handler) ingestTracking(c *gin.Context) {
	if strings.TrimSpace(c.GetHeader("Idempotency-Key")) == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	caller, _ := c.Get("service_name")
	source, _ := caller.(string)
	if source != "driver-dispatch-delivery-service" && source != "driver-hauling-service" {
		respond.Error(c, apperrors.Forbidden("Authoritative tracking caller required.", nil))
		return
	}
	var b TrackingEvent
	if !bind(c, &b) {
		return
	}
	b.SourceService = source
	v, d, e := h.s.IngestTracking(c, b)
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, gin.H{"projection": v, "disposition": d})
}
func (h *Handler) listGeofenceEvents(c *gin.Context) {
	t, ok := h.gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	f := GeofenceEventFilter{GeofenceID: c.Query("geofenceId"), CanonicalTripReference: c.Query("canonicalTripReference"), EventRule: strings.ToUpper(c.Query("eventRule")), BranchID: c.Query("branchId")}
	page, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	f.Limit, f.Offset = page.Limit, page.Offset
	for name, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := c.Query(name); raw != "" {
			parsed, e := time.Parse(time.RFC3339, raw)
			if e != nil {
				respond.Error(c, apperrors.BadRequest(name+" must be an RFC 3339 timestamp.", e))
				return
			}
			*target = &parsed
		}
	}
	v, e := h.s.ListGeofenceEvents(c, t, f)
	write(c, v, e)
}
func (h *Handler) listRescueCases(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueRead)
	if !ok {
		return
	}
	page, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ListRescueCases(c, t, RescueFilter{BranchID: c.Query("branchId"), Status: c.Query("status"), TriggerType: c.Query("triggerType"), Limit: page.Limit, Offset: page.Offset})
	write(c, v, e)
}
func (h *Handler) getRescueCase(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueRead)
	if !ok {
		return
	}
	v, e := h.s.GetRescueCase(c, t, c.Param("rescueCaseId"))
	write(c, v, e)
}
func (h *Handler) openRescueCase(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b OpenRescueCaseInput
	if !bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.command(c, t, "POST /rescue-cases", b, func() (int, any, error) { v, e := h.s.OpenRescueCase(c, t, b); return 201, v, e })
}
func (h *Handler) assignRescueReplacement(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b RescueReplacementInput
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.AssignRescueReplacement(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
func (h *Handler) resolveRescueCase(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b ResolveRescueCaseInput
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.ResolveRescueCase(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
func (h *Handler) cancelRescueCase(c *gin.Context) {
	t, ok := h.gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b CancelRescueCaseInput
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = version(c, b.ExpectedVersion)
	h.command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.CancelRescueCase(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
func write(c *gin.Context, v any, e error) {
	if e != nil {
		if errors.Is(e, ErrConflict) {
			e = apperrors.Conflict("Expected version or idempotency fingerprint conflicts with current state.", e)
		}
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}

var _ = time.Second
