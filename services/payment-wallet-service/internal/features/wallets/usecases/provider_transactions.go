package walletusecases

import (
	"context"
	"strings"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *WalletService) ProviderTransactions(ctx context.Context, providerType, providerID string, limit, offset int) ([]walletmodels.WalletTransaction, error) {
	if strings.TrimSpace(providerType) == "" || strings.TrimSpace(providerID) == "" {
		return nil, apperrors.Unauthorized("A provider account is required.", nil)
	}
	if limit < 1 || limit > 101 || offset < 0 {
		return nil, apperrors.BadRequest("Transaction pagination is invalid.", nil)
	}
	return s.repository.ListProviderTransactions(ctx, providerType+":"+providerID, s.defaultCurrency, limit, offset)
}
