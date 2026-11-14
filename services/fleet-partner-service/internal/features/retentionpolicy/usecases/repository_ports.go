package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Get(ctx context.Context, organizationID string, class string) (models.Policy, error)
	List(ctx context.Context, organizationID string) ([]models.Policy, error)
	Upsert(ctx context.Context, policy models.Policy, expectedVersion int64) (models.Policy, error)
}
