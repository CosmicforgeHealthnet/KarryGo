package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/walletclient"
)

// fakeWallet records postings and can be told to fail.
type fakeWallet struct {
	mu       sync.Mutex
	credits  []walletclient.ReferralPostingRequest
	reversal []walletclient.ReferralPostingRequest
	failNext int
	// phantom adds credits the wallet holds that referral-service never
	// sent, keyed by owner id (for reconciliation tests).
	phantom map[string]int64
}

func (w *fakeWallet) post(list *[]walletclient.ReferralPostingRequest, request walletclient.ReferralPostingRequest) (walletclient.ReferralPosting, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failNext > 0 {
		w.failNext--
		return walletclient.ReferralPosting{}, errors.New("wallet unavailable")
	}
	*list = append(*list, request)
	return walletclient.ReferralPosting{TransactionReference: "ldt_" + uuid.NewString()}, nil
}

func (w *fakeWallet) CreditReferral(_ context.Context, request walletclient.ReferralPostingRequest) (walletclient.ReferralPosting, error) {
	return w.post(&w.credits, request)
}

func (w *fakeWallet) ReverseReferral(_ context.Context, request walletclient.ReferralPostingRequest) (walletclient.ReferralPosting, error) {
	return w.post(&w.reversal, request)
}

// ReferralTotals reports what was posted to the wallet for one owner, as the
// real wallet's ledger would.
func (w *fakeWallet) ReferralTotals(_ context.Context, ownerType, ownerID, _ string) (walletclient.ReferralTotals, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	totals := walletclient.ReferralTotals{OwnerType: ownerType, OwnerID: ownerID, CreditedKobo: w.phantom[ownerID]}
	for _, c := range w.credits {
		if c.OwnerID == ownerID && c.OwnerType == ownerType {
			totals.CreditedKobo += c.AmountKobo
		}
	}
	for _, r := range w.reversal {
		if r.OwnerID == ownerID && r.OwnerType == ownerType {
			totals.ReversedKobo += r.AmountKobo
		}
	}
	return totals, nil
}

func (w *fakeWallet) creditsTo(ownerID string) []walletclient.ReferralPostingRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []walletclient.ReferralPostingRequest
	for _, c := range w.credits {
		if c.OwnerID == ownerID {
			out = append(out, c)
		}
	}
	return out
}

// clock is a controllable "now" for the service.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// programme bundles a service with its clock and fake wallet.
type programme struct {
	svc    *referralusecases.Service
	clock  *clock
	wallet *fakeWallet
	pool   *pgxpool.Pool
}

func newProgramme(t *testing.T) programme {
	t.Helper()
	pool := testPool(t)
	c := &clock{now: time.Now().UTC().Truncate(time.Second)}
	w := &fakeWallet{}
	svc := referralusecases.NewService(referralusecases.Options{
		Store:         referralStore(pool),
		Rules:         defaultRules(),
		ShareLinkBase: "https://example.test/r/",
		Now:           c.Now,
		Wallet:        w,
	})
	return programme{svc: svc, clock: c, wallet: w, pool: pool}
}

// paidJob settles a job between a customer and a driver at completedAt.
func (p programme) paidJob(t *testing.T, customerID, driverActor, driverID string, completedAt time.Time, feeKobo int64) referralcontract.JobSettled {
	t.Helper()
	job := jobSettled(customerID, driverID, completedAt)
	job.ProviderType = referralcontract.WalletProviderTypeForActor(driverActor)
	job.PlatformFeeKobo = feeKobo
	job.ProviderNetKobo = job.AmountKobo - feeKobo
	mustHandle(t, p.svc, referralusecases.ProducerWallet, envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	return job
}

// bookings gives a customer n paid bookings ending just before at.
func (p programme) bookings(t *testing.T, customerID string, n int, before time.Time) {
	t.Helper()
	for i := 1; i <= n; i++ {
		p.paidJob(t, customerID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), before.Add(-time.Duration(i)*time.Hour), 10_000)
	}
}

