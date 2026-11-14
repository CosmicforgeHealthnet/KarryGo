package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Index(ctx context.Context, organizationID string, in usecases.IndexInput) (models.Document, error)
	RemoveFromIndex(ctx context.Context, organizationID string, subjectType string, subjectReference string) error
	ReplayIndexJob(ctx context.Context, organizationID string, jobID string) error
	Search(ctx context.Context, t tenant.Context, in usecases.SearchInput) ([]models.Document, error)
}
