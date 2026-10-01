package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/walletclient"
)

// reverse reports a refund of (part of) a paid job, as the wallet would.
func (p programme) reverse(t *testing.T, job referralcontract.JobSettled, amountKobo, feeKobo int64, fully bool) referralcontract.JobSettlementReversed {
	t.Helper()
	reversal := referralcontract.JobSettlementReversed{
		PaymentIntentID: job.PaymentIntentID, PaymentReference: job.PaymentReference,
		SourceService: job.SourceService, SourceReference: job.SourceReference,
		CustomerID: job.CustomerID, ProviderID: job.ProviderID, ProviderType: job.ProviderType,
		ReversalReference: "rev_" + uuid.NewString(), ReversedAmountKobo: amountKobo, ReversedPlatformFeeKobo: feeKobo,
		Currency: "NGN", ReversalKind: "refund", FullyReversed: fully, OccurredAt: p.clock.Now(),
	}
	p.deliverReversal(t, reversal)
	return reversal
}

func (p programme) deliverReversal(t *testing.T, reversal referralcontract.JobSettlementReversed) referralusecases.Outcome {
	t.Helper()
	return mustHandle(t, p.svc, referralusecases.ProducerWallet,
		envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettlementReversed, reversal.ReversalReference, reversal))
}

func (w *fakeWallet) reversalsTo(ownerID string) []walletclient.ReferralPostingRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []walletclient.ReferralPostingRequest
	for _, r := range w.reversal {
		if r.OwnerID == ownerID {
			out = append(out, r)
		}
	}
	return out
}

func (w *fakeWallet) setFailing(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failNext = n
}

func customerTrack(t *testing.T, p programme, referred account) (status, qualifyingReference string) {
	t.Helper()
	err := p.pool.QueryRow(context.Background(), `
		SELECT status, qualifying_source_reference FROM referral_tracks WHERE referral_id = $1 AND model = 'customer'`,
		referralOf(t, p.pool, referred)).Scan(&status, &qualifyingReference)
	if err != nil {
		t.Fatal(err)
	}
	return status, qualifyingReference
}

// qualifiedDriver has a customer refer a driver who is approved and does the
// five activation jobs, so the referral is qualified and its window open.
func (p programme) qualifiedDriver(t *testing.T, driverActor string) (referrer, driver account, start time.Time) {
	t.Helper()
	referrer, driver = newAccount(referralcontract.ActorCustomer), newAccount(driverActor)
	signUp(t, p.svc, referrer, "", "", true)
	start = p.clock.Now()
	p.bookings(t, referrer.actorID, 5, start)
	signUp(t, p.svc, driver, codeOf(t, p.svc, referrer), "", true)
	p.approve(t, driver, start)
	for i := 1; i <= 5; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), driverActor, driver.actorID, start.Add(time.Duration(i)*time.Minute), 750_000)
	}
	if status, _, _, _ := p.trackStatus(t, "provider"); status != "qualified" {
		t.Fatalf("setup: driver track = %s", status)
	}
	return referrer, driver, start
}

