package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
)

// Repository is the persistence fuel depends on.
type Repository interface {
	InsertFuel(ctx context.Context, q database.Querier, v *models.FuelRecord) error
	GetFuel(ctx context.Context, org, id string) (models.FuelRecord, error)
	ListFuel(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.FuelRecord, error)
	DecideFuel(ctx context.Context, q database.Querier, org, id string, expected int64, to, actor string) (models.FuelRecord, error)
	InsertApprovalHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error
}
