// Package http serves the Section 16 dispatch routes: dispatch plans,
// candidates, the control tower, geofences, operational alerts and rescue
// cases. Route-plan routes live in features/routes/http and tracking routes
// in features/tracking/http.
package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/section16http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	PermissionDispatchRead     = "dispatch.read"
	PermissionDispatchPlan     = "dispatch.plan"
	PermissionDispatchApprove  = "dispatch.approve"
	PermissionDispatchPublish  = "dispatch.publish"
	PermissionControlTowerRead = "control_tower.read"
	PermissionGeofencesRead    = "geofences.read"
	PermissionGeofencesManage  = "geofences.manage"
	PermissionAlertsRead       = "alerts.read"
	PermissionAlertsAck        = "alerts.acknowledge"
	PermissionAlertsResolve    = "alerts.resolve"
	PermissionAlertsReplay     = "alerts.replay"
	PermissionRescueRead       = "rescue.read"
	PermissionRescueManage     = "rescue.manage"
)

// Route is one route manifest entry.
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
		{"GET", "/control-tower", "getControlTower", PermissionControlTowerRead, "CONTROL_TOWER_ENABLED", "corporate", false, false},
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

// Service is the use-case surface the dispatch handlers call.
type Service interface {
	Board(ctx context.Context, t tenant.Context, f models.PageFilter) (models.Board, error)
	ControlTower(ctx context.Context, t tenant.Context, f models.PageFilter) (models.Board, error)
	Unassigned(ctx context.Context, t tenant.Context, f models.PageFilter) ([]map[string]any, error)
	GenerateCandidates(ctx context.Context, t tenant.Context, branchID, sourceType, sourceID string) ([]models.Candidate, error)
	CreatePlan(ctx context.Context, t tenant.Context, in models.CreatePlanInput) (models.DispatchPlan, error)
	ListPlans(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.DispatchPlan, string, error)
	GetPlan(ctx context.Context, t tenant.Context, id string) (models.DispatchPlan, error)
	EditPlan(ctx context.Context, t tenant.Context, id string, in models.EditPlanInput) (models.DispatchPlan, error)
	ApprovePlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error)
	PublishPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error)
	CancelPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error)
	ListGeofences(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.Geofence, string, error)
	CreateGeofence(ctx context.Context, t tenant.Context, in models.CreateGeofenceInput) (models.Geofence, error)
	GetGeofence(ctx context.Context, t tenant.Context, id string) (models.Geofence, error)
	UpdateGeofence(ctx context.Context, t tenant.Context, id string, in models.CreateGeofenceInput, expected int64) (models.Geofence, error)
	SetGeofenceActive(ctx context.Context, t tenant.Context, id, status string, expected int64) (models.Geofence, error)
	ListGeofenceEvents(ctx context.Context, t tenant.Context, f models.GeofenceEventFilter) ([]models.GeofenceEvent, string, error)
	ListAlerts(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.OperationalAlert, string, error)
	GetAlert(ctx context.Context, t tenant.Context, id string) (models.OperationalAlert, error)
	AcknowledgeAlert(ctx context.Context, t tenant.Context, id string, expected int64) (models.OperationalAlert, error)
	ResolveAlert(ctx context.Context, t tenant.Context, id string, expected int64, code, reason string) (models.OperationalAlert, error)
	ReplayJob(ctx context.Context, t tenant.Context, id string) error
	ListRescueCases(ctx context.Context, t tenant.Context, f models.RescueFilter) ([]models.RescueCase, string, error)
	GetRescueCase(ctx context.Context, t tenant.Context, id string) (models.RescueCase, error)
	OpenRescueCase(ctx context.Context, t tenant.Context, in models.OpenRescueCaseInput) (models.RescueCase, error)
	AssignRescueReplacement(ctx context.Context, t tenant.Context, id string, in models.RescueReplacementInput) (models.RescueCase, error)
	ResolveRescueCase(ctx context.Context, t tenant.Context, id string, in models.ResolveRescueCaseInput) (models.RescueCase, error)
	CancelRescueCase(ctx context.Context, t tenant.Context, id string, in models.CancelRescueCaseInput) (models.RescueCase, error)
}

type Handler struct {
	s Service
	k section16http.Kit
}

