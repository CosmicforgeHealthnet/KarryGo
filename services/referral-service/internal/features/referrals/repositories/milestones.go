package referralrepositories

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/services/referral-service/internal/database"
)

// CountQualifiedCustomerReferrals counts a referrer's customer referrals that
// have qualified and are in good standing. Referrals under review, suspended
// or disqualified do not count (until cleared, for the first two).
func (s *Store) CountQualifiedCustomerReferrals(ctx context.Context, q database.Querier, referrerPersonID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM referrals r
		JOIN referral_tracks t ON t.referral_id = r.id AND t.model = 'customer'
		WHERE r.referrer_person_id = $1 AND r.status = 'active' AND t.status = 'qualified'`, referrerPersonID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count qualified referrals: %w", err)
	}
	return n, nil
}

// InsertMilestone records that a person reached a milestone, reporting false
// when they already had (a milestone is granted once, ever).
func (s *Store) InsertMilestone(ctx context.Context, q database.Querier, personID, milestone string, at time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO referral_milestones (person_id, milestone, achieved_at) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, personID, milestone, at)
	if err != nil {
		return false, fmt.Errorf("insert milestone: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SetMilestoneReward links a milestone to the reward it paid.
func (s *Store) SetMilestoneReward(ctx context.Context, q database.Querier, personID, milestone, rewardID string) error {
	_, err := q.Exec(ctx, `UPDATE referral_milestones SET reward_id = $3 WHERE person_id = $1 AND milestone = $2`, personID, milestone, rewardID)
	return err
}

// SetPowerUser gives a person the Power User badge, keeping the first date.
func (s *Store) SetPowerUser(ctx context.Context, q database.Querier, personID string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE referral_persons SET power_user_since = COALESCE(power_user_since, $2), updated_at = now()
		WHERE id = $1`, personID, at)
	return err
}

// Milestone is a milestone a person reached.
type Milestone struct {
	Milestone  string    `json:"milestone"`
	AchievedAt time.Time `json:"achieved_at"`
	RewardID   string    `json:"reward_id,omitempty"`
}

// MilestonesForPerson lists the milestones a person reached.
func (s *Store) MilestonesForPerson(ctx context.Context, q database.Querier, personID string) ([]Milestone, error) {
	rows, err := q.Query(ctx, `
		SELECT milestone, achieved_at, COALESCE(reward_id, '') FROM referral_milestones
		WHERE person_id = $1 ORDER BY achieved_at`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.Milestone, &m.AchievedAt, &m.RewardID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// NthQualificationTime is when a referrer's nth qualified customer referral
// (in good standing, in time order) qualified: the moment a milestone of n
// referrals was reached, whatever order the events were processed in.
func (s *Store) NthQualificationTime(ctx context.Context, q database.Querier, referrerPersonID string, n int) (time.Time, error) {
	var at time.Time
	err := q.QueryRow(ctx, `
		SELECT t.qualified_at FROM referrals r
		JOIN referral_tracks t ON t.referral_id = r.id AND t.model = 'customer'
		WHERE r.referrer_person_id = $1 AND r.status = 'active' AND t.status = 'qualified' AND t.qualified_at IS NOT NULL
		ORDER BY t.qualified_at, r.id OFFSET $2 LIMIT 1`, referrerPersonID, n-1).Scan(&at)
	return at, notFound(err)
}
