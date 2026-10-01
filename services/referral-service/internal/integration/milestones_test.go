package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/walletclient"
)

// adminReq is an operator acting through the admin service.
var adminReq = referralusecases.AdminRequest{Caller: "admin-backoffice-service", PerformedBy: "ops@karrygo.test", Reason: "integration test"}

// personOf is the person behind an account.
func personOf(t *testing.T, pool *pgxpool.Pool, a account) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT person_id FROM referral_actors WHERE actor_type = $1 AND actor_id = $2`,
		a.actorType, a.actorID).Scan(&id); err != nil {
		t.Fatalf("person of %s: %v", a.actorID, err)
	}
	return id
}

// referralOf is the referral an account's person joined through.
func referralOf(t *testing.T, pool *pgxpool.Pool, a account) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		SELECT r.id FROM referrals r JOIN referral_actors a ON a.person_id = r.referred_person_id
		WHERE a.actor_type = $1 AND a.actor_id = $2`, a.actorType, a.actorID).Scan(&id); err != nil {
		t.Fatalf("referral of %s: %v", a.actorID, err)
	}
	return id
}

func creditsFor(credits []walletclient.ReferralPostingRequest, reason string) []walletclient.ReferralPostingRequest {
	var out []walletclient.ReferralPostingRequest
	for _, c := range credits {
		if c.Reason == reason {
			out = append(out, c)
		}
	}
	return out
}

// referFriends signs up n new customers with the referrer's code.
func (p programme) referFriends(t *testing.T, referrer account, n int) []account {
	t.Helper()
	code := codeOf(t, p.svc, referrer)
	friends := make([]account, n)
	for i := range friends {
		friends[i] = newAccount(referralcontract.ActorCustomer)
		signUp(t, p.svc, friends[i], code, "", true)
	}
	return friends
}

// book gives a customer one paid ride with a stranger at completedAt.
func (p programme) book(t *testing.T, customer account, completedAt time.Time) referralcontract.JobSettled {
	t.Helper()
	return p.paidJob(t, customer.actorID, referralcontract.ActorTaxi, "drv_"+uuid.NewString(), completedAt, 10_000)
}

