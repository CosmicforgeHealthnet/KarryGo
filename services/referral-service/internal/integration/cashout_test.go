package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func eligibility(t *testing.T, p programme, driver account, bank string) referralcontract.WithdrawalEligibility {
	t.Helper()
	answer, err := p.svc.CheckWithdrawal(context.Background(), referralcontract.WithdrawalEligibilityRequest{
		ActorType: driver.actorType, ActorID: driver.actorID, AmountKobo: 500_000,
		BankAccountHash: referralcontract.HashBankAccount(testPepper, "058", bank),
	})
	if err != nil {
		t.Fatalf("CheckWithdrawal() error = %v", err)
	}
	return answer
}

// A driver in good standing may cash out; a freeze, a block or an open review
// stops them; an unknown account is refused.
func TestWithdrawalEligibility(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	chidi := newAccount(referralcontract.ActorHauling)
	if answer := eligibility(t, p, chidi, "0123456789"); answer.Eligible || answer.Reason != referralcontract.IneligibleUnknownAccount {
		t.Fatalf("unannounced account = %+v", answer)
	}
	signUp(t, p.svc, chidi, "", "", true)
	if answer := eligibility(t, p, chidi, "0123456789"); !answer.Eligible {
		t.Fatalf("driver in good standing = %+v", answer)
	}
	person := personOf(t, p.pool, chidi)
	if err := p.svc.FreezePerson(ctx, person, adminReq); err != nil {
		t.Fatal(err)
	}
	if answer := eligibility(t, p, chidi, "0123456789"); answer.Eligible || answer.Reason != referralcontract.IneligibleUnderReview {
		t.Fatalf("frozen driver = %+v", answer)
	}
	if err := p.svc.ClearPerson(ctx, person, adminReq); err != nil {
		t.Fatal(err)
	}
	if err := p.svc.BlockPerson(ctx, person, adminReq); err != nil {
		t.Fatal(err)
	}
	if answer := eligibility(t, p, chidi, "0123456789"); answer.Eligible || answer.Reason != referralcontract.IneligibleBlocked {
		t.Fatalf("blocked driver = %+v", answer)
	}
}

