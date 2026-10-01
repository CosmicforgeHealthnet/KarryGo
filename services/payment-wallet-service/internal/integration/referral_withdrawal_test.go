package integration_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/referralcontract"
)

var referralPepper = []byte("test-identity-pepper")

// transferPaystack is Paystack for the transfer path: a funded balance and
// transfers that start (or, when told, fail to).
type transferPaystack struct {
	walletclients.PaystackClient
	mu       sync.Mutex
	failNext bool
}

func (p *transferPaystack) CheckBalance(context.Context) (walletclients.BalanceResponse, error) {
	return walletclients.BalanceResponse{Items: []walletclients.BalanceItem{{Currency: "NGN", Balance: 1_000_000_000}}}, nil
}

func (p *transferPaystack) InitiateTransfer(_ context.Context, request walletclients.InitiateTransferRequest) (walletclients.InitiateTransferResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		return walletclients.InitiateTransferResponse{}, errors.New("paystack unavailable")
	}
	return walletclients.InitiateTransferResponse{ID: 1, Reference: request.Reference, TransferCode: "TRF_" + uuid.NewString(), Status: "pending"}, nil
}

// fakeEligibility stands in for referral-service's cash-out check.
type fakeEligibility struct {
	mu       sync.Mutex
	answer   referralcontract.WithdrawalEligibility
	err      error
	requests []referralcontract.WithdrawalEligibilityRequest
}

func (f *fakeEligibility) CheckWithdrawal(_ context.Context, request referralcontract.WithdrawalEligibilityRequest) (referralcontract.WithdrawalEligibility, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	return f.answer, f.err
}

func (f *fakeEligibility) set(answer referralcontract.WithdrawalEligibility, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answer, f.err = answer, err
}

func (f *fakeEligibility) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// cashOut is one driver ready to withdraw: a PIN, a bank account and a
// wallet service wired to the fakes.
type cashOut struct {
	pool        *pgxpool.Pool
	repo        *walletrepositories.PostgresWalletRepository
	svc         *walletusecases.WalletService
	paystack    *transferPaystack
	eligibility *fakeEligibility
	driverID    string
	ledgerOwner string
	account     walletmodels.ProviderBankAccount
}

const driverPIN = "4826"

