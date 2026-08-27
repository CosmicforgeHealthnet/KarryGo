package finance_projection

import (
	"context"
	"time"
)

const (
	PermissionRead     = "vendor.finance.read"
	PermissionWithdraw = "vendor.finance.withdraw"
	WithdrawalStepUp   = "finance.withdrawal"
)

type Actor struct {
	StaffID   string
	VendorID  string
	SessionID string
}

type Authorizer interface {
	Authorize(context.Context, string, string, *string) (bool, error)
}

type StepUpConsumer interface {
	ConsumeGrant(context.Context, string, string, string, string, string) error
}

type Summary struct {
	OwnerID       string `json:"owner_id"`
	Currency      string `json:"currency"`
	AvailableKobo int64  `json:"available_kobo"`
	PendingKobo   int64  `json:"pending_kobo"`
}

type Transaction struct {
	Reference       string    `json:"reference"`
	TransactionType string    `json:"transaction_type"`
	Side            string    `json:"side"`
	AmountKobo      int64     `json:"amount_kobo"`
	Currency        string    `json:"currency"`
	Memo            string    `json:"memo"`
	CreatedAt       time.Time `json:"created_at"`
}

type BankAccount struct {
	ExternalRef         string `json:"external_ref"`
	BankCode            string `json:"bank_code"`
	MaskedAccount       string `json:"masked_account"`
	ResolvedAccountName string `json:"resolved_account_name"`
	Status              string `json:"status"`
}

type Withdrawal struct {
	ID            string `json:"id"`
	Reference     string `json:"reference"`
	BankAccountID string `json:"bank_account_id"`
	AmountKobo    int64  `json:"amount_kobo"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
}

type WithdrawalCommand struct {
	BankAccountID  string
	AmountKobo     int64
	Currency       string
	IdempotencyKey string
	StepUpGrantID  string
}

type Authority interface {
	Summary(context.Context, string) (Summary, error)
	Transactions(context.Context, string, int) ([]Transaction, error)
	BankAccounts(context.Context, string) ([]BankAccount, error)
	RequestWithdrawal(context.Context, string, WithdrawalCommand) (Withdrawal, error)
	Withdrawals(context.Context, string) ([]Withdrawal, error)
	Withdrawal(context.Context, string, string) (Withdrawal, error)
}
