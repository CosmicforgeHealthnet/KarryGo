package referralrepositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
)

const rewardColumns = `id, person_id, beneficiary_actor_type, beneficiary_actor_id, COALESCE(referral_id, ''), model,
	reward_type, balance, amount_kobo, currency, status, status_reason, basis_kobo, rate_bps,
	source_service, source_reference, activity_as_of, decide_after, COALESCE(reverses_reward_id, ''),
	idempotency_key, wallet_state, wallet_reference, wallet_attempts, wallet_lease_until, created_at`

func scanReward(row pgx.Row) (referralmodels.Reward, error) {
	var r referralmodels.Reward
	err := row.Scan(&r.ID, &r.PersonID, &r.BeneficiaryActorType, &r.BeneficiaryActorID, &r.ReferralID, &r.Model,
		&r.RewardType, &r.Balance, &r.AmountKobo, &r.Currency, &r.Status, &r.StatusReason, &r.BasisKobo, &r.RateBPS,
		&r.SourceService, &r.SourceReference, &r.ActivityAsOf, &r.DecideAfter, &r.ReversesRewardID,
		&r.IdempotencyKey, &r.WalletState, &r.WalletReference, &r.WalletAttempts, &r.WalletLeaseUntil, &r.CreatedAt)
	return r, notFound(err)
}

func scanRewards(rows pgx.Rows) ([]referralmodels.Reward, error) {
	defer rows.Close()
	var rewards []referralmodels.Reward
	for rows.Next() {
		r, err := scanReward(rows)
		if err != nil {
			return nil, err
		}
		rewards = append(rewards, r)
	}
	return rewards, rows.Err()
}

