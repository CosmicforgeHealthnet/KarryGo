package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
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
