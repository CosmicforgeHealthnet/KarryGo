package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AppendHistory(ctx context.Context, q database.Querier, h models.ShipmentHistory) error
	Create(ctx context.Context, q database.Querier, s models.Shipment) (models.Shipment, error)
	CreateCargoItem(ctx context.Context, q database.Querier, c models.CargoItem) (models.CargoItem, error)
	Get(ctx context.Context, q database.Querier, organizationID string, shipmentID string) (models.Shipment, error)
	List(ctx context.Context, organizationID string, filter repositories.Filter) ([]models.Shipment, error)
	ListCargoItems(ctx context.Context, q database.Querier, organizationID string, shipmentID string) ([]models.CargoItem, error)
	Lock(ctx context.Context, q database.Querier, organizationID string, shipmentID string) (models.Shipment, error)
	SetStatus(ctx context.Context, q database.Querier, organizationID string, shipmentID string, expectedVersion int64, status string, actorID string) (models.Shipment, error)
	UpdateFields(ctx context.Context, q database.Querier, organizationID string, shipmentID string, expectedVersion int64, s models.Shipment) (models.Shipment, error)
}
