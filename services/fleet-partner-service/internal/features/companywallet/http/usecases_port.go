package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	DecideAllocation(ctx context.Context, t tenant.Context, id string, expectedVersion int64, approve bool, reasonCode string) (models.AllocationInstruction, error)
	DecideWithdrawal(ctx context.Context, t tenant.Context, membershipID string, id string, expectedVersion int64, approve bool, reasonCode string) (models.WithdrawalRequest, error)
	GetPolicy(ctx context.Context, t tenant.Context) (models.WithdrawalPolicy, error)
	GetSubWallet(ctx context.Context, t tenant.Context, membershipID string) (usecases.SubWalletView, error)
	GetTransaction(ctx context.Context, t tenant.Context, id string) (models.TransactionProjection, error)
	GetWallet(ctx context.Context, t tenant.Context, authoritative bool) (usecases.WalletView, error)
	ListAllocations(ctx context.Context, t tenant.Context, filter repositories.AllocationFilter) ([]models.AllocationInstruction, error)
	ListTransactions(ctx context.Context, t tenant.Context, filter repositories.TransactionFilter) ([]models.TransactionProjection, error)
	ListWithdrawals(ctx context.Context, t tenant.Context, filter repositories.WithdrawalFilter) ([]models.WithdrawalRequest, error)
	ProviderStatement(ctx context.Context, in usecases.StatementInput) (usecases.ProviderStatement, error)
	ReplayAllocation(ctx context.Context, t tenant.Context, membershipID string, id string) error
	ReplayWithdrawal(ctx context.Context, t tenant.Context, membershipID string, id string) error
	RequestAllocation(ctx context.Context, in usecases.AllocationInput) (models.AllocationInstruction, error)
	RequestWithdrawal(ctx context.Context, in usecases.WithdrawalInput) (models.WithdrawalRequest, error)
	SetPolicy(ctx context.Context, in usecases.SetPolicyInput) (models.WithdrawalPolicy, error)
}
