package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AppendHistory(ctx context.Context, q database.Querier, entry *models.HistoryEntry) error
	AppendVerification(ctx context.Context, q database.Querier, decision *models.VerificationDecision) error
	BranchBelongsToOrganization(ctx context.Context, q database.Querier, organizationID string, branchID string) (bool, error)
	ClaimDeletionRequests(ctx context.Context, q database.Querier, owner string, lease time.Duration, limit int) ([]models.DeletionRequest, error)
	ClaimExpiredDocuments(ctx context.Context, q database.Querier, limit int) ([]models.Document, error)
	ClaimReconciliationCases(ctx context.Context, q database.Querier, owner string, lease time.Duration, limit int) ([]models.ReconciliationCase, error)
	ClaimStaleProjections(ctx context.Context, q database.Querier, staleAfter time.Duration, limit int) ([]models.Document, error)
	Create(ctx context.Context, q database.Querier, document *models.Document) error
	CreateDeletionRequest(ctx context.Context, q database.Querier, record *models.DeletionRequest) error
	CustomerForOrder(ctx context.Context, q database.Querier, organizationID string, orderID string) (string, error)
	DeletionRequestByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, key string) (*models.DeletionRequest, error)
	FailReconciliationCase(ctx context.Context, q database.Querier, id string, errorCode string, backoff time.Duration, maxAttempts int) error
	FindByIdempotencyKey(ctx context.Context, q database.Querier, organizationID string, key string) (*models.Document, error)
	GetByID(ctx context.Context, q database.Querier, organizationID string, documentID string) (*models.Document, error)
	GetByMediaObject(ctx context.Context, q database.Querier, mediaObjectID string) (*models.Document, error)
	LatestDeletionRequest(ctx context.Context, q database.Querier, organizationID string, documentID string) (*models.DeletionRequest, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Document, error)
	ListCases(ctx context.Context, q database.Querier, organizationID string, documentID string) ([]models.ReconciliationCase, error)
	ListHistory(ctx context.Context, q database.Querier, organizationID string, documentID string) ([]models.HistoryEntry, error)
	ListVerifications(ctx context.Context, q database.Querier, organizationID string, documentID string) ([]models.VerificationDecision, error)
	LockByID(ctx context.Context, q database.Querier, organizationID string, documentID string) (*models.Document, error)
	LockSourceVersion(ctx context.Context, q database.Querier, mediaObjectID string) (repositories.SourceVersion, error)
	OpenCaseFor(ctx context.Context, q database.Querier, documentID string) (bool, error)
	OpenReconciliationCase(ctx context.Context, q database.Querier, record *models.ReconciliationCase) error
	OwnerBelongsToOrganization(ctx context.Context, q database.Querier, organizationID string, ownerType string, ownerID string) (bool, error)
	RecordAccess(ctx context.Context, q database.Querier, entry *models.AccessAuditEntry) error
	RecordSourceVersion(ctx context.Context, q database.Querier, record repositories.SourceVersion) error
	RecoverExpiredCaseLeases(ctx context.Context, q database.Querier) (int64, error)
	RecoverExpiredDeletionLeases(ctx context.Context, q database.Querier) (int64, error)
	ReplayDeletionRequest(ctx context.Context, q database.Querier, organizationID string, id string) error
	ReplayReconciliationCase(ctx context.Context, q database.Querier, organizationID string, id string) error
	ResolveReconciliationCase(ctx context.Context, q database.Querier, id string) error
	SetDeletionStatus(ctx context.Context, q database.Querier, id string, status string, blockReason string, errorCode string, backoff time.Duration) error
	Update(ctx context.Context, q database.Querier, document *models.Document, expectedVersion int64) error
}
