package referralrepositories

import (
	"context"
	"time"

	"cosmicforge/logistics/services/referral-service/internal/database"
)

// DashboardReferral is one referral as its referrer sees it: how far the
// referred person has got and what it has paid the referrer so far.
type DashboardReferral struct {
	ID                string
	AttributedAt      time.Time
	Status            string
	ReferredActorType string // the referred person's first account type
	TrackModel        string // the track shown: provider if they drive, else customer
	TrackStatus       string
	JobsRequired      int
	JobsCounted       int
	QualifiedAt       *time.Time
	WindowEndsAt      *time.Time
	EarnedKobo        int64 // paid and approved (before take-backs)
	PendingKobo       int64 // worked out, not decided yet
	HeldKobo          int64 // approved but frozen for review
	TakenBackKobo     int64 // reversed after a refund (a positive number)
}

// DashboardReferrals lists the referrals a person made, newest first, with
// each one's progress and the referrer's rewards from it.
func (s *Store) DashboardReferrals(ctx context.Context, q database.Querier, referrerPersonID string, limit int) ([]DashboardReferral, error) {
	rows, err := q.Query(ctx, `
		SELECT r.id, r.attributed_at, r.status, p.home_actor_type,
		       COALESCE(t.model, ''), COALESCE(t.status, ''), COALESCE(t.jobs_required, 0), COALESCE(t.jobs_counted, 0),
		       t.qualified_at, t.window_ends_at,
		       COALESCE(sum(l.amount_kobo) FILTER (WHERE l.reward_type <> 'reversal' AND l.status IN ('available', 'reversed')), 0),
		       COALESCE(sum(l.amount_kobo) FILTER (WHERE l.status IN ('pending_qualification', 'pending_activity')), 0),
		       COALESCE(sum(l.amount_kobo) FILTER (WHERE l.status = 'frozen'), 0),
		       COALESCE(-sum(l.amount_kobo) FILTER (WHERE l.reward_type = 'reversal'), 0)
		FROM referrals r
		JOIN referral_persons p ON p.id = r.referred_person_id
		LEFT JOIN LATERAL (
		    SELECT model, status, jobs_required, jobs_counted, qualified_at, window_ends_at
		    FROM referral_tracks WHERE referral_id = r.id
		    ORDER BY (model = 'provider') DESC LIMIT 1
		) t ON true
		LEFT JOIN reward_ledger l ON l.referral_id = r.id AND l.person_id = r.referrer_person_id
		WHERE r.referrer_person_id = $1
		GROUP BY r.id, p.home_actor_type, t.model, t.status, t.jobs_required, t.jobs_counted, t.qualified_at, t.window_ends_at
		ORDER BY r.attributed_at DESC
		LIMIT $2`, referrerPersonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardReferral
	for rows.Next() {
		var d DashboardReferral
		if err := rows.Scan(&d.ID, &d.AttributedAt, &d.Status, &d.ReferredActorType,
			&d.TrackModel, &d.TrackStatus, &d.JobsRequired, &d.JobsCounted, &d.QualifiedAt, &d.WindowEndsAt,
			&d.EarnedKobo, &d.PendingKobo, &d.HeldKobo, &d.TakenBackKobo); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DashboardTotals sums every reward a person has, by balance.
type DashboardTotals struct {
	Balance       string
	EarnedKobo    int64
	PendingKobo   int64
	HeldKobo      int64
	TakenBackKobo int64
}

// DashboardTotalsForPerson is the same sums as DashboardReferrals across all
// of a person's rewards, including welcome credits and milestones.
func (s *Store) DashboardTotalsForPerson(ctx context.Context, q database.Querier, personID string) ([]DashboardTotals, error) {
	rows, err := q.Query(ctx, `
		SELECT balance,
		       COALESCE(sum(amount_kobo) FILTER (WHERE reward_type <> 'reversal' AND status IN ('available', 'reversed')), 0),
		       COALESCE(sum(amount_kobo) FILTER (WHERE status IN ('pending_qualification', 'pending_activity')), 0),
		       COALESCE(sum(amount_kobo) FILTER (WHERE status = 'frozen'), 0),
		       COALESCE(-sum(amount_kobo) FILTER (WHERE reward_type = 'reversal'), 0)
		FROM reward_ledger WHERE person_id = $1
		GROUP BY balance ORDER BY balance`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardTotals
	for rows.Next() {
		var t DashboardTotals
		if err := rows.Scan(&t.Balance, &t.EarnedKobo, &t.PendingKobo, &t.HeldKobo, &t.TakenBackKobo); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
