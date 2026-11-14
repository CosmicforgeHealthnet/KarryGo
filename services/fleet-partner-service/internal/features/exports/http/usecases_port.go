package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Cancel(ctx context.Context, organizationID string, id string, expectedVersion int64) (models.Job, error)
	Get(ctx context.Context, organizationID string, id string) (models.Job, error)
	List(ctx context.Context, organizationID string, limit int) ([]models.Job, error)
	Replay(ctx context.Context, organizationID string, id string) error
	RequestDownloadURL(ctx context.Context, t tenant.Context, id string) (usecases.DownloadGrant, error)
	RequestExport(ctx context.Context, in usecases.RequestExportInput) (models.Job, error)
}
