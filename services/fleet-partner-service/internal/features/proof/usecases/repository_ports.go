package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof/models"
)

// Repository is the persistence this feature needs. A nil Querier runs on the
// pool; a transaction composes the calls atomically.
type Repository interface {
	InsertPolicy(ctx context.Context, q database.Querier, v models.Policy) (models.Policy, error)
	UpdatePolicy(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Policy) (models.Policy, error)
	InsertSnapshot(ctx context.Context, q database.Querier, snap *models.PolicySnapshot) error
	GetPolicy(ctx context.Context, org, id string) (models.Policy, error)
	ListPolicies(ctx context.Context, org string) ([]models.Policy, error)
	GetSnapshot(ctx context.Context, org, id string) (models.PolicySnapshot, error)
	GetRecordByIdempotencyKey(ctx context.Context, org, key string) (models.Record, error)
	GetRecord(ctx context.Context, q database.Querier, org, id string, share bool) (models.Record, error)
	InsertRecord(ctx context.Context, q database.Querier, v models.Record) (models.Record, error)
	InsertMediaVerified(ctx context.Context, q database.Querier, org, recordID, mediaID string, sourceVersion int64) error
	ListRecords(ctx context.Context, org string) ([]models.Record, error)
	CountSatisfying(ctx context.Context, org, snapshot, proofType, order, shipment, pkg, stop string) (int, error)
	ListVerifications(ctx context.Context, q database.Querier, org, id string) ([]models.Verification, error)
	LockDecision(ctx context.Context, q database.Querier, recordID string) error
	InsertDecision(ctx context.Context, q database.Querier, org, recordID, decision, authority, actor string, reasonCodes []byte) error
	ActiveOrganizationIDs(ctx context.Context) ([]string, error)
	ListPendingMediaRecords(ctx context.Context, org string) ([]models.PendingMediaRecord, error)
	InsertMediaReconciliation(ctx context.Context, org, recordID, status, mediaID string, sourceVersion int64, reason string) error
}