func (p programme) decideAndPost(t *testing.T) {
	t.Helper()
	if _, err := p.svc.DecidePendingRewards(context.Background(), 100); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, err := p.svc.PostRewardsToWallet(context.Background(), 100); err != nil {
		t.Fatalf("post: %v", err)
	}
}

func rewardStatus(t *testing.T, pool *pgxpool.Pool, rewardType, beneficiaryID string) (status, reason, walletState string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT status, status_reason, wallet_state FROM reward_ledger
		WHERE reward_type = $1 AND beneficiary_actor_id = $2`, rewardType, beneficiaryID).Scan(&status, &reason, &walletState)
	if err != nil {
		t.Fatalf("read %s reward for %s: %v", rewardType, beneficiaryID, err)
	}
	return status, reason, walletState
}

// Step 4 "done when": the whole customer journey works end to end.
func TestCustomerReferralJourney(t *testing.T) {
	p := newProgramme(t)
	svc := p.svc
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	p.bookings(t, ada.actorID, 5, p.clock.Now()) // Ada is an active customer
	signUp(t, svc, bola, codeOf(t, svc, ada), "", true)

	// Bola's welcome and first-booking credits are available at once.
	for _, rewardType := range []string{"customer_welcome_credit", "first_booking_credit"} {
		if status, _, wallet := rewardStatus(t, p.pool, rewardType, bola.actorID); status != "available" || wallet != "pending" {
			t.Fatalf("%s: status=%s wallet=%s", rewardType, status, wallet)
		}
	}

	job := p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), p.clock.Now(), 150_000)
	if status, _, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "pending_activity" {
		t.Fatalf("bonus status = %s, want pending_activity", status)
	}

	// Nothing is decided before the hour's wait for late activity is up.
	if n, _ := svc.DecidePendingRewards(context.Background(), 100); n != 0 {
		t.Fatalf("decided %d rewards before the wait was over", n)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)

	if status, _, wallet := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "available" || wallet != "posted" {
		t.Fatalf("bonus after decision: status=%s wallet=%s", status, wallet)
	}
	adaCredits := p.wallet.creditsTo(ada.actorID)
	if len(adaCredits) != 1 || adaCredits[0].AmountKobo != 50_000 || adaCredits[0].Balance != "earnings" || adaCredits[0].OwnerType != "customer" {
		t.Fatalf("Ada's wallet credits = %+v", adaCredits)
	}
	bolaCredits := p.wallet.creditsTo(bola.actorID)
	if len(bolaCredits) != 2 || bolaCredits[0].Balance != "promotional" || bolaCredits[1].Balance != "promotional" {
		t.Fatalf("Bola's wallet credits = %+v", bolaCredits)
	}

	// The same job replayed under a new event id, and Bola's second booking,
	// pay nothing more.
	mustHandle(t, svc, referralusecases.ProducerWallet, envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), p.clock.Now(), 150_000)
	p.clock.Advance(2 * time.Hour)
	p.decideAndPost(t)
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'customer_referral_bonus'`); n != 1 {
		t.Fatalf("bonuses = %d, want exactly 1", n)
	}
	if got := p.wallet.creditsTo(ada.actorID); len(got) != 1 {
		t.Fatalf("Ada was credited %d times, want once", len(got))
	}
}

