package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	AddItem(ctx context.Context, t tenant.Context, id string, in usecases.AddItemInput) (models.Item, error)
	AmendSeal(ctx context.Context, t tenant.Context, id string, in usecases.SealAmendmentInput) (models.Manifest, error)
	Cancel(ctx context.Context, t tenant.Context, id string, in usecases.CancelInput) (models.Manifest, error)
	Close(ctx context.Context, t tenant.Context, id string, in usecases.CloseInput) (models.Manifest, models.Snapshot, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Manifest, error)
	Get(ctx context.Context, t tenant.Context, id string) (models.Manifest, []models.Item, error)
	List(ctx context.Context, t tenant.Context, status string, branch string) ([]models.Manifest, error)
	Update(ctx context.Context, t tenant.Context, id string, in usecases.UpdateInput) (models.Manifest, error)
}
