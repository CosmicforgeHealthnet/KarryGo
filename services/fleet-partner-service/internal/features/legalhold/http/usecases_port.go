package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Create(ctx context.Context, input usecases.CreateInput) (models.Hold, error)
	Get(ctx context.Context, organizationID string, id string) (models.Hold, error)
	List(ctx context.Context, filter repositories.ListFilter) ([]models.Hold, error)
	Release(ctx context.Context, input usecases.ReleaseInput) (models.Hold, error)
}
