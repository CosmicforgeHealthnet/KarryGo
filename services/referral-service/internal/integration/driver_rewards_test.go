package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// approve announces a driver becoming fully approved, from its own service.
func (p programme) approve(t *testing.T, a account, at time.Time) {
	t.Helper()
	payload := referralcontract.ProviderOnboardingCompleted{ActorType: a.actorType, ActorID: a.actorID, CompletedAt: at}
	producer := producerFor(a.actorType)
	mustHandle(t, p.svc, producer, envelopeJSON(t, producer, referralcontract.EventProviderOnboardingCompleted, a.actorID, payload))
}

func (p programme) trackStatus(t *testing.T, model string) (status string, jobsCounted int, windowStart, windowEnd *time.Time) {
	t.Helper()
	err := p.pool.QueryRow(context.Background(), `SELECT status, jobs_counted, window_starts_at, window_ends_at FROM referral_tracks WHERE model = $1`, model).
		Scan(&status, &jobsCounted, &windowStart, &windowEnd)
	if err != nil {
		t.Fatalf("read %s track: %v", model, err)
	}
	return status, jobsCounted, windowStart, windowEnd
}

func (p programme) commissions(t *testing.T, status string) int {
	t.Helper()
	return countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'provider_commission' AND status = $1`, status)
}

// The plan's example: a customer refers a driver. Commission on the first
// five jobs is held and released at qualification; every later job inside
// the 90-day window pays 10% of the platform's cut; the window is judged by
// when each job finished.
func TestCustomerRefersADriver(t *testing.T) {
	p := newProgramme(t)
	ada, chidi := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorHauling)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start) // Ada is an active customer
	signUp(t, p.svc, chidi, codeOf(t, p.svc, ada), "", true)
	p.approve(t, chidi, start)

	// Five activation hauls, each with a ₦7,500 platform cut.
	for i := 1; i <= 5; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour), 750_000)
		if i < 5 && p.commissions(t, "pending_qualification") != i {
			t.Fatalf("after job %d: held commissions = %d", i, p.commissions(t, "pending_qualification"))
		}
	}
	status, counted, windowStart, windowEnd := p.trackStatus(t, "provider")
	if status != "qualified" || counted != 5 || windowStart == nil || !windowStart.Equal(start.Add(5*time.Hour)) || !windowEnd.Equal(windowStart.Add(90*24*time.Hour)) {
		t.Fatalf("track = %s/%d window %v..%v", status, counted, windowStart, windowEnd)
	}
	if held := p.commissions(t, "pending_activity"); held != 5 {
		t.Fatalf("released commissions = %d, want the 5 held ones", held)
	}
	if st, _, _ := rewardStatus(t, p.pool, "provider_welcome_credit", chidi.actorID); st != "available" {
		t.Fatalf("driver welcome credit = %s", st)
	}

	// Job 6, inside the window.
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(6*time.Hour), 750_000)
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if got := p.commissions(t, "available"); got != 6 {
		t.Fatalf("paid commissions = %d, want 6", got)
	}
	credits := p.wallet.creditsTo(ada.actorID)
	if len(credits) != 6 {
		t.Fatalf("Ada was credited %d times, want 6", len(credits))
	}
	for _, credit := range credits {
		if credit.AmountKobo != 75_000 || credit.Balance != "earnings" || credit.OwnerType != "customer" {
			t.Fatalf("commission credit = %+v, want ₦750 of earnings to Ada's customer account", credit)
		}
	}
	welcome := p.wallet.creditsTo(chidi.actorID)
	if len(welcome) != 1 || welcome[0].OwnerType != "provider" || welcome[0].ProviderType != "truck_provider" || welcome[0].Balance != "promotional" {
		t.Fatalf("driver welcome credit = %+v", welcome)
	}

	// A haul finished on day 91 earns nothing; one finished on day 89 but paid
	// out after the window was marked expired still pays.
	late := windowStart.Add(91 * 24 * time.Hour)
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, late, 750_000)
	p.clock.Advance(100 * 24 * time.Hour)
	if n, err := p.svc.ExpireDriverWindows(context.Background()); err != nil || n != 1 {
		t.Fatalf("expired %d windows, %v", n, err)
	}
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, windowStart.Add(89*24*time.Hour), 750_000)
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_track_jobs WHERE counted_as = 'not_counted' AND reason = 'outside_window'`); n != 1 {
		t.Fatalf("outside-window jobs = %d, want the day-91 haul", n)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'provider_commission'`); n != 7 {
		t.Fatalf("commissions = %d, want 7 (day-89 haul pays, day-91 does not)", n)
	}
}

// Approval can come after the activation jobs: the window then opens at the
// approval, and jobs done while waiting for approval (beyond the first five)
// earn nothing.
func TestApprovalAfterActivationJobs(t *testing.T) {
	p := newProgramme(t)
	ada, dayo := newAccount(referralcontract.ActorDispatch), newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, dayo, codeOf(t, p.svc, ada), "", true)
	start := p.clock.Now()
	for i := 1; i <= 6; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorDispatch, dayo.actorID, start.Add(time.Duration(i)*time.Hour), 100_000)
	}
	if status, counted, _, _ := p.trackStatus(t, "provider"); status != "pending_qualification" || counted != 5 {
		t.Fatalf("before approval: %s/%d", status, counted)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_track_jobs WHERE reason = 'awaiting_approval'`); n != 1 {
		t.Fatalf("jobs waiting on approval = %d, want the 6th", n)
	}
	approvedAt := start.Add(10 * time.Hour)
	p.approve(t, dayo, approvedAt)
	status, _, windowStart, _ := p.trackStatus(t, "provider")
	if status != "qualified" || !windowStart.Equal(approvedAt) {
		t.Fatalf("after approval: %s window from %v, want qualified from %v", status, windowStart, approvedAt)
	}
	if held := p.commissions(t, "pending_activity"); held != 5 {
		t.Fatalf("released = %d, want exactly the first 5", held)
	}
}

