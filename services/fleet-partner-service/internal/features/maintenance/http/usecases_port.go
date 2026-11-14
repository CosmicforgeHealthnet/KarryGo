package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
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
