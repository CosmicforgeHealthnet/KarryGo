package walletrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// customerSpendOrder is the order a wallet payment draws on a customer's
// balances: spend-only promotional credit first, then ordinary wallet money,
// then referral earnings -- the one balance a customer may later cash out.
var customerSpendOrder = []string{
	walletmodels.AccountCustomerPromotional,
	walletmodels.AccountCustomerAvailable,
	walletmodels.AccountCustomerReferralAvailable,
}

type fundingSource struct {
	account walletmodels.Account
	balance int64
}

type fundingPart struct {
	account walletmodels.Account
	amount  int64
}

// planFunding draws amount from sources in order. A negative balance (a
// reversed referral reward that was already spent) contributes nothing; it is
// a debt, not a source. ok is false when the sources cannot cover amount.
func planFunding(sources []fundingSource, amount int64) ([]fundingPart, bool) {
	remaining := amount
	var plan []fundingPart
	for _, source := range sources {
		if remaining == 0 {
			break
		}
		if source.balance <= 0 {
			continue
		}
		take := min(source.balance, remaining)
		plan = append(plan, fundingPart{account: source.account, amount: take})
		remaining -= take
	}
	return plan, remaining == 0
}

type refundPart struct {
	accountType string
	amount      int64
}

// allocateRefund returns refunded money to the balances that paid, most
// valuable first (the reverse of the spend order). A payment with no recorded
// breakdown (held before breakdowns existed) refunds to the wallet balance,
// exactly as before.
func allocateRefund(breakdown map[string]int64, amount int64) []refundPart {
	if amount <= 0 {
		return nil
	}
	if len(breakdown) == 0 {
		return []refundPart{{accountType: walletmodels.AccountCustomerAvailable, amount: amount}}
	}
	remaining := amount
	var parts []refundPart
	for i := len(customerSpendOrder) - 1; i >= 0 && remaining > 0; i-- {
		accountType := customerSpendOrder[i]
		paid := breakdown[accountType]
		if paid <= 0 {
			continue
		}
		give := min(paid, remaining)
		parts = append(parts, refundPart{accountType: accountType, amount: give})
		remaining -= give
	}
	if remaining > 0 {
		// Cannot happen while a refund never exceeds the payment; if it ever
		// did, the excess lands in the wallet balance rather than vanishing.
		parts = append(parts, refundPart{accountType: walletmodels.AccountCustomerAvailable, amount: remaining})
	}
	return parts
}

// referralAccountTypes gives the earnings and promotional account types for
// an owner type.
func referralAccountTypes(ownerType string) (earnings string, promotional string, ok bool) {
	switch ownerType {
	case walletmodels.OwnerTypeCustomer:
		return walletmodels.AccountCustomerReferralAvailable, walletmodels.AccountCustomerPromotional, true
	case walletmodels.OwnerTypeProvider:
		return walletmodels.AccountProviderReferralAvailable, walletmodels.AccountProviderPromotional, true
	}
	return "", "", false
}

// Referral balance kinds.
const (
	ReferralBalanceEarnings    = "earnings"
	ReferralBalancePromotional = "promotional"
)

// Referral ledger transaction types.
const (
	TransactionReferralCredit   = "referral_credit"
	TransactionReferralReversal = "referral_reversal"
)

// ReferralPostingInput is one referral credit or reversal. OwnerID is the
// ledger owner id: the customer id, or "<provider_type>:<provider id>".
type ReferralPostingInput struct {
	OwnerType      string
	OwnerID        string
	Balance        string
	AmountKobo     int64
	Currency       string
	IdempotencyKey string
	Reference      string
	Reason         string
}

// ReferralPosting is the ledger outcome of a referral credit or reversal.
type ReferralPosting struct {
	TransactionReference string `json:"transaction_reference"`
	Duplicate            bool   `json:"duplicate"`
	AccountType          string `json:"account_type"`
	BalanceKobo          int64  `json:"balance_kobo"`
}

// PostReferralCredit pays a referral reward: debit platform_referral_expense,
// credit the beneficiary's referral balance. Idempotent on IdempotencyKey --
// a repeat returns the original posting with Duplicate set.
func (r *PostgresWalletRepository) PostReferralCredit(ctx context.Context, input ReferralPostingInput) (ReferralPosting, error) {
	return r.postReferral(ctx, input, TransactionReferralCredit)
}

// PostReferralReversal takes a referral reward back: debit the beneficiary's
// referral balance, credit platform_referral_expense.
//
// It deliberately skips the balance check every other debit in this service
// makes: a reward that was already spent or withdrawn must still be
// reversible, so the balance is allowed to go negative and future referral
// credits net against the debt. Do not "fix" this by adding a balance check.
func (r *PostgresWalletRepository) PostReferralReversal(ctx context.Context, input ReferralPostingInput) (ReferralPosting, error) {
	return r.postReferral(ctx, input, TransactionReferralReversal)
}

