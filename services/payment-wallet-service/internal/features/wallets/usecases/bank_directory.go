package walletusecases

import (
	"context"
	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *WalletService) SupportedBanks(ctx context.Context) ([]walletclients.SupportedBank, error) {
	directory, ok := s.paystack.(walletclients.BankDirectoryClient)
	if !ok {
		return nil, apperrors.Unavailable("Bank directory is not configured. Please try again later.", nil)
	}
	banks, err := directory.ListBanks(ctx)
	if err != nil {
		return nil, apperrors.Unavailable("Banks could not be loaded. Please try again.", err)
	}
	return banks, nil
}
