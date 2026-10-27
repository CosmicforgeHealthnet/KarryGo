package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/referralcontract"
)

// A driver refers another driver: the dashboard shows the referral move from
// signed up, through its activation jobs, to earning, with the commission
// held until qualification and then paid.
func TestDriverDashboardFollowsAReferral(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	ada, chidi := newAccount(referralcontract.ActorHauling), newAccount(referralcontract.ActorHauling)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	// Ada keeps herself active with paid jobs of her own.
	for i := 0; i < 10; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, ada.actorID, start.Add(-time.Duration(i+1)*time.Hour), 500_000)
	}
	signUp(t, p.svc, chidi, codeOf(t, p.svc, ada), "", true)

	view, err := p.svc.Dashboard(ctx, ada.actorType, ada.actorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Code == "" || view.ReferralCount != 1 || len(view.Milestones) != 2 {
		t.Fatalf("dashboard = %+v", view)
	}
	r := view.Referrals[0]
	if r.Kind != "driver" || r.Stage != "signed_up" || r.JobsRequired != 5 || r.JobsCounted != 0 {
		t.Fatalf("new referral = %+v", r)
	}

	p.approve(t, chidi, start)
	for i := 1; i <= 3; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour), 750_000)
	}
	view, _ = p.svc.Dashboard(ctx, ada.actorType, ada.actorID)
	r = view.Referrals[0]
	if r.Stage != "in_progress" || r.JobsCounted != 3 || r.PendingKobo != 3*75_000 || r.EarnedKobo != 0 {
		t.Fatalf("after 3 jobs = %+v", r)
	}

	for i := 4; i <= 6; i++ {
		p.paidJob(t, "cust_"+uuid.NewString(), referralcontract.ActorHauling, chidi.actorID, start.Add(time.Duration(i)*time.Hour), 750_000)
	}
	p.clock.Advance(61 * time.Minute)
	p.decideAndPost(t)
	view, _ = p.svc.Dashboard(ctx, ada.actorType, ada.actorID)
	r = view.Referrals[0]
	if r.Stage != "earning" || r.JobsCounted != 5 || r.EarningUntil == nil || r.EarnedKobo != 6*75_000 {
		t.Fatalf("qualified referral = %+v", r)
	}
	if view.Earnings.NetKobo != 6*75_000 || view.Rules.ProviderActivationJobs != 5 || view.Rules.ProviderWindowDays != 90 {
		t.Fatalf("earnings %+v rules %+v", view.Earnings, view.Rules)
	}

	// The referred driver's own dashboard has no referrals of their own.
	own, err := p.svc.Dashboard(ctx, chidi.actorType, chidi.actorID)
	if err != nil || own.ReferralCount != 0 || own.Referrals == nil {
		t.Fatalf("referred driver's dashboard = %+v, %v", own, err)
	}
}
