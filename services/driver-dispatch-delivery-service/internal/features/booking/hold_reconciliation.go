package booking

import (
	"context"
	"log"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"github.com/google/uuid"
)

type HoldRetryPolicy struct {
	Lease, BaseBackoff, MaxBackoff, AbandonAfter time.Duration
	MaxAttempts                                  int
}
type abandonedHoldReader interface {
	ReconcileAbandonedHold(context.Context, Booking) (string, string, error)
}

// RunHoldReconciliationWorker never restarts matching or a debit. It only
// resolves/compensates an abandoned source-scoped hold through Wallet.
func (s *Service) RunHoldReconciliationWorker(ctx context.Context, interval time.Duration, policy HoldRetryPolicy) {
	repo, ok := s.repository.(*PostgresRepository)
	remote, configured := s.payments.(abandonedHoldReader)
	if !ok || !configured {
		return
	}
	defer observability.WorkerRunning("wallet_hold")()
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if policy.Lease <= 0 {
		policy.Lease = time.Minute
	}
	if policy.AbandonAfter <= 0 {
		policy.AbandonAfter = 2 * time.Minute
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 5 * time.Second
	}
	if policy.MaxBackoff < policy.BaseBackoff {
		policy.MaxBackoff = 15 * time.Minute
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 10
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		started := time.Now()
		owner := "booking-hold:" + uuid.NewString()
		ids, err := repo.claimAbandonedHolds(ctx, s.now(), owner, policy)
		if err != nil {
			observability.Worker("wallet_hold", started, err)
			log.Printf("booking hold reconciliation claim failed")
			continue
		}
		var outcome error
		for _, id := range ids {
			state, found, err := repo.GetWithDerivedState(ctx, id)
			if err != nil || !found {
				outcome = err
				continue
			}
			payment, ref, readErr := remote.ReconcileAbandonedHold(ctx, state.Booking)
			if readErr != nil {
				outcome = readErr
			}
			if err = repo.finishAbandonedHold(ctx, id, owner, payment, ref, readErr == nil, s.now(), policy); err != nil {
				outcome = err
				log.Printf("booking hold reconciliation persistence failed")
			}
		}
		observability.Worker("wallet_hold", started, outcome)
	}
}

func (r *PostgresRepository) claimAbandonedHolds(ctx context.Context, now time.Time, owner string, p HoldRetryPolicy) ([]string, error) {
	rows, err := r.db.Query(ctx, `WITH due AS (
		SELECT b.id FROM dispatch_bookings b WHERE b.payment_status='unpaid' AND b.payment_method='wallet'
		AND NOT EXISTS(SELECT 1 FROM dispatch_delivery_missions m WHERE m.booking_id=b.id)
		AND NOT EXISTS(SELECT 1 FROM commerce_missions m WHERE m.booking_id=b.id)
		AND (b.cancel_reason='payment_outcome_unknown' OR (b.status='searching' AND b.updated_at<$1::timestamptz-$5::interval
		AND NOT EXISTS(SELECT 1 FROM request_broadcasts rb WHERE rb.booking_id=b.id) AND NOT EXISTS(SELECT 1 FROM trips t WHERE t.booking_id=b.id)))
		AND b.hold_reconciliation_state IN ('PENDING','UNKNOWN') AND b.hold_reconciliation_attempts<$4
		AND b.hold_reconciliation_next_at<=$1 AND (b.hold_reconciliation_lease_expires_at IS NULL OR b.hold_reconciliation_lease_expires_at<=$1)
		ORDER BY b.hold_reconciliation_next_at FOR UPDATE SKIP LOCKED LIMIT 50
	) UPDATE dispatch_bookings b SET status='cancelled',cancel_reason='payment_outcome_unknown',cancelled_at=COALESCE(cancelled_at,$1),
		hold_reconciliation_lease_owner=$2,hold_reconciliation_lease_expires_at=$1::timestamptz+$3::interval
	FROM due WHERE b.id=due.id RETURNING b.id::text`, now, owner, p.Lease.String(), p.MaxAttempts, p.AbandonAfter.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresRepository) finishAbandonedHold(ctx context.Context, id, owner, payment, ref string, confirmed bool, now time.Time, p HoldRetryPolicy) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_bookings SET
		hold_reconciliation_attempts=hold_reconciliation_attempts+1,
		hold_reconciliation_state=CASE WHEN $5 THEN 'ACKNOWLEDGED' WHEN hold_reconciliation_attempts+1 >= $7 THEN 'DEAD_LETTER' ELSE 'UNKNOWN' END,
		hold_reconciliation_next_at=$6+LEAST($8::double precision*power(2,LEAST(hold_reconciliation_attempts,20)),$9::double precision)*interval '1 second',
		hold_reconciliation_lease_owner=NULL,hold_reconciliation_lease_expires_at=NULL,
		payment_status=CASE WHEN $5 THEN $3 ELSE payment_status END,payment_ref=CASE WHEN $4<>'' THEN $4 ELSE payment_ref END
		WHERE id=$1::uuid AND hold_reconciliation_lease_owner=$2 AND cancel_reason='payment_outcome_unknown'
		AND payment_status NOT IN ('paid','refunded')`, id, owner, payment, ref, confirmed, now, p.MaxAttempts, p.BaseBackoff.Seconds(), p.MaxBackoff.Seconds())
	return err
}
