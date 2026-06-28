package walletrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

const postedStatus = "posted"

type CreatePaymentIntentInput struct {
	ID               string
	Reference        string
	SourceService    string
	SourceReference  string
	CustomerID       string
	CustomerEmail    string
	ProviderID       string
	ProviderType     string
	AmountKobo       int64
	PlatformFeeKobo  int64
	Currency         string
	PaymentMethod    string
	Status           string
	PaystackRef      string
	AuthorizationURL string
	AccessCode       string
	Metadata         map[string]interface{}
}

type CreateWithdrawalInput struct {
	Reference     string
	ProviderType  string
	ProviderID    string
	BankAccountID string
	AmountKobo    int64
	Currency      string
	IdempotencyKey string
}

type CreateRefundInput struct {
	Reference       string
	PaymentIntentID string
	AmountKobo      int64
	Currency        string
	Reason          string
	IdempotencyKey  string
}

type PostgresWalletRepository struct {
	db *pgxpool.Pool
}

func NewPostgresWalletRepository(db *pgxpool.Pool) *PostgresWalletRepository {
	return &PostgresWalletRepository{db: db}
}

func (r *PostgresWalletRepository) GetWalletSummary(ctx context.Context, ownerType string, ownerID string, currency string) (walletmodels.WalletSummary, error) {
	currency = walletmodels.DefaultCurrency(currency)
	available, err := r.accountBalance(ctx, ownerType, ownerID, availableAccountFor(ownerType), currency)
	if err != nil {
		return walletmodels.WalletSummary{}, err
	}
	pending, err := r.accountBalance(ctx, ownerType, ownerID, pendingAccountFor(ownerType), currency)
	if err != nil {
		return walletmodels.WalletSummary{}, err
	}

	return walletmodels.WalletSummary{
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		Currency:       currency,
		AvailableKobo: available,
		PendingKobo:    pending,
	}, nil
}

func (r *PostgresWalletRepository) ListWalletTransactions(ctx context.Context, ownerType string, ownerID string, currency string, limit int) ([]walletmodels.WalletTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			t.reference,
			t.transaction_type,
			e.side,
			e.amount_kobo,
			e.currency,
			e.memo,
			e.created_at
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.owner_type = $1 AND a.owner_id = $2 AND e.currency = $3
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $4
	`, ownerType, ownerID, walletmodels.DefaultCurrency(currency), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []walletmodels.WalletTransaction
	for rows.Next() {
		var transaction walletmodels.WalletTransaction
		if err := rows.Scan(
			&transaction.Reference,
			&transaction.TransactionType,
			&transaction.Side,
			&transaction.AmountKobo,
			&transaction.Currency,
			&transaction.Memo,
			&transaction.CreatedAt,
		); err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	return transactions, rows.Err()
}

func (r *PostgresWalletRepository) CreatePaymentIntent(ctx context.Context, input CreatePaymentIntentInput) (walletmodels.PaymentIntent, bool, error) {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Reference == "" {
		input.Reference = "pi_" + uuid.NewString()
	}
	if input.Status == "" {
		input.Status = walletmodels.PaymentStatusRequiresPay
	}
	input.Currency = walletmodels.DefaultCurrency(input.Currency)
	metadata, err := json.Marshal(nonNilMap(input.Metadata))
	if err != nil {
		return walletmodels.PaymentIntent{}, false, err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO payment_intents (
			id,
			reference,
			source_service,
			source_reference,
			customer_id,
			customer_email,
			provider_id,
			provider_type,
			amount_kobo,
			platform_fee_kobo,
			currency,
			payment_method,
			status,
			paystack_reference,
			authorization_url,
			access_code,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''), $17)
	`, input.ID, input.Reference, input.SourceService, input.SourceReference, input.CustomerID, input.CustomerEmail, input.ProviderID, input.ProviderType, input.AmountKobo, input.PlatformFeeKobo, input.Currency, input.PaymentMethod, input.Status, input.PaystackRef, input.AuthorizationURL, input.AccessCode, metadata)
	if err != nil {
		if isUniqueViolation(err) {
			existing, getErr := r.GetPaymentIntentBySource(ctx, input.SourceService, input.SourceReference)
			return existing, true, getErr
		}
		return walletmodels.PaymentIntent{}, false, err
	}

	intent, err := r.GetPaymentIntentByReference(ctx, input.Reference)
	return intent, false, err
}

