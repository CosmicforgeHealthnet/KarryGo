// Package referralrepositories is referral-service's persistence. Every
// method takes a database.Querier so callers compose several writes into one
// transaction.
package referralrepositories

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
)

// Store is the Postgres-backed repository.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the repository.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the pool for transactions and read-only queries.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// InsertSettledJob records a paid-out job. It reports false when the job was
// already recorded -- the business-level duplicate check, independent of
// event ids.
func (s *Store) InsertSettledJob(ctx context.Context, q database.Querier, job referralmodels.SettledJob) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO settled_jobs (
			source_service, source_reference, payment_reference, customer_id, provider_actor_type,
			provider_id, wallet_provider_type, amount_kobo, platform_fee_kobo, currency,
			job_completed_at, job_completed_at_source, settled_at, in_programme)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (source_service, source_reference) DO NOTHING`,
		job.SourceService, job.SourceReference, job.PaymentReference, job.CustomerID, job.ProviderActorType,
		job.ProviderID, job.WalletProviderType, job.AmountKobo, job.PlatformFeeKobo, job.Currency,
		job.JobCompletedAt, job.JobCompletedAtSource, job.SettledAt, job.InProgramme,
	)
	if err != nil {
		return false, fmt.Errorf("insert settled job: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// RecordActivity notes that an account took part in a paid job.
func (s *Store) RecordActivity(ctx context.Context, q database.Querier, actorType, actorID, sourceService, sourceReference string, completedAt time.Time) error {
	if actorID == "" {
		return nil
	}
	_, err := q.Exec(ctx, `
		INSERT INTO actor_activity_jobs (actor_type, actor_id, source_service, source_reference, job_completed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`, actorType, actorID, sourceService, sourceReference, completedAt)
	if err != nil {
		return fmt.Errorf("record activity: %w", err)
	}
	return nil
}

// CountActivity counts an account's paid jobs completed in (asOf-window,
// asOf], stopping early at limit: the rule only asks "at least N?", so the
// cost stays constant however many jobs an account has ever done.
func (s *Store) CountActivity(ctx context.Context, q database.Querier, actorType, actorID string, asOf time.Time, window time.Duration, limit int) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT 1 FROM actor_activity_jobs
			WHERE actor_type = $1 AND actor_id = $2 AND reversed_at IS NULL
			  AND job_completed_at > $3 AND job_completed_at <= $4
			LIMIT $5
		) recent`, actorType, actorID, asOf.Add(-window), asOf, limit).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count activity: %w", err)
	}
	return count, nil
}

// LockJob serializes everything done about one paid job -- its settlement and
// its reversals -- for the rest of the transaction.
func (s *Store) LockJob(ctx context.Context, q database.Querier, sourceService, sourceReference string) error {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7072))`, sourceService+"|"+sourceReference); err != nil {
		return fmt.Errorf("lock job: %w", err)
	}
	return nil
}
