package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ReferenceStore lists the repositories.ReferenceRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type ReferenceStore interface {
	AppendHistory(ctx context.Context, q database.Querier, entry *models.HistoryEntry) error
	AssignmentBelongsToOrganization(ctx context.Context, q database.Querier, organizationID string, assignmentID string) (bool, error)
	Create(ctx context.Context, q database.Querier, organizationID string, reference *models.Reference) error
	FindByExternalReference(ctx context.Context, q database.Querier, organizationID string, referenceType string, authority string, externalReference string) (*models.Reference, error)
	FindByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) (*models.Reference, error)
	GetByID(ctx context.Context, organizationID string, referenceID string) (*models.Reference, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Reference, error)
	ListHistory(ctx context.Context, organizationID string, referenceID string) ([]models.HistoryEntry, error)
	TripProjectionForOrganization(ctx context.Context, q database.Querier, organizationID string, platformTripID string) (string, error)
}
