package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Create(ctx context.Context, q database.Querier, item models.CatalogItem) (models.CatalogItem, error)
	Get(ctx context.Context, q database.Querier, organizationID string, itemID string) (models.CatalogItem, error)
	List(ctx context.Context, organizationID string, filter repositories.Filter) ([]models.CatalogItem, error)
	SetStatus(ctx context.Context, q database.Querier, organizationID string, itemID string, expectedVersion int64, newStatus string, actorID string, archived bool) (models.CatalogItem, error)
	UpdateFields(ctx context.Context, q database.Querier, organizationID string, itemID string, expectedVersion int64, item models.CatalogItem) (models.CatalogItem, error)
}
