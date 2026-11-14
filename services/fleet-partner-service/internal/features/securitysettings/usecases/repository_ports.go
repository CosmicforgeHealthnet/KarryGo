package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Get(ctx context.Context, organizationID string) (models.Settings, error)
	Upsert(ctx context.Context, settings models.Settings, expectedVersion int64) (models.Settings, error)
}
