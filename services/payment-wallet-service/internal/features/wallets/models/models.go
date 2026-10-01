package walletmodels

import (
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	CurrencyNGN = "NGN"

	OwnerTypeCustomer = "customer"
	OwnerTypeProvider = "provider"
	OwnerTypeSystem   = "system"
	OwnerTypeJob      = "job"
	OwnerTypePlatform = "platform"

	AccountPaystackReceivable = "paystack_receivable"
	AccountPaystackBalance    = "paystack_balance"
	AccountCustomerAvailable  = "customer_available"
	AccountJobEscrow          = "job_escrow"
	AccountProviderPayable    = "provider_payable"
	AccountWithdrawalPending  = "withdrawal_pending"
	AccountRefundPending      = "refund_pending"
	AccountPlatformRevenue    = "platform_revenue"

	// Referral programme balances. Earnings are withdrawable (drivers) or
	// spendable (customers); promotional credit is spend-only and never
	// withdrawable. platform_referral_expense funds every referral credit.
	AccountCustomerReferralAvailable = "customer_referral_available"
	AccountProviderReferralAvailable = "provider_referral_available"
	AccountCustomerPromotional       = "customer_promotional"
	AccountProviderPromotional       = "provider_promotional"
	AccountPlatformReferralExpense   = "platform_referral_expense"

	NormalDebit  = "debit"
	NormalCredit = "credit"

	SideDebit  = "debit"
	SideCredit = "credit"

	PaymentMethodWallet   = "wallet"
	PaymentMethodPaystack = "paystack"

	PaymentStatusPending      = "pending"
	PaymentStatusRequiresPay  = "requires_payment"
	PaymentStatusHeld         = "held"
	PaymentStatusCompleted    = "completed"
	PaymentStatusRefunded     = "refunded"
	PaymentStatusPartRefunded = "partially_refunded"
	PaymentStatusFailed       = "failed"

	WithdrawalStatusPending     = "pending"
	WithdrawalStatusProcessing  = "processing"
	WithdrawalStatusRequiresOTP = "requires_otp"
	WithdrawalStatusPaid        = "paid"
	WithdrawalStatusFailed      = "failed"
	WithdrawalStatusReversed    = "reversed"

	RefundStatusPending    = "pending"
	RefundStatusProcessing = "processing"
	RefundStatusProcessed  = "processed"
	RefundStatusFailed     = "failed"
)

