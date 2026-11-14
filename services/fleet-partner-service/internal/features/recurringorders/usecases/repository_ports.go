package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Attempt(ctx context.Context, q database.Querier, org string, occ string, n int, outcome string, code string) error
	ClaimDue(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Template, error)
	CompleteOccurrence(ctx context.Context, q database.Querier, o models.Occurrence, orderID string, next time.Time) error
	Create(ctx context.Context, q database.Querier, t models.Template) (models.Template, error)
	CreateOccurrence(ctx context.Context, q database.Querier, o models.Occurrence) (models.Occurrence, bool, error)
	Fail(ctx context.Context, owner string, id string, code string, max int) error
	FailOccurrence(ctx context.Context, owner string, template models.Template, occurrence models.Occurrence, code string, max int) error
	Get(ctx context.Context, q database.Querier, org string, id string) (models.Template, error)
	Heartbeat(ctx context.Context, owner string, id string, lease time.Duration) error
	List(ctx context.Context, org string, status string) ([]models.Template, error)
	Replay(ctx context.Context, org string, id string) error
	SetStatus(ctx context.Context, q database.Querier, org string, id string, version int64, status string, actor string) (models.Template, error)
}