// InsertReward records a reward. It reports false -- and writes nothing --
// when a reward with the same idempotency key already exists, which is what
// makes every reward happen at most once however often its cause is replayed.
func (s *Store) InsertReward(ctx context.Context, q database.Querier, r referralmodels.Reward) (bool, error) {
	if r.ID == "" {
		r.ID = "rw_" + uuid.NewString()
	}
	if r.WalletState == "" {
		r.WalletState = referralmodels.WalletNotSent
	}
	if r.Currency == "" {
		r.Currency = "NGN"
	}
	nextAttempt := r.WalletDueAt
	if r.WalletState == referralmodels.WalletPending && nextAttempt == nil {
		now := time.Now()
		nextAttempt = &now
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO reward_ledger (
			id, person_id, beneficiary_actor_type, beneficiary_actor_id, referral_id, model, reward_type, balance,
			amount_kobo, currency, status, status_reason, basis_kobo, rate_bps, source_service, source_reference,
			activity_as_of, decide_after, reverses_reward_id, idempotency_key, wallet_state, wallet_next_attempt_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			NULLIF($19, ''), $20, $21, $22)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		r.ID, r.PersonID, r.BeneficiaryActorType, r.BeneficiaryActorID, r.ReferralID, r.Model, r.RewardType, r.Balance,
		r.AmountKobo, r.Currency, r.Status, r.StatusReason, r.BasisKobo, r.RateBPS, r.SourceService, r.SourceReference,
		r.ActivityAsOf, r.DecideAfter, r.ReversesRewardID, r.IdempotencyKey, r.WalletState, nextAttempt)
	if err != nil {
		return false, fmt.Errorf("insert reward: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimRewardsToDecide locks up to limit rewards whose activity decision is
// due. Call inside a transaction; SKIP LOCKED lets several replicas decide in
// parallel without taking the same row.
func (s *Store) ClaimRewardsToDecide(ctx context.Context, q database.Querier, now time.Time, limit int) ([]referralmodels.Reward, error) {
	rows, err := q.Query(ctx, `SELECT `+rewardColumns+` FROM reward_ledger
		WHERE status = $1 AND decide_after <= $2 AND reward_type <> 'reversal'
		ORDER BY decide_after LIMIT $3 FOR UPDATE SKIP LOCKED`, referralmodels.RewardPendingActivity, now, limit)
	if err != nil {
		return nil, err
	}
	return scanRewards(rows)
}

// SetRewardStatus moves a reward along. Approving it (available) queues it
// for the wallet. Rejecting it takes it out of the wallet queue and rejects
// any take-back recorded against it: a reward that is never paid has nothing
// to take back.
func (s *Store) SetRewardStatus(ctx context.Context, q database.Querier, id, status, reason string, now time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE reward_ledger SET status = $2, status_reason = $3, decided_at = COALESCE(decided_at, $4), updated_at = now(),
			wallet_state = CASE
				WHEN $2 = 'available' AND wallet_state = 'not_sent' THEN 'pending'
				WHEN $2 = 'rejected' AND wallet_state = 'pending' THEN 'not_sent'
				ELSE wallet_state END,
			wallet_next_attempt_at = CASE WHEN $2 = 'available' AND wallet_state = 'not_sent' THEN $4 ELSE wallet_next_attempt_at END
		WHERE id = $1`, id, status, reason, now)
	if err != nil || status != referralmodels.RewardRejected {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE reward_ledger SET status = 'rejected', status_reason = $2, decided_at = $3, updated_at = now(),
			wallet_state = CASE WHEN wallet_state = 'pending' THEN 'not_sent' ELSE wallet_state END
		WHERE reverses_reward_id = $1 AND status <> 'rejected' AND wallet_state <> 'posted'`,
		id, referralmodels.ReasonOriginalRejected, now)
	return err
}

// ReleaseHeldRewards moves a referral's rewards that waited for
// qualification into the activity decision queue.
func (s *Store) ReleaseHeldRewards(ctx context.Context, q database.Querier, referralID, model string, decideAfter time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE reward_ledger SET status = $4, decide_after = $5, updated_at = now()
		WHERE referral_id = $1 AND model = $2 AND status = $3`,
		referralID, model, referralmodels.RewardPendingQualification, referralmodels.RewardPendingActivity, decideAfter)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ClaimRewardsToPost leases up to limit approved rewards that still need to
// reach the wallet. The lease stops two replicas posting the same row at once;
// the wallet's idempotency key covers a crash between posting and recording.
//
// A reward that was taken back before it was posted still posts (status
// reversed) so that its take-back has something to undo, and a take-back is
// only posted once the reward it undoes has been: money is never taken back
// before it was paid.
func (s *Store) ClaimRewardsToPost(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]referralmodels.Reward, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE reward_ledger SET wallet_lease_until = $2
		WHERE id IN (
			SELECT r.id FROM reward_ledger r
			WHERE r.wallet_state = 'pending' AND r.status IN ('available', 'reversed')
			  AND (r.wallet_next_attempt_at IS NULL OR r.wallet_next_attempt_at <= $1)
			  AND (r.wallet_lease_until IS NULL OR r.wallet_lease_until < $1)
			  AND (r.reverses_reward_id IS NULL OR EXISTS (
			      SELECT 1 FROM reward_ledger o WHERE o.id = r.reverses_reward_id AND o.wallet_state = 'posted'))
			ORDER BY r.created_at LIMIT $3
			FOR UPDATE OF r SKIP LOCKED)
		RETURNING `+rewardColumns, now, now.Add(lease), limit)
	if err != nil {
		return nil, err
	}
	rewards, err := scanRewards(rows)
	if err != nil {
		return nil, err
	}
	// RETURNING does not keep the subquery's order; post oldest first so the
	// wallet's history reads in the order things happened.
	sort.SliceStable(rewards, func(i, j int) bool {
		if !rewards[i].CreatedAt.Equal(rewards[j].CreatedAt) {
			return rewards[i].CreatedAt.Before(rewards[j].CreatedAt)
		}
		return rewards[i].ID < rewards[j].ID
	})
	return rewards, nil
}

// MarkRewardPosted records a successful wallet posting. If an operator froze
// the reward while the posting was in flight, the row is left queued; it is
// posted again when cleared, and the wallet's idempotency key returns the
// original posting instead of paying twice.
func (s *Store) MarkRewardPosted(ctx context.Context, id, walletReference string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE reward_ledger SET wallet_state = 'posted', wallet_reference = $2, wallet_lease_until = NULL,
			wallet_last_error = '', updated_at = now()
		WHERE id = $1 AND wallet_state = 'pending' AND status IN ('available', 'reversed')`, id, walletReference)
	return err
}

// MarkRewardPostFailed schedules another wallet attempt.
func (s *Store) MarkRewardPostFailed(ctx context.Context, id string, postErr error, nextAttempt time.Time) error {
	message := postErr.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE reward_ledger SET wallet_attempts = wallet_attempts + 1, wallet_next_attempt_at = $2,
			wallet_last_error = $3, wallet_lease_until = NULL, updated_at = now()
		WHERE id = $1 AND wallet_state = 'pending'`, id, nextAttempt, message)
	return err
}

