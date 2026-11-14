package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/usecases"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Get(ctx context.Context, organizationID string) (models.Settings, error)
	Update(ctx context.Context, input usecases.UpdateInput) (models.Settings, []models.ValidationError, error)
}
