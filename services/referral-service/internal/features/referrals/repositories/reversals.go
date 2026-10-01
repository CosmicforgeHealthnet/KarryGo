package referralrepositories

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// InsertJobReversal records a reversal of a paid-out job. It reports false
// when the reversal reference was already recorded (a replay).
func (s *Store) InsertJobReversal(ctx context.Context, q database.Querier, r referralcontract.JobSettlementReversed) (bool, error) {
	occurredAt := r.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO settled_job_reversals (
			reversal_reference, source_service, source_reference, reversed_amount_kobo,
			reversed_platform_fee_kobo, reversal_kind, fully_reversed, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (reversal_reference) DO NOTHING`,
		r.ReversalReference, r.SourceService, r.SourceReference, r.ReversedAmountKobo,
		r.ReversedPlatformFeeKobo, r.ReversalKind, r.FullyReversed, occurredAt.UTC())
	if err != nil {
		return false, fmt.Errorf("insert job reversal: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// JobReversals sums every reversal recorded against a job.
type JobReversals struct {
	Count           int
	AmountKobo      int64
	PlatformFeeKobo int64
	// AnyFull is whether any single reversal said it reversed the whole job.
	AnyFull bool
}

// JobReversalTotals sums the reversals recorded against a job.
func (s *Store) JobReversalTotals(ctx context.Context, q database.Querier, sourceService, sourceReference string) (JobReversals, error) {
	var t JobReversals
	err := q.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(reversed_amount_kobo), 0), COALESCE(sum(reversed_platform_fee_kobo), 0),
			COALESCE(bool_or(fully_reversed), false)
		FROM settled_job_reversals WHERE source_service = $1 AND source_reference = $2`,
		sourceService, sourceReference).Scan(&t.Count, &t.AmountKobo, &t.PlatformFeeKobo, &t.AnyFull)
	return t, err
}

// MarkJobReversalsApplied stamps a job's reversals as applied.
func (s *Store) MarkJobReversalsApplied(ctx context.Context, q database.Querier, sourceService, sourceReference string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE settled_job_reversals SET applied_at = $3
		WHERE source_service = $1 AND source_reference = $2 AND applied_at IS NULL`, sourceService, sourceReference, at)
	return err
}

// GetSettledJob loads one recorded paid job, optionally locking it so its
// settlement and reversals are applied one at a time.
func (s *Store) GetSettledJob(ctx context.Context, q database.Querier, sourceService, sourceReference string, lock bool) (referralmodels.SettledJob, error) {
	query := `SELECT source_service, source_reference, payment_reference, customer_id, provider_actor_type, provider_id,
			wallet_provider_type, amount_kobo, platform_fee_kobo, currency, job_completed_at, job_completed_at_source,
			settled_at, in_programme
		FROM settled_jobs WHERE source_service = $1 AND source_reference = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var j referralmodels.SettledJob
	err := q.QueryRow(ctx, query, sourceService, sourceReference).Scan(&j.SourceService, &j.SourceReference, &j.PaymentReference,
		&j.CustomerID, &j.ProviderActorType, &j.ProviderID, &j.WalletProviderType, &j.AmountKobo, &j.PlatformFeeKobo,
		&j.Currency, &j.JobCompletedAt, &j.JobCompletedAtSource, &j.SettledAt, &j.InProgramme)
	return j, notFound(err)
}

// MarkJobReversed records that a paid job was fully reversed: it no longer
// counts as anyone's activity, and catch-ups skip it.
func (s *Store) MarkJobReversed(ctx context.Context, q database.Querier, sourceService, sourceReference string, at time.Time) error {
	if _, err := q.Exec(ctx, `
		UPDATE settled_jobs SET reversed_at = $3
		WHERE source_service = $1 AND source_reference = $2 AND reversed_at IS NULL`, sourceService, sourceReference, at); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		UPDATE actor_activity_jobs SET reversed_at = $3
		WHERE source_service = $1 AND source_reference = $2 AND reversed_at IS NULL`, sourceService, sourceReference, at)
	return err
}

// TrackJob is how one paid job counted towards a track.
type TrackJob struct {
	CountedAs  string
	Reason     string
	ReversedAt *time.Time
}

// GetTrackJob loads how a job counted towards a referral's track.
func (s *Store) GetTrackJob(ctx context.Context, q database.Querier, referralID, model, sourceService, sourceReference string) (TrackJob, error) {
	var tj TrackJob
	err := q.QueryRow(ctx, `
		SELECT counted_as, reason, reversed_at FROM referral_track_jobs
		WHERE referral_id = $1 AND model = $2 AND source_service = $3 AND source_reference = $4`,
		referralID, model, sourceService, sourceReference).Scan(&tj.CountedAs, &tj.Reason, &tj.ReversedAt)
	return tj, notFound(err)
}

// MarkTrackJobReversed stamps a track job as reversed, reporting whether it
// was not already.
func (s *Store) MarkTrackJobReversed(ctx context.Context, q database.Querier, referralID, model, sourceService, sourceReference string, at time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE referral_track_jobs SET reversed_at = $5
		WHERE referral_id = $1 AND model = $2 AND source_service = $3 AND source_reference = $4 AND reversed_at IS NULL`,
		referralID, model, sourceService, sourceReference, at)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
