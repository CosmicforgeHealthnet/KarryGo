package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases is the slice of the fuel use cases the handler calls.
type UseCases interface {
	ListFuelPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.FuelRecord, *models.FuelRecord, error)
	GetFuel(ctx context.Context, t tenant.Context, id string) (models.FuelRecord, error)
	CreateFuel(ctx context.Context, t tenant.Context, v models.FuelRecord) (models.FuelRecord, error)
	FuelAction(ctx context.Context, t tenant.Context, id, to, reason string, expected int64) (models.FuelRecord, error)
}
