package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	AppendAllocationHistory(ctx context.Context, q database.Querier, organizationID string, instructionID string, previous string, next string, reasonCode string, actorType string, actorID string, correlationID string) error
	AppendWithdrawalHistory(ctx context.Context, q database.Querier, organizationID string, withdrawalID string, previous string, next string, reasonCode string, actorType string, actorID string, correlationID string) error
	ClaimAllocations(ctx context.Context, limit int) ([]models.AllocationInstruction, error)
	ClaimWithdrawals(ctx context.Context, limit int) ([]models.WithdrawalRequest, error)
	CountLiveAllocationForPurpose(ctx context.Context, q database.Querier, organizationID string, membershipID string, referenceType string, referenceID string, allocationType string) (bool, error)
	CreateAllocation(ctx context.Context, q database.Querier, instruction *models.AllocationInstruction) (bool, error)
	CreateWithdrawal(ctx context.Context, q database.Querier, request *models.WithdrawalRequest) (bool, error)
	DecideWithdrawal(ctx context.Context, q database.Querier, organizationID string, id string, expectedVersion int64, status string, decidedBy string, reasonCode string, approvalRequestID string) error
	FindAllocationByKey(ctx context.Context, q database.Querier, organizationID string, key string) (models.AllocationInstruction, error)
	GetAllocation(ctx context.Context, q database.Querier, organizationID string, id string, branchIDs []string) (models.AllocationInstruction, error)
	GetOrganizationLink(ctx context.Context, q database.Querier, organizationID string) (models.OrganizationWalletLink, error)
	GetPolicy(ctx context.Context, q database.Querier, organizationID string) (models.WithdrawalPolicy, error)
	GetSubWalletLink(ctx context.Context, q database.Querier, organizationID string, membershipID string) (models.ProviderSubWalletLink, error)
	GetTransaction(ctx context.Context, organizationID string, id string, branchIDs []string) (models.TransactionProjection, error)
	GetWithdrawal(ctx context.Context, q database.Querier, organizationID string, id string, branchIDs []string) (models.WithdrawalRequest, error)
	ListAllocations(ctx context.Context, organizationID string, branchIDs []string, filter repositories.AllocationFilter) ([]models.AllocationInstruction, error)
	ListTransactions(ctx context.Context, organizationID string, branchIDs []string, filter repositories.TransactionFilter) ([]models.TransactionProjection, error)
	ListWithdrawals(ctx context.Context, organizationID string, branchIDs []string, filter repositories.WithdrawalFilter) ([]models.WithdrawalRequest, error)
	LockAllocation(ctx context.Context, q database.Querier, organizationID string, id string) (models.AllocationInstruction, error)
	LockWithdrawal(ctx context.Context, q database.Querier, organizationID string, id string) (models.WithdrawalRequest, error)
	MarkOrganizationLinkStale(ctx context.Context, q database.Querier, organizationID string, reason string) error
	ReplayAllocation(ctx context.Context, q database.Querier, organizationID string, id string) error
	ReplayWithdrawal(ctx context.Context, q database.Querier, organizationID string, id string) error
	SetAllocationStatus(ctx context.Context, q database.Querier, organizationID string, id string, expectedVersion int64, status string, walletOperationID string, walletTransactionReference string, safeErrorCode string, updatedBy string) error
	SetWithdrawalStatus(ctx context.Context, q database.Querier, organizationID string, id string, expectedVersion int64, status string, walletOperationID string, walletReference string, safeErrorCode string) error
	UpsertOrganizationLink(ctx context.Context, q database.Querier, organizationID string, walletAccountID string, status string, currency string, freshness string, lastEventID string, sourceVersion int64) error
	UpsertPolicy(ctx context.Context, q database.Querier, policy models.WithdrawalPolicy, expectedVersion int64) (models.WithdrawalPolicy, error)
	UpsertSubWalletLink(ctx context.Context, q database.Querier, organizationID string, membershipID string, walletAccountID string, providerID string, providerService string, currency string, status string, freshness string, lastEventID string, sourceVersion int64) error
}
