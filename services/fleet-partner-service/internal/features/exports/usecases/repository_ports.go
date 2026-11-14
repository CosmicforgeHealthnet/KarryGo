package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Attempt(ctx context.Context, organizationID string, exportID string, outcome string, code string, n int) error
	Cancel(ctx context.Context, q database.Querier, organizationID string, id string, expectedVersion int64) (models.Job, error)
	Claim(ctx context.Context, owner string, lease time.Duration, backoff time.Duration, limit int) ([]models.Job, error)
	Complete(ctx context.Context, q database.Querier, id string, mediaFileID string, rowCount int, expiresAt *time.Time) error
	Create(ctx context.Context, q database.Querier, j models.Job) (models.Job, error)
	ExpireReady(ctx context.Context, batch int) (int64, error)
	Fail(ctx context.Context, owner string, id string, code string, maxAttempts int) error
	FindByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, key string) (models.Job, error)
	Get(ctx context.Context, q database.Querier, organizationID string, id string) (models.Job, error)
	Heartbeat(ctx context.Context, owner string, id string, lease time.Duration) error
	List(ctx context.Context, organizationID string, limit int) ([]models.Job, error)
	Replay(ctx context.Context, organizationID string, id string) error
}