func (r *PostgresWalletRepository) UpdatePaymentIntentPaystack(ctx context.Context, intentID string, paystackRef string, authorizationURL string, accessCode string) (walletmodels.PaymentIntent, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE payment_intents
		SET paystack_reference = $2,
			authorization_url = $3,
			access_code = $4,
			status = $5,
			updated_at = now()
		WHERE id = $1
	`, intentID, paystackRef, authorizationURL, accessCode, walletmodels.PaymentStatusPending)
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	return r.GetPaymentIntentByPaystackReference(ctx, paystackRef)
}

func (r *PostgresWalletRepository) GetPaymentIntentByReference(ctx context.Context, reference string) (walletmodels.PaymentIntent, error) {
	row := r.db.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE reference = $1`, reference)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) GetPaymentIntentByPaystackReference(ctx context.Context, reference string) (walletmodels.PaymentIntent, error) {
	row := r.db.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE paystack_reference = $1`, reference)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) GetPaymentIntentBySource(ctx context.Context, service string, reference string) (walletmodels.PaymentIntent, error) {
	row := r.db.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE source_service = $1 AND source_reference = $2`, service, reference)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) HoldPaymentFromWallet(ctx context.Context, intentID string, idempotencyKey string) (walletmodels.PaymentIntent, error) {
	var intent walletmodels.PaymentIntent
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		intent, err = r.getPaymentIntentForUpdate(ctx, tx, intentID)
		if err != nil {
			return err
		}
		if intent.Status == walletmodels.PaymentStatusHeld || intent.Status == walletmodels.PaymentStatusCompleted {
			return nil
		}
		if intent.PaymentMethod != walletmodels.PaymentMethodWallet {
			return apperrors.BadRequest("Payment intent is not configured for wallet payment.", nil)
		}
		customerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeCustomer, intent.CustomerID, walletmodels.AccountCustomerAvailable, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		escrowAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, jobOwnerID(intent.SourceService, intent.SourceReference), walletmodels.AccountJobEscrow, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		balance, err := r.accountBalanceTx(ctx, tx, customerAccount.ID)
		if err != nil {
			return err
		}
		if balance < intent.AmountKobo {
			return apperrors.Conflict("Wallet balance is not enough for this payment.", nil)
		}

		err = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "wallet_payment_hold",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
			IdempotencyKey:  idempotencyKey,
		}, []walletmodels.LedgerEntry{
			{AccountID: customerAccount.ID, Side: walletmodels.SideDebit, AmountKobo: intent.AmountKobo, Currency: intent.Currency, Memo: "Customer wallet payment hold"},
			{AccountID: escrowAccount.ID, Side: walletmodels.SideCredit, AmountKobo: intent.AmountKobo, Currency: intent.Currency, Memo: "Job escrow funded from wallet"},
		})
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `UPDATE payment_intents SET status = $2, updated_at = now() WHERE id = $1`, intent.ID, walletmodels.PaymentStatusHeld)
		if err != nil {
			return err
		}
		intent.Status = walletmodels.PaymentStatusHeld
		return nil
	})
	return intent, err
}

func (r *PostgresWalletRepository) ApplyPaystackChargeSuccess(ctx context.Context, paystackRef string) (walletmodels.PaymentIntent, error) {
	var intent walletmodels.PaymentIntent
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		intent, err = r.getPaymentIntentByPaystackForUpdate(ctx, tx, paystackRef)
		if err != nil {
			return err
		}
		if intent.Status == walletmodels.PaymentStatusHeld || intent.Status == walletmodels.PaymentStatusCompleted {
			return nil
		}

		receivable, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "paystack", walletmodels.AccountPaystackReceivable, intent.Currency, walletmodels.NormalDebit)
		if err != nil {
			return err
		}
		var creditAccount walletmodels.Account
		nextStatus := walletmodels.PaymentStatusHeld
		memo := "Job escrow funded from Paystack"
		if isTopUpIntent(intent) {
			creditAccount, err = r.ensureAccount(ctx, tx, walletmodels.OwnerTypeCustomer, intent.CustomerID, walletmodels.AccountCustomerAvailable, intent.Currency, walletmodels.NormalCredit)
			nextStatus = walletmodels.PaymentStatusCompleted
			memo = "Customer wallet top-up"
		} else {
			creditAccount, err = r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, jobOwnerID(intent.SourceService, intent.SourceReference), walletmodels.AccountJobEscrow, intent.Currency, walletmodels.NormalCredit)
		}
		if err != nil {
			return err
		}

		err = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "paystack_charge_success",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
			ExternalRef:     paystackRef,
		}, []walletmodels.LedgerEntry{
			{AccountID: receivable.ID, Side: walletmodels.SideDebit, AmountKobo: intent.AmountKobo, Currency: intent.Currency, Memo: "Paystack receivable"},
			{AccountID: creditAccount.ID, Side: walletmodels.SideCredit, AmountKobo: intent.AmountKobo, Currency: intent.Currency, Memo: memo},
		})
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `UPDATE payment_intents SET status = $2, updated_at = now() WHERE id = $1`, intent.ID, nextStatus)
		if err != nil {
			return err
		}
		intent.Status = nextStatus
		return nil
	})
	return intent, err
}

