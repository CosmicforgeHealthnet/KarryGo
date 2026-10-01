package walletusecases

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// referralProviderTypes are the provider_type values drivers' ledgers are
// keyed by. A referral credit for any other provider type is refused, so
// money can only land in an account a driver's wallet screens actually read.
var referralProviderTypes = map[string]bool{"truck_provider": true, "dispatch": true, "taxi": true}

// ReferralPostingRequest is a referral credit or reversal from
// referral-service. For customers OwnerID is the customer id; for providers
// it is the provider id and ProviderType names the provider's ledger.
type ReferralPostingRequest struct {
	OwnerType      string `json:"owner_type"`
	OwnerID        string `json:"owner_id"`
	ProviderType   string `json:"provider_type,omitempty"`
	Balance        string `json:"balance"`
	AmountKobo     int64  `json:"amount_kobo"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key"`
	Reference      string `json:"reference"`
	Reason         string `json:"reason"`
}

// PostReferralCredit pays a referral reward into a referral balance.
func (s *WalletService) PostReferralCredit(ctx context.Context, request ReferralPostingRequest) (walletrepositories.ReferralPosting, error) {
	input, err := referralPostingInput(request)
	if err != nil {
		return walletrepositories.ReferralPosting{}, err
	}
	return s.repository.PostReferralCredit(ctx, input)
}

// PostReferralReversal takes a referral reward back. The balance may go
// negative; see PostgresWalletRepository.PostReferralReversal.
func (s *WalletService) PostReferralReversal(ctx context.Context, request ReferralPostingRequest) (walletrepositories.ReferralPosting, error) {
	input, err := referralPostingInput(request)
	if err != nil {
		return walletrepositories.ReferralPosting{}, err
	}
	return s.repository.PostReferralReversal(ctx, input)
}

// ReferralTotals reports one owner's referral credits, reversals and balances
// for reconciliation.
func (s *WalletService) ReferralTotals(ctx context.Context, ownerType, ownerID, providerType string) (walletrepositories.ReferralTotals, error) {
	ledgerType, ledgerID, err := referralLedgerOwner(ownerType, ownerID, providerType)
	if err != nil {
		return walletrepositories.ReferralTotals{}, err
	}
	return s.repository.GetReferralTotals(ctx, ledgerType, ledgerID, s.defaultCurrency)
}

func referralPostingInput(request ReferralPostingRequest) (walletrepositories.ReferralPostingInput, error) {
	ledgerType, ledgerID, err := referralLedgerOwner(request.OwnerType, request.OwnerID, request.ProviderType)
	if err != nil {
		return walletrepositories.ReferralPostingInput{}, err
	}
	var fields []apperrors.FieldViolation
	if request.AmountKobo <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Amount must be positive."})
	}
	if request.Balance != walletrepositories.ReferralBalanceEarnings && request.Balance != walletrepositories.ReferralBalancePromotional {
		fields = append(fields, apperrors.FieldViolation{Field: "balance", Message: "Balance must be earnings or promotional."})
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "idempotency_key", Message: "Idempotency key is required."})
	}
	if strings.TrimSpace(request.Reference) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "reference", Message: "Reference is required."})
	}
	if len(fields) > 0 {
		return walletrepositories.ReferralPostingInput{}, apperrors.Validation("Check the referral posting.", fields)
	}
	return walletrepositories.ReferralPostingInput{
		OwnerType:      ledgerType,
		OwnerID:        ledgerID,
		Balance:        request.Balance,
		AmountKobo:     request.AmountKobo,
		Currency:       request.Currency,
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		Reference:      strings.TrimSpace(request.Reference),
		Reason:         request.Reason,
	}, nil
}

// referralLedgerOwner maps a referral beneficiary onto the ledger owner key
// the rest of the wallet uses for that person's account.
func referralLedgerOwner(ownerType, ownerID, providerType string) (string, string, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return "", "", apperrors.Validation("Check the referral posting.", []apperrors.FieldViolation{{Field: "owner_id", Message: "Owner id is required."}})
	}
	switch ownerType {
	case walletmodels.OwnerTypeCustomer:
		return walletmodels.OwnerTypeCustomer, ownerID, nil
	case walletmodels.OwnerTypeProvider:
		if !referralProviderTypes[providerType] {
			return "", "", apperrors.Validation("Check the referral posting.", []apperrors.FieldViolation{{Field: "provider_type", Message: "Provider type must be truck_provider, dispatch or taxi."}})
		}
		return walletmodels.OwnerTypeProvider, providerType + ":" + ownerID, nil
	}
	return "", "", apperrors.Validation("Check the referral posting.", []apperrors.FieldViolation{{Field: "owner_type", Message: "Owner type must be customer or provider."}})
}

// ReferralEligibilityChecker is referral-service's cash-out check
// (referralcontract.Client in production).
type ReferralEligibilityChecker interface {
	CheckWithdrawal(ctx context.Context, request referralcontract.WithdrawalEligibilityRequest) (referralcontract.WithdrawalEligibility, error)
}

