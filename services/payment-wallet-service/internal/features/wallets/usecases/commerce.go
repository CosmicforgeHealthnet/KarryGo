package walletusecases

import (
	"context"
	"strings"

	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

const vendorBusinessBankOwnerType = "vendor_business"

type CreateCommerceIntentInput struct {
	ParentCommerceReference string
	CustomerID              string
	CustomerEmail           string
	QuoteReference          string
	QuoteVersion            int64
	QuoteFingerprint        string
	FinalAuthorizedMinor    int64
	MaxAuthorizedMinor      int64
	AllocationFingerprint   string
	Currency                string
	PaymentMethod           string
	IdempotencyKey          string
	Allocations             []walletmodels.CommerceAllocation
}

func (s *WalletService) CreateCommerceIntent(ctx context.Context, input CreateCommerceIntentInput) (walletmodels.CommercePaymentIntent, error) {
	return s.repository.CreateCommercePaymentIntent(ctx, walletrepositories.CreateCommerceIntentInput{
		ParentCommerceReference: input.ParentCommerceReference, CustomerID: input.CustomerID, CustomerEmail: input.CustomerEmail,
		QuoteReference: input.QuoteReference, QuoteVersion: input.QuoteVersion, QuoteFingerprint: input.QuoteFingerprint,
		FinalAuthorizedMinor: input.FinalAuthorizedMinor, MaxAuthorizedMinor: input.MaxAuthorizedMinor,
		AllocationFingerprint: input.AllocationFingerprint, Currency: input.Currency, PaymentMethod: input.PaymentMethod,
		IdempotencyKey: input.IdempotencyKey, Allocations: input.Allocations,
	})
}

func (s *WalletService) CommerceIntent(ctx context.Context, parentReference string) (walletmodels.CommercePaymentIntent, error) {
	return s.repository.GetCommercePaymentIntent(ctx, strings.TrimSpace(parentReference))
}

func (s *WalletService) HoldCommerceIntent(ctx context.Context, parentReference, idempotencyKey string) (walletmodels.CommercePaymentIntent, error) {
	return s.repository.HoldCommercePayment(ctx, parentReference, idempotencyKey)
}

func (s *WalletService) ReleaseVendorPayable(ctx context.Context, input walletrepositories.ReleaseVendorPayableInput) (walletmodels.CommerceFinancialOperation, error) {
	return s.repository.ReleaseVendorPayable(ctx, input)
}

func (s *WalletService) ReleaseCommerceDelivery(ctx context.Context, input walletrepositories.ReleaseCommerceDeliveryInput) (walletmodels.CommerceFinancialOperation, error) {
	return s.repository.ReleaseCommerceDelivery(ctx, input)
}

func (s *WalletService) RefundCommerceAllocation(ctx context.Context, input walletrepositories.RefundCommerceAllocationInput) (walletmodels.CommerceFinancialOperation, error) {
	return s.repository.RefundCommerceAllocation(ctx, input)
}

func (s *WalletService) CommerceOperation(ctx context.Context, operationType, idempotencyKey string) (walletmodels.CommerceFinancialOperation, error) {
	return s.repository.GetCommerceFinancialOperation(ctx, strings.ToUpper(strings.TrimSpace(operationType)), strings.TrimSpace(idempotencyKey))
}

func (s *WalletService) VendorPayableSummary(ctx context.Context, vendorID string) (walletmodels.WalletSummary, error) {
	if _, err := s.repository.ProvisionVendorPayable(ctx, strings.TrimSpace(vendorID), s.defaultCurrency); err != nil {
		return walletmodels.WalletSummary{}, err
	}
	return s.repository.VendorPayableSummary(ctx, strings.TrimSpace(vendorID), s.defaultCurrency)
}

func (s *WalletService) VendorTransactions(ctx context.Context, vendorID string, limit int) ([]walletmodels.WalletTransaction, error) {
	return s.repository.ListWalletTransactions(ctx, walletmodels.OwnerTypeVendor, strings.TrimSpace(vendorID), s.defaultCurrency, limit)
}

type VendorWithdrawalInput struct {
	VendorID        string
	BankAccountID   string
	AmountKobo      int64
	Currency        string
	IdempotencyKey  string
	StepUpReference string
}

func (s *WalletService) RequestVendorWithdrawal(ctx context.Context, input VendorWithdrawalInput) (walletmodels.Withdrawal, error) {
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if strings.TrimSpace(input.VendorID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "vendor_id", Message: "Vendor id is required."})
	}
	if strings.TrimSpace(input.BankAccountID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "bank_account_id", Message: "Bank account is required."})
	}
	if strings.TrimSpace(input.StepUpReference) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "step_up_reference", Message: "Step-up evidence is required."})
	}
	if input.AmountKobo < s.withdrawalMinKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Withdrawal amount is below the minimum."})
	}
	if s.withdrawalMaxKobo > 0 && input.AmountKobo > s.withdrawalMaxKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Withdrawal amount exceeds the maximum."})
	}
	if len(fields) > 0 {
		return walletmodels.Withdrawal{}, apperrors.Validation("Check your withdrawal request.", fields)
	}
	currency := walletmodels.DefaultCurrency(input.Currency)
	if existing, err := s.repository.GetWithdrawalByIdempotency(ctx, vendorBusinessBankOwnerType, input.VendorID, input.IdempotencyKey); err == nil {
		if existing.AmountKobo != input.AmountKobo || existing.BankAccountID != input.BankAccountID || existing.Currency != currency {
			return walletmodels.Withdrawal{}, apperrors.Conflict("Withdrawal idempotency key was reused with different details.", nil)
		}
		return existing, nil
	}
	account, err := s.repository.GetProviderBankAccount(ctx, vendorBusinessBankOwnerType, input.VendorID, input.BankAccountID)
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	if account.Status != "active" {
		return walletmodels.Withdrawal{}, apperrors.Conflict("The selected bank account is not active.", nil)
	}
	if !s.paystackBalanceCanCover(ctx, currency, input.AmountKobo) {
		return walletmodels.Withdrawal{}, apperrors.Conflict("Paystack Balance is not enough to process this withdrawal yet.", nil)
	}
	withdrawal, created, err := s.repository.RequestWithdrawal(ctx, walletrepositories.CreateWithdrawalInput{Reference: walletrepositories.Reference("wd"), ProviderType: vendorBusinessBankOwnerType, ProviderID: input.VendorID, BankAccountID: input.BankAccountID, AmountKobo: input.AmountKobo, Currency: currency, IdempotencyKey: input.IdempotencyKey})
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	if !created {
		return withdrawal, nil
	}
	transfer, err := s.paystack.InitiateTransfer(ctx, walletclients.InitiateTransferRequest{AmountKobo: input.AmountKobo, RecipientCode: account.RecipientCode, Reason: "Karry Go Vendor withdrawal " + withdrawal.Reference, Reference: withdrawal.Reference})
	if err != nil {
		// The transfer outcome is ambiguous. Keep the atomic reservation and let
		// read-back/recovery establish the Paystack result; never restore funds
		// while a transfer may have committed.
		return withdrawal, apperrors.Unavailable("Vendor withdrawal outcome is pending authoritative read-back.", err)
	}
	requiresOTP := transfer.Status == "otp" || transfer.Status == "pending_otp"
	return s.repository.MarkWithdrawalTransferStarted(ctx, withdrawal.ID, transfer.TransferCode, stringID(transfer.ID), requiresOTP)
}

func (s *WalletService) VendorWithdrawals(ctx context.Context, vendorID string, limit int) ([]walletmodels.Withdrawal, error) {
	return s.repository.ListVendorWithdrawals(ctx, strings.TrimSpace(vendorID), limit)
}

func (s *WalletService) VendorWithdrawal(ctx context.Context, vendorID, reference string) (walletmodels.Withdrawal, error) {
	return s.repository.GetVendorWithdrawal(ctx, strings.TrimSpace(vendorID), strings.TrimSpace(reference))
}