func NewHandler(s Service, a *accesscontrol.Authorizer, idem *idempotency.Store) *Handler {
	k := section16http.Kit{Idempotency: idem, Conflict: models.ErrConflict}
	if a != nil {
		k.Authorizer = a
	}
	return &Handler{s: s, k: k}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{"getDispatchBoard": h.board, "getUnassignedWork": h.unassigned, "getAssignmentCandidates": h.candidates, "createDispatchPlan": h.createPlan, "listDispatchPlans": h.listPlans, "getDispatchPlan": h.getPlan, "updateDispatchPlan": h.updatePlan, "approveDispatchPlan": h.approvePlan, "publishDispatchPlan": h.publishPlan, "cancelDispatchPlan": h.cancelPlan, "getControlTower": h.controlTower, "listGeofences": h.listGeofences, "createGeofence": h.createGeofence, "getGeofence": h.getGeofence, "updateGeofence": h.updateGeofence, "activateGeofence": h.activateGeofence, "deactivateGeofence": h.deactivateGeofence, "listGeofenceEvents": h.listGeofenceEvents, "listOperationalAlerts": h.listAlerts, "getOperationalAlert": h.getAlert, "acknowledgeOperationalAlert": h.ackAlert, "resolveOperationalAlert": h.resolveAlert, "replaySection16Job": h.replay, "listRescueCases": h.listRescueCases, "openRescueCase": h.openRescueCase, "getRescueCase": h.getRescueCase, "assignRescueReplacement": h.assignRescueReplacement, "resolveRescueCase": h.resolveRescueCase, "cancelRescueCase": h.cancelRescueCase}
	for _, r := range TenantRoutes() {
		g.Handle(r.Method, r.Path, handlers[r.OperationID])
	}
}

// parseFilter reads the shared list filter. Malformed, negative or oversized
// limit/offset values, a malformed cursor and a cursor sent with an offset
// are a validation error rather than a SQL failure.
func parseFilter(c *gin.Context) (models.PageFilter, error) {
	page, e := section16http.ParsePage(c)
	if e != nil {
		return models.PageFilter{}, e
	}
	return models.PageFilter{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Service: strings.ToUpper(c.Query("service")), ProviderID: c.Query("providerId"), VehicleReference: c.Query("vehicleOrTruckReference"), AlertType: strings.ToUpper(c.Query("alertType")), Limit: page.Limit, Offset: page.Offset, After: page.After, AfterID: page.AfterID}, nil
}

type versionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
	OverrideReason  string `json:"overrideReason,omitempty"`
}