func (r *PostgresWalletRepository) CompleteJob(ctx context.Context, sourceService string, sourceReference string) (walletmodels.PaymentIntent, error) {
	var intent walletmodels.PaymentIntent
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		intent, err = r.getPaymentIntentBySourceForUpdate(ctx, tx, sourceService, sourceReference)
		if err != nil {
			return err
		}
		if intent.Status == walletmodels.PaymentStatusCompleted {
			return nil
		}
		if intent.Status != walletmodels.PaymentStatusHeld {
			return apperrors.Conflict("Payment is not held for settlement.", nil)
		}
		if intent.ProviderID == "" || intent.ProviderType == "" {
			return apperrors.BadRequest("Provider details are required before settlement.", nil)
		}
		if intent.PlatformFeeKobo > intent.AmountKobo {
			return apperrors.Internal("Platform fee cannot exceed payment amount.", nil)
		}

		escrowAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, jobOwnerID(intent.SourceService, intent.SourceReference), walletmodels.AccountJobEscrow, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		providerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(intent.ProviderType, intent.ProviderID), walletmodels.AccountProviderPayable, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		platformAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformRevenue, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}

		entries := []walletmodels.LedgerEntry{
			{AccountID: escrowAccount.ID, Side: walletmodels.SideDebit, AmountKobo: intent.AmountKobo, Currency: intent.Currency, Memo: "Release job escrow"},
			{AccountID: providerAccount.ID, Side: walletmodels.SideCredit, AmountKobo: intent.AmountKobo - intent.PlatformFeeKobo, Currency: intent.Currency, Memo: "Provider earning"},
		}
		if intent.PlatformFeeKobo > 0 {
			entries = append(entries, walletmodels.LedgerEntry{AccountID: platformAccount.ID, Side: walletmodels.SideCredit, AmountKobo: intent.PlatformFeeKobo, Currency: intent.Currency, Memo: "Platform revenue"})
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "job_settlement",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
		}, entries); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE payment_intents SET status = $2, updated_at = now() WHERE id = $1`, intent.ID, walletmodels.PaymentStatusCompleted)
		if err != nil {
			return err
		}
		intent.Status = walletmodels.PaymentStatusCompleted
		return nil
	})
	return intent, err
}

func (r *PostgresWalletRepository) CreateProviderBankAccount(ctx context.Context, account walletmodels.ProviderBankAccount) (walletmodels.ProviderBankAccount, error) {
	if account.ID == "" {
		account.ID = uuid.NewString()
	}
	account.Currency = walletmodels.DefaultCurrency(account.Currency)
	if account.Status == "" {
		account.Status = "active"
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO provider_bank_accounts (
			id,
			provider_type,
			provider_id,
			bank_code,
			bank_name,
			account_number,
			account_name,
			recipient_code,
			currency,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (provider_type, provider_id, bank_code, account_number)
		DO UPDATE SET
			account_name = EXCLUDED.account_name,
			recipient_code = EXCLUDED.recipient_code,
			status = EXCLUDED.status,
			updated_at = now()
	`, account.ID, account.ProviderType, account.ProviderID, account.BankCode, account.BankName, account.AccountNumber, account.AccountName, account.RecipientCode, account.Currency, account.Status)
	if err != nil {
		return walletmodels.ProviderBankAccount{}, err
	}
	return r.GetProviderBankAccountByRecipient(ctx, account.RecipientCode)
}

func (r *PostgresWalletRepository) GetProviderBankAccount(ctx context.Context, providerType string, providerID string, accountID string) (walletmodels.ProviderBankAccount, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_type, provider_id, bank_code, bank_name, account_number, account_name, recipient_code, currency, status, created_at, updated_at
		FROM provider_bank_accounts
		WHERE id = $1 AND provider_type = $2 AND provider_id = $3
	`, accountID, providerType, providerID)
	return scanProviderBankAccount(row)
}

func (r *PostgresWalletRepository) ListProviderBankAccounts(ctx context.Context, providerType string, providerID string) ([]walletmodels.ProviderBankAccount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, provider_type, provider_id, bank_code, bank_name, account_number, account_name, recipient_code, currency, status, created_at, updated_at
		FROM provider_bank_accounts
		WHERE provider_type = $1 AND provider_id = $2
		ORDER BY created_at DESC
	`, providerType, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []walletmodels.ProviderBankAccount{}
	for rows.Next() {
		account, err := scanProviderBankAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *PostgresWalletRepository) GetProviderBankAccountByRecipient(ctx context.Context, recipientCode string) (walletmodels.ProviderBankAccount, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_type, provider_id, bank_code, bank_name, account_number, account_name, recipient_code, currency, status, created_at, updated_at
		FROM provider_bank_accounts
		WHERE recipient_code = $1
	`, recipientCode)
	return scanProviderBankAccount(row)
}

func (r *PostgresWalletRepository) RequestWithdrawal(ctx context.Context, input CreateWithdrawalInput) (walletmodels.Withdrawal, error) {
	if input.Reference == "" {
		input.Reference = "wd_" + uuid.NewString()
	}
	input.Currency = walletmodels.DefaultCurrency(input.Currency)
	if input.IdempotencyKey != "" {
		existing, err := r.GetWithdrawalByIdempotency(ctx, input.ProviderType, input.ProviderID, input.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if apperrors.From(err).Code != apperrors.CodeNotFound {
			return walletmodels.Withdrawal{}, err
		}
	}

	var withdrawal walletmodels.Withdrawal
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		_, err := r.getBankAccountForUpdate(ctx, tx, input.ProviderType, input.ProviderID, input.BankAccountID)
		if err != nil {
			return err
		}
		providerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(input.ProviderType, input.ProviderID), walletmodels.AccountProviderPayable, input.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		pendingAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(input.ProviderType, input.ProviderID), walletmodels.AccountWithdrawalPending, input.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		balance, err := r.accountBalanceTx(ctx, tx, providerAccount.ID)
		if err != nil {
			return err
		}
		if balance < input.AmountKobo {
			return apperrors.Conflict("Provider earnings are not enough for this withdrawal.", nil)
		}

		id := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO withdrawals (
				id,
				reference,
				provider_type,
				provider_id,
				bank_account_id,
				amount_kobo,
				currency,
				status,
				idempotency_key
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, id, input.Reference, input.ProviderType, input.ProviderID, input.BankAccountID, input.AmountKobo, input.Currency, walletmodels.WithdrawalStatusPending, input.IdempotencyKey)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "withdrawal_reserved",
			Status:          postedStatus,
			SourceService:   "payment-wallet-service",
			SourceReference: input.Reference,
			IdempotencyKey:  input.IdempotencyKey,
		}, []walletmodels.LedgerEntry{
			{AccountID: providerAccount.ID, Side: walletmodels.SideDebit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: "Provider withdrawal reserved"},
			{AccountID: pendingAccount.ID, Side: walletmodels.SideCredit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: "Withdrawal pending transfer"},
		}); err != nil {
			return err
		}
		var getErr error
		withdrawal, getErr = r.getWithdrawalForUpdate(ctx, tx, id)
		return getErr
	})
	return withdrawal, err
}

func (r *PostgresWalletRepository) GetWithdrawalByIdempotency(ctx context.Context, providerType string, providerID string, idempotencyKey string) (walletmodels.Withdrawal, error) {
	row := r.db.QueryRow(ctx, selectWithdrawalSQL()+` WHERE provider_type = $1 AND provider_id = $2 AND idempotency_key = $3`, providerType, providerID, idempotencyKey)
	return scanWithdrawal(row)
}

func (r *PostgresWalletRepository) MarkWithdrawalTransferStarted(ctx context.Context, withdrawalID string, transferCode string, transferID string, requiresOTP bool) (walletmodels.Withdrawal, error) {
	status := walletmodels.WithdrawalStatusProcessing
	if requiresOTP {
		status = walletmodels.WithdrawalStatusRequiresOTP
	}
	_, err := r.db.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2,
			paystack_transfer_code = NULLIF($3, ''),
			paystack_transfer_id = NULLIF($4, ''),
			updated_at = now()
		WHERE id = $1
	`, withdrawalID, status, transferCode, transferID)
	if err != nil {
		return walletmodels.Withdrawal{}, err
	}
	row := r.db.QueryRow(ctx, selectWithdrawalSQL()+` WHERE id = $1`, withdrawalID)
	return scanWithdrawal(row)
}

