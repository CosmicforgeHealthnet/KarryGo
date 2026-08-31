package walletusecases

import (
	"context"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

type TransitionDeliveryResponsibilityInput struct {
	VendorOrderID, OperationRef, IdempotencyKey, ReasonCode string
	PlatformExecutionCostMinor                              int64
}

func (s *WalletService) TransitionDeliveryResponsibility(ctx context.Context, input TransitionDeliveryResponsibilityInput) (walletmodels.DeliveryResponsibilityTransition, error) {
	return s.repository.TransitionDeliveryResponsibility(ctx, walletrepositories.TransitionDeliveryResponsibilityInput{VendorOrderID: input.VendorOrderID, OperationRef: input.OperationRef, IdempotencyKey: input.IdempotencyKey, ReasonCode: input.ReasonCode, PlatformExecutionCostMinor: input.PlatformExecutionCostMinor})
}

func (s *WalletService) DeliveryResponsibilityTransition(ctx context.Context, operationRef string) (walletmodels.DeliveryResponsibilityTransition, error) {
	return s.repository.GetDeliveryResponsibilityTransition(ctx, operationRef)
}
