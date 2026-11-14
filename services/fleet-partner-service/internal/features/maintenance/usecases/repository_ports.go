package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
)

// Repository is the persistence maintenance depends on.
type Repository interface {
	InsertSchedule(ctx context.Context, q database.Querier, v models.Schedule) (models.Schedule, error)
	UpdateSchedule(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Schedule) (models.Schedule, error)
	GetSchedule(ctx context.Context, org, id string) (models.Schedule, error)
	ListSchedules(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.Schedule, error)
	ListDueSchedules(ctx context.Context, org string) ([]models.DueSchedule, error)
	InsertWorkOrder(ctx context.Context, q database.Querier, v models.WorkOrder) (models.WorkOrder, error)
	InsertSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error)
	EnsureSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error)
	SystemWorkOrderID(ctx context.Context, q database.Querier, org, sourceType, sourceRef string) (string, error)
	GetWorkOrder(ctx context.Context, org, id string) (models.WorkOrder, error)
	ListWorkOrders(ctx context.Context, org string) ([]models.WorkOrder, error)
	UpdateWorkOrder(ctx context.Context, q database.Querier, org, id string, expected int64, in models.WorkAction, to, approver string, evidence []byte) (models.WorkOrder, error)
	InsertWorkOrderHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error
	ApproveReturnToService(ctx context.Context, q database.Querier, org, id string, expected int64) (models.WorkOrder, error)
	InsertReturnToServiceDecision(ctx context.Context, q database.Querier, org, workOrderID, assetID, reason string, evidence []byte, actor string) error
}