func (h *Handler) board(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.Board(c, t, f)
	h.k.Write(c, v, e)
}
func (h *Handler) controlTower(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionControlTowerRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.ControlTower(c, t, f)
	h.k.Write(c, v, e)
}
func (h *Handler) unassigned(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, e := h.s.Unassigned(c, t, f)
	h.k.Write(c, v, e)
}
func (h *Handler) candidates(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchRead)
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
	h.k.Write(c, v, e)
}
func (h *Handler) createPlan(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchPlan)
	if !ok {
		return
	}
	var b models.CreatePlanInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.k.Command(c, t, "POST /dispatch/plans", b, func() (int, any, error) { v, e := h.s.CreatePlan(c, t, b); return http.StatusCreated, v, e })
}
func (h *Handler) listPlans(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListPlans(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) getPlan(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchRead)
	if !ok {
		return
	}
	v, e := h.s.GetPlan(c, t, c.Param("planId"))
	h.k.Write(c, v, e)
}
func (h *Handler) updatePlan(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionDispatchPlan)
	if !ok {
		return
	}
	var b models.EditPlanInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	v, e := h.s.EditPlan(c, t, c.Param("planId"), b)
	h.k.Write(c, v, e)
}
func (h *Handler) planAction(c *gin.Context, p, to string, run func(tenant.Context, versionBody) (models.DispatchPlan, error)) {
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
func (h *Handler) approvePlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchApprove, models.PlanApproved, func(t tenant.Context, b versionBody) (models.DispatchPlan, error) {
		return h.s.ApprovePlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
}
func (h *Handler) publishPlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchPublish, models.PlanPublished, func(t tenant.Context, b versionBody) (models.DispatchPlan, error) {
		return h.s.PublishPlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
}
func (h *Handler) cancelPlan(c *gin.Context) {
	h.planAction(c, PermissionDispatchPlan, models.PlanCancelled, func(t tenant.Context, b versionBody) (models.DispatchPlan, error) {
		return h.s.CancelPlan(c, t, c.Param("planId"), b.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	})
}
func (h *Handler) listGeofences(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListGeofences(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) createGeofence(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b models.CreateGeofenceInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.k.Command(c, t, "POST /geofences", b, func() (int, any, error) { v, e := h.s.CreateGeofence(c, t, b); return 201, v, e })
}
func (h *Handler) getGeofence(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	v, e := h.s.GetGeofence(c, t, c.Param("geofenceId"))
	h.k.Write(c, v, e)
}
func (h *Handler) updateGeofence(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b struct {
		models.CreateGeofenceInput
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !section16http.Bind(c, &b) {
		return
	}
	v, e := h.s.UpdateGeofence(c, t, c.Param("geofenceId"), b.CreateGeofenceInput, section16http.Version(c, b.ExpectedVersion))
	h.k.Write(c, v, e)
}
func (h *Handler) activateGeofence(c *gin.Context)   { h.geofenceState(c, "ACTIVE") }
func (h *Handler) deactivateGeofence(c *gin.Context) { h.geofenceState(c, "INACTIVE") }
func (h *Handler) geofenceState(c *gin.Context, status string) {
	t, ok := h.k.Gate(c, PermissionGeofencesManage)
	if !ok {
		return
	}
	var b versionBody
	if !section16http.Bind(c, &b) {
		return
	}
	v, e := h.s.SetGeofenceActive(c, t, c.Param("geofenceId"), status, section16http.Version(c, b.ExpectedVersion))
	h.k.Write(c, v, e)
}
func (h *Handler) listAlerts(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionAlertsRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListAlerts(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) getAlert(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionAlertsRead)
	if !ok {
		return
	}
	v, e := h.s.GetAlert(c, t, c.Param("alertId"))
	h.k.Write(c, v, e)
}
func (h *Handler) ackAlert(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionAlertsAck)
	if !ok {
		return
	}
	var b versionBody
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.AcknowledgeAlert(c, t, c.Param("alertId"), b.ExpectedVersion)
		return 200, v, e
	})
}
func (h *Handler) resolveAlert(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionAlertsResolve)
	if !ok {
		return
	}
	var b struct {
		ExpectedVersion  int64  `json:"expectedVersion"`
		ResolutionCode   string `json:"resolutionCode"`
		ResolutionReason string `json:"resolutionReason"`
	}
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.ResolveAlert(c, t, c.Param("alertId"), b.ExpectedVersion, b.ResolutionCode, b.ResolutionReason)
		return 200, v, e
	})
}
func (h *Handler) replay(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionAlertsReplay)
	if !ok {
		return
	}
	b := struct{}{}
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		e := h.s.ReplayJob(c, t, c.Param("jobId"))
		return 202, gin.H{"jobId": c.Param("jobId"), "status": "PENDING"}, e
	})
}
func (h *Handler) listGeofenceEvents(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionGeofencesRead)
	if !ok {
		return
	}
	f := models.GeofenceEventFilter{GeofenceID: c.Query("geofenceId"), CanonicalTripReference: c.Query("canonicalTripReference"), EventRule: strings.ToUpper(c.Query("eventRule")), BranchID: c.Query("branchId")}
	page, e := section16http.ParsePage(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	f.Limit, f.Offset, f.After, f.AfterID = page.Limit, page.Offset, page.After, page.AfterID
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
	v, next, e := h.s.ListGeofenceEvents(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) listRescueCases(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueRead)
	if !ok {
		return
	}
	page, e := section16http.ParsePage(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListRescueCases(c, t, models.RescueFilter{BranchID: c.Query("branchId"), Status: c.Query("status"), TriggerType: c.Query("triggerType"), Limit: page.Limit, Offset: page.Offset, After: page.After, AfterID: page.AfterID})
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) getRescueCase(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueRead)
	if !ok {
		return
	}
	v, e := h.s.GetRescueCase(c, t, c.Param("rescueCaseId"))
	h.k.Write(c, v, e)
}
func (h *Handler) openRescueCase(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b models.OpenRescueCaseInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.IdempotencyKey = c.GetHeader("Idempotency-Key")
	h.k.Command(c, t, "POST /rescue-cases", b, func() (int, any, error) { v, e := h.s.OpenRescueCase(c, t, b); return 201, v, e })
}
func (h *Handler) assignRescueReplacement(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b models.RescueReplacementInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.AssignRescueReplacement(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
func (h *Handler) resolveRescueCase(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b models.ResolveRescueCaseInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.ResolveRescueCase(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
func (h *Handler) cancelRescueCase(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionRescueManage)
	if !ok {
		return
	}
	var b models.CancelRescueCaseInput
	if !section16http.Bind(c, &b) {
		return
	}
	b.ExpectedVersion = section16http.Version(c, b.ExpectedVersion)
	h.k.Command(c, t, c.Request.Method+" "+c.FullPath(), b, func() (int, any, error) {
		v, e := h.s.CancelRescueCase(c, t, c.Param("rescueCaseId"), b)
		return 200, v, e
	})
}