func (r *PostgresWalletRepository) ApplyTransferSuccess(ctx context.Context, transferCode string) (walletmodels.Withdrawal, error) {
	var withdrawal walletmodels.Withdrawal
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		withdrawal, err = r.getWithdrawalByTransferForUpdate(ctx, tx, transferCode)
		if err != nil {
			return err
		}
		if withdrawal.Status == walletmodels.WithdrawalStatusPaid {
			return nil
		}
		pendingAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(withdrawal.ProviderType, withdrawal.ProviderID), walletmodels.AccountWithdrawalPending, withdrawal.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		paystackBalance, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "paystack", walletmodels.AccountPaystackBalance, withdrawal.Currency, walletmodels.NormalDebit)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "withdrawal_paid",
			Status:          postedStatus,
			SourceService:   "payment-wallet-service",
			SourceReference: withdrawal.Reference,
			ExternalRef:     transferCode,
		}, []walletmodels.LedgerEntry{
			{AccountID: pendingAccount.ID, Side: walletmodels.SideDebit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Clear withdrawal pending"},
			{AccountID: paystackBalance.ID, Side: walletmodels.SideCredit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Paystack transfer paid"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE withdrawals SET status = $2, updated_at = now() WHERE id = $1`, withdrawal.ID, walletmodels.WithdrawalStatusPaid)
		if err != nil {
			return err
		}
		withdrawal.Status = walletmodels.WithdrawalStatusPaid
		return nil
	})
	return withdrawal, err
}

