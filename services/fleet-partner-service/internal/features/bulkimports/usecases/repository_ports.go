package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	Attempt(ctx context.Context, org string, id string, outcome string, code string, n int) error
	Cancel(ctx context.Context, q database.Querier, org string, id string, version int64) (models.Import, error)
	CancelGeneric(ctx context.Context, q database.Querier, organizationID string, id string, expectedVersion int64) (models.Import, error)
	Claim(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error)
	ClaimForCommit(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error)
	ClaimForRetry(ctx context.Context, owner string, lease time.Duration, backoff time.Duration, limit int) ([]models.Import, error)
	ClaimForValidation(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error)
	Create(ctx context.Context, q database.Querier, i models.Import) (models.Import, error)
	CreateGeneric(ctx context.Context, q database.Querier, i models.Import) (models.Import, error)
	Fail(ctx context.Context, owner string, id string, code string, max int) error
	FailGeneric(ctx context.Context, owner string, id string, code string, maxAttempts int) error
	Finalize(ctx context.Context, q database.Querier, id string, status string, report string, imported int, failed int) error
	FinalizeCommit(ctx context.Context, q database.Querier, id string, status string, report string, committed int, failed int) error
	Get(ctx context.Context, q database.Querier, org string, id string) (models.Import, error)
	GetGeneric(ctx context.Context, q database.Querier, organizationID string, id string) (models.Import, error)
	GetGenericRow(ctx context.Context, q database.Querier, organizationID string, importID string, rowID string) (models.Row, error)
	Heartbeat(ctx context.Context, owner string, id string, lease time.Duration) error
	InsertGenericRow(ctx context.Context, q database.Querier, row models.Row) error
	InsertRow(ctx context.Context, q database.Querier, row models.Row) error
	IsCancelled(ctx context.Context, org string, id string) (bool, error)
	List(ctx context.Context, org string) ([]models.Import, error)
	ListGeneric(ctx context.Context, organizationID string) ([]models.Import, error)
	ListLatestGenericRows(ctx context.Context, q database.Querier, organizationID string, importID string, status string) ([]models.Row, error)
	ListRows(ctx context.Context, q database.Querier, org string, id string, status string) ([]models.Row, error)
	MarkRow(ctx context.Context, q database.Querier, id string, status string, order string, code string) error
	MarkRowCommitted(ctx context.Context, q database.Querier, rowID string, subjectReference string) error
	MarkRowFailed(ctx context.Context, q database.Querier, rowID string, code string, message string) error
	Replay(ctx context.Context, org string, id string) error
	ReplayGeneric(ctx context.Context, organizationID string, id string) error
	RequeueImportForCommit(ctx context.Context, q database.Querier, organizationID string, id string) error
	RetryImport(ctx context.Context, q database.Querier, organizationID string, importID string) (int64, error)
	RetryRow(ctx context.Context, q database.Querier, organizationID string, importID string, rowID string) error
	SetValidated(ctx context.Context, q database.Querier, id string, total int, valid int, invalid int, status string) error
	SetValidationOutcome(ctx context.Context, q database.Querier, id string, total int, valid int, invalid int, status string) error
	SubmitCorrectedRow(ctx context.Context, q database.Querier, original models.Row, expectedRowVersion int, status string, rowFingerprint string, data any) (models.Row, error)
}
