// Package http serves the Section 18 maintenance routes: preventive
// schedules and work orders, including authorized return to service.
package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// UseCases is the slice of the maintenance use cases the handler calls.
type UseCases interface {
	ListSchedulesPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.Schedule, *models.Schedule, error)
	GetSchedule(ctx context.Context, t tenant.Context, id string) (models.Schedule, error)
	SaveSchedule(ctx context.Context, t tenant.Context, id string, expected int64, v models.Schedule) (models.Schedule, error)
	ListWorkOrders(ctx context.Context, t tenant.Context) ([]models.WorkOrder, error)
	GetWorkOrder(ctx context.Context, t tenant.Context, id string) (models.WorkOrder, error)
	CreateWorkOrder(ctx context.Context, t tenant.Context, v models.WorkOrder) (models.WorkOrder, error)
	UpdateWorkOrder(ctx context.Context, t tenant.Context, id, action string, in models.WorkAction) (models.WorkOrder, error)
	ReturnToService(ctx context.Context, t tenant.Context, id string, in models.WorkAction) (models.WorkOrder, error)
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/maintenance/schedules", "listMaintenanceSchedules", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/maintenance/schedules", "createMaintenanceSchedule", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/maintenance/schedules/:scheduleId", "getMaintenanceSchedule", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"PATCH", "/maintenance/schedules/:scheduleId", "updateMaintenanceSchedule", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", false, true},
		{"GET", "/maintenance/work-orders", "listWorkOrders", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/maintenance/work-orders", "createWorkOrder", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/maintenance/work-orders/:workOrderId", "getWorkOrder", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"PATCH", "/maintenance/work-orders/:workOrderId", "updateWorkOrder", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", false, true}, {"POST", "/maintenance/work-orders/:workOrderId/approve", "approveWorkOrder", "maintenance.approve", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/maintenance/work-orders/:workOrderId/complete", "completeWorkOrder", "maintenance.complete", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/maintenance/work-orders/:workOrderId/return-to-service", "returnToService", "maintenance.return_to_service", "MAINTENANCE_ENABLED", "corporate", true, true}}
}

func (h *Handler) listSchedules(c *gin.Context, t tenant.Context) (any, error) {
	p, e := pagination.Parse(c.Query("limit"), "")
	if e != nil {
		return nil, e
	}
	limit := 0
	if c.Query("limit") != "" {
		limit = p.Limit
	}
	after, afterID, e := keyset(c)
	if e != nil {
		return nil, e
	}
	items, last, e := h.s.ListSchedulesPage(c, t, models.Page{Limit: limit, After: after, AfterID: afterID})
	if e != nil {
		return nil, e
	}
	next := ""
	if last != nil {
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	return listPage{data: items, next: next}, nil
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	id := ""
	for _, p := range []string{"scheduleId", "workOrderId"} {
		if c.Param(p) != "" {
			id = c.Param(p)
		}
	}
	h.serve(c, t, r, func() (any, error) {
		switch r.OperationID {
		case "listMaintenanceSchedules":
			return h.listSchedules(c, t)
		case "getMaintenanceSchedule":
			return h.s.GetSchedule(c, t, id)
		case "createMaintenanceSchedule":
			var b models.Schedule
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveSchedule(c, t, "", 0, b)
		case "updateMaintenanceSchedule":
			var b models.Schedule
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveSchedule(c, t, id, ver(c, b.Version), b)
		case "listWorkOrders":
			return h.s.ListWorkOrders(c, t)
		case "getWorkOrder":
			return h.s.GetWorkOrder(c, t, id)
		case "createWorkOrder":
			var b models.WorkOrder
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CreateWorkOrder(c, t, b)
		case "updateWorkOrder", "approveWorkOrder", "completeWorkOrder", "returnToService":
			var b models.WorkAction
			if !bind(c, &b) {
				return nil, errBound
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			if r.OperationID == "returnToService" {
				return h.s.ReturnToService(c, t, id, b)
			}
			a := ""
			if r.OperationID == "approveWorkOrder" {
				a = "APPROVE"
			}
			if r.OperationID == "completeWorkOrder" {
				a = "COMPLETE"
			}
			return h.s.UpdateWorkOrder(c, t, id, a, b)
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	})
}
