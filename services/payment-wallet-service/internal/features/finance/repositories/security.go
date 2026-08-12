package repositories

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
)

// ClaimServiceNonce records a one-time private-call nonce. Duplicate service
// and nonce pairs are rejected even while the HMAC timestamp is still fresh.
func ClaimServiceNonce(ctx context.Context, q database.Querier, serviceName, nonce,
	fingerprint string, expiresAt time.Time) error {
	if serviceName == "" || nonce == "" || fingerprint == "" {
		return ErrConflict
	}
	tag, err := q.Exec(ctx, `INSERT INTO finance_service_request_nonces
		(service_name,nonce,request_fingerprint,expires_at,created_at)
		VALUES($1,$2,$3,$4,now()) ON CONFLICT DO NOTHING`, serviceName, nonce, fingerprint, expiresAt)
	if err != nil {
		return fmt.Errorf("finance: claim service nonce: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func DeleteExpiredServiceNonces(ctx context.Context, q database.Querier, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	tag, err := q.Exec(ctx, `DELETE FROM finance_service_request_nonces WHERE ctid IN
		(SELECT ctid FROM finance_service_request_nonces WHERE expires_at<now() LIMIT $1)`, limit)
	if err != nil {
		return 0, fmt.Errorf("finance: delete expired nonces: %w", err)
	}
	return tag.RowsAffected(), nil
}

func AcquireWorkerLease(ctx context.Context, q database.Querier, name, owner string,
	lease time.Duration) (bool, error) {
	tag, err := q.Exec(ctx, `INSERT INTO finance_worker_leases
		(worker_name,lease_owner,lease_expires_at,updated_at)
		VALUES($1,$2,now()+$3::interval,now()) ON CONFLICT(worker_name) DO UPDATE SET
		lease_owner=EXCLUDED.lease_owner,lease_expires_at=EXCLUDED.lease_expires_at,updated_at=now()
		WHERE finance_worker_leases.lease_expires_at<now() OR finance_worker_leases.lease_owner=$2`,
		name, owner, lease.String())
	if err != nil {
		return false, fmt.Errorf("finance: acquire worker lease: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func ReleaseWorkerLease(ctx context.Context, q database.Querier, name, owner string) error {
	_, err := q.Exec(ctx, `DELETE FROM finance_worker_leases WHERE worker_name=$1 AND lease_owner=$2`, name, owner)
	return err
}