// An approval that arrives before the driver's own sign-up announcement is
// kept and applied when the announcement lands.
func TestApprovalBeforeSignUpAnnouncement(t *testing.T) {
	p := newProgramme(t)
	ada, tunde := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorTaxi)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.approve(t, tunde, start)
	signUp(t, p.svc, tunde, codeOf(t, p.svc, ada), "", true)
	for i := 1; i <= 5; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorTaxi, tunde.actorID, start.Add(time.Duration(i)*time.Hour), 50_000)
	}
	if status, _, _, _ := p.trackStatus(t, "provider"); status != "qualified" {
		t.Fatalf("track = %s; the early approval was lost", status)
	}
}

// Jobs with no platform fee earn no commission and do not count towards
// activation.
func TestZeroFeeJobsDoNotCount(t *testing.T) {
	p := newProgramme(t)
	ada, dayo := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, dayo, codeOf(t, p.svc, ada), "", true)
	p.approve(t, dayo, p.clock.Now())
	for i := 1; i <= 5; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorDispatch, dayo.actorID, p.clock.Now().Add(time.Duration(i)*time.Minute), 0)
	}
	if status, counted, _, _ := p.trackStatus(t, "provider"); status != "pending_qualification" || counted != 0 {
		t.Fatalf("zero-fee jobs moved the track to %s/%d", status, counted)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'provider_commission'`); n != 0 {
		t.Fatalf("zero-fee jobs created %d commissions", n)
	}
}

// Held commission is judged job by job: the referrer's activity at the moment
// each job finished decides that job's payment.
func TestHeldCommissionIsJudgedPerJob(t *testing.T) {
	p := newProgramme(t)
	ada, chidi := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorHauling)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, chidi, codeOf(t, p.svc, ada), "", true)
	start := p.clock.Now()
	p.approve(t, chidi, start)
	// Jobs 1 and 2 happen before Ada becomes active; 3-5 after.
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(1*time.Hour), 750_000)
	p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(2*time.Hour), 750_000)
	// Ada's five bookings all fall strictly between job 2 and job 3.
	for i := 1; i <= 5; i++ {
		p.paidJob(t, ada.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), start.Add(2*time.Hour+time.Duration(i*10)*time.Minute), 10_000)
	}
	for i := 3; i <= 5; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour+30*time.Minute), 750_000)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if got, want := p.commissions(t, "rejected"), 2; got != want {
		t.Fatalf("rejected = %d, want %d (Ada was inactive for jobs 1-2)", got, want)
	}
	if got, want := p.commissions(t, "available"), 3; got != want {
		t.Fatalf("paid = %d, want %d", got, want)
	}
}

// Replaying a driver's jobs cannot count them twice.
func TestReplayedDriverJobsCountOnce(t *testing.T) {
	p := newProgramme(t)
	ada, dayo := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, dayo, codeOf(t, p.svc, ada), "", true)
	job := p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorDispatch, dayo.actorID, p.clock.Now(), 100_000)
	for i := 0; i < 3; i++ {
		mustHandle(t, p.svc, referralusecases.ProducerWallet, envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	}
	if _, counted, _, _ := p.trackStatus(t, "provider"); counted != 1 {
		t.Fatalf("jobs counted = %d after replays, want 1", counted)
	}
}
