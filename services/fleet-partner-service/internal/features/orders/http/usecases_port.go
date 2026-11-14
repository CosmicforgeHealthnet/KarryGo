package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	AddStop(ctx context.Context, t tenant.Context, orderID string, in usecases.AddStopInput) (models.OrderStop, error)
	Approve(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64) (models.Order, error)
	Cancel(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64, reasonCode string) (models.Order, error)
	Confirm(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64) (models.Order, error)
	ConvertConversionIntent(ctx context.Context, t tenant.Context, acceptedVersionID string, in usecases.ConvertConversionIntentInput) (models.Order, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Order, error)
	Get(ctx context.Context, t tenant.Context, orderID string) (models.Order, error)
	History(ctx context.Context, t tenant.Context, orderID string) ([]models.OrderHistory, error)
	List(ctx context.Context, t tenant.Context, in usecases.ListInput) ([]models.Order, string, error)
	ListStops(ctx context.Context, t tenant.Context, orderID string) ([]models.OrderStop, error)
	Reject(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64, reasonCode string) (models.Order, error)
	Submit(ctx context.Context, t tenant.Context, orderID string, expectedVersion int64, idempotencyKey string) (models.Order, error)
	Update(ctx context.Context, t tenant.Context, orderID string, in usecases.UpdateInput) (models.Order, error)
}