// The 5th qualified customer referral earns the free-delivery credit and the
// 10th the ₦7,500 bonus and the Power User badge -- each exactly once.
func TestMilestonesAtFiveAndTen(t *testing.T) {
	p := newProgramme(t)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	friends := p.referFriends(t, ada, 10)
	var jobs []referralcontract.JobSettled
	for i, friend := range friends {
		jobs = append(jobs, p.book(t, friend, start.Add(time.Duration(i+1)*time.Minute)))
		want := 0
		if i+1 >= 5 {
			want = 1
		}
		if i+1 >= 10 {
			want = 2
		}
		if got := countRows(t, p.pool, `SELECT count(*) FROM referral_milestones`); got != want {
			t.Fatalf("after %d qualified referrals: %d milestones, want %d", i+1, got, want)
		}
	}
	var fifthAt time.Time
	if err := p.pool.QueryRow(context.Background(), `SELECT achieved_at FROM referral_milestones WHERE milestone = 'qualified_5'`).Scan(&fifthAt); err != nil {
		t.Fatal(err)
	}
	if !fifthAt.Equal(jobs[4].JobCompletedAt) {
		t.Fatalf("5-referral milestone reached at %v, want the 5th qualifying booking at %v", fifthAt, jobs[4].JobCompletedAt)
	}

	// Replaying a qualifying booking changes nothing.
	mustHandle(t, p.svc, referralusecases.ProducerWallet, envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, "replay", jobs[9]))
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_milestones`); n != 2 {
		t.Fatalf("milestones = %d, want 2", n)
	}

	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	credits := p.wallet.creditsTo(ada.actorID)
	voucher := creditsFor(credits, "milestone_free_delivery")
	if len(voucher) != 1 || voucher[0].Balance != "promotional" || voucher[0].AmountKobo != 150_000 || voucher[0].OwnerType != "customer" {
		t.Fatalf("free-delivery credit = %+v", voucher)
	}
	bonus := creditsFor(credits, "milestone_bonus")
	if len(bonus) != 1 || bonus[0].Balance != "earnings" || bonus[0].AmountKobo != 750_000 {
		t.Fatalf("10-referral bonus = %+v", bonus)
	}
	if got := len(creditsFor(credits, "customer_referral_bonus")); got != 10 {
		t.Fatalf("₦500 bonuses = %d, want 10", got)
	}
	view, err := p.svc.MyCode(context.Background(), ada.actorType, ada.actorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.QualifiedCustomerReferrals != 10 || !view.PowerUser {
		t.Fatalf("Ada's view = %+v, want 10 qualified referrals and the badge", view)
	}
}

// A milestone's reward is referral income: an inactive referrer loses it (no
// back-payment), but keeps the badge, which recognises the count itself.
func TestMilestoneRewardsNeedAnActiveReferrer(t *testing.T) {
	p := newProgramme(t)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true) // no bookings: inactive
	start := p.clock.Now()
	for i, friend := range p.referFriends(t, ada, 10) {
		p.book(t, friend, start.Add(time.Duration(i+1)*time.Minute))
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	for _, rewardType := range []string{"milestone_free_delivery", "milestone_bonus"} {
		if st, reason, _ := rewardStatus(t, p.pool, rewardType, ada.actorID); st != "rejected" || reason != "referrer_inactive" {
			t.Fatalf("%s = %s/%s, want rejected for inactivity", rewardType, st, reason)
		}
	}
	if len(p.wallet.creditsTo(ada.actorID)) != 0 {
		t.Fatal("an inactive referrer was paid")
	}
	if view, _ := p.svc.MyCode(context.Background(), ada.actorType, ada.actorID); !view.PowerUser {
		t.Fatal("the badge was withheld")
	}
}

// Only referrals in good standing count: one under review drops out until it
// is cleared, a disqualified one drops out for good, and a milestone already
// reached is never granted twice.
func TestMilestoneCountsOnlyReferralsInGoodStanding(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	friends := p.referFriends(t, ada, 6)
	for i := 0; i < 4; i++ {
		p.book(t, friends[i], start.Add(time.Duration(i+1)*time.Minute))
	}
	if err := p.svc.FreezeReferral(ctx, referralOf(t, p.pool, friends[3]), adminReq); err != nil {
		t.Fatal(err)
	}
	p.book(t, friends[4], start.Add(5*time.Minute))
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_milestones`); n != 0 {
		t.Fatalf("a referral under review counted towards a milestone")
	}
	if err := p.svc.ClearReferral(ctx, referralOf(t, p.pool, friends[3]), adminReq); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_milestones WHERE milestone = 'qualified_5'`); n != 1 {
		t.Fatal("clearing the fifth referral did not reach the milestone")
	}
	if err := p.svc.DisqualifyReferral(ctx, referralOf(t, p.pool, friends[0]), adminReq); err != nil {
		t.Fatal(err)
	}
	if view, _ := p.svc.MyCode(ctx, ada.actorType, ada.actorID); view.QualifiedCustomerReferrals != 4 {
		t.Fatalf("qualified referrals = %d after a disqualification, want 4", view.QualifiedCustomerReferrals)
	}
	p.book(t, friends[5], start.Add(6*time.Minute)) // back to five
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'milestone_free_delivery'`); n != 1 {
		t.Fatalf("free-delivery credits = %d, want exactly 1", n)
	}
}

// Spend-only milestone credit goes to the referrer's customer account even
// when their code came with a driver account; earnings stay with the latter.
func TestMilestoneCreditGoesWhereItCanBeSpent(t *testing.T) {
	p := newProgramme(t)
	ada := newAccount(referralcontract.ActorDispatch)
	signUp(t, p.svc, ada, "", "", true)
	adaCustomer := account{actorType: referralcontract.ActorCustomer, actorID: "cust_" + uuid.NewString(), phone: ada.phone}
	signUp(t, p.svc, adaCustomer, "", "", true)
	start := p.clock.Now()
	p.bookings(t, adaCustomer.actorID, 5, start)
	for i, friend := range p.referFriends(t, ada, 5) {
		p.book(t, friend, start.Add(time.Duration(i+1)*time.Minute))
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	if voucher := creditsFor(p.wallet.creditsTo(adaCustomer.actorID), "milestone_free_delivery"); len(voucher) != 1 {
		t.Fatalf("free-delivery credit on Ada's customer account = %+v", voucher)
	}
	if bonuses := creditsFor(p.wallet.creditsTo(ada.actorID), "customer_referral_bonus"); len(bonuses) != 5 || bonuses[0].OwnerType != "provider" {
		t.Fatalf("₦500 bonuses on Ada's rider account = %+v", bonuses)
	}
}