func newCashOut(t *testing.T) cashOut {
	t.Helper()
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	c := cashOut{
		pool: pool, repo: repo, paystack: &transferPaystack{},
		eligibility: &fakeEligibility{answer: referralcontract.WithdrawalEligibility{Eligible: true}},
		driverID:    "drv_" + uuid.NewString(),
	}
	c.ledgerOwner = "taxi:" + c.driverID
	c.svc = walletusecases.NewWalletService(walletusecases.Options{
		Repository: repo, Paystack: c.paystack, DefaultCurrency: "NGN", WithdrawalMinKobo: 10_000,
		ReferralEligibility: c.eligibility, ReferralPepper: referralPepper, ReferralWithdrawalMinKobo: 500_000,
	})
	ctx := context.Background()
	if err := c.svc.SetTransactionPin(ctx, "taxi", c.driverID, driverPIN); err != nil {
		t.Fatal(err)
	}
	account, err := repo.CreateProviderBankAccount(ctx, walletmodels.ProviderBankAccount{
		ProviderType: "taxi", ProviderID: c.driverID, BankCode: "058", AccountNumber: "0123456789",
		AccountName: "Test Driver", RecipientCode: "RCP_" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	c.account = account
	return c
}

func (c cashOut) creditReferral(t *testing.T, amount int64) {
	t.Helper()
	if _, err := c.svc.PostReferralCredit(context.Background(), walletusecases.ReferralPostingRequest{
		OwnerType: "provider", OwnerID: c.driverID, ProviderType: "taxi", Balance: "earnings", AmountKobo: amount,
		IdempotencyKey: "rw-" + uuid.NewString(), Reference: "rw_" + uuid.NewString(), Reason: "provider_commission",
	}); err != nil {
		t.Fatal(err)
	}
}

// earnFromJob pays the driver job earnings through a real settled job.
func (c cashOut) earnFromJob(t *testing.T, amount, fee int64) {
	t.Helper()
	ctx := context.Background()
	customerID := "cust_" + uuid.NewString()
	fundCustomer(t, c.repo, customerID, amount)
	intent, _, err := c.repo.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		SourceService: "driver-taxi-service", SourceReference: "booking_" + uuid.NewString(),
		CustomerID: customerID, ProviderID: c.driverID, ProviderType: "taxi",
		AmountKobo: amount, PlatformFeeKobo: fee, Currency: "NGN", PaymentMethod: walletmodels.PaymentMethodWallet,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.repo.HoldPaymentFromWallet(ctx, intent.ID, "hold_"+intent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.repo.CompleteJob(ctx, intent.SourceService, intent.SourceReference, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func (c cashOut) withdrawReferral(amount int64, key string) (walletmodels.Withdrawal, error) {
	return c.svc.RequestReferralWithdrawal(context.Background(), walletusecases.RequestWithdrawalInput{
		ProviderType: "taxi", ProviderID: c.driverID, BankAccountID: c.account.ID,
		AmountKobo: amount, IdempotencyKey: key, Pin: driverPIN,
	})
}

func (c cashOut) balances(t *testing.T) (referral, jobEarnings int64) {
	t.Helper()
	s := summary(t, c.repo, walletmodels.OwnerTypeProvider, c.ledgerOwner)
	return s.ReferralKobo, s.AvailableKobo
}

// key makes an idempotency key unique to this run: ledger transactions carry
// the withdrawal's key under a global unique index, and the test database is
// shared between runs.
func key(name string) string {
	return name + "-" + uuid.NewString()
}

func errorStatus(err error) int {
	if appErr := apperrors.From(err); appErr != nil {
		return appErr.Status
	}
	return 0
}

// Step 7 "done when": a driver with ₦5,000 or more of referral earnings can
// withdraw it and anything less is refused; the payout comes only from the
// referral balance, and referral-service is asked first with the bank
// account hashed, never sent raw.
func TestDriverWithdrawsReferralEarnings(t *testing.T) {
	c := newCashOut(t)
	c.creditReferral(t, 750_000)
	c.earnFromJob(t, 1_000_000, 150_000)
	_, jobBefore := c.balances(t)

	if _, err := c.withdrawReferral(499_999, key("too-small")); errorStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Check your withdrawal") {
		t.Fatalf("₦4,999.99 withdrawal = %v, want a validation error", err)
	}
	if c.eligibility.calls() != 0 {
		t.Fatal("referral-service was asked about a request that fails validation")
	}

	cashOutKey := key("cash-out")
	withdrawal, err := c.withdrawReferral(600_000, cashOutKey)
	if err != nil {
		t.Fatalf("₦6,000 withdrawal: %v", err)
	}
	if withdrawal.FundingSource != walletmodels.WithdrawalFromReferralEarnings || withdrawal.Status != walletmodels.WithdrawalStatusProcessing {
		t.Fatalf("withdrawal = %+v", withdrawal)
	}
	referral, job := c.balances(t)
	if referral != 150_000 || job != jobBefore {
		t.Fatalf("after cash-out: referral %d, job earnings %d; want 150000 and job earnings untouched (%d)", referral, job, jobBefore)
	}
	request := c.eligibility.requests[0]
	if request.ActorType != referralcontract.ActorTaxi || request.ActorID != c.driverID || request.AmountKobo != 600_000 ||
		request.BankAccountHash != referralcontract.HashBankAccount(referralPepper, "058", "0123456789") {
		t.Fatalf("eligibility request = %+v", request)
	}

	// The remaining ₦1,500 is below the minimum however it is asked for.
	if _, err := c.withdrawReferral(150_000, key("cash-out")); err == nil {
		t.Fatal("a withdrawal below the minimum went through")
	}
	// A replay returns the original, even if the driver is under review now.
	c.eligibility.set(referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleUnderReview}, nil)
	replayed, err := c.withdrawReferral(600_000, cashOutKey)
	if err != nil || replayed.ID != withdrawal.ID {
		t.Fatalf("replay = %+v, %v; want the original withdrawal", replayed, err)
	}
}

// Job earnings and referral earnings never pay for each other's withdrawals.
func TestWithdrawalsKeepTheirBalancesApart(t *testing.T) {
	c := newCashOut(t)
	ctx := context.Background()
	c.earnFromJob(t, 2_000_000, 300_000) // ₦17,000 of job earnings
	if _, err := c.withdrawReferral(500_000, key("referral-from-job-money")); errorStatus(err) != http.StatusConflict {
		t.Fatalf("referral withdrawal with no referral earnings = %v, want refused", err)
	}
	c.creditReferral(t, 900_000)
	_, jobBefore := c.balances(t)
	if _, err := c.svc.RequestWithdrawal(ctx, walletusecases.RequestWithdrawalInput{
		ProviderType: "taxi", ProviderID: c.driverID, BankAccountID: c.account.ID,
		AmountKobo: jobBefore + 100_000, IdempotencyKey: key("job-from-referral-money"), Pin: driverPIN,
	}); err == nil {
		t.Fatal("a job-earnings withdrawal spent referral earnings")
	}
	if referral, job := c.balances(t); referral != 900_000 || job != jobBefore {
		t.Fatalf("balances moved: referral %d, job %d", referral, job)
	}
	// One idempotency key cannot be both kinds of withdrawal.
	shared := key("shared")
	if _, err := c.svc.RequestWithdrawal(ctx, walletusecases.RequestWithdrawalInput{
		ProviderType: "taxi", ProviderID: c.driverID, BankAccountID: c.account.ID,
		AmountKobo: 100_000, IdempotencyKey: shared, Pin: driverPIN,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.withdrawReferral(500_000, shared); errorStatus(err) != http.StatusConflict {
		t.Fatalf("reusing a job withdrawal's key = %v, want a conflict", err)
	}
}

// Referral-service decides: under review or blocked stops the payout, and no
// answer at all means no payout.
func TestReferralCashOutFailsClosed(t *testing.T) {
	c := newCashOut(t)
	c.creditReferral(t, 750_000)
	cases := []struct {
		answer referralcontract.WithdrawalEligibility
		err    error
		status int
	}{
		{referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleUnderReview}, nil, http.StatusForbidden},
		{referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleBlocked}, nil, http.StatusForbidden},
		{referralcontract.WithdrawalEligibility{}, errors.New("referral-service down"), http.StatusServiceUnavailable},
	}
	for i, tc := range cases {
		c.eligibility.set(tc.answer, tc.err)
		if _, err := c.withdrawReferral(500_000, key("refused")); errorStatus(err) != tc.status {
			t.Fatalf("case %d: error = %v, want HTTP %d", i, err, tc.status)
		}
	}
	if referral, _ := c.balances(t); referral != 750_000 {
		t.Fatalf("referral balance = %d after refused cash-outs, want untouched", referral)
	}
	if n := countWithdrawals(t, c.pool, c.driverID); n != 0 {
		t.Fatalf("%d withdrawals were created", n)
	}
	unconfigured := walletusecases.NewWalletService(walletusecases.Options{Repository: c.repo, Paystack: c.paystack, ReferralWithdrawalMinKobo: 500_000})
	if _, err := unconfigured.RequestReferralWithdrawal(context.Background(), walletusecases.RequestWithdrawalInput{
		ProviderType: "taxi", ProviderID: c.driverID, BankAccountID: c.account.ID, AmountKobo: 500_000, IdempotencyKey: key("unconfigured"), Pin: driverPIN,
	}); errorStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("with no referral-service configured = %v, want unavailable", err)
	}
	if _, err := c.svc.RequestReferralWithdrawal(context.Background(), walletusecases.RequestWithdrawalInput{
		ProviderType: "vendor_business", ProviderID: "v1", BankAccountID: c.account.ID, AmountKobo: 500_000, IdempotencyKey: key("vendor"), Pin: driverPIN,
	}); errorStatus(err) != http.StatusForbidden {
		t.Fatalf("a vendor asking = %v, want forbidden", err)
	}
}

// A payout that fails -- before the transfer starts, or when Paystack
// reports it failed -- goes back to the referral balance, never to job
// earnings.
func TestFailedReferralPayoutReturnsToReferralEarnings(t *testing.T) {
	c := newCashOut(t)
	ctx := context.Background()
	c.creditReferral(t, 1_200_000)
	_, jobBefore := c.balances(t)

	c.paystack.mu.Lock()
	c.paystack.failNext = true
	c.paystack.mu.Unlock()
	if _, err := c.withdrawReferral(500_000, key("transfer-fails-to-start")); errorStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("error = %v, want unavailable", err)
	}
	if referral, job := c.balances(t); referral != 1_200_000 || job != jobBefore {
		t.Fatalf("after a failed start: referral %d, job %d", referral, job)
	}

	withdrawal, err := c.withdrawReferral(700_000, key("transfer-fails-later"))
	if err != nil {
		t.Fatal(err)
	}
	var transferCode string
	if err := c.pool.QueryRow(ctx, `SELECT paystack_transfer_code FROM withdrawals WHERE id = $1`, withdrawal.ID).Scan(&transferCode); err != nil {
		t.Fatal(err)
	}
	if _, err := c.repo.ApplyTransferFailure(ctx, transferCode, "failed", "account closed"); err != nil {
		t.Fatal(err)
	}
	if referral, job := c.balances(t); referral != 1_200_000 || job != jobBefore {
		t.Fatalf("after a failed transfer: referral %d, job %d; want everything back in referral earnings", referral, job)
	}
}

func countWithdrawals(t *testing.T, pool *pgxpool.Pool, providerID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM withdrawals WHERE provider_id = $1`, providerID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
