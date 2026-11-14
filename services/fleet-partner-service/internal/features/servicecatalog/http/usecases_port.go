package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Activate(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error)
	Archive(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.CatalogItem, error)
	Deactivate(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64) (models.CatalogItem, error)
	Get(ctx context.Context, t tenant.Context, itemID string) (models.CatalogItem, error)
	List(ctx context.Context, t tenant.Context, input usecases.ListInput) ([]models.CatalogItem, string, error)
	Update(ctx context.Context, t tenant.Context, itemID string, expectedVersion int64, in usecases.UpdateInput) (models.CatalogItem, error)
}
