package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Template, error)
	Get(ctx context.Context, t tenant.Context, id string) (models.Template, error)
	List(ctx context.Context, t tenant.Context, status string) ([]models.Template, error)
	Replay(ctx context.Context, t tenant.Context, id string) error
	SetActive(ctx context.Context, t tenant.Context, id string, version int64, active bool) (models.Template, error)
}