func (r *PostgresWalletRepository) postReferral(ctx context.Context, input ReferralPostingInput, transactionType string) (ReferralPosting, error) {
	earningsType, promotionalType, ok := referralAccountTypes(input.OwnerType)
	if !ok {
		return ReferralPosting{}, apperrors.BadRequest("Referral credits go to customers or providers only.", nil)
	}
	accountType := earningsType
	switch input.Balance {
	case ReferralBalanceEarnings:
	case ReferralBalancePromotional:
		accountType = promotionalType
	default:
		return ReferralPosting{}, apperrors.BadRequest("Referral balance must be earnings or promotional.", nil)
	}
	if input.AmountKobo <= 0 || input.IdempotencyKey == "" || input.Reference == "" || input.OwnerID == "" {
		return ReferralPosting{}, apperrors.BadRequest("Referral posting needs an owner, a positive amount, a reference and an idempotency key.", nil)
	}
	input.Currency = walletmodels.DefaultCurrency(input.Currency)

	var posting ReferralPosting
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		// Serialize retries of the same posting, then check for a prior one:
		// a unique violation would abort the transaction, so it is not relied on.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 61))`, transactionType+":"+input.IdempotencyKey); err != nil {
			return err
		}
		target, err := r.ensureAccount(ctx, tx, input.OwnerType, input.OwnerID, accountType, input.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		posting.AccountType = accountType
		var existing string
		err = tx.QueryRow(ctx, `SELECT reference FROM ledger_transactions WHERE idempotency_key = $1 AND transaction_type = $2`, input.IdempotencyKey, transactionType).Scan(&existing)
		switch {
		case err == nil:
			posting.TransactionReference = existing
			posting.Duplicate = true
		case errors.Is(err, pgx.ErrNoRows):
			expense, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go", walletmodels.AccountPlatformReferralExpense, input.Currency, walletmodels.NormalDebit)
			if err != nil {
				return err
			}
			debit, credit := expense.ID, target.ID
			memo := "Referral reward"
			if transactionType == TransactionReferralReversal {
				debit, credit = target.ID, expense.ID
				memo = "Referral reward reversed"
			}
			posting.TransactionReference = "ldt_" + uuid.NewString()
			if err := r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
				ID:              uuid.NewString(),
				Reference:       posting.TransactionReference,
				TransactionType: transactionType,
				Status:          postedStatus,
				SourceService:   "referral-service",
				SourceReference: input.Reference,
				IdempotencyKey:  input.IdempotencyKey,
				Metadata:        map[string]interface{}{"reason": input.Reason},
			}, []walletmodels.LedgerEntry{
				{AccountID: debit, Side: walletmodels.SideDebit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: memo},
				{AccountID: credit, Side: walletmodels.SideCredit, AmountKobo: input.AmountKobo, Currency: input.Currency, Memo: memo},
			}); err != nil {
				return err
			}
		default:
			return fmt.Errorf("look up referral posting: %w", err)
		}
		posting.BalanceKobo, err = r.accountBalanceTx(ctx, tx, target.ID)
		return err
	})
	return posting, err
}

// ReferralTotals is what the ledger holds for one owner's referral balances:
// everything credited and reversed through the referral API, and the current
// balances (which also reflect spending and withdrawals).
type ReferralTotals struct {
	OwnerType              string `json:"owner_type"`
	OwnerID                string `json:"owner_id"`
	CreditedKobo           int64  `json:"credited_kobo"`
	ReversedKobo           int64  `json:"reversed_kobo"`
	EarningsBalanceKobo    int64  `json:"earnings_balance_kobo"`
	PromotionalBalanceKobo int64  `json:"promotional_balance_kobo"`
}

// GetReferralTotals sums the referral API's postings for one owner, for
// reconciliation against referral-service's own ledger.
func (r *PostgresWalletRepository) GetReferralTotals(ctx context.Context, ownerType, ownerID, currency string) (ReferralTotals, error) {
	earningsType, promotionalType, ok := referralAccountTypes(ownerType)
	if !ok {
		return ReferralTotals{}, apperrors.BadRequest("Referral totals exist for customers or providers only.", nil)
	}
	currency = walletmodels.DefaultCurrency(currency)
	totals := ReferralTotals{OwnerType: ownerType, OwnerID: ownerID}
	err := r.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(e.amount_kobo) FILTER (WHERE t.transaction_type = $4 AND e.side = 'credit'), 0),
			COALESCE(SUM(e.amount_kobo) FILTER (WHERE t.transaction_type = $5 AND e.side = 'debit'), 0)
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.owner_type = $1 AND a.owner_id = $2 AND a.currency = $3
		  AND a.account_type IN ($6, $7)`,
		ownerType, ownerID, currency, TransactionReferralCredit, TransactionReferralReversal, earningsType, promotionalType,
	).Scan(&totals.CreditedKobo, &totals.ReversedKobo)
	if err != nil {
		return ReferralTotals{}, err
	}
	if totals.EarningsBalanceKobo, err = r.accountBalance(ctx, ownerType, ownerID, earningsType, currency); err != nil {
		return ReferralTotals{}, err
	}
	if totals.PromotionalBalanceKobo, err = r.accountBalance(ctx, ownerType, ownerID, promotionalType, currency); err != nil {
		return ReferralTotals{}, err
	}
	return totals, nil
}
