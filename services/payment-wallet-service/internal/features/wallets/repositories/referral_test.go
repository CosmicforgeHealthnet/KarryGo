package walletrepositories

import (
	"reflect"
	"testing"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

func source(accountType string, balance int64) fundingSource {
	return fundingSource{account: walletmodels.Account{ID: accountType, AccountType: accountType}, balance: balance}
}

func TestPlanFundingSpendsPromotionalFirstAndSkipsDebts(t *testing.T) {
	sources := []fundingSource{
		source(walletmodels.AccountCustomerPromotional, 30_000),
		source(walletmodels.AccountCustomerAvailable, 50_000),
		source(walletmodels.AccountCustomerReferralAvailable, 100_000),
	}
	plan, ok := planFunding(sources, 100_000)
	if !ok {
		t.Fatal("expected the balances to cover the payment")
	}
	got := map[string]int64{}
	for _, part := range plan {
		got[part.account.AccountType] = part.amount
	}
	want := map[string]int64{
		walletmodels.AccountCustomerPromotional:       30_000,
		walletmodels.AccountCustomerAvailable:         50_000,
		walletmodels.AccountCustomerReferralAvailable: 20_000,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}

	// A negative referral balance is a debt, not money to spend.
	_, ok = planFunding([]fundingSource{
		source(walletmodels.AccountCustomerPromotional, 0),
		source(walletmodels.AccountCustomerAvailable, 10_000),
		source(walletmodels.AccountCustomerReferralAvailable, -5_000),
	}, 10_001)
	if ok {
		t.Fatal("a payment larger than the positive balances must be refused")
	}
}

func TestAllocateRefundReturnsMoneyWhereItCameFrom(t *testing.T) {
	breakdown := map[string]int64{
		walletmodels.AccountCustomerPromotional:       30_000,
		walletmodels.AccountCustomerAvailable:         50_000,
		walletmodels.AccountCustomerReferralAvailable: 20_000,
	}
	parts := allocateRefund(breakdown, 100_000)
	got := map[string]int64{}
	for _, part := range parts {
		got[part.accountType] = part.amount
	}
	if !reflect.DeepEqual(got, breakdown) {
		t.Fatalf("full refund = %v, want the original breakdown %v", got, breakdown)
	}
	// A partial refund returns the most valuable balance first.
	parts = allocateRefund(breakdown, 25_000)
	if len(parts) != 2 || parts[0].accountType != walletmodels.AccountCustomerReferralAvailable || parts[0].amount != 20_000 ||
		parts[1].accountType != walletmodels.AccountCustomerAvailable || parts[1].amount != 5_000 {
		t.Fatalf("partial refund = %+v", parts)
	}
	// Payments held before breakdowns existed refund to the wallet, as before.
	parts = allocateRefund(nil, 7_000)
	if len(parts) != 1 || parts[0].accountType != walletmodels.AccountCustomerAvailable || parts[0].amount != 7_000 {
		t.Fatalf("legacy refund = %+v", parts)
	}
}