// Step 6 "done when": refunding a job reverses exactly that job's reward. A
// paid ₦500 bonus is taken back through the wallet, and the referred
// customer's next genuine booking qualifies the referral again.
func TestRefundedQualifyingBookingTakesBackTheBonus(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	first := p.book(t, bola, start.Add(time.Minute))
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if got := len(creditsFor(p.wallet.creditsTo(ada.actorID), "customer_referral_bonus")); got != 1 {
		t.Fatalf("bonuses paid = %d, want 1", got)
	}
	second := p.book(t, bola, start.Add(2*time.Hour))

	p.reverse(t, first, first.AmountKobo, first.PlatformFeeKobo, true)
	var originalStatus string
	if err := p.pool.QueryRow(context.Background(), `
		SELECT status FROM reward_ledger WHERE reward_type = 'customer_referral_bonus' AND source_reference = $1`,
		first.SourceReference).Scan(&originalStatus); err != nil || originalStatus != "reversed" {
		t.Fatalf("refunded booking's bonus = %s (%v), want reversed", originalStatus, err)
	}
	if status, ref := customerTrack(t, p, bola); status != "qualified" || ref != second.SourceReference {
		t.Fatalf("track = %s on %s, want re-qualified on the second booking %s", status, ref, second.SourceReference)
	}
	p.decideAndPost(t)
	takeBacks := p.wallet.reversalsTo(ada.actorID)
	if len(takeBacks) != 1 || takeBacks[0].AmountKobo != 50_000 || takeBacks[0].Balance != "earnings" || takeBacks[0].Reason != "reversal" {
		t.Fatalf("take-backs = %+v, want one ₦500 from Ada's earnings", takeBacks)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if got := len(creditsFor(p.wallet.creditsTo(ada.actorID), "customer_referral_bonus")); got != 2 {
		t.Fatalf("bonuses paid = %d, want 2 (one taken back, one for the new qualifying booking)", got)
	}
	if n, err := p.svc.ActivityCount(context.Background(), referralcontract.ActorCustomer, bola.actorID, start.Add(3*time.Hour)); err != nil || n != 1 {
		t.Fatalf("Bola's paid bookings = %d (%v), want 1: the refunded one no longer counts", n, err)
	}
}

// A refund that lands before the bonus was decided simply cancels it: no
// money moves either way.
func TestRefundBeforePayoutCancelsTheBonus(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	first := p.book(t, bola, start.Add(time.Minute))
	p.reverse(t, first, first.AmountKobo, first.PlatformFeeKobo, true)
	if st, reason, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); st != "rejected" || reason != "job_reversed" {
		t.Fatalf("bonus = %s/%s, want rejected for the refund", st, reason)
	}
	if status, _ := customerTrack(t, p, bola); status != "pending_qualification" {
		t.Fatalf("track = %s, want back to pending", status)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if len(creditsFor(p.wallet.creditsTo(ada.actorID), "customer_referral_bonus")) != 0 || len(p.wallet.reversalsTo(ada.actorID)) != 0 {
		t.Fatal("money moved for a bonus cancelled before payout")
	}
	p.book(t, bola, start.Add(3*time.Hour))
	if status, _ := customerTrack(t, p, bola); status != "qualified" {
		t.Fatalf("the next genuine booking did not qualify the referral (%s)", status)
	}
}

// A partial refund takes back the commission on the part of the platform fee
// that was refunded, worked out cumulatively; replays change nothing; a final
// full refund takes back exactly what is left.
func TestPartialRefundsTakeBackTheirShareOfCommission(t *testing.T) {
	p := newProgramme(t)
	ada, chidi, start := p.qualifiedDriver(t, referralcontract.ActorHauling)
	job := p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Hour), 750_000)
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)

	first := p.reverse(t, job, 300_000, 300_000, false)
	if outcome := p.deliverReversal(t, first); outcome != referralusecases.OutcomeDuplicate {
		t.Fatalf("replayed reversal = %s, want duplicate", outcome)
	}
	p.reverse(t, job, 300_000, 300_000, false)
	p.reverse(t, job, 400_000, 150_000, true)
	p.decideAndPost(t)

	var amounts []int64
	for _, r := range p.wallet.reversalsTo(ada.actorID) {
		amounts = append(amounts, r.AmountKobo)
	}
	if len(amounts) != 3 || amounts[0] != 30_000 || amounts[1] != 30_000 || amounts[2] != 15_000 {
		t.Fatalf("take-backs = %v, want [30000 30000 15000] (₦750 in total)", amounts)
	}
	var status string
	if err := p.pool.QueryRow(context.Background(), `
		SELECT status FROM reward_ledger WHERE reward_type = 'provider_commission' AND source_reference = $1`, job.SourceReference).Scan(&status); err != nil || status != "reversed" {
		t.Fatalf("commission = %s (%v), want reversed", status, err)
	}
	// Only that job's commission was touched, and the qualified track stands.
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'provider_commission' AND status = 'available'`); n != 5 {
		t.Fatalf("other commissions still paid = %d, want the 5 activation ones", n)
	}
	if st, _, _, _ := p.trackStatus(t, "provider"); st != "qualified" {
		t.Fatalf("a refund un-qualified the driver track (%s)", st)
	}
}

// Money is never taken back before it was paid: a take-back waits in the
// queue until the wallet has the reward it undoes.
func TestTakeBackWaitsForTheRewardItUndoes(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	ada, chidi, start := p.qualifiedDriver(t, referralcontract.ActorHauling)
	job := p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Hour), 750_000)
	p.clock.Advance(61 * time.Minute)
	if _, err := p.svc.DecidePendingRewards(ctx, 100); err != nil {
		t.Fatal(err)
	}
	p.wallet.setFailing(1_000)
	_, _ = p.svc.PostRewardsToWallet(ctx, 100) // the wallet is down

	p.reverse(t, job, job.AmountKobo, job.PlatformFeeKobo, true)
	p.wallet.setFailing(0)
	p.clock.Advance(time.Minute)
	if _, err := p.svc.PostRewardsToWallet(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if len(p.wallet.reversalsTo(ada.actorID)) != 0 {
		t.Fatal("the take-back was posted in the same pass as the credit it undoes")
	}
	if got := len(creditsFor(p.wallet.creditsTo(ada.actorID), "provider_commission")); got != 6 {
		t.Fatalf("commissions posted = %d, want 6", got)
	}
	if _, err := p.svc.PostRewardsToWallet(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if takeBacks := p.wallet.reversalsTo(ada.actorID); len(takeBacks) != 1 || takeBacks[0].AmountKobo != 75_000 {
		t.Fatalf("take-backs = %+v, want the full ₦750", takeBacks)
	}
}

// Before qualification, a refunded job stops counting towards the driver's
// five activation jobs and its held commission is cancelled; the next
// genuine job takes its place.
func TestRefundedActivationJobNoLongerCounts(t *testing.T) {
	p := newProgramme(t)
	ada, dayo := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, dayo, codeOf(t, p.svc, ada), "", true)
	p.approve(t, dayo, start)
	var jobs []referralcontract.JobSettled
	for i := 1; i <= 3; i++ {
		jobs = append(jobs, p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorDispatch, dayo.actorID, start.Add(time.Duration(i)*time.Hour), 100_000))
	}
	p.reverse(t, jobs[1], jobs[1].AmountKobo, jobs[1].PlatformFeeKobo, true)
	if status, counted, _, _ := p.trackStatus(t, "provider"); status != "pending_qualification" || counted != 2 {
		t.Fatalf("after the refund: %s/%d, want pending with 2 jobs", status, counted)
	}
	if n := p.commissions(t, "rejected"); n != 1 {
		t.Fatalf("cancelled commissions = %d, want the refunded job's", n)
	}
	for i := 4; i <= 6; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorDispatch, dayo.actorID, start.Add(time.Duration(i)*time.Hour), 100_000)
	}
	status, counted, windowStart, _ := p.trackStatus(t, "provider")
	if status != "qualified" || counted != 5 || !windowStart.Equal(start.Add(6*time.Hour)) {
		t.Fatalf("track = %s/%d from %v, want qualified on 5 genuine jobs from the 6th", status, counted, windowStart)
	}
	if held := p.commissions(t, "pending_activity"); held != 5 {
		t.Fatalf("released commissions = %d, want 5 (not the refunded job's)", held)
	}
}

// A reversal can overtake its settlement (they are separate events); it is
// kept and applied the moment the settlement arrives.
func TestReversalBeforeItsSettlement(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	job := jobSettled(bola.actorID, "drv_"+uuid.NewString(), start.Add(time.Minute))
	if outcome := p.deliverReversal(t, referralcontract.JobSettlementReversed{
		SourceService: job.SourceService, SourceReference: job.SourceReference, ReversalReference: "rev_" + uuid.NewString(),
		ReversedAmountKobo: job.AmountKobo, ReversedPlatformFeeKobo: job.PlatformFeeKobo, FullyReversed: true, OccurredAt: start,
	}); outcome != referralusecases.OutcomeProcessed {
		t.Fatalf("early reversal = %s", outcome)
	}
	mustHandle(t, p.svc, referralusecases.ProducerWallet, envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	if status, _ := customerTrack(t, p, bola); status != "pending_qualification" {
		t.Fatalf("a refunded booking qualified the referral (%s)", status)
	}
	if st, reason, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); st != "rejected" || reason != "job_reversed" {
		t.Fatalf("bonus = %s/%s", st, reason)
	}
	if n, _ := p.svc.ActivityCount(context.Background(), referralcontract.ActorCustomer, bola.actorID, start.Add(time.Hour)); n != 0 {
		t.Fatalf("a refunded booking counts as activity (%d)", n)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM settled_job_reversals WHERE applied_at IS NULL`); n != 0 {
		t.Fatalf("%d reversals left unapplied", n)
	}
}