// RequestReferralWithdrawal pays a driver's referral earnings out to one of
// their bank accounts, with their transaction PIN, through the same Paystack
// transfer as any withdrawal. It differs from RequestWithdrawal -- which only
// ever pays job earnings -- in three ways: the money comes only from the
// referral earnings balance, the minimum is higher (₦5,000 by default), and
// referral-service must first confirm the driver is not under review. It
// fails closed: no answer from referral-service means no payout. Customers
// cannot use it; they have no identity verification yet.
func (s *WalletService) RequestReferralWithdrawal(ctx context.Context, input RequestWithdrawalInput) (walletmodels.Withdrawal, error) {
	if !referralProviderTypes[input.ProviderType] {
		return walletmodels.Withdrawal{}, apperrors.Forbidden("Referral earnings can only be withdrawn by drivers and riders.", nil)
	}
	if s.referralEligibility == nil || len(s.referralPepper) == 0 {
		return walletmodels.Withdrawal{}, apperrors.Unavailable("Referral earnings withdrawals are not available yet.", nil)
	}
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if input.AmountKobo < s.referralWithdrawalMinKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo",
			Message: fmt.Sprintf("The minimum referral earnings withdrawal is ₦%s.", nairaAmount(s.referralWithdrawalMinKobo))})
	}
	if s.withdrawalMaxKobo > 0 && input.AmountKobo > s.withdrawalMaxKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Withdrawal amount exceeds the maximum."})
	}
	if input.BankAccountID == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "bank_account_id", Message: "Bank account is required."})
	}
	if input.Pin == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "pin", Message: "Transaction PIN is required."})
	}
	if len(fields) > 0 {
		return walletmodels.Withdrawal{}, apperrors.Validation("Check your withdrawal request.", fields)
	}
	if err := s.VerifyTransactionPin(ctx, input.ProviderType, input.ProviderID, input.Pin); err != nil {
		return walletmodels.Withdrawal{}, err
	}
	// A retry of a withdrawal already made returns it as it is, without
	// asking again (a review opened since must not hide a payout that exists).
	if existing, err := s.repository.GetWithdrawalByIdempotency(ctx, input.ProviderType, input.ProviderID, input.IdempotencyKey); err == nil {
		if existing.FundingSource != walletmodels.WithdrawalFromReferralEarnings {
			return walletmodels.Withdrawal{}, apperrors.Conflict("Withdrawal idempotency key was already used for a different kind of withdrawal.", nil)
		}
		return existing, nil
	} else if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeNotFound {
		return walletmodels.Withdrawal{}, err
	}
	account, err := s.repository.GetProviderBankAccount(ctx, input.ProviderType, input.ProviderID, input.BankAccountID)
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	answer, err := s.referralEligibility.CheckWithdrawal(ctx, referralcontract.WithdrawalEligibilityRequest{
		ActorType:       referralcontract.ActorTypeForWalletProvider(input.ProviderType),
		ActorID:         input.ProviderID,
		AmountKobo:      input.AmountKobo,
		BankAccountHash: referralcontract.HashBankAccount(s.referralPepper, account.BankCode, account.AccountNumber),
	})
	if err != nil {
		return walletmodels.Withdrawal{}, apperrors.Unavailable("Referral earnings cannot be withdrawn right now. Please try again shortly.", err)
	}
	if !answer.Eligible {
		return walletmodels.Withdrawal{}, referralIneligible(answer.Reason)
	}
	if !s.paystackBalanceCanCover(ctx, walletmodels.DefaultCurrency(input.Currency), input.AmountKobo) {
		return walletmodels.Withdrawal{}, apperrors.Conflict("Paystack Balance is not enough to process this withdrawal yet.", nil)
	}
	withdrawal, created, err := s.repository.RequestWithdrawal(ctx, walletrepositories.CreateWithdrawalInput{
		Reference:      walletrepositories.Reference("wd"),
		ProviderType:   input.ProviderType,
		ProviderID:     input.ProviderID,
		BankAccountID:  input.BankAccountID,
		AmountKobo:     input.AmountKobo,
		Currency:       walletmodels.DefaultCurrency(input.Currency),
		IdempotencyKey: input.IdempotencyKey,
		FundingSource:  walletmodels.WithdrawalFromReferralEarnings,
	})
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	if !created {
		return withdrawal, nil
	}
	return s.startWithdrawalTransfer(ctx, withdrawal, account.RecipientCode, "Karry Go referral earnings withdrawal ")
}

// referralIneligible explains a refused referral cash-out to the driver.
func referralIneligible(reason string) error {
	switch reason {
	case referralcontract.IneligibleUnderReview:
		return apperrors.New(http.StatusForbidden, "referral_under_review", "Your referral earnings are under review and cannot be withdrawn yet.", nil)
	case referralcontract.IneligibleBlocked:
		return apperrors.New(http.StatusForbidden, "referral_blocked", "Your account can no longer withdraw referral earnings.", nil)
	case referralcontract.IneligibleUnknownAccount:
		return apperrors.Conflict("Your referral account is still being set up. Please try again shortly.", nil)
	}
	return apperrors.Forbidden("Referral earnings cannot be withdrawn right now.", nil)
}

// nairaAmount formats kobo as naira with thousands separators: 500000 -> "5,000".
func nairaAmount(kobo int64) string {
	naira := kobo / 100
	digits := fmt.Sprintf("%d", naira)
	var out []byte
	for i, d := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, d)
	}
	if rest := kobo % 100; rest != 0 {
		return fmt.Sprintf("%s.%02d", out, rest)
	}
	return string(out)
}
