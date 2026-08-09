package integration

import (
	"context"

	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
)

// JobPaymentBridge lets the wallet payment flow record authoritative job
// payment verifications without importing the finance use-case package.
//
// The bridge acts as the Wallet service identity, which holds
// finance.payment.confirm. That is the point of the separation: the record is
// written by the service that owns the payment, and only read by the services
// that need to trust it.
type JobPaymentBridge struct {
	Service *usecases.Service
	// ActorID is the service identity the record is attributed to.
	ActorID string
}

// NewJobPaymentBridge builds the bridge for one finance service.
func NewJobPaymentBridge(service *usecases.Service) JobPaymentBridge {
	return JobPaymentBridge{Service: service, ActorID: "payment-wallet-service"}
}

// RecordJobPaymentFromWallet writes the authoritative verification.
func (b JobPaymentBridge) RecordJobPaymentFromWallet(ctx context.Context,
	record walletusecases.JobPaymentRecord) error {
	if b.Service == nil {
		return nil
	}
	actor := b.ActorID
	if actor == "" {
		actor = "payment-wallet-service"
	}
	_, err := b.Service.RecordJobPayment(ctx, usecases.JobPaymentInput{
		Meta: usecases.Meta{
			Actor: financeauth.Actor{Type: financeauth.ActorService, ID: actor,
				OrganizationID: record.OrganizationID},
			IdempotencyKey: record.IdempotencyKey,
			CorrelationID:  record.CorrelationID,
		},
		OrganizationID:           record.OrganizationID,
		SourceService:            record.SourceService,
		DispatchRequestReference: record.DispatchRequestReference,
		TripReference:            record.TripReference,
		BookingReference:         record.BookingReference,
		PaymentReference:         record.PaymentReference,
		AmountMinor:              record.AmountMinor,
		Currency:                 record.Currency,
		Status:                   record.Status,
		PaidAt:                   record.PaidAt,
	})
	return err
}

var _ walletusecases.JobPaymentRecorder = JobPaymentBridge{}