type Account struct {
	ID            string
	OwnerType     string
	OwnerID       string
	AccountType   string
	Currency      string
	NormalBalance string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type LedgerTransaction struct {
	ID              string
	Reference       string
	TransactionType string
	Status          string
	SourceService   string
	SourceReference string
	IdempotencyKey  string
	ExternalRef     string
	Metadata        map[string]interface{}
	CreatedAt       time.Time
}

type LedgerEntry struct {
	AccountID  string
	Side       string
	AmountKobo int64
	Currency   string
	Memo       string
}

type PaymentIntent struct {
	ID               string                 `json:"id"`
	Reference        string                 `json:"reference"`
	SourceService    string                 `json:"source_service"`
	SourceReference  string                 `json:"source_reference"`
	CustomerID       string                 `json:"customer_id"`
	CustomerEmail    string                 `json:"customer_email,omitempty"`
	ProviderID       string                 `json:"provider_id,omitempty"`
	ProviderType     string                 `json:"provider_type,omitempty"`
	AmountKobo       int64                  `json:"amount_kobo"`
	PlatformFeeKobo  int64                  `json:"platform_fee_kobo"`
	Currency         string                 `json:"currency"`
	PaymentMethod    string                 `json:"payment_method"`
	Status           string                 `json:"status"`
	PaystackRef      string                 `json:"paystack_reference,omitempty"`
	AuthorizationURL string                 `json:"authorization_url,omitempty"`
	AccessCode       string                 `json:"access_code,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	// FundingBreakdown is which balances paid a wallet-funded hold, keyed by
	// account type. Refunds return money to the same balances.
	FundingBreakdown map[string]int64 `json:"funding_breakdown,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type WalletSummary struct {
	OwnerType     string `json:"owner_type"`
	OwnerID       string `json:"owner_id"`
	Currency      string `json:"currency"`
	AvailableKobo int64  `json:"available_kobo"`
	EscrowKobo    int64  `json:"escrow_kobo"`
	PendingKobo   int64  `json:"pending_kobo"`
	// ReferralKobo is referral earnings (withdrawable for drivers from the
	// referral minimum; spendable on bookings for customers).
	ReferralKobo int64 `json:"referral_kobo"`
	// PromotionalKobo is spend-only credit (welcome credits, vouchers).
	PromotionalKobo int64 `json:"promotional_kobo"`
	// SpendableKobo is what a customer can pay a booking with: promotional
	// credit + wallet balance + referral earnings.
	SpendableKobo int64 `json:"spendable_kobo,omitempty"`
}

type WalletTransaction struct {
	Reference       string    `json:"reference"`
	TransactionType string    `json:"transaction_type"`
	Side            string    `json:"side"`
	AmountKobo      int64     `json:"amount_kobo"`
	Currency        string    `json:"currency"`
	Memo            string    `json:"memo"`
	CreatedAt       time.Time `json:"created_at"`
}

type ProviderBankAccount struct {
	ID            string    `json:"id"`
	ProviderType  string    `json:"provider_type"`
	ProviderID    string    `json:"provider_id"`
	BankCode      string    `json:"bank_code"`
	BankName      string    `json:"bank_name"`
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	RecipientCode string    `json:"recipient_code"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TransactionPin is the provider's withdrawal authorization PIN. The hash is
// never serialized; lockout state guards the low-entropy PIN against brute
// force (verification is refused while LockedUntil is in the future).
type TransactionPin struct {
	ID             string     `json:"id"`
	ProviderType   string     `json:"provider_type"`
	ProviderID     string     `json:"provider_id"`
	PinHash        string     `json:"-"`
	FailedAttempts int        `json:"failed_attempts"`
	LockedUntil    *time.Time `json:"locked_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Withdrawal funding sources: the balance a withdrawal is paid from.
const (
	WithdrawalFromJobEarnings      = "job_earnings"
	WithdrawalFromReferralEarnings = "referral_earnings"
)

type Withdrawal struct {
	ID                   string    `json:"id"`
	Reference            string    `json:"reference"`
	ProviderType         string    `json:"provider_type"`
	ProviderID           string    `json:"provider_id"`
	BankAccountID        string    `json:"bank_account_id"`
	AmountKobo           int64     `json:"amount_kobo"`
	Currency             string    `json:"currency"`
	Status               string    `json:"status"`
	FundingSource        string    `json:"funding_source"`
	PaystackTransferCode string    `json:"paystack_transfer_code,omitempty"`
	FailureReason        string    `json:"failure_reason,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Refund struct {
	ID                string    `json:"id"`
	Reference         string    `json:"reference"`
	PaymentIntentID   string    `json:"payment_intent_id"`
	AmountKobo        int64     `json:"amount_kobo"`
	Currency          string    `json:"currency"`
	Reason            string    `json:"reason,omitempty"`
	Status            string    `json:"status"`
	PaystackRefundRef string    `json:"paystack_refund_reference,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func DefaultCurrency(value string) string {
	if value == "" {
		return CurrencyNGN
	}
	return value
}

func CalculatePlatformFee(amountKobo int64, bps int64) int64 {
	if amountKobo <= 0 || bps <= 0 {
		return 0
	}
	return (amountKobo * bps) / 10000
}

func ValidateBalanced(entries []LedgerEntry) error {
	if len(entries) < 2 {
		return apperrors.Internal("Ledger transaction must contain at least two entries.", nil)
	}

	totals := map[string]struct {
		debit  int64
		credit int64
	}{}
	for _, entry := range entries {
		if entry.AmountKobo <= 0 {
			return apperrors.Internal("Ledger entry amount must be positive.", nil)
		}
		currency := DefaultCurrency(entry.Currency)
		total := totals[currency]
		switch entry.Side {
		case SideDebit:
			total.debit += entry.AmountKobo
		case SideCredit:
			total.credit += entry.AmountKobo
		default:
			return apperrors.Internal("Ledger entry side is invalid.", nil)
		}
		totals[currency] = total
	}

	for _, total := range totals {
		if total.debit != total.credit {
			return apperrors.Internal("Ledger transaction is not balanced.", nil)
		}
	}
	return nil
}