func (r *PostgresWalletRepository) ApplyTransferFailure(ctx context.Context, transferCode string, status string, reason string) (walletmodels.Withdrawal, error) {
	var withdrawal walletmodels.Withdrawal
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		withdrawal, err = r.getWithdrawalByTransferForUpdate(ctx, tx, transferCode)
		if err != nil {
			return err
		}
		if withdrawal.Status == walletmodels.WithdrawalStatusFailed || withdrawal.Status == walletmodels.WithdrawalStatusReversed {
			return nil
		}
		pendingAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(withdrawal.ProviderType, withdrawal.ProviderID), walletmodels.AccountWithdrawalPending, withdrawal.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		providerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(withdrawal.ProviderType, withdrawal.ProviderID), walletmodels.AccountProviderPayable, withdrawal.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		nextStatus := walletmodels.WithdrawalStatusFailed
		if status == "reversed" {
			nextStatus = walletmodels.WithdrawalStatusReversed
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "withdrawal_reversed",
			Status:          postedStatus,
			SourceService:   "payment-wallet-service",
			SourceReference: withdrawal.Reference,
			ExternalRef:     transferCode,
		}, []walletmodels.LedgerEntry{
			{AccountID: pendingAccount.ID, Side: walletmodels.SideDebit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Clear failed withdrawal"},
			{AccountID: providerAccount.ID, Side: walletmodels.SideCredit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Restore provider earnings"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE withdrawals SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`, withdrawal.ID, nextStatus, reason)
		if err != nil {
			return err
		}
		withdrawal.Status = nextStatus
		withdrawal.FailureReason = reason
		return nil
	})
	return withdrawal, err
}

func (r *PostgresWalletRepository) CancelWithdrawalReservation(ctx context.Context, withdrawalID string, reason string) (walletmodels.Withdrawal, error) {
	var withdrawal walletmodels.Withdrawal
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		withdrawal, err = r.getWithdrawalForUpdate(ctx, tx, withdrawalID)
		if err != nil {
			return err
		}
		if withdrawal.Status == walletmodels.WithdrawalStatusFailed {
			return nil
		}
		pendingAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(withdrawal.ProviderType, withdrawal.ProviderID), walletmodels.AccountWithdrawalPending, withdrawal.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		providerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider, providerOwnerID(withdrawal.ProviderType, withdrawal.ProviderID), walletmodels.AccountProviderPayable, withdrawal.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "withdrawal_cancelled",
			Status:          postedStatus,
			SourceService:   "payment-wallet-service",
			SourceReference: withdrawal.Reference,
			Metadata:        map[string]interface{}{"reason": reason},
		}, []walletmodels.LedgerEntry{
			{AccountID: pendingAccount.ID, Side: walletmodels.SideDebit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Clear cancelled withdrawal"},
			{AccountID: providerAccount.ID, Side: walletmodels.SideCredit, AmountKobo: withdrawal.AmountKobo, Currency: withdrawal.Currency, Memo: "Restore provider earnings"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE withdrawals SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`, withdrawal.ID, walletmodels.WithdrawalStatusFailed, reason)
		if err != nil {
			return err
		}
		withdrawal.Status = walletmodels.WithdrawalStatusFailed
		withdrawal.FailureReason = reason
		return nil
	})
	return withdrawal, err
}

func (r *PostgresWalletRepository) CreateRefundReservation(ctx context.Context, input CreateRefundInput) (walletmodels.Refund, error) {
	if input.Reference == "" {
		input.Reference = Reference("rf")
	}
	input.Currency = walletmodels.DefaultCurrency(input.Currency)
	if input.IdempotencyKey != "" {
		existing, err := r.GetRefundByIdempotency(ctx, input.PaymentIntentID, input.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if apperrors.From(err).Code != apperrors.CodeNotFound {
			return walletmodels.Refund{}, err
		}
	}

	var refund walletmodels.Refund
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		intent, err := r.getPaymentIntentForUpdate(ctx, tx, input.PaymentIntentID)
		if err != nil {
			return err
		}
		if intent.Status != walletmodels.PaymentStatusHeld {
			return apperrors.Conflict("Only held payments can be refunded automatically in v1.", nil)
		}
		if input.AmountKobo > intent.AmountKobo {
			return apperrors.BadRequest("Refund amount cannot exceed the payment amount.", nil)
		}

		escrowAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, jobOwnerID(intent.SourceService, intent.SourceReference), walletmodels.AccountJobEscrow, input.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		refundPending, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "refunds", walletmodels.AccountRefundPending, input.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		escrowBalance, err := r.accountBalanceTx(ctx, tx, escrowAccount.ID)
		if err != nil {
			return err
		}
		if escrowBalance < input.AmountKobo {
			return apperrors.Conflict("Escrow balance is not enough for this refund.", nil)
		}

		id := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO refunds (
				id,
				reference,
				payment_intent_id,
				amount_kobo,
				currency,
				reason,
				status,
				idempotency_key
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, id, input.Reference, input.PaymentIntentID, input.AmountKobo, input.Currency, input.Reason, walletmodels.RefundStatusPending, input.IdempotencyKey)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "refund_reserved",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
			IdempotencyKey:  input.IdempotencyKey,
		}, []walletmodels.LedgerEntry{
			{AccountID: escrowAccount.ID, Side: walletmodels.SideDebit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: "Refund reserved from escrow"},
			{AccountID: refundPending.ID, Side: walletmodels.SideCredit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: "Refund pending Paystack processing"},
		}); err != nil {
			return err
		}
		nextStatus := walletmodels.PaymentStatusPartRefunded
		if input.AmountKobo == intent.AmountKobo {
			nextStatus = walletmodels.PaymentStatusRefunded
		}
		if _, err := tx.Exec(ctx, `UPDATE payment_intents SET status = $2, updated_at = now() WHERE id = $1`, intent.ID, nextStatus); err != nil {
			return err
		}
		var getErr error
		refund, getErr = r.getRefundForUpdate(ctx, tx, id)
		return getErr
	})
	return refund, err
}

func (r *PostgresWalletRepository) GetRefundByIdempotency(ctx context.Context, paymentIntentID string, idempotencyKey string) (walletmodels.Refund, error) {
	row := r.db.QueryRow(ctx, selectRefundSQL()+` WHERE payment_intent_id = $1 AND idempotency_key = $2`, paymentIntentID, idempotencyKey)
	return scanRefund(row)
}

func (r *PostgresWalletRepository) MarkRefundProcessing(ctx context.Context, refundID string, paystackRefundRef string) (walletmodels.Refund, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE refunds
		SET status = $2,
			paystack_refund_reference = NULLIF($3, ''),
			updated_at = now()
		WHERE id = $1
	`, refundID, walletmodels.RefundStatusProcessing, paystackRefundRef)
	if err != nil {
		return walletmodels.Refund{}, err
	}
	row := r.db.QueryRow(ctx, selectRefundSQL()+` WHERE id = $1`, refundID)
	return scanRefund(row)
}

func (r *PostgresWalletRepository) MarkRefundProcessed(ctx context.Context, paystackRefundRef string) (walletmodels.Refund, error) {
	var refund walletmodels.Refund
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		refund, err = r.getRefundByPaystackForUpdate(ctx, tx, paystackRefundRef)
		if err != nil {
			return err
		}
		if refund.Status == walletmodels.RefundStatusProcessed {
			return nil
		}
		refundPending, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "refunds", walletmodels.AccountRefundPending, refund.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		paystackBalance, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "paystack", walletmodels.AccountPaystackBalance, refund.Currency, walletmodels.NormalDebit)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "refund_processed",
			Status:          postedStatus,
			SourceService:   "payment-wallet-service",
			SourceReference: refund.Reference,
			ExternalRef:     paystackRefundRef,
		}, []walletmodels.LedgerEntry{
			{AccountID: refundPending.ID, Side: walletmodels.SideDebit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Clear refund pending"},
			{AccountID: paystackBalance.ID, Side: walletmodels.SideCredit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Paystack refund paid"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refunds SET status = $2, updated_at = now() WHERE id = $1`, refund.ID, walletmodels.RefundStatusProcessed)
		if err != nil {
			return err
		}
		refund.Status = walletmodels.RefundStatusProcessed
		return nil
	})
	return refund, err
}

func (r *PostgresWalletRepository) MarkRefundFailed(ctx context.Context, paystackRefundRef string, reason string) (walletmodels.Refund, error) {
	var refund walletmodels.Refund
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		refund, err = r.getRefundByPaystackForUpdate(ctx, tx, paystackRefundRef)
		if err != nil {
			return err
		}
		if refund.Status == walletmodels.RefundStatusFailed {
			return nil
		}
		intent, err := r.getPaymentIntentForUpdate(ctx, tx, refund.PaymentIntentID)
		if err != nil {
			return err
		}
		refundPending, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "refunds", walletmodels.AccountRefundPending, refund.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		escrowAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob, jobOwnerID(intent.SourceService, intent.SourceReference), walletmodels.AccountJobEscrow, refund.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "refund_failed",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
			ExternalRef:     paystackRefundRef,
			Metadata:        map[string]interface{}{"reason": reason},
		}, []walletmodels.LedgerEntry{
			{AccountID: refundPending.ID, Side: walletmodels.SideDebit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Clear failed refund pending"},
			{AccountID: escrowAccount.ID, Side: walletmodels.SideCredit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Restore job escrow after failed refund"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refunds SET status = $2, updated_at = now() WHERE id = $1`, refund.ID, walletmodels.RefundStatusFailed)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE payment_intents SET status = $2, updated_at = now() WHERE id = $1`, intent.ID, walletmodels.PaymentStatusHeld)
		if err != nil {
			return err
		}
		refund.Status = walletmodels.RefundStatusFailed
		return nil
	})
	return refund, err
}

func (r *PostgresWalletRepository) MarkWalletRefundProcessed(ctx context.Context, refundID string) (walletmodels.Refund, error) {
	var refund walletmodels.Refund
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		refund, err = r.getRefundForUpdate(ctx, tx, refundID)
		if err != nil {
			return err
		}
		if refund.Status == walletmodels.RefundStatusProcessed {
			return nil
		}
		intent, err := r.getPaymentIntentForUpdate(ctx, tx, refund.PaymentIntentID)
		if err != nil {
			return err
		}
		refundPending, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeSystem, "refunds", walletmodels.AccountRefundPending, refund.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		customerAccount, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeCustomer, intent.CustomerID, walletmodels.AccountCustomerAvailable, refund.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              uuid.NewString(),
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: "wallet_refund_processed",
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
		}, []walletmodels.LedgerEntry{
			{AccountID: refundPending.ID, Side: walletmodels.SideDebit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Clear wallet refund pending"},
			{AccountID: customerAccount.ID, Side: walletmodels.SideCredit, AmountKobo: refund.AmountKobo, Currency: refund.Currency, Memo: "Refund to customer wallet"},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refunds SET status = $2, updated_at = now() WHERE id = $1`, refund.ID, walletmodels.RefundStatusProcessed)
		if err != nil {
			return err
		}
		refund.Status = walletmodels.RefundStatusProcessed
		return nil
	})
	return refund, err
}

func (r *PostgresWalletRepository) StoreWebhookEvent(ctx context.Context, eventKey string, eventType string, reference string, payload []byte) (bool, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO paystack_webhook_events (id, event_key, event_type, reference, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), eventKey, eventType, reference, payload)
	if err != nil {
		if isUniqueViolation(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *PostgresWalletRepository) MarkWebhookProcessed(ctx context.Context, eventKey string, processingErr error) error {
	var message *string
	if processingErr != nil {
		value := processingErr.Error()
		message = &value
	}
	_, err := r.db.Exec(ctx, `
		UPDATE paystack_webhook_events
		SET processed_at = now(),
			processing_error = $2
		WHERE event_key = $1
	`, eventKey, message)
	return err
}

func (r *PostgresWalletRepository) accountBalance(ctx context.Context, ownerType string, ownerID string, accountType string, currency string) (int64, error) {
	if accountType == "" {
		return 0, nil
	}
	row := r.db.QueryRow(ctx, accountBalanceSQL()+` WHERE a.owner_type = $1 AND a.owner_id = $2 AND a.account_type = $3 AND a.currency = $4`, ownerType, ownerID, accountType, walletmodels.DefaultCurrency(currency))
	var balance int64
	if err := row.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *PostgresWalletRepository) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (r *PostgresWalletRepository) ensureAccount(ctx context.Context, tx pgx.Tx, ownerType string, ownerID string, accountType string, currency string, normalBalance string) (walletmodels.Account, error) {
	id := uuid.NewString()
	row := tx.QueryRow(ctx, `
		INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (owner_type, owner_id, account_type, currency)
		DO UPDATE SET updated_at = wallet_accounts.updated_at
		RETURNING id::text, owner_type, owner_id, account_type, currency, normal_balance, status, created_at, updated_at
	`, id, ownerType, ownerID, accountType, walletmodels.DefaultCurrency(currency), normalBalance)
	return scanAccount(row)
}

func (r *PostgresWalletRepository) accountBalanceTx(ctx context.Context, tx pgx.Tx, accountID string) (int64, error) {
	if _, err := tx.Exec(ctx, `SELECT id FROM wallet_accounts WHERE id = $1 FOR UPDATE`, accountID); err != nil {
		return 0, err
	}
	row := tx.QueryRow(ctx, accountBalanceSQL()+` WHERE a.id = $1`, accountID)
	var balance int64
	if err := row.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *PostgresWalletRepository) postLedgerTransaction(ctx context.Context, tx pgx.Tx, transaction walletmodels.LedgerTransaction, entries []walletmodels.LedgerEntry) error {
	if err := walletmodels.ValidateBalanced(entries); err != nil {
		return err
	}
	metadata, err := json.Marshal(nonNilMap(transaction.Metadata))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_transactions (
			id,
			reference,
			transaction_type,
			status,
			source_service,
			source_reference,
			idempotency_key,
			external_reference,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, transaction.ID, transaction.Reference, transaction.TransactionType, transaction.Status, transaction.SourceService, transaction.SourceReference, transaction.IdempotencyKey, transaction.ExternalRef, metadata)
	if err != nil {
		if isUniqueViolation(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		_, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, transaction.ID, entry.AccountID, entry.Side, entry.AmountKobo, walletmodels.DefaultCurrency(entry.Currency), entry.Memo)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresWalletRepository) getPaymentIntentForUpdate(ctx context.Context, tx pgx.Tx, id string) (walletmodels.PaymentIntent, error) {
	row := tx.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE id = $1 FOR UPDATE`, id)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) getPaymentIntentByPaystackForUpdate(ctx context.Context, tx pgx.Tx, paystackRef string) (walletmodels.PaymentIntent, error) {
	row := tx.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE paystack_reference = $1 FOR UPDATE`, paystackRef)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) getPaymentIntentBySourceForUpdate(ctx context.Context, tx pgx.Tx, service string, reference string) (walletmodels.PaymentIntent, error) {
	row := tx.QueryRow(ctx, selectPaymentIntentSQL()+` WHERE source_service = $1 AND source_reference = $2 FOR UPDATE`, service, reference)
	return scanPaymentIntent(row)
}

func (r *PostgresWalletRepository) getBankAccountForUpdate(ctx context.Context, tx pgx.Tx, providerType string, providerID string, accountID string) (walletmodels.ProviderBankAccount, error) {
	row := tx.QueryRow(ctx, `
		SELECT id::text, provider_type, provider_id, bank_code, bank_name, account_number, account_name, recipient_code, currency, status, created_at, updated_at
		FROM provider_bank_accounts
		WHERE id = $1 AND provider_type = $2 AND provider_id = $3
		FOR UPDATE
	`, accountID, providerType, providerID)
	return scanProviderBankAccount(row)
}

func (r *PostgresWalletRepository) getWithdrawalForUpdate(ctx context.Context, tx pgx.Tx, id string) (walletmodels.Withdrawal, error) {
	row := tx.QueryRow(ctx, selectWithdrawalSQL()+` WHERE id = $1 FOR UPDATE`, id)
	return scanWithdrawal(row)
}

func (r *PostgresWalletRepository) getWithdrawalByTransferForUpdate(ctx context.Context, tx pgx.Tx, transferCode string) (walletmodels.Withdrawal, error) {
	row := tx.QueryRow(ctx, selectWithdrawalSQL()+` WHERE paystack_transfer_code = $1 FOR UPDATE`, transferCode)
	return scanWithdrawal(row)
}

func (r *PostgresWalletRepository) getRefundForUpdate(ctx context.Context, tx pgx.Tx, id string) (walletmodels.Refund, error) {
	row := tx.QueryRow(ctx, selectRefundSQL()+` WHERE id = $1 FOR UPDATE`, id)
	return scanRefund(row)
}

func (r *PostgresWalletRepository) getRefundByPaystackForUpdate(ctx context.Context, tx pgx.Tx, paystackRefundRef string) (walletmodels.Refund, error) {
	row := tx.QueryRow(ctx, selectRefundSQL()+` WHERE paystack_refund_reference = $1 FOR UPDATE`, paystackRefundRef)
	return scanRefund(row)
}

func selectPaymentIntentSQL() string {
	return `
		SELECT
			id::text,
			reference,
			source_service,
			source_reference,
			customer_id,
			customer_email,
			provider_id,
			provider_type,
			amount_kobo,
			platform_fee_kobo,
			currency,
			payment_method,
			status,
			COALESCE(paystack_reference, ''),
			COALESCE(authorization_url, ''),
			COALESCE(access_code, ''),
			metadata,
			created_at,
			updated_at
		FROM payment_intents`
}

func selectWithdrawalSQL() string {
	return `
		SELECT
			id::text,
			reference,
			provider_type,
			provider_id,
			bank_account_id::text,
			amount_kobo,
			currency,
			status,
			COALESCE(paystack_transfer_code, ''),
			COALESCE(failure_reason, ''),
			created_at,
			updated_at
		FROM withdrawals`
}

func selectRefundSQL() string {
	return `
		SELECT
			id::text,
			reference,
			payment_intent_id::text,
			amount_kobo,
			currency,
			reason,
			status,
			COALESCE(paystack_refund_reference, ''),
			created_at,
			updated_at
		FROM refunds`
}

func accountBalanceSQL() string {
	return `
		SELECT COALESCE(SUM(
			CASE
				WHEN a.normal_balance = 'debit' AND e.side = 'debit' THEN e.amount_kobo
				WHEN a.normal_balance = 'debit' AND e.side = 'credit' THEN -e.amount_kobo
				WHEN a.normal_balance = 'credit' AND e.side = 'credit' THEN e.amount_kobo
				WHEN a.normal_balance = 'credit' AND e.side = 'debit' THEN -e.amount_kobo
				ELSE 0
			END
		), 0)
		FROM wallet_accounts a
		LEFT JOIN ledger_entries e ON e.account_id = a.id`
}

func scanAccount(row pgx.Row) (walletmodels.Account, error) {
	var account walletmodels.Account
	err := row.Scan(
		&account.ID,
		&account.OwnerType,
		&account.OwnerID,
		&account.AccountType,
		&account.Currency,
		&account.NormalBalance,
		&account.Status,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return walletmodels.Account{}, mapNotFound(err, "Wallet account could not be found.")
	}
	return account, nil
}

func scanPaymentIntent(row pgx.Row) (walletmodels.PaymentIntent, error) {
	var intent walletmodels.PaymentIntent
	var metadataBytes []byte
	err := row.Scan(
		&intent.ID,
		&intent.Reference,
		&intent.SourceService,
		&intent.SourceReference,
		&intent.CustomerID,
		&intent.CustomerEmail,
		&intent.ProviderID,
		&intent.ProviderType,
		&intent.AmountKobo,
		&intent.PlatformFeeKobo,
		&intent.Currency,
		&intent.PaymentMethod,
		&intent.Status,
		&intent.PaystackRef,
		&intent.AuthorizationURL,
		&intent.AccessCode,
		&metadataBytes,
		&intent.CreatedAt,
		&intent.UpdatedAt,
	)
	if err != nil {
		return walletmodels.PaymentIntent{}, mapNotFound(err, "Payment could not be found.")
	}
	intent.Metadata = map[string]interface{}{}
	if len(metadataBytes) > 0 {
		_ = json.Unmarshal(metadataBytes, &intent.Metadata)
	}
	return intent, nil
}

func scanProviderBankAccount(row pgx.Row) (walletmodels.ProviderBankAccount, error) {
	var account walletmodels.ProviderBankAccount
	err := row.Scan(
		&account.ID,
		&account.ProviderType,
		&account.ProviderID,
		&account.BankCode,
		&account.BankName,
		&account.AccountNumber,
		&account.AccountName,
		&account.RecipientCode,
		&account.Currency,
		&account.Status,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return walletmodels.ProviderBankAccount{}, mapNotFound(err, "Bank account could not be found.")
	}
	return account, nil
}

func scanWithdrawal(row pgx.Row) (walletmodels.Withdrawal, error) {
	var withdrawal walletmodels.Withdrawal
	err := row.Scan(
		&withdrawal.ID,
		&withdrawal.Reference,
		&withdrawal.ProviderType,
		&withdrawal.ProviderID,
		&withdrawal.BankAccountID,
		&withdrawal.AmountKobo,
		&withdrawal.Currency,
		&withdrawal.Status,
		&withdrawal.PaystackTransferCode,
		&withdrawal.FailureReason,
		&withdrawal.CreatedAt,
		&withdrawal.UpdatedAt,
	)
	if err != nil {
		return walletmodels.Withdrawal{}, mapNotFound(err, "Withdrawal could not be found.")
	}
	return withdrawal, nil
}

func scanRefund(row pgx.Row) (walletmodels.Refund, error) {
	var refund walletmodels.Refund
	err := row.Scan(
		&refund.ID,
		&refund.Reference,
		&refund.PaymentIntentID,
		&refund.AmountKobo,
		&refund.Currency,
		&refund.Reason,
		&refund.Status,
		&refund.PaystackRefundRef,
		&refund.CreatedAt,
		&refund.UpdatedAt,
	)
	if err != nil {
		return walletmodels.Refund{}, mapNotFound(err, "Refund could not be found.")
	}
	return refund, nil
}

func mapNotFound(err error, message string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound(message, err)
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nonNilMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func jobOwnerID(sourceService string, sourceReference string) string {
	return sourceService + ":" + sourceReference
}

func providerOwnerID(providerType string, providerID string) string {
	return providerType + ":" + providerID
}

func isTopUpIntent(intent walletmodels.PaymentIntent) bool {
	return intent.SourceService == "payment-wallet-service" && intent.ProviderID == "" && intent.ProviderType == ""
}

func availableAccountFor(ownerType string) string {
	switch ownerType {
	case walletmodels.OwnerTypeCustomer:
		return walletmodels.AccountCustomerAvailable
	case walletmodels.OwnerTypeProvider:
		return walletmodels.AccountProviderPayable
	default:
		return ""
	}
}

func pendingAccountFor(ownerType string) string {
	switch ownerType {
	case walletmodels.OwnerTypeProvider:
		return walletmodels.AccountWithdrawalPending
	default:
		return ""
	}
}

func Reference(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.NewString())
}

func Now() time.Time {
	return time.Now()
}