// Repeated refunds on one referral put it under review: later commission
// freezes instead of paying until an operator looks.
func TestRepeatedRefundsPutTheReferralUnderReview(t *testing.T) {
	p := newProgramme(t)
	_, chidi, start := p.qualifiedDriver(t, referralcontract.ActorHauling)
	for i := 1; i <= 3; i++ {
		job := p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour), 750_000)
		p.reverse(t, job, job.AmountKobo, job.PlatformFeeKobo, true)
	}
	var status string
	if err := p.pool.QueryRow(context.Background(), `SELECT status FROM referrals`).Scan(&status); err != nil || status != "under_review" {
		t.Fatalf("referral = %s (%v), want under review after 3 refunds", status, err)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'repeated_reversals' AND status = 'open'`); n != 1 {
		t.Fatalf("repeated-refund flags = %d, want 1", n)
	}
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(5*time.Hour), 750_000)
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if n := p.commissions(t, "frozen"); n != 6 {
		t.Fatalf("frozen commissions = %d, want the 5 held ones and the new one", n)
	}
}

// A settlement and its reversal handled at the same moment must never miss
// each other (each would otherwise see the other as "not arrived yet"), and
// the same reversal delivered twice at once applies once.
func TestConcurrentSettlementAndReversal(t *testing.T) {
	p := newProgramme(t)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	friends := p.referFriends(t, ada, 8)
	for i, friend := range friends {
		job := jobSettled(friend.actorID, "drv_"+uuid.NewString(), start.Add(time.Duration(i+1)*time.Minute))
		reversal := referralcontract.JobSettlementReversed{
			SourceService: job.SourceService, SourceReference: job.SourceReference, ReversalReference: "rev_" + uuid.NewString(),
			ReversedAmountKobo: job.AmountKobo, ReversedPlatformFeeKobo: job.PlatformFeeKobo, FullyReversed: true, OccurredAt: start,
		}
		deliveries := [][]byte{
			envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job),
			envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettlementReversed, reversal.ReversalReference, reversal),
			envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettlementReversed, reversal.ReversalReference, reversal),
		}
		errs := make(chan error, len(deliveries))
		for _, raw := range deliveries {
			go func(raw []byte) {
				_, err := p.svc.HandleEvent(context.Background(), referralusecases.ProducerWallet, raw)
				errs <- err
			}(raw)
		}
		for range deliveries {
			if err := <-errs; err != nil {
				t.Fatalf("delivery failed: %v", err)
			}
		}
		if status, _ := customerTrack(t, p, friend); status != "pending_qualification" {
			t.Fatalf("round %d: a refunded booking left the referral %s", i, status)
		}
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'customer_referral_bonus' AND status <> 'rejected'`); n != 0 {
		t.Fatalf("%d bonuses survived their booking's refund", n)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM settled_job_reversals`); n != len(friends) {
		t.Fatalf("reversals recorded = %d, want %d (one per reference)", n, len(friends))
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_milestones`); n != 0 {
		t.Fatal("refunded bookings reached a milestone")
	}
}

// An operator taking rewards back is not a refund: it never counts towards
// the repeated-refund review.
func TestAdminTakeBacksAreNotRefunds(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	_, chidi, start := p.qualifiedDriver(t, referralcontract.ActorHauling)
	for i := 1; i <= 3; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour), 750_000)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	rows, err := p.pool.Query(ctx, `SELECT id FROM reward_ledger WHERE reward_type = 'provider_commission' AND status = 'available' LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 {
		t.Fatalf("paid commissions = %d, want at least 3 to take back", len(ids))
	}
	for _, id := range ids {
		if err := p.svc.ReverseReward(ctx, id, adminReq); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	if err := p.pool.QueryRow(ctx, `SELECT status FROM referrals`).Scan(&status); err != nil || status != "active" {
		t.Fatalf("referral = %s (%v) after operator take-backs, want still active", status, err)
	}
}