// A referrer who was not active when the referral qualified loses that bonus
// (no back-payment), though the referral still counts as qualified.
func TestInactiveReferrerForfeitsTheBonus(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	p.bookings(t, ada.actorID, 4, p.clock.Now()) // one short
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), p.clock.Now(), 150_000)
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)

	status, reason, wallet := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID)
	if status != "rejected" || reason != "referrer_inactive" || wallet != "not_sent" {
		t.Fatalf("bonus = %s/%s/%s, want rejected/referrer_inactive/not_sent", status, reason, wallet)
	}
	if got := p.wallet.creditsTo(ada.actorID); len(got) != 0 {
		t.Fatal("an inactive referrer was paid")
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_tracks WHERE status = 'qualified'`); n != 1 {
		t.Fatal("the referral must still count as qualified")
	}
	// Becoming active later does not bring the lost bonus back.
	p.bookings(t, ada.actorID, 5, p.clock.Now())
	p.clock.Advance(2 * time.Hour)
	p.decideAndPost(t)
	if status, _, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "rejected" {
		t.Fatalf("a forfeited bonus came back as %s", status)
	}
}

// Activity is judged as of the moment the qualifying job finished, not when
// the decision happens.
func TestActivityIsJudgedAsOfTheJob(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	jobTime := p.clock.Now()
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), jobTime, 150_000)
	// Ada's bookings all happen after Bola's job.
	p.bookings(t, ada.actorID, 5, jobTime.Add(10*time.Hour))
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if status, reason, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "rejected" || reason != "referrer_inactive" {
		t.Fatalf("bonus = %s/%s; activity after the job must not count", status, reason)
	}
}

// Activity that happened before the job but arrived late (within the wait)
// still counts: that is what the wait is for.
func TestLateArrivingActivityStillCounts(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	jobTime := p.clock.Now()
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), jobTime, 150_000)
	p.clock.Advance(30 * time.Minute)
	p.bookings(t, ada.actorID, 5, jobTime) // completed before the job, delivered after it
	p.clock.Advance(31 * time.Minute)
	p.decideAndPost(t)
	if status, _, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "available" {
		t.Fatalf("bonus = %s; activity delivered late but completed in time must count", status)
	}
}

// A job whose announcement beats the customer's own sign-up announcement is
// caught up when the sign-up arrives.
func TestLateSignUpAnnouncementCatchesUp(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	p.bookings(t, ada.actorID, 5, p.clock.Now())
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), p.clock.Now(), 150_000)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true) // arrives after the job
	if status, _, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "pending_activity" {
		t.Fatalf("bonus = %s after catch-up, want pending_activity", status)
	}
}

// A wallet outage delays payment; it never loses or duplicates it.
func TestWalletFailureRetriesWithTheSameKey(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	p.wallet.failNext = 1
	if _, err := p.svc.PostRewardsToWallet(context.Background(), 1); err == nil {
		t.Fatal("expected the wallet failure to be reported")
	}
	if _, _, wallet := rewardStatus(t, p.pool, "customer_welcome_credit", bola.actorID); wallet != "pending" {
		t.Fatalf("after a failure wallet_state = %s, want pending", wallet)
	}
	// Retried only after the backoff.
	if n, _ := p.svc.PostRewardsToWallet(context.Background(), 100); n != 1 {
		t.Fatalf("posted %d immediately; the failed reward must wait for its backoff", n)
	}
	p.clock.Advance(time.Minute)
	if n, err := p.svc.PostRewardsToWallet(context.Background(), 100); err != nil || n != 1 {
		t.Fatalf("retry posted %d, %v", n, err)
	}
	credits := p.wallet.creditsTo(bola.actorID)
	if len(credits) != 2 || credits[0].IdempotencyKey == credits[1].IdempotencyKey {
		t.Fatalf("credits = %+v; two distinct rewards, each posted once", credits)
	}
}

// Rewards for someone under review are held, not paid.
func TestFrozenReferrerRewardsAreHeld(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	p.bookings(t, ada.actorID, 5, p.clock.Now())
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	if _, err := p.pool.Exec(context.Background(), `UPDATE referral_persons SET status = 'frozen' WHERE id = (SELECT person_id FROM referral_actors WHERE actor_id = $1)`, ada.actorID); err != nil {
		t.Fatal(err)
	}
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), p.clock.Now(), 150_000)
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if status, _, wallet := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); status != "frozen" || wallet != "not_sent" {
		t.Fatalf("bonus = %s/%s, want frozen and unpaid", status, wallet)
	}
}
