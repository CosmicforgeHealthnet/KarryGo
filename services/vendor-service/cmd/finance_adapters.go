package main

import (
	"context"

	finance "cosmicforge/logistics/services/vendor-service/internal/features/finance_projection"
	"cosmicforge/logistics/shared/go/walletclient"
)

type vendorFinanceAuthority struct{ client walletclient.Client }

func (a vendorFinanceAuthority) Summary(ctx context.Context, vendorID string) (finance.Summary, error) {
	value, err := a.client.VendorSummary(ctx, vendorID)
	return finance.Summary{OwnerID: value.OwnerID, Currency: value.Currency, AvailableKobo: value.AvailableKobo, PendingKobo: value.PendingKobo}, err
}

func (a vendorFinanceAuthority) Transactions(ctx context.Context, vendorID string, limit int) ([]finance.Transaction, error) {
	values, err := a.client.VendorTransactions(ctx, vendorID, limit)
	result := make([]finance.Transaction, 0, len(values))
	for _, value := range values {
		result = append(result, finance.Transaction{Reference: value.Reference, TransactionType: value.TransactionType, Side: value.Side, AmountKobo: value.AmountKobo, Currency: value.Currency, Memo: value.Memo, CreatedAt: value.CreatedAt})
	}
	return result, err
}

func (a vendorFinanceAuthority) BankAccounts(ctx context.Context, vendorID string) ([]finance.BankAccount, error) {
	values, err := a.client.VendorBankAccounts(ctx, vendorID)
	result := make([]finance.BankAccount, 0, len(values))
	for _, value := range values {
		result = append(result, finance.BankAccount{ExternalRef: value.ExternalRef, BankCode: value.BankCode, MaskedAccount: value.MaskedAccount, ResolvedAccountName: value.ResolvedAccountName, Status: value.Status})
	}
	return result, err
}

func (a vendorFinanceAuthority) RequestWithdrawal(ctx context.Context, vendorID string, command finance.WithdrawalCommand) (finance.Withdrawal, error) {
	value, err := a.client.RequestVendorWithdrawal(ctx, vendorID, walletclient.VendorWithdrawalRequest{BankAccountID: command.BankAccountID, AmountKobo: command.AmountKobo, Currency: command.Currency, IdempotencyKey: command.IdempotencyKey, StepUpReference: command.StepUpGrantID})
	return finance.Withdrawal{ID: value.ID, Reference: value.Reference, BankAccountID: value.BankAccountID, AmountKobo: value.AmountKobo, Currency: value.Currency, Status: value.Status}, err
}

func (a vendorFinanceAuthority) Withdrawals(ctx context.Context, vendorID string) ([]finance.Withdrawal, error) {
	values, err := a.client.VendorWithdrawals(ctx, vendorID)
	result := make([]finance.Withdrawal, 0, len(values))
	for _, value := range values {
		result = append(result, finance.Withdrawal{ID: value.ID, Reference: value.Reference, BankAccountID: value.BankAccountID, AmountKobo: value.AmountKobo, Currency: value.Currency, Status: value.Status})
	}
	return result, err
}

func (a vendorFinanceAuthority) Withdrawal(ctx context.Context, vendorID, reference string) (finance.Withdrawal, error) {
	value, err := a.client.VendorWithdrawal(ctx, vendorID, reference)
	return finance.Withdrawal{ID: value.ID, Reference: value.Reference, BankAccountID: value.BankAccountID, AmountKobo: value.AmountKobo, Currency: value.Currency, Status: value.Status}, err
}
