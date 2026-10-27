package referralusecases

import (
	"context"
	"time"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
)

// DashboardTotalsView is a person's rewards on one balance. Earnings can be
// withdrawn by drivers; promotional credit is spend-only.
type DashboardTotalsView struct {
	EarnedKobo    int64 `json:"earned_kobo"`
	PendingKobo   int64 `json:"pending_kobo"`
	HeldKobo      int64 `json:"held_kobo"`
	TakenBackKobo int64 `json:"taken_back_kobo"`
	NetKobo       int64 `json:"net_kobo"`
}

// DashboardReferralView is one referral without anything that identifies the
// referred person: the referrer only learns how far they have got.
type DashboardReferralView struct {
	ID string `json:"id"`
	// Kind is "driver" or "customer": the account the referred person opened.
	Kind         string     `json:"kind"`
	JoinedAt     time.Time  `json:"joined_at"`
	Status       string     `json:"status"`
	Stage        string     `json:"stage"`
	JobsRequired int        `json:"jobs_required"`
	JobsCounted  int        `json:"jobs_counted"`
	QualifiedAt  *time.Time `json:"qualified_at,omitempty"`
	// EarningUntil is when a qualified driver referral stops paying commission.
	EarningUntil *time.Time `json:"earning_until,omitempty"`
	EarnedKobo   int64      `json:"earned_kobo"`
	PendingKobo  int64      `json:"pending_kobo"`
}

// DashboardMilestoneView is one customer-referral milestone.
type DashboardMilestoneView struct {
	Key        string     `json:"key"`
	Referrals  int        `json:"referrals"`
	AmountKobo int64      `json:"amount_kobo"`
	Balance    string     `json:"balance"`
	Badge      bool       `json:"badge"`
	AchievedAt *time.Time `json:"achieved_at,omitempty"`
}

// DashboardRulesView is how the programme pays, so the app can explain it
// without hard-coding amounts that operators may change.
type DashboardRulesView struct {
	CustomerReferralBonusKobo int64 `json:"customer_referral_bonus_kobo"`
	ProviderCommissionBPS     int64 `json:"provider_commission_bps"`
	ProviderActivationJobs    int   `json:"provider_activation_jobs"`
	ProviderWindowDays        int   `json:"provider_window_days"`
}

// DashboardView is everything a referrer's dashboard shows.
type DashboardView struct {
	MyCodeView
	Earnings      DashboardTotalsView      `json:"earnings"`
	Promotional   DashboardTotalsView      `json:"promotional"`
	Referrals     []DashboardReferralView  `json:"referrals"`
	ReferralCount int                      `json:"referral_count"`
	Milestones    []DashboardMilestoneView `json:"milestones"`
	Rules         DashboardRulesView       `json:"rules"`
}

// Referral stages shown in the app.
const (
	StageSignedUp    = "signed_up"   // waiting for their first paid job(s)
	StageInProgress  = "in_progress" // some qualifying jobs done
	StageEarning     = "earning"     // qualified; a driver referral pays per job
	StageQualified   = "qualified"   // qualified; a customer referral has paid its bonus
	StageEnded       = "ended"       // the earning window is over
	StageUnderReview = "under_review"
	StageNotEligible = "not_eligible"
)

const dashboardReferralLimit = 100

// Dashboard returns the referrer's dashboard for the account calling it.
func (s *Service) Dashboard(ctx context.Context, actorType, actorID string) (DashboardView, error) {
	code, err := s.MyCode(ctx, actorType, actorID)
	if err != nil {
		return DashboardView{}, err
	}
	pool := s.store.Pool()
	actor, err := s.store.GetActor(ctx, pool, actorType, actorID)
	if err != nil {
		return DashboardView{}, err
	}
	view := DashboardView{MyCodeView: code, Referrals: []DashboardReferralView{}}

	totals, err := s.store.DashboardTotalsForPerson(ctx, pool, actor.PersonID)
	if err != nil {
		return DashboardView{}, err
	}
	for _, t := range totals {
		v := DashboardTotalsView{EarnedKobo: t.EarnedKobo, PendingKobo: t.PendingKobo, HeldKobo: t.HeldKobo,
			TakenBackKobo: t.TakenBackKobo, NetKobo: t.EarnedKobo - t.TakenBackKobo}
		if t.Balance == referralmodels.BalancePromotional {
			view.Promotional = v
		} else {
			view.Earnings = v
		}
	}

	referrals, err := s.store.DashboardReferrals(ctx, pool, actor.PersonID, dashboardReferralLimit)
	if err != nil {
		return DashboardView{}, err
	}
	now := s.now()
	for _, r := range referrals {
		kind := "customer"
		if referralmodels.ModelForActor(r.ReferredActorType) == referralmodels.ModelProvider || r.TrackModel == referralmodels.ModelProvider {
			kind = "driver"
		}
		view.Referrals = append(view.Referrals, DashboardReferralView{
			ID: r.ID, Kind: kind, JoinedAt: r.AttributedAt, Status: r.Status, Stage: referralStage(r.Status, r.TrackModel, r.TrackStatus, r.JobsCounted, r.WindowEndsAt, now),
			JobsRequired: r.JobsRequired, JobsCounted: shownJobs(r.JobsCounted, r.JobsRequired),
			QualifiedAt: r.QualifiedAt, EarningUntil: r.WindowEndsAt,
			EarnedKobo: r.EarnedKobo - r.TakenBackKobo, PendingKobo: r.PendingKobo + r.HeldKobo,
		})
	}
	view.ReferralCount = len(view.Referrals)

	achieved, err := s.store.MilestonesForPerson(ctx, pool, actor.PersonID)
	if err != nil {
		return DashboardView{}, err
	}
	for _, rule := range s.rules.Milestones() {
		m := DashboardMilestoneView{Key: rule.Key, Referrals: rule.Referrals, AmountKobo: rule.AmountKobo, Balance: rule.Balance, Badge: rule.Badge}
		for _, a := range achieved {
			if a.Milestone == rule.Key {
				at := a.AchievedAt
				m.AchievedAt = &at
			}
		}
		view.Milestones = append(view.Milestones, m)
	}

	view.Rules = DashboardRulesView{
		CustomerReferralBonusKobo: s.rules.CustomerReferralBonusKobo,
		ProviderCommissionBPS:     s.rules.ProviderCommissionBPS,
		ProviderActivationJobs:    s.rules.ProviderActivationJobs,
		ProviderWindowDays:        int(s.rules.ProviderWindow / (24 * time.Hour)),
	}
	return view, nil
}

func referralStage(referralStatus, model, trackStatus string, jobsCounted int, windowEnds *time.Time, now time.Time) string {
	switch referralStatus {
	case referralmodels.ReferralUnderReview, referralmodels.ReferralSuspended:
		return StageUnderReview
	case referralmodels.ReferralDisqualified:
		return StageNotEligible
	}
	switch trackStatus {
	case referralmodels.TrackExpired:
		return StageEnded
	case referralmodels.TrackQualified:
		if model == referralmodels.ModelProvider {
			if windowEnds != nil && !now.Before(*windowEnds) {
				return StageEnded
			}
			return StageEarning
		}
		return StageQualified
	}
	if jobsCounted > 0 {
		return StageInProgress
	}
	return StageSignedUp
}

// shownJobs caps the count at what qualification needs: after that the app
// shows earnings, not a progress bar.
func shownJobs(counted, required int) int {
	if required > 0 && counted > required {
		return required
	}
	return counted
}
