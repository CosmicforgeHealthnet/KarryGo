// Package http serves the Section 18 inspection routes: checklist templates
// and inspections.
package http

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// UseCases is the slice of the inspection use cases the handler calls.
type UseCases interface {
	ListTemplates(ctx context.Context, t tenant.Context) ([]models.Template, error)
	GetTemplate(ctx context.Context, t tenant.Context, id string) (models.Template, error)
	SaveTemplate(ctx context.Context, t tenant.Context, id string, expected int64, v models.Template) (models.Template, error)
	ListInspectionsPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.Inspection, *models.Inspection, error)
	GetInspection(ctx context.Context, t tenant.Context, id string) (models.Inspection, error)
	CreateInspection(ctx context.Context, t tenant.Context, v models.Inspection) (models.Inspection, error)
	CompleteInspection(ctx context.Context, t tenant.Context, id string, in models.CompleteInspectionInput) (models.Inspection, error)
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/inspection-templates", "listInspectionTemplates", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspection-templates", "createInspectionTemplate", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", true, false}, {"GET", "/inspection-templates/:templateId", "getInspectionTemplate", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"PATCH", "/inspection-templates/:templateId", "updateInspectionTemplate", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", false, true},
		{"GET", "/inspections", "listInspections", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspections", "createInspection", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", true, false}, {"GET", "/inspections/:inspectionId", "getInspection", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspections/:inspectionId/complete", "completeInspection", "inspections.complete", "INSPECTIONS_ENABLED", "corporate", true, true}}
}

func (h *Handler) listInspections(c *gin.Context, t tenant.Context) (any, error) {
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
	items, last, e := h.s.ListInspectionsPage(c, t, models.Page{Status: strings.ToUpper(c.Query("status")), Limit: limit, After: after, AfterID: afterID})
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
	for _, p := range []string{"templateId", "inspectionId"} {
		if c.Param(p) != "" {
			id = c.Param(p)
		}
	}
	h.serve(c, t, r, func() (any, error) {
		switch r.OperationID {
		case "listInspectionTemplates":
			return h.s.ListTemplates(c, t)
		case "getInspectionTemplate":
			return h.s.GetTemplate(c, t, id)
		case "createInspectionTemplate":
			var b models.Template
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveTemplate(c, t, "", 0, b)
		case "updateInspectionTemplate":
			var b models.Template
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveTemplate(c, t, id, ver(c, b.Version), b)
		case "listInspections":
			return h.listInspections(c, t)
		case "getInspection":
			return h.s.GetInspection(c, t, id)
		case "createInspection":
			var b models.Inspection
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CreateInspection(c, t, b)
		case "completeInspection":
			var b models.CompleteInspectionInput
			if !bind(c, &b) {
				return nil, errBound
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			return h.s.CompleteInspection(c, t, id, b)
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	})
}
