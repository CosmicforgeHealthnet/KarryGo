package crm

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// The points expiry sweeper.
//
// There is no separate queue table and no lease bookkeeping here, because the
// due work is already durable: a lot with remaining points and a past expiry
// IS the outstanding job, and it stays outstanding until a committed
// transaction settles it. That gives crash recovery and multi-worker safety
// for free — a worker that dies mid-batch has simply not committed, and two
// workers cannot settle the same account because they contend on the same
// account row.
//
// It also means the sweeper is an optimisation, not a correctness requirement.
// Every balance-sensitive command settles expiry itself before doing anything
// else (see mutateLoyalty), so a sweeper that has been down for hours delays
// the bookkeeping without ever letting an expired point be spent.

// LoyaltyExpiryBatchSize is the default number of accounts settled per pass.
const LoyaltyExpiryBatchSize = 500

// RunLoyaltyExpiryPass settles due expiry for up to `limit` accounts.
//
// Each account is its own transaction: one account whose settlement fails
// cannot roll back the accounts already swept, and the failed one stays due
// for the next pass.
func (s *Service) RunLoyaltyExpiryPass(ctx context.Context, limit int) (int, error) {
	if !s.flags.LoyaltyEnabled || s.flags.CommerceKillSwitch {
		// Disabled means "stop working", not "discard the work". Due lots
		// remain due and are caught up when the flag comes back.
		return 0, nil
	}
	if limit <= 0 || limit > LoyaltyExpiryBatchSize {
		limit = LoyaltyExpiryBatchSize
	}
	started := s.now()
	settled := 0
	for settled < limit {
		done, err := s.settleOneDueAccount(ctx, s.now())
		if err != nil {
			return settled, err
		}
		if !done {
			break
		}
		settled++
	}
	if settled > 0 {
		s.metrics.Add("crm_loyalty_expiration_batch_duration_seconds", s.now().Sub(started).Seconds())
	}
	if due, err := s.DueExpiryLotCount(ctx); err == nil {
		s.metrics.Add("crm_loyalty_expiration_due_lots", float64(due))
	}
	return settled, nil
}

// settleOneDueAccount claims a single account with due lots and settles it.
//
// SKIP LOCKED on the account row is the whole concurrency story: several
// workers may run the same pass and will each pick a different account, and an
// account currently being debited by a request is skipped rather than blocked
// (that request settles the expiry itself anyway).
func (s *Service) settleOneDueAccount(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	account, err := scanLoyaltyAccount(tx.QueryRow(ctx, `SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts a
		WHERE a.status=$1 AND EXISTS (
			SELECT 1 FROM crm_loyalty_point_lots l
			WHERE l.business_id=a.business_id AND l.loyalty_account_id=a.id
			  AND l.remaining_points>0 AND l.expires_at IS NOT NULL AND l.expires_at<=$2)
		ORDER BY a.id
		LIMIT 1 FOR UPDATE SKIP LOCKED`, LoyaltyAccountActive, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	actor := Actor{VendorID: account.BusinessID, CorrelationID: "loyalty-expiry"}
	program, err := s.loadProgramForUpdate(ctx, tx, account.BusinessID)
	if err != nil {
		return false, err
	}
	expired, err := s.settleExpiredLots(ctx, tx, actor, &account, program, now)
	if err != nil {
		return false, err
	}
	if expired == 0 {
		// Another transaction settled it between the claim and the read; the
		// commit is still correct and simply does nothing.
		return true, tx.Commit(ctx)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// DueExpiryLotCount reports how many lots are waiting to be swept, which is
// the sweeper's real backlog signal. It carries no tenant label so it can be
// logged as an ordinary low-cardinality metric.
func (s *Service) DueExpiryLotCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_point_lots
		WHERE remaining_points>0 AND expires_at IS NOT NULL AND expires_at<=$1`, s.now()).Scan(&count)
	return count, err
}
