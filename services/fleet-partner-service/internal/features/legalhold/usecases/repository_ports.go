package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Create(ctx context.Context, hold models.Hold) (models.Hold, error)
	Get(ctx context.Context, organizationID string, id string) (models.Hold, error)
	IsUnderHold(ctx context.Context, organizationID string, scopeType string, scopeReference string) (bool, error)
	List(ctx context.Context, filter repositories.ListFilter) ([]models.Hold, error)
	Release(ctx context.Context, organizationID string, id string, expectedVersion int64, releasedByActorID string, releaseReason string) (models.Hold, error)
}
