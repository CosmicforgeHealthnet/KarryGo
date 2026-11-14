package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Cancel(ctx context.Context, t tenant.Context, id string, version int64) (models.Import, error)
	CancelGeneric(ctx context.Context, t tenant.Context, id string, expectedVersion int64) (models.Import, error)
	Commit(ctx context.Context, t tenant.Context, id string) (models.Import, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Import, error)
	CreateGeneric(ctx context.Context, t tenant.Context, in usecases.CreateGenericInput) (models.Import, error)
	Get(ctx context.Context, t tenant.Context, id string) (models.Import, []models.Row, error)
	GetGeneric(ctx context.Context, t tenant.Context, id string) (models.Import, []models.Row, error)
	List(ctx context.Context, t tenant.Context) ([]models.Import, error)
	ListGeneric(ctx context.Context, t tenant.Context) ([]models.Import, error)
	Replay(ctx context.Context, t tenant.Context, id string) error
	ReplayGeneric(ctx context.Context, t tenant.Context, id string) error
	RetryImport(ctx context.Context, t tenant.Context, id string) (int64, error)
	RetryRow(ctx context.Context, t tenant.Context, importID string, rowID string) error
	SubmitCorrection(ctx context.Context, t tenant.Context, in usecases.SubmitCorrectionInput) (models.Row, error)
	Validate(ctx context.Context, t tenant.Context, id string) (models.Import, error)
}
