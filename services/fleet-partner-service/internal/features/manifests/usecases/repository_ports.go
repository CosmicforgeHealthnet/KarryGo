package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AddItem(ctx context.Context, q database.Querier, i models.Item) (models.Item, error)
	AmendSeal(ctx context.Context, q database.Querier, org string, id string, version int64, seal string, actor string) (models.Manifest, error)
	AppendHistory(ctx context.Context, q database.Querier, org string, id string, from string, to string, reason string, actorType string, actor string, corr string) error
	Create(ctx context.Context, q database.Querier, m models.Manifest) (models.Manifest, error)
	CreateSnapshot(ctx context.Context, q database.Querier, s models.Snapshot) (models.Snapshot, error)
	Get(ctx context.Context, q database.Querier, org string, id string) (models.Manifest, error)
	GetSnapshot(ctx context.Context, q database.Querier, org string, manifest string) (models.Snapshot, error)
	HasActiveMembership(ctx context.Context, q database.Querier, org string, kind string, item string, except string) (bool, error)
	List(ctx context.Context, org string, status string, branch string) ([]models.Manifest, error)
	ListItems(ctx context.Context, q database.Querier, org string, id string) ([]models.Item, error)
	Lock(ctx context.Context, q database.Querier, org string, id string) (models.Manifest, error)
	SetStatus(ctx context.Context, q database.Querier, org string, id string, version int64, status string, actor string) (models.Manifest, error)
	Update(ctx context.Context, q database.Querier, org string, id string, version int64, route string, vehicle string, provider string, seal string, actor string) (models.Manifest, error)
}
