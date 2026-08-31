package trip

import (
	"context"
	"log"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"github.com/google/uuid"
)

// SettlementReconciler is implemented by the existing Booking authority. It
// selects the legacy Wallet or Commerce allocation contract; Trip owns neither.
type SettlementReconciler interface {
	HandleTripCompleted(context.Context, string, time.Time) error
}

type SettlementRetryPolicy struct {
	Lease, BaseBackoff, MaxBackoff time.Duration
	MaxAttempts                    int
}

type PendingSettlement struct {
	TripID, BookingID string
	CompletedAt       time.Time
	Attempts          int
}

type settlementStore interface {
	ClaimSettlements(context.Context, time.Time, string, SettlementRetryPolicy) ([]PendingSettlement, error)
	FinishSettlement(context.Context, string, string, bool, time.Time, time.Duration, int) error
}

func (s *Service) WithSettlementReconciler(authority SettlementReconciler, policy SettlementRetryPolicy) *Service {
	if policy.Lease <= 0 {
		policy.Lease = time.Minute
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 10
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 5 * time.Second
	}
	if policy.MaxBackoff < policy.BaseBackoff {
		policy.MaxBackoff = 15 * time.Minute
	}
	s.settlement, s.settlementPolicy = authority, policy
	return s
}

func (s *Service) reconcileSettlements(ctx context.Context) {
	started := time.Now()
	var outcome error
	defer func() { observability.Worker("wallet_settlement", started, outcome) }()
	store, ok := s.repository.(settlementStore)
	if !ok || s.settlement == nil {
		return
	}
	owner := "trip-settlement:" + uuid.NewString()
	items, err := store.ClaimSettlements(ctx, s.now(), owner, s.settlementPolicy)
	if err != nil {
		outcome = err
		log.Printf("trip settlement claim failed")
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		err := s.settlement.HandleTripCompleted(ctx, item.BookingID, item.CompletedAt)
		if err != nil {
			outcome = err
		}
		delay := s.settlementPolicy.BaseBackoff
		for n := 0; n < item.Attempts && delay < s.settlementPolicy.MaxBackoff; n++ {
			delay *= 2
			if delay > s.settlementPolicy.MaxBackoff {
				delay = s.settlementPolicy.MaxBackoff
			}
		}
		if persistErr := store.FinishSettlement(ctx, item.TripID, owner, err == nil, s.now(), delay, s.settlementPolicy.MaxAttempts); persistErr != nil {
			outcome = persistErr
			log.Printf("trip settlement outcome persistence failed")
		}
	}
}

// Completion itself is the durable intent: a crash before the remote call
// leaves a completed Trip in PENDING. No pub/sub delivery is required to recover.
func (r *PostgresRepository) ClaimSettlements(ctx context.Context, now time.Time, owner string, policy SettlementRetryPolicy) ([]PendingSettlement, error) {
	rows, err := r.db.Query(ctx, `WITH due AS (
		SELECT id FROM trips WHERE status='completed' AND wallet_settlement_state IN ('PENDING','UNKNOWN')
		AND wallet_settlement_attempts<$4 AND wallet_settlement_next_at<=$1
		AND (wallet_settlement_lease_expires_at IS NULL OR wallet_settlement_lease_expires_at<=$1)
		ORDER BY wallet_settlement_next_at FOR UPDATE SKIP LOCKED LIMIT 50
	) UPDATE trips t SET wallet_settlement_lease_owner=$2,wallet_settlement_lease_expires_at=$1::timestamptz+$3::interval
	FROM due WHERE t.id=due.id RETURNING t.id::text,t.booking_id::text,t.completed_at,t.wallet_settlement_attempts`, now, owner, policy.Lease.String(), policy.MaxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PendingSettlement
	for rows.Next() {
		var item PendingSettlement
		if err = rows.Scan(&item.TripID, &item.BookingID, &item.CompletedAt, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) FinishSettlement(ctx context.Context, tripID, owner string, confirmed bool, now time.Time, delay time.Duration, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `UPDATE trips SET wallet_settlement_attempts=wallet_settlement_attempts+1,
		wallet_settlement_state=CASE WHEN $3 THEN 'ACKNOWLEDGED' WHEN wallet_settlement_attempts+1 >= $6 THEN 'DEAD_LETTER' ELSE 'UNKNOWN' END,
		wallet_settlement_error_code=CASE WHEN $3 THEN '' ELSE 'WALLET_CONFIRMATION_PENDING' END,
		wallet_settlement_next_at=$4::timestamptz+$5::interval,wallet_settlement_lease_owner=NULL,wallet_settlement_lease_expires_at=NULL
		WHERE id=$1::uuid AND wallet_settlement_lease_owner=$2 AND wallet_settlement_state IN ('PENDING','UNKNOWN')`, tripID, owner, confirmed, now, delay.String(), maxAttempts)
	return err
}
