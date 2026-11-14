package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
)

// Store is the persistence finance orchestration needs.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error

	Create(ctx context.Context, q database.Querier, o *models.Operation) (bool, error)
	Claim(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Operation, error)
	Outcome(ctx context.Context, q database.Querier, o models.Operation, status, reference, code string, next time.Time, max int) error
	Recover(ctx context.Context) (int64, error)
	ReplayIn(ctx context.Context, q database.Querier, id string) error
	QueueStale(ctx context.Context, q database.Querier, cutoff time.Time, limit int) ([]models.Projection, error)
	ClaimProjections(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Projection, error)
	RecoverProjectionLeases(ctx context.Context) (int64, error)
	ProjectionOutcome(ctx context.Context, q database.Querier, p models.Projection, reconciliationStatus, freshness, safeStatus, code string, sourceVersion int64, next time.Time, maxAttempts int) error
	ReplayProjectionIn(ctx context.Context, q database.Querier, id string) error

	LockOperationVersion(ctx context.Context, q database.Querier, organizationID, id string) (int64, error)
	LockProjectionVersion(ctx context.Context, q database.Querier, organizationID, id string) (int64, error)
	CurrentProjection(ctx context.Context, organizationID, projectionType, reference string) (models.Projection, error)
	LatestSubscriptionStatus(ctx context.Context, organizationID string) (string, bool, error)
	OpenReconciliation(ctx context.Context, organizationID, projectionType, reference string) (bool, error)
	PaymentProjectionState(ctx context.Context, organizationID, reference string) (string, string, int64, bool, error)
	TripCompletion(ctx context.Context, organizationID, tripID string) (models.TripCompletion, error)
	OperationRequester(ctx context.Context, organizationID, id string) (string, string, error)

	LockSourceVersion(ctx context.Context, q database.Querier, aggregateType, aggregateID string) (int64, string, bool, error)
	UpsertProjection(ctx context.Context, q database.Querier, w models.ProjectionWrite) error
	UpsertPaymentVerification(ctx context.Context, q database.Querier, w models.ProjectionWrite) error
	UpsertSourceVersion(ctx context.Context, q database.Querier, aggregateType, aggregateID string, version int64, eventID, payloadHash string) error

	ListOperations(ctx context.Context, scope models.Scope, filter models.OperationFilter) ([]models.OperationSummary, error)
	GetOperation(ctx context.Context, scope models.Scope, id string) (models.OperationSummary, error)
	OperationHistory(ctx context.Context, organizationID, id string) ([]models.HistoryEntry, error)
	OperationAudit(ctx context.Context, organizationID, id string) ([]models.AuditEntry, error)
	ListProjections(ctx context.Context, scope models.Scope, filter models.ProjectionFilter) ([]models.ProjectionSummary, error)
	GetProjection(ctx context.Context, scope models.Scope, id string) (models.ProjectionSummary, error)
	RequestReadBack(ctx context.Context, q database.Querier, organizationID, id string) (bool, error)
}
