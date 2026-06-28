package walletusecases

import (
	"context"
	"net/mail"
	"strconv"
	"strings"

	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type WalletService struct {
	repository        *walletrepositories.PostgresWalletRepository
	paystack          walletclients.PaystackClient
	notifier          *walletclients.WalletNotifier
	defaultCurrency   string
	platformFeeBPS    int64
	callbackBaseURL   string
	withdrawalMinKobo int64
	withdrawalMaxKobo int64
}

type Options struct {
	Repository        *walletrepositories.PostgresWalletRepository
	Paystack          walletclients.PaystackClient
	Notifier          *walletclients.WalletNotifier
	DefaultCurrency   string
	PlatformFeeBPS    int64
	CallbackBaseURL   string
	WithdrawalMinKobo int64
	WithdrawalMaxKobo int64
}

type TopUpInput struct {
	CustomerID     string
	CustomerEmail  string
	AmountKobo     int64
	Currency       string
	IdempotencyKey string
}

type CreatePaymentIntentInput struct {
	SourceService   string                 `json:"source_service"`
	SourceReference string                 `json:"source_reference"`
	CustomerID      string                 `json:"customer_id"`
	CustomerEmail   string                 `json:"customer_email"`
	ProviderID      string                 `json:"provider_id"`
	ProviderType    string                 `json:"provider_type"`
	AmountKobo      int64                  `json:"amount_kobo"`
	Currency        string                 `json:"currency"`
	PaymentMethod   string                 `json:"payment_method"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	IdempotencyKey  string                 `json:"idempotency_key,omitempty"`
}

type RegisterBankAccountInput struct {
	ProviderType  string
	ProviderID    string
	BankCode      string
	BankName      string
	AccountNumber string
	Currency      string
}

type ResolveBankAccountInput struct {
	AccountNumber string
	BankCode      string
}

type ResolvedBankAccount struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankID        int64  `json:"bank_id"`
}

type RequestWithdrawalInput struct {
	ProviderType   string
	ProviderID     string
	BankAccountID  string
	AmountKobo     int64
	Currency       string
	IdempotencyKey string
}

type RefundInput struct {
	PaymentReference string
	AmountKobo       int64
	Currency         string
	Reason           string
	IdempotencyKey   string
}

func NewWalletService(opts Options) *WalletService {
	notifier := opts.Notifier
	if notifier == nil {
		notifier = walletclients.NewWalletNotifier("", nil)
	}
	return &WalletService{
		repository:        opts.Repository,
		paystack:          opts.Paystack,
		notifier:          notifier,
		defaultCurrency:   walletmodels.DefaultCurrency(opts.DefaultCurrency),
		platformFeeBPS:    opts.PlatformFeeBPS,
		callbackBaseURL:   strings.TrimRight(opts.CallbackBaseURL, "/"),
		withdrawalMinKobo: opts.WithdrawalMinKobo,
		withdrawalMaxKobo: opts.WithdrawalMaxKobo,
	}
}

func (s *WalletService) WalletSummary(ctx context.Context, ownerType string, ownerID string) (walletmodels.WalletSummary, error) {
	return s.repository.GetWalletSummary(ctx, ownerType, ownerID, s.defaultCurrency)
}

func (s *WalletService) WalletTransactions(ctx context.Context, ownerType string, ownerID string, limit int) ([]walletmodels.WalletTransaction, error) {
	return s.repository.ListWalletTransactions(ctx, ownerType, ownerID, s.defaultCurrency, limit)
}

func (s *WalletService) PaymentByReference(ctx context.Context, reference string) (walletmodels.PaymentIntent, error) {
	if strings.TrimSpace(reference) == "" {
		return walletmodels.PaymentIntent{}, apperrors.BadRequest("Payment reference is required.", nil)
	}
	return s.repository.GetPaymentIntentByReference(ctx, reference)
}

func (s *WalletService) CreateTopUp(ctx context.Context, input TopUpInput) (walletmodels.PaymentIntent, error) {
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if strings.TrimSpace(input.CustomerID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "customer_id", Message: "Customer id is required."})
	}
	if !validEmail(input.CustomerEmail) {
		fields = append(fields, apperrors.FieldViolation{Field: "customer_email", Message: "A valid profile email is required before payment."})
	}
	if len(fields) > 0 {
		return walletmodels.PaymentIntent{}, apperrors.Validation("Check your payment request.", fields)
	}

	currency := walletmodels.DefaultCurrency(input.Currency)
	paystackRef := walletrepositories.Reference("psk")
	sourceRef := "topup:" + input.CustomerID + ":" + input.IdempotencyKey
	intent, existed, err := s.repository.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		Reference:       walletrepositories.Reference("pi"),
		SourceService:   "payment-wallet-service",
		SourceReference: sourceRef,
		CustomerID:      input.CustomerID,
		CustomerEmail:   input.CustomerEmail,
		AmountKobo:      input.AmountKobo,
		Currency:        currency,
		PaymentMethod:   walletmodels.PaymentMethodPaystack,
		Status:          walletmodels.PaymentStatusPending,
		PaystackRef:     paystackRef,
		Metadata:        map[string]interface{}{"kind": "wallet_topup"},
	})
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	if existed && intent.AuthorizationURL != "" {
		return intent, nil
	}
	if intent.PaystackRef != "" {
		paystackRef = intent.PaystackRef
	}
	initialized, err := s.paystack.InitializeTransaction(ctx, walletclients.InitializeTransactionRequest{
		Email:       input.CustomerEmail,
		AmountKobo:  input.AmountKobo,
		Reference:   paystackRef,
		CallbackURL: s.callbackURL("topups", paystackRef),
		Metadata: map[string]interface{}{
			"kind":        "wallet_topup",
			"customer_id": input.CustomerID,
		},
	})
	if err != nil {
		return walletmodels.PaymentIntent{}, apperrors.Unavailable("Paystack payment could not be initialized.", err)
	}
	return s.repository.UpdatePaymentIntentPaystack(ctx, intent.ID, initialized.Reference, initialized.AuthorizationURL, initialized.AccessCode)
}

// VerifyTopUp verifies a customer top-up directly with Paystack and credits the
// wallet if the charge succeeded. It lets the app confirm a top-up immediately
// after checkout without depending on the asynchronous Paystack webhook (which
// cannot reach local/dev hosts). Safe to call repeatedly: the underlying credit
// is idempotent.
func (s *WalletService) VerifyTopUp(ctx context.Context, customerID string, reference string) (walletmodels.PaymentIntent, error) {
	if strings.TrimSpace(reference) == "" {
		return walletmodels.PaymentIntent{}, apperrors.BadRequest("Payment reference is required.", nil)
	}
	intent, err := s.repository.GetPaymentIntentByReference(ctx, reference)
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	if intent.CustomerID != customerID {
		return walletmodels.PaymentIntent{}, apperrors.Forbidden("You do not have access to this payment.", nil)
	}
	if intent.PaymentMethod != walletmodels.PaymentMethodPaystack || intent.PaystackRef == "" {
		return walletmodels.PaymentIntent{}, apperrors.Conflict("This payment cannot be verified.", nil)
	}
	// Already credited — return the current state without re-verifying.
	if intent.Status == walletmodels.PaymentStatusCompleted {
		return intent, nil
	}

	verified, err := s.paystack.VerifyTransaction(ctx, intent.PaystackRef)
	if err != nil {
		return walletmodels.PaymentIntent{}, apperrors.Unavailable("Paystack transaction could not be verified.", err)
	}
	if verified.Status != "success" {
		// Not paid yet (abandoned or pending) — report the current intent state.
		return intent, nil
	}
	if verified.AmountKobo != intent.AmountKobo || walletmodels.DefaultCurrency(verified.Currency) != intent.Currency {
		return walletmodels.PaymentIntent{}, apperrors.Conflict("Paystack transaction amount does not match payment intent.", nil)
	}
	applied, err := s.repository.ApplyPaystackChargeSuccess(ctx, intent.PaystackRef)
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	// The webhook may also deliver charge.success; the idempotency key derived
	// from the intent reference dedupes the two notifications.
	s.notifyChargeSuccess(ctx, applied)
	return applied, nil
}

func (s *WalletService) CreatePaymentIntent(ctx context.Context, input CreatePaymentIntentInput) (walletmodels.PaymentIntent, error) {
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if strings.TrimSpace(input.SourceService) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "source_service", Message: "Source service is required."})
	}
	if strings.TrimSpace(input.SourceReference) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "source_reference", Message: "Source reference is required."})
	}
	if strings.TrimSpace(input.CustomerID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "customer_id", Message: "Customer id is required."})
	}
	if input.PaymentMethod != walletmodels.PaymentMethodWallet && input.PaymentMethod != walletmodels.PaymentMethodPaystack {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Payment method must be wallet or paystack."})
	}
	if input.PaymentMethod == walletmodels.PaymentMethodPaystack && !validEmail(input.CustomerEmail) {
		fields = append(fields, apperrors.FieldViolation{Field: "customer_email", Message: "A valid customer email is required for Paystack."})
	}
	if len(fields) > 0 {
		return walletmodels.PaymentIntent{}, apperrors.Validation("Check your payment request.", fields)
	}

	currency := walletmodels.DefaultCurrency(input.Currency)
	status := walletmodels.PaymentStatusRequiresPay
	paystackRef := ""
	if input.PaymentMethod == walletmodels.PaymentMethodPaystack {
		status = walletmodels.PaymentStatusPending
		paystackRef = walletrepositories.Reference("psk")
	}
	intent, existed, err := s.repository.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		Reference:       walletrepositories.Reference("pi"),
		SourceService:   input.SourceService,
		SourceReference: input.SourceReference,
		CustomerID:      input.CustomerID,
		CustomerEmail:   input.CustomerEmail,
		ProviderID:      input.ProviderID,
		ProviderType:    input.ProviderType,
		AmountKobo:      input.AmountKobo,
		PlatformFeeKobo: walletmodels.CalculatePlatformFee(input.AmountKobo, s.platformFeeBPS),
		Currency:        currency,
		PaymentMethod:   input.PaymentMethod,
		Status:          status,
		PaystackRef:     paystackRef,
		Metadata:        input.Metadata,
	})
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	if input.PaymentMethod != walletmodels.PaymentMethodPaystack || (existed && intent.AuthorizationURL != "") {
		return intent, nil
	}
	if intent.PaystackRef != "" {
		paystackRef = intent.PaystackRef
	}
	initialized, err := s.paystack.InitializeTransaction(ctx, walletclients.InitializeTransactionRequest{
		Email:       input.CustomerEmail,
		AmountKobo:  input.AmountKobo,
		Reference:   paystackRef,
		CallbackURL: s.callbackURL("payments", paystackRef),
		Metadata: map[string]interface{}{
			"source_service":   input.SourceService,
			"source_reference": input.SourceReference,
			"customer_id":      input.CustomerID,
			"provider_id":      input.ProviderID,
			"provider_type":    input.ProviderType,
		},
	})
	if err != nil {
		return walletmodels.PaymentIntent{}, apperrors.Unavailable("Paystack payment could not be initialized.", err)
	}
	return s.repository.UpdatePaymentIntentPaystack(ctx, intent.ID, initialized.Reference, initialized.AuthorizationURL, initialized.AccessCode)
}

func (s *WalletService) PayFromWallet(ctx context.Context, paymentIntentID string, idempotencyKey string) (walletmodels.PaymentIntent, error) {
	if idempotencyKey == "" {
		return walletmodels.PaymentIntent{}, apperrors.Validation("Check your payment request.", []apperrors.FieldViolation{{Field: "idempotency_key", Message: "Idempotency key is required."}})
	}
	return s.repository.HoldPaymentFromWallet(ctx, paymentIntentID, idempotencyKey)
}

func (s *WalletService) CompleteJob(ctx context.Context, sourceService string, sourceReference string) (walletmodels.PaymentIntent, error) {
	if sourceService == "" || sourceReference == "" {
		return walletmodels.PaymentIntent{}, apperrors.BadRequest("Source service and reference are required.", nil)
	}
	return s.repository.CompleteJob(ctx, sourceService, sourceReference)
}

func (s *WalletService) RegisterBankAccount(ctx context.Context, input RegisterBankAccountInput) (walletmodels.ProviderBankAccount, error) {
	fields := []apperrors.FieldViolation{}
	if input.ProviderType == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "provider_type", Message: "Provider type is required."})
	}
	if input.ProviderID == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "provider_id", Message: "Provider id is required."})
	}
	if input.BankCode == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "bank_code", Message: "Bank code is required."})
	}
	if input.AccountNumber == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "account_number", Message: "Account number is required."})
	}
	if len(fields) > 0 {
		return walletmodels.ProviderBankAccount{}, apperrors.Validation("Check your bank details.", fields)
	}
	resolved, err := s.paystack.ResolveBankAccount(ctx, walletclients.ResolveBankAccountRequest{
		AccountNumber: input.AccountNumber,
		BankCode:      input.BankCode,
	})
	if err != nil {
		return walletmodels.ProviderBankAccount{}, apperrors.Unavailable("Bank account could not be verified.", err)
	}
	recipient, err := s.paystack.CreateTransferRecipient(ctx, walletclients.CreateTransferRecipientRequest{
		Name:          resolved.AccountName,
		AccountNumber: input.AccountNumber,
		BankCode:      input.BankCode,
		Currency:      walletmodels.DefaultCurrency(input.Currency),
	})
	if err != nil {
		return walletmodels.ProviderBankAccount{}, apperrors.Unavailable("Transfer recipient could not be created.", err)
	}
	return s.repository.CreateProviderBankAccount(ctx, walletmodels.ProviderBankAccount{
		ProviderType:  input.ProviderType,
		ProviderID:    input.ProviderID,
		BankCode:      input.BankCode,
		BankName:      input.BankName,
		AccountNumber: resolved.AccountNumber,
		AccountName:   resolved.AccountName,
		RecipientCode: recipient.RecipientCode,
		Currency:      walletmodels.DefaultCurrency(input.Currency),
	})
}

func (s *WalletService) ListBankAccounts(ctx context.Context, providerType string, providerID string) ([]walletmodels.ProviderBankAccount, error) {
	return s.repository.ListProviderBankAccounts(ctx, providerType, providerID)
}

func (s *WalletService) ResolveBankAccount(ctx context.Context, input ResolveBankAccountInput) (ResolvedBankAccount, error) {
	var fields []apperrors.FieldViolation
	if input.AccountNumber == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "account_number", Message: "Account number is required."})
	}
	if input.BankCode == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "bank_code", Message: "Bank code is required."})
	}
	if len(fields) > 0 {
		return ResolvedBankAccount{}, apperrors.Validation("Check your bank details.", fields)
	}
	resolved, err := s.paystack.ResolveBankAccount(ctx, walletclients.ResolveBankAccountRequest{
		AccountNumber: input.AccountNumber,
		BankCode:      input.BankCode,
	})
	if err != nil {
		return ResolvedBankAccount{}, apperrors.Unavailable("Bank account could not be verified.", err)
	}
	return ResolvedBankAccount{
		AccountNumber: resolved.AccountNumber,
		AccountName:   resolved.AccountName,
		BankID:        resolved.BankID,
	}, nil
}

func (s *WalletService) RequestWithdrawal(ctx context.Context, input RequestWithdrawalInput) (walletmodels.Withdrawal, error) {
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if input.AmountKobo < s.withdrawalMinKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Withdrawal amount is below the minimum."})
	}
	if s.withdrawalMaxKobo > 0 && input.AmountKobo > s.withdrawalMaxKobo {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Withdrawal amount exceeds the maximum."})
	}
	if input.BankAccountID == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "bank_account_id", Message: "Bank account is required."})
	}
	if len(fields) > 0 {
		return walletmodels.Withdrawal{}, apperrors.Validation("Check your withdrawal request.", fields)
	}
	account, err := s.repository.GetProviderBankAccount(ctx, input.ProviderType, input.ProviderID, input.BankAccountID)
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	if !s.paystackBalanceCanCover(ctx, walletmodels.DefaultCurrency(input.Currency), input.AmountKobo) {
		return walletmodels.Withdrawal{}, apperrors.Conflict("Paystack Balance is not enough to process this withdrawal yet.", nil)
	}
	withdrawal, err := s.repository.RequestWithdrawal(ctx, walletrepositories.CreateWithdrawalInput{
		Reference:      walletrepositories.Reference("wd"),
		ProviderType:   input.ProviderType,
		ProviderID:     input.ProviderID,
		BankAccountID:  input.BankAccountID,
		AmountKobo:     input.AmountKobo,
		Currency:       walletmodels.DefaultCurrency(input.Currency),
		IdempotencyKey: input.IdempotencyKey,
	})
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	transfer, err := s.paystack.InitiateTransfer(ctx, walletclients.InitiateTransferRequest{
		AmountKobo:    input.AmountKobo,
		RecipientCode: account.RecipientCode,
		Reason:        "Karry Go provider withdrawal " + withdrawal.Reference,
		Reference:     withdrawal.Reference,
	})
	if err != nil {
		restored, restoreErr := s.repository.CancelWithdrawalReservation(ctx, withdrawal.ID, err.Error())
		if restoreErr != nil {
			return walletmodels.Withdrawal{}, restoreErr
		}
		return restored, apperrors.Unavailable("Paystack transfer could not be started.", err)
	}
	requiresOTP := transfer.Status == "otp" || transfer.Status == "pending_otp"
	return s.repository.MarkWithdrawalTransferStarted(ctx, withdrawal.ID, transfer.TransferCode, stringID(transfer.ID), requiresOTP)
}

func (s *WalletService) RequestRefund(ctx context.Context, input RefundInput) (walletmodels.Refund, error) {
	fields := validateAmountAndIdempotency(input.AmountKobo, input.IdempotencyKey)
	if input.PaymentReference == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_reference", Message: "Payment reference is required."})
	}
	if len(fields) > 0 {
		return walletmodels.Refund{}, apperrors.Validation("Check your refund request.", fields)
	}
	intent, err := s.repository.GetPaymentIntentByReference(ctx, input.PaymentReference)
	if err != nil {
		return walletmodels.Refund{}, err
	}
	refund, err := s.repository.CreateRefundReservation(ctx, walletrepositories.CreateRefundInput{
		Reference:       walletrepositories.Reference("rf"),
		PaymentIntentID: intent.ID,
		AmountKobo:      input.AmountKobo,
		Currency:        walletmodels.DefaultCurrency(input.Currency),
		Reason:          input.Reason,
		IdempotencyKey:  input.IdempotencyKey,
	})
	if err != nil {
		return walletmodels.Refund{}, err
	}
	if intent.PaymentMethod == walletmodels.PaymentMethodWallet {
		return s.repository.MarkWalletRefundProcessed(ctx, refund.ID)
	}
	paystackRefund, err := s.paystack.CreateRefund(ctx, walletclients.CreateRefundRequest{
		TransactionReference: intent.PaystackRef,
		AmountKobo:           input.AmountKobo,
		Currency:             walletmodels.DefaultCurrency(input.Currency),
		CustomerNote:         input.Reason,
		MerchantNote:         refund.Reference,
	})
	if err != nil {
		return walletmodels.Refund{}, apperrors.Unavailable("Paystack refund could not be started.", err)
	}
	return s.repository.MarkRefundProcessing(ctx, refund.ID, paystackRefund.Reference)
}

func (s *WalletService) HandlePaystackWebhook(ctx context.Context, rawBody []byte, signature string) error {
	if !s.paystack.VerifyWebhookSignature(rawBody, signature) {
		return apperrors.Unauthorized("Paystack webhook signature is invalid.", nil)
	}
	event, data, err := walletclients.ParseWebhook(rawBody)
	if err != nil {
		return apperrors.BadRequest("Paystack webhook payload is invalid.", err)
	}
	eventKey := walletclients.WebhookEventKey(event, data)
	stored, err := s.repository.StoreWebhookEvent(ctx, eventKey, event.Event, firstNonEmpty(data.Reference, data.TransferCode), rawBody)
	if err != nil {
		return err
	}
	if !stored {
		return nil
	}

	processErr := s.processPaystackEvent(ctx, event.Event, data)
	if err := s.repository.MarkWebhookProcessed(ctx, eventKey, processErr); err != nil {
		return err
	}
	return processErr
}

func (s *WalletService) processPaystackEvent(ctx context.Context, eventType string, data walletclients.WebhookData) error {
	switch eventType {
	case "charge.success":
		verified, err := s.paystack.VerifyTransaction(ctx, data.Reference)
		if err != nil {
			return apperrors.Unavailable("Paystack transaction could not be verified.", err)
		}
		if verified.Status != "success" {
			return apperrors.Conflict("Paystack transaction is not successful.", nil)
		}
		intent, err := s.repository.GetPaymentIntentByPaystackReference(ctx, data.Reference)
		if err != nil {
			return err
		}
		if verified.AmountKobo != intent.AmountKobo || walletmodels.DefaultCurrency(verified.Currency) != intent.Currency {
			return apperrors.Conflict("Paystack transaction amount does not match payment intent.", nil)
		}
		applied, err := s.repository.ApplyPaystackChargeSuccess(ctx, data.Reference)
		if err != nil {
			return err
		}
		s.notifyChargeSuccess(ctx, applied)
		return nil
	case "transfer.success":
		withdrawal, err := s.repository.ApplyTransferSuccess(ctx, data.TransferCode)
		if err != nil {
			return err
		}
		s.notifier.NotifyWithdrawalCompleted(ctx, withdrawal.ProviderID, withdrawal.Reference, withdrawal.AmountKobo)
		return nil
	case "transfer.failed":
		withdrawal, err := s.repository.ApplyTransferFailure(ctx, data.TransferCode, "failed", data.Reason)
		if err != nil {
			return err
		}
		s.notifier.NotifyWithdrawalFailed(ctx, withdrawal.ProviderID, withdrawal.Reference, withdrawal.FailureReason)
		return nil
	case "transfer.reversed":
		withdrawal, err := s.repository.ApplyTransferFailure(ctx, data.TransferCode, "reversed", data.Reason)
		if err != nil {
			return err
		}
		s.notifier.NotifyWithdrawalReversed(ctx, withdrawal.ProviderID, withdrawal.Reference, withdrawal.FailureReason)
		return nil
	case "refund.processed":
		_, err := s.repository.MarkRefundProcessed(ctx, data.Reference)
		return err
	case "refund.failed":
		_, err := s.repository.MarkRefundFailed(ctx, data.Reference, data.Reason)
		return err
	default:
		return nil
	}
}

// notifyChargeSuccess routes a successful charge to the right customer
// notification: a wallet top-up versus a booking/job payment, distinguished by
// the intent metadata "kind" set at creation time.
func (s *WalletService) notifyChargeSuccess(ctx context.Context, intent walletmodels.PaymentIntent) {
	if kind, _ := intent.Metadata["kind"].(string); kind == "wallet_topup" {
		s.notifier.NotifyTopUpSuccess(ctx, intent.CustomerID, intent.Reference, intent.AmountKobo)
		return
	}
	s.notifier.NotifyPaymentSuccess(ctx, intent.CustomerID, intent.Reference, intent.AmountKobo)
}

func (s *WalletService) paystackBalanceCanCover(ctx context.Context, currency string, amountKobo int64) bool {
	balance, err := s.paystack.CheckBalance(ctx)
	if err != nil {
		return false
	}
	for _, item := range balance.Items {
		if walletmodels.DefaultCurrency(item.Currency) == walletmodels.DefaultCurrency(currency) && item.Balance >= amountKobo {
			return true
		}
	}
	return false
}

func (s *WalletService) callbackURL(kind string, reference string) string {
	if s.callbackBaseURL == "" {
		return ""
	}
	return s.callbackBaseURL + "/" + kind + "/" + reference
}

func validateAmountAndIdempotency(amountKobo int64, idempotencyKey string) []apperrors.FieldViolation {
	var fields []apperrors.FieldViolation
	if amountKobo <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "amount_kobo", Message: "Amount must be greater than zero."})
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "idempotency_key", Message: "Idempotency key is required."})
	}
	return fields
}

func validEmail(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	_, err := mail.ParseAddress(value)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringID(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
