package vendors

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpiryDecider is recorded in history for system-driven expiry.
const ExpiryDecider = "system:expiry"

// ExpiryReason is the reason code an expired verification carries, taken from
// the formal review dictionary.
const ExpiryReason = "COMPLIANCE_LAPSE"

// RunExpirySweep moves VERIFIED cases whose expiry has passed to EXPIRED,
// one case per transaction, through the same state machine and with the same
// durable Vendor callback a reviewer's decision produces. This is how an
// expired FOOD_KITCHEN (or any other) verification stops counting in Vendor.
func RunExpirySweep(ctx context.Context, db *pgxpool.Pool, limit int, now func() time.Time) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	expired := 0
	for expired < limit {
		done, err := expireOne(ctx, db, now().UTC())
		if err != nil {
			return expired, err
		}
		if !done {
			return expired, nil
		}
		expired++
	}
	return expired, nil
}

func expireOne(ctx context.Context, db *pgxpool.Pool, now time.Time) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,source_version FROM vendor_verification_cases WHERE status='VERIFIED' AND expires_at IS NOT NULL AND expires_at<=$1 ORDER BY expires_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, now).Scan(&id, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = ApplyDecision(ctx, tx, DecisionInput{CaseRef: id, ExpectedVersion: version, Status: StatusExpired, ReasonCode: ExpiryReason, DecidedBy: ExpiryDecider, DecidedByKind: DecidedBySystem, OperationRef: fmt.Sprintf("expiry:%s:v%d", id, version), Now: now}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
