package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	AddCargoItem(ctx context.Context, t tenant.Context, shipmentID string, in usecases.AddCargoItemInput) (models.CargoItem, error)
	Cancel(ctx context.Context, t tenant.Context, shipmentID string, expectedVersion int64, reasonCode string) (models.Shipment, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Shipment, error)
	Get(ctx context.Context, t tenant.Context, shipmentID string) (models.Shipment, error)
	List(ctx context.Context, t tenant.Context, in usecases.ListInput) ([]models.Shipment, string, error)
	ListCargoItems(ctx context.Context, t tenant.Context, shipmentID string) ([]models.CargoItem, error)
	MarkReady(ctx context.Context, t tenant.Context, shipmentID string, expectedVersion int64) (models.Shipment, error)
	Update(ctx context.Context, t tenant.Context, shipmentID string, expectedVersion int64, in usecases.UpdateInput) (models.Shipment, error)
}
