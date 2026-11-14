package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Get(ctx context.Context, organizationID string, class string) (models.Policy, error)
	List(ctx context.Context, organizationID string) ([]models.Policy, error)
	Update(ctx context.Context, input usecases.UpdateInput) (models.Policy, error)
}