// GetReward loads one reward, optionally locking it.
func (s *Store) GetReward(ctx context.Context, q database.Querier, id string, lock bool) (referralmodels.Reward, error) {
	query := `SELECT ` + rewardColumns + ` FROM reward_ledger WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanReward(q.QueryRow(ctx, query, id))
}

// RewardsForSource lists the rewards a job created, oldest first.
func (s *Store) RewardsForSource(ctx context.Context, q database.Querier, sourceService, sourceReference string) ([]referralmodels.Reward, error) {
	rows, err := q.Query(ctx, `SELECT `+rewardColumns+` FROM reward_ledger
		WHERE source_service = $1 AND source_reference = $2 AND reward_type <> 'reversal'
		ORDER BY created_at FOR UPDATE`, sourceService, sourceReference)
	if err != nil {
		return nil, err
	}
	return scanRewards(rows)
}

// RewardsForPerson lists a person's rewards (as beneficiary), newest first,
// in the given statuses (all statuses when none are given).
func (s *Store) RewardsForPerson(ctx context.Context, q database.Querier, personID string, statuses []string) ([]referralmodels.Reward, error) {
	rows, err := q.Query(ctx, `SELECT `+rewardColumns+` FROM reward_ledger
		WHERE person_id = $1 AND (COALESCE(cardinality($2::text[]), 0) = 0 OR status = ANY($2))
		ORDER BY created_at DESC FOR UPDATE`, personID, statuses)
	if err != nil {
		return nil, err
	}
	return scanRewards(rows)
}

// RecentRewardsForPerson lists a person's latest rewards without locking
// them (for display).
func (s *Store) RecentRewardsForPerson(ctx context.Context, q database.Querier, personID string, limit int) ([]referralmodels.Reward, error) {
	rows, err := q.Query(ctx, `SELECT `+rewardColumns+` FROM reward_ledger
		WHERE person_id = $1 ORDER BY created_at DESC, id LIMIT $2`, personID, limit)
	if err != nil {
		return nil, err
	}
	return scanRewards(rows)
}

// RewardsForReferral lists a referral's rewards in the given statuses (all
// statuses when none are given).
func (s *Store) RewardsForReferral(ctx context.Context, q database.Querier, referralID string, statuses []string) ([]referralmodels.Reward, error) {
	rows, err := q.Query(ctx, `SELECT `+rewardColumns+` FROM reward_ledger
		WHERE referral_id = $1 AND (COALESCE(cardinality($2::text[]), 0) = 0 OR status = ANY($2))
		ORDER BY created_at FOR UPDATE`, referralID, statuses)
	if err != nil {
		return nil, err
	}
	return scanRewards(rows)
}

// SettledJobsForActor lists the in-programme paid jobs an account took part
// in, oldest first -- used to catch up when an account's announcement arrives
// after some of its jobs.
func (s *Store) SettledJobsForActor(ctx context.Context, q database.Querier, actorType, actorID string) ([]referralmodels.SettledJob, error) {
	query := `SELECT source_service, source_reference, payment_reference, customer_id, provider_actor_type, provider_id,
			wallet_provider_type, amount_kobo, platform_fee_kobo, currency, job_completed_at, job_completed_at_source,
			settled_at, in_programme
		FROM settled_jobs WHERE in_programme AND reversed_at IS NULL AND `
	args := []any{actorID}
	if actorType == "customer" {
		query += `customer_id = $1`
	} else {
		query += `provider_actor_type = $2 AND provider_id = $1`
		args = append(args, actorType)
	}
	rows, err := q.Query(ctx, query+` ORDER BY job_completed_at, source_reference`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []referralmodels.SettledJob
	for rows.Next() {
		var j referralmodels.SettledJob
		if err := rows.Scan(&j.SourceService, &j.SourceReference, &j.PaymentReference, &j.CustomerID, &j.ProviderActorType, &j.ProviderID,
			&j.WalletProviderType, &j.AmountKobo, &j.PlatformFeeKobo, &j.Currency, &j.JobCompletedAt, &j.JobCompletedAtSource,
			&j.SettledAt, &j.InProgramme); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// InsertTrackJob records how a paid job counted towards a track, once. It
// reports false when the job was already recorded (a replay).
func (s *Store) InsertTrackJob(ctx context.Context, q database.Querier, referralID, model string, job referralmodels.SettledJob, countedAs, reason string) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO referral_track_jobs (referral_id, model, source_service, source_reference, job_completed_at, counted_as, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING`, referralID, model, job.SourceService, job.SourceReference, job.JobCompletedAt, countedAs, reason)
	if err != nil {
		return false, fmt.Errorf("insert track job: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// LastActivationJobAt is when the latest activation job of a driver track
// was completed.
func (s *Store) LastActivationJobAt(ctx context.Context, q database.Querier, referralID string) (*time.Time, error) {
	var at *time.Time
	err := q.QueryRow(ctx, `
		SELECT max(job_completed_at) FROM referral_track_jobs
		WHERE referral_id = $1 AND model = 'provider' AND counted_as = 'activation' AND reversed_at IS NULL`, referralID).Scan(&at)
	return at, err
}

// ReversedAmount is how much of a reward has been taken back so far, as a
// positive amount (take-backs of a reward that was never paid do not count).
func (s *Store) ReversedAmount(ctx context.Context, q database.Querier, rewardID string) (int64, error) {
	var total int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(-sum(amount_kobo), 0) FROM reward_ledger
		WHERE reverses_reward_id = $1 AND status <> 'rejected'`, rewardID).Scan(&total)
	return total, err
}

// CountReversedJobs counts the distinct paid jobs whose rewards on a
// referral were taken back or cancelled because the job was refunded or
// reversed. An operator's take-back is not a refund and does not count.
func (s *Store) CountReversedJobs(ctx context.Context, q database.Querier, referralID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT DISTINCT source_service, source_reference FROM reward_ledger
			WHERE referral_id = $1 AND source_reference <> '' AND status_reason = $2
		) reversed`, referralID, referralmodels.ReasonJobReversed).Scan(&n)
	return n, err
}

// ExpireProviderWindows marks qualified driver tracks whose window has ended.
func (s *Store) ExpireProviderWindows(ctx context.Context, q database.Querier, now time.Time) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE referral_tracks SET status = 'expired', updated_at = now()
		WHERE model = 'provider' AND status = 'qualified' AND window_ends_at < $1`, now)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
