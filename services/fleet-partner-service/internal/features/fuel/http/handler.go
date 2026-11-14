// Package http serves the Section 18 fuel and energy record routes.
package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// UseCases is the slice of the fuel use cases the handler calls.
type UseCases interface {
	ListFuelPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.FuelRecord, *models.FuelRecord, error)
	GetFuel(ctx context.Context, t tenant.Context, id string) (models.FuelRecord, error)
	CreateFuel(ctx context.Context, t tenant.Context, v models.FuelRecord) (models.FuelRecord, error)
	FuelAction(ctx context.Context, t tenant.Context, id, to, reason string, expected int64) (models.FuelRecord, error)
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/fuel-records", "listFuelRecords", "fuel.read", "FUEL_RECORDS_ENABLED", "corporate", false, false}, {"POST", "/fuel-records", "createFuelRecord", "fuel.manage", "FUEL_RECORDS_ENABLED", "corporate", true, false}, {"GET", "/fuel-records/:fuelRecordId", "getFuelRecord", "fuel.read", "FUEL_RECORDS_ENABLED", "corporate", false, false}, {"POST", "/fuel-records/:fuelRecordId/approve", "approveFuelRecord", "fuel.approve", "FUEL_RECORDS_ENABLED", "corporate", true, true}, {"POST", "/fuel-records/:fuelRecordId/reject", "rejectFuelRecord", "fuel.approve", "FUEL_RECORDS_ENABLED", "corporate", true, true}}
}

func (h *Handler) listFuel(c *gin.Context, t tenant.Context) (any, error) {
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
	items, last, e := h.s.ListFuelPage(c, t, models.Page{Limit: limit, After: after, AfterID: afterID})
	if e != nil {
		return nil, e
	}
	next := ""
	if last != nil {
		next = cursor.EncodeTime(last.RecordedAt, last.ID)
	}
	return listPage{data: items, next: next}, nil
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	id := c.Param("fuelRecordId")
	h.serve(c, t, r, func() (any, error) {
		switch r.OperationID {
		case "listFuelRecords":
			return h.listFuel(c, t)
		case "getFuelRecord":
			return h.s.GetFuel(c, t, id)
		case "createFuelRecord":
			var b models.FuelRecord
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CreateFuel(c, t, b)
		case "approveFuelRecord", "rejectFuelRecord":
			var b vb
			if !bind(c, &b) {
				return nil, errBound
			}
			to := "APPROVED"
			if r.OperationID == "rejectFuelRecord" {
				to = "REJECTED"
			}
			return h.s.FuelAction(c, t, id, to, b.Reason, ver(c, b.ExpectedVersion))
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	})
}
