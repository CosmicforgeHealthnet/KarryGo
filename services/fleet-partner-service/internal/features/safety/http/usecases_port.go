package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Get(ctx context.Context, organizationID string, referenceID string) (*models.Reference, error)
	History(ctx context.Context, organizationID string, referenceID string) ([]models.HistoryEntry, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Reference, error)
	RecordReference(ctx context.Context, input usecases.RecordReferenceInput) (*usecases.RecordReferenceResult, error)
}
