package walletusecases

import (
	"context"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// BankAccountStore is the persistence surface for provider payout accounts.
// It is a narrow interface (rather than the concrete repository) so the
// default-account rules can be unit-tested without Postgres;
// PostgresWalletRepository implements it.
type BankAccountStore interface {
	CreateProviderBankAccount(ctx context.Context, account walletmodels.ProviderBankAccount) (walletmodels.ProviderBankAccount, error)
	GetProviderBankAccount(ctx context.Context, providerType string, providerID string, accountID string) (walletmodels.ProviderBankAccount, error)
	ListProviderBankAccounts(ctx context.Context, providerType string, providerID string) ([]walletmodels.ProviderBankAccount, error)
	SetDefaultProviderBankAccount(ctx context.Context, providerType string, providerID string, accountID string) (walletmodels.ProviderBankAccount, error)
	RemoveProviderBankAccount(ctx context.Context, providerType string, providerID string, accountID string) error
}

// SetDefaultBankAccount marks the account as the provider's default payout
// destination (clearing any previous default).
func (s *WalletService) SetDefaultBankAccount(ctx context.Context, providerType string, providerID string, accountID string) (walletmodels.ProviderBankAccount, error) {
	if accountID == "" {
		return walletmodels.ProviderBankAccount{}, apperrors.Validation("Check your request.", []apperrors.FieldViolation{
			{Field: "bank_account_id", Message: "Bank account is required."},
		})
	}
	return s.bankStore.SetDefaultProviderBankAccount(ctx, providerType, providerID, accountID)
}

// DeleteBankAccount soft-removes a payout account. If the removed account was
// the default, the newest remaining active account is promoted so the
// provider always has a preselected payout account.
func (s *WalletService) DeleteBankAccount(ctx context.Context, providerType string, providerID string, accountID string) error {
	account, err := s.bankStore.GetProviderBankAccount(ctx, providerType, providerID, accountID)
	if err != nil {
		return err
	}
	if err := s.bankStore.RemoveProviderBankAccount(ctx, providerType, providerID, accountID); err != nil {
		return err
	}
	if !account.IsDefault {
		return nil
	}
	remaining, err := s.bankStore.ListProviderBankAccounts(ctx, providerType, providerID)
	if err != nil || len(remaining) == 0 {
		// Promotion is best-effort; the account is already removed.
		return nil
	}
	_, _ = s.bankStore.SetDefaultProviderBankAccount(ctx, providerType, providerID, remaining[0].ID)
	return nil
}

// ensureDefaultBankAccount promotes a freshly registered account to default
// when it is the provider's only active account.
func (s *WalletService) ensureDefaultBankAccount(ctx context.Context, account walletmodels.ProviderBankAccount) walletmodels.ProviderBankAccount {
	if account.IsDefault {
		return account
	}
	accounts, err := s.bankStore.ListProviderBankAccounts(ctx, account.ProviderType, account.ProviderID)
	if err != nil {
		return account
	}
	for _, other := range accounts {
		if other.ID != account.ID && other.IsDefault {
			return account
		}
	}
	promoted, err := s.bankStore.SetDefaultProviderBankAccount(ctx, account.ProviderType, account.ProviderID, account.ID)
	if err != nil {
		return account
	}
	return promoted
}
