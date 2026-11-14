package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AppendHistory(ctx context.Context, q database.Querier, h models.OrderHistory) error
	Cancel(ctx context.Context, q database.Querier, organizationID string, orderID string, expectedVersion int64, reasonCode string, actorID string) (models.Order, error)
	Confirm(ctx context.Context, q database.Querier, organizationID string, orderID string, expectedVersion int64, catalogSnapshot models.JSONMap, confirmationSnapshot models.JSONMap, actorID string) (models.Order, error)
	Create(ctx context.Context, q database.Querier, o models.Order) (models.Order, error)
	CreateStop(ctx context.Context, q database.Querier, stop models.OrderStop) (models.OrderStop, error)
	Get(ctx context.Context, q database.Querier, organizationID string, orderID string) (models.Order, error)
	List(ctx context.Context, organizationID string, filter repositories.Filter) ([]models.Order, error)
	ListHistory(ctx context.Context, organizationID string, orderID string) ([]models.OrderHistory, error)
	ListStops(ctx context.Context, q database.Querier, organizationID string, orderID string) ([]models.OrderStop, error)
	Lock(ctx context.Context, q database.Querier, organizationID string, orderID string) (models.Order, error)
	MarkAwaitingApproval(ctx context.Context, q database.Querier, organizationID string, orderID string, expectedVersion int64, approvalRequestID string, actorID string) (models.Order, error)
	ReturnToDraft(ctx context.Context, q database.Querier, organizationID string, orderID string, expectedVersion int64, actorID string) (models.Order, error)
	UpdateFields(ctx context.Context, q database.Querier, organizationID string, orderID string, expectedVersion int64, o models.Order, returnToDraft bool) (models.Order, error)
}
