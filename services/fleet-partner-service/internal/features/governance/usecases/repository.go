package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/repositories"
)

// Repository is the persistence the governance use cases depend on;
// *repositories.Repository implements it.
type Repository interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	SecurityEvents(ctx context.Context, organizationID string, since time.Time) ([]map[string]any, error)
	InsertPrivacyRequest(ctx context.Context, q database.Querier, id, organizationID string, in models.PrivacyRequestInput, actor string, due time.Time) (bool, error)
	OpenPrivacyRequest(ctx context.Context, q database.Querier, organizationID string, in models.PrivacyRequestInput) (string, error)
	GetPrivacyRequest(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListPrivacyRequests(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockPrivacyRequest(ctx context.Context, q database.Querier, organizationID, id string) (models.PrivacyState, error)
	ActiveHolds(ctx context.Context, q database.Querier, organizationID, scopeType, reference string) ([]string, error)
	UpdatePrivacyRequest(ctx context.Context, q database.Querier, organizationID, id, status, actor, reason string, holds []string, report json.RawMessage) (int64, error)
	InsertPrivacyHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actor, note string) error
}

var _ Repository = (*repositories.Repository)(nil)
