package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Get(ctx context.Context, q database.Querier, organizationID string, subjectType string, subjectReference string) (models.Document, error)
	MarkUnavailable(ctx context.Context, q database.Querier, organizationID string, subjectType string, subjectReference string) error
	Query(ctx context.Context, organizationID string, filter repositories.QueryFilter) ([]models.Document, error)
	RequiredPermissions(ctx context.Context, organizationID string, filter repositories.QueryFilter) ([]string, error)
	Upsert(ctx context.Context, q database.Querier, d models.Document) (models.Document, error)
}
