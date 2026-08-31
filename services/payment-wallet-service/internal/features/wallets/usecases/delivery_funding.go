package walletusecases

import (
	"context"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

type DeliveryFundingInput struct {
	SourceService              string
	SourceReference            string
	VendorOrderID              string
	SponsoringVendorID         string
	ProviderType               string
	ProviderID                 string
	Currency                   string
	AuthoritativeDeliveryMinor int64
	CustomerContributionMinor  int64
	VendorSubsidyMinor         int64
	PlatformSubsidyMinor       int64
	PlatformApprovalRef        string
	CustomerPaymentReference   string
	IdempotencyKey             string
}

func (s *WalletService) ApplyDeliveryFunding(ctx context.Context, input DeliveryFundingInput) (walletmodels.DeliveryFundingAllocation, error) {
	return s.repository.ApplyDeliveryFunding(ctx, walletrepositories.ApplyDeliveryFundingInput{SourceService: input.SourceService, SourceReference: input.SourceReference,
		VendorOrderID: input.VendorOrderID, SponsoringVendorID: input.SponsoringVendorID, ProviderType: input.ProviderType, ProviderID: input.ProviderID,
		Currency: input.Currency, AuthoritativeDeliveryMinor: input.AuthoritativeDeliveryMinor, CustomerContributionMinor: input.CustomerContributionMinor,
		VendorSubsidyMinor: input.VendorSubsidyMinor, PlatformSubsidyMinor: input.PlatformSubsidyMinor, PlatformApprovalRef: input.PlatformApprovalRef,
		CustomerPaymentReference: input.CustomerPaymentReference, IdempotencyKey: input.IdempotencyKey})
}

func (s *WalletService) DeliveryFunding(ctx context.Context, sourceService, sourceReference string) (walletmodels.DeliveryFundingAllocation, error) {
	return s.repository.GetDeliveryFunding(ctx, sourceService, sourceReference)
}
