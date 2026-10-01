package integration_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

func referralCredit(customerID string, balance string, amount int64) walletrepositories.ReferralPostingInput {
	return walletrepositories.ReferralPostingInput{
		OwnerType: walletmodels.OwnerTypeCustomer, OwnerID: customerID, Balance: balance, AmountKobo: amount,
		IdempotencyKey: "rw-key-" + uuid.NewString(), Reference: "rw_" + uuid.NewString(), Reason: "test",
	}
}

// fundCustomer tops a wallet up through the real Paystack-success path.
func fundCustomer(t *testing.T, repo *walletrepositories.PostgresWalletRepository, customerID string, amount int64) {
	t.Helper()
	ctx := context.Background()
	paystackRef := "ps_" + uuid.NewString()
	if _, _, err := repo.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		SourceService: "payment-wallet-service", SourceReference: "topup_" + uuid.NewString(),
		CustomerID: customerID, AmountKobo: amount, Currency: "NGN",
		PaymentMethod: walletmodels.PaymentMethodPaystack, Status: walletmodels.PaymentStatusPending, PaystackRef: paystackRef,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyPaystackChargeSuccess(ctx, paystackRef); err != nil {
		t.Fatal(err)
	}
}

func summary(t *testing.T, repo *walletrepositories.PostgresWalletRepository, ownerType, ownerID string) walletmodels.WalletSummary {
	t.Helper()
	s, err := repo.GetWalletSummary(context.Background(), ownerType, ownerID, "NGN")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A credit posts once however often it is retried.
func TestReferralCreditIsIdempotent(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	ctx := context.Background()
	customer := "cust_" + uuid.NewString()
	input := referralCredit(customer, walletrepositories.ReferralBalanceEarnings, 50_000)

	first, err := repo.PostReferralCredit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repo.PostReferralCredit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Duplicate || retry.TransactionReference != first.TransactionReference || retry.BalanceKobo != 50_000 {
		t.Fatalf("retry = %+v, want the original posting and an unchanged balance", retry)
	}
	if got := summary(t, repo, walletmodels.OwnerTypeCustomer, customer); got.ReferralKobo != 50_000 || got.AvailableKobo != 0 {
		t.Fatalf("summary = %+v", got)
	}
}

// Taking back a reward that was already spent drives the balance negative;
// later credits net against the debt.
func TestReferralReversalCanGoNegative(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	ctx := context.Background()
	customer := "cust_" + uuid.NewString()
	reversal := referralCredit(customer, walletrepositories.ReferralBalanceEarnings, 50_000)
	posted, err := repo.PostReferralReversal(ctx, reversal)
	if err != nil {
		t.Fatalf("reversal with no balance must still post: %v", err)
	}
	if posted.BalanceKobo != -50_000 {
		t.Fatalf("balance after reversal = %d, want -50000", posted.BalanceKobo)
	}
	credit, err := repo.PostReferralCredit(ctx, referralCredit(customer, walletrepositories.ReferralBalanceEarnings, 70_000))
	if err != nil {
		t.Fatal(err)
	}
	if credit.BalanceKobo != 20_000 {
		t.Fatalf("balance after a later credit = %d, want 20000", credit.BalanceKobo)
	}
	totals, err := repo.GetReferralTotals(ctx, walletmodels.OwnerTypeCustomer, customer, "NGN")
	if err != nil || totals.CreditedKobo != 70_000 || totals.ReversedKobo != 50_000 || totals.EarningsBalanceKobo != 20_000 {
		t.Fatalf("totals = %+v, %v", totals, err)
	}
}

// Wallet payments spend promotional credit first, then wallet money, then
// referral earnings -- and a refund returns each part where it came from.
func TestWalletPaymentSpendOrderAndRefundReturnsToSource(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	ctx := context.Background()
	customer := "cust_" + uuid.NewString()
	fundCustomer(t, repo, customer, 50_000)
	if _, err := repo.PostReferralCredit(ctx, referralCredit(customer, walletrepositories.ReferralBalancePromotional, 30_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PostReferralCredit(ctx, referralCredit(customer, walletrepositories.ReferralBalanceEarnings, 100_000)); err != nil {
		t.Fatal(err)
	}
	if got := summary(t, repo, walletmodels.OwnerTypeCustomer, customer); got.SpendableKobo != 180_000 {
		t.Fatalf("spendable = %d, want 180000", got.SpendableKobo)
	}

	intent, _, err := repo.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		SourceService: "driver-taxi-service", SourceReference: "booking_" + uuid.NewString(),
		CustomerID: customer, ProviderType: "taxi", AmountKobo: 100_000, Currency: "NGN", PaymentMethod: walletmodels.PaymentMethodWallet,
	})
	if err != nil {
		t.Fatal(err)
	}
	held, err := repo.HoldPaymentFromWallet(ctx, intent.ID, "hold_"+intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"customer_promotional": 30_000, "customer_available": 50_000, "customer_referral_available": 20_000}
	for accountType, amount := range want {
		if held.FundingBreakdown[accountType] != amount {
			t.Fatalf("breakdown = %v, want %v", held.FundingBreakdown, want)
		}
	}
	after := summary(t, repo, walletmodels.OwnerTypeCustomer, customer)
	if after.PromotionalKobo != 0 || after.AvailableKobo != 0 || after.ReferralKobo != 80_000 {
		t.Fatalf("after payment = %+v", after)
	}

	// Refund the whole payment through the wallet refund path.
	refund, err := repo.CreateRefundReservation(ctx, walletrepositories.CreateRefundInput{
		PaymentIntentID: intent.ID, AmountKobo: 100_000, Currency: "NGN", Reason: "cancelled", IdempotencyKey: "refund_" + intent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkWalletRefundProcessed(ctx, refund.ID); err != nil {
		t.Fatal(err)
	}
	restored := summary(t, repo, walletmodels.OwnerTypeCustomer, customer)
	if restored.PromotionalKobo != 30_000 || restored.AvailableKobo != 50_000 || restored.ReferralKobo != 100_000 {
		t.Fatalf("after refund = %+v; each part must return to its own balance", restored)
	}
}

// Referral money for drivers lands in the account their wallet screens read,
// and an ordinary earnings withdrawal can never spend it.
func TestProviderReferralEarningsAreSeparateFromJobEarnings(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	ctx := context.Background()
	provider := "drv_" + uuid.NewString()
	if _, err := repo.PostReferralCredit(ctx, walletrepositories.ReferralPostingInput{
		OwnerType: walletmodels.OwnerTypeProvider, OwnerID: "truck_provider:" + provider, Balance: walletrepositories.ReferralBalanceEarnings,
		AmountKobo: 600_000, IdempotencyKey: "k-" + provider, Reference: "rw_" + provider,
	}); err != nil {
		t.Fatal(err)
	}
	got := summary(t, repo, walletmodels.OwnerTypeProvider, "truck_provider:"+provider)
	if got.ReferralKobo != 600_000 || got.AvailableKobo != 0 {
		t.Fatalf("driver summary = %+v; referral earnings must not appear as job earnings", got)
	}
	account, err := repo.CreateProviderBankAccount(ctx, walletmodels.ProviderBankAccount{
		ProviderType: "truck_provider", ProviderID: provider, BankCode: "058", AccountNumber: "0123456789",
		AccountName: "Test Driver", RecipientCode: "RCP_" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.RequestWithdrawal(ctx, walletrepositories.CreateWithdrawalInput{
		ProviderType: "truck_provider", ProviderID: provider, BankAccountID: account.ID, AmountKobo: 100_000, IdempotencyKey: "w-" + provider,
	}); err == nil {
		t.Fatal("a job-earnings withdrawal spent referral earnings")
	}
}