// One bank account receiving two people's referral money is flagged, and
// neither may cash out until a reviewer resolves it; the same person reusing
// their own account is fine.
func TestSharedBankAccountStopsCashOut(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	chidi, dayo := newAccount(referralcontract.ActorHauling), newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, chidi, "", "", true)
	signUp(t, p.svc, dayo, "", "", true)
	for i := 0; i < 2; i++ {
		if answer := eligibility(t, p, chidi, "1111111111"); !answer.Eligible {
			t.Fatalf("reusing one's own account: %+v", answer)
		}
	}
	if answer := eligibility(t, p, dayo, "1111111111"); answer.Eligible || answer.Reason != referralcontract.IneligibleUnderReview {
		t.Fatalf("second person on the account = %+v", answer)
	}
	if answer := eligibility(t, p, chidi, "1111111111"); answer.Eligible {
		t.Fatal("the account's first user was not held once it turned out to be shared")
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'duplicate_bank' AND status = 'open'`); n != 2 {
		t.Fatalf("duplicate-bank flags = %d, want one per person", n)
	}
	flags, err := p.svc.ListFlags(ctx, "open", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range flags {
		if flag.PersonID == personOf(t, p.pool, dayo) {
			if err := p.svc.ResolveFlag(ctx, flag.ID, "cleared", adminReq); err != nil {
				t.Fatal(err)
			}
		}
	}
	if answer := eligibility(t, p, dayo, "1111111111"); !answer.Eligible {
		t.Fatalf("after the reviewer cleared Dayo: %+v", answer)
	}
}

// Only payment-wallet may ask, over a signed request.
func TestWithdrawalEligibilityOverHTTP(t *testing.T) {
	p := newProgramme(t)
	chidi := newAccount(referralcontract.ActorTaxi)
	signUp(t, p.svc, chidi, "", "", true)
	walletSecret, adminSecret := []byte("wallet-secret"), []byte("admin-secret")
	router := signedRouter(t, p.pool, p.svc, serviceauth.Secrets{"payment-wallet-service": walletSecret, "admin-backoffice-service": adminSecret})
	body, _ := json.Marshal(referralcontract.WithdrawalEligibilityRequest{
		ActorType: chidi.actorType, ActorID: chidi.actorID, AmountKobo: 500_000, BankAccountHash: "hash-" + uuid.NewString(),
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedRequest(t, http.MethodPost, referralcontract.WithdrawalEligibilityPath, "admin-backoffice-service", adminSecret, body))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin service asking = %d, want 403", recorder.Code)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	client := referralcontract.Client{BaseURL: server.URL, ServiceName: "payment-wallet-service", Secret: walletSecret}
	var request referralcontract.WithdrawalEligibilityRequest
	_ = json.Unmarshal(body, &request)
	answer, err := client.CheckWithdrawal(context.Background(), request)
	if err != nil || !answer.Eligible {
		t.Fatalf("wallet asking = %+v, %v", answer, err)
	}
}

// Reconciliation: what referral-service sent every account matches the
// wallet's ledger; a posting the wallet has but we never sent is recorded,
// and the record resolves itself once the two agree again. A posting in
// flight (sent, not yet confirmed) is not a mismatch.
func TestReconcileWithWallet(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	first := p.book(t, bola, start.Add(time.Minute))
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	p.reverse(t, first, first.AmountKobo, first.PlatformFeeKobo, true)
	p.decideAndPost(t)

	result, err := p.svc.ReconcileWithWallet(ctx, 1) // one account per page: exercises paging
	if err != nil || result.Checked != 2 || result.Mismatched != 0 {
		t.Fatalf("clean run = %+v, %v; want 2 accounts checked, none mismatched", result, err)
	}

	p.wallet.mu.Lock()
	p.wallet.phantom = map[string]int64{ada.actorID: 1_000}
	p.wallet.mu.Unlock()
	if result, err := p.svc.ReconcileWithWallet(ctx, 50); err != nil || result.Mismatched != 1 {
		t.Fatalf("phantom credit run = %+v, %v", result, err)
	}
	if result, err := p.svc.ReconcileWithWallet(ctx, 50); err != nil || result.Mismatched != 1 {
		t.Fatalf("second run = %+v, %v", result, err)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reconciliation_mismatches WHERE resolved_at IS NULL`); n != 1 {
		t.Fatalf("open mismatches = %d, want the one, refreshed rather than duplicated", n)
	}
	open, err := p.svc.OpenMismatches(ctx, 10)
	if err != nil || len(open) != 1 || open[0].ActorID != ada.actorID || open[0].WalletCreditKobo != open[0].PostedCreditKobo+1_000 {
		t.Fatalf("open mismatches = %+v, %v", open, err)
	}

	p.wallet.mu.Lock()
	p.wallet.phantom = nil
	p.wallet.mu.Unlock()
	if result, err := p.svc.ReconcileWithWallet(ctx, 50); err != nil || result.Mismatched != 0 {
		t.Fatalf("after the fix = %+v, %v", result, err)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reconciliation_mismatches WHERE resolved_at IS NOT NULL`); n != 1 {
		t.Fatal("the mismatch was not resolved when the two agreed again")
	}

	// A crash after the wallet paid but before we recorded it: in flight,
	// not a mismatch.
	if _, err := p.pool.Exec(ctx, `UPDATE reward_ledger SET wallet_state = 'pending' WHERE beneficiary_actor_id = $1 AND reward_type = 'customer_welcome_credit'`, bola.actorID); err != nil {
		t.Fatal(err)
	}
	if result, err := p.svc.ReconcileWithWallet(ctx, 50); err != nil || result.Mismatched != 0 {
		t.Fatalf("in-flight run = %+v, %v", result, err)
	}
}
