// Package privacy owns Vendor-local lifecycle enforcement. It does not erase
// canonical customer, media, payment, OTP/QR, or provider data owned by other
// services.
package privacy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
)

const (
	DefaultBatchSize   = 50
	DefaultMaxAttempts = 10
)

// RetentionWorker minimizes the operational receiver/location projection once
// a terminal order's policy window expires. Canonical order and financial
// evidence is preserved, and active legal holds always win.
type RetentionWorker struct {
	db          *pgxpool.Pool
	maxAttempts int
	now         func() time.Time
}

func NewRetentionWorker(db *pgxpool.Pool, maxAttempts int) *RetentionWorker {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	return &RetentionWorker{db: db, maxAttempts: maxAttempts, now: func() time.Time { return time.Now().UTC() }}
}

// RunPass processes at most limit jobs. Each job is a short transaction and
// uses FOR UPDATE SKIP LOCKED, making multiple instances and crash rollback
// safe without holding a lease across remote work (there is no remote work).
func (w *RetentionWorker) RunPass(ctx context.Context, limit int) (int, error) {
	if w == nil || w.db == nil {
		return 0, errors.New("privacy retention: database is not configured")
	}
	if limit <= 0 {
		limit = DefaultBatchSize
	}
	if limit > 100 {
		limit = 100
	}
	// Discover legacy terminal orders in the same bounded budget. Future
	// terminal transitions are enqueued by the database trigger.
	if err := w.enqueueDue(ctx, limit); err != nil {
		return 0, err
	}
	processed := 0
	for processed < limit {
		jobID, found, err := w.processOne(ctx)
		if err != nil {
			if jobID != "" {
				_ = w.markFailure(ctx, jobID, err)
			}
			return processed, err
		}
		if !found {
			break
		}
		processed++
	}
	return processed, nil
}

func (w *RetentionWorker) enqueueDue(ctx context.Context, limit int) error {
	_, err := w.db.Exec(ctx, `WITH due AS (
		SELECT p.id,p.customer_id,p.terminal_at,r.id AS policy_ref,r.retain_for
		FROM commerce_parent_orders p
		JOIN vendor_privacy_retention_policies r
		  ON r.data_class='ORDER_RECEIVER_LOCATION' AND r.active
		LEFT JOIN vendor_privacy_retention_jobs j ON j.parent_order_id=p.id
		WHERE p.terminal_at IS NOT NULL AND j.parent_order_id IS NULL
		  AND p.terminal_at+r.retain_for <= $2
		ORDER BY p.terminal_at,p.id LIMIT $1
	)
	INSERT INTO vendor_privacy_retention_jobs
		(id,parent_order_id,customer_id,policy_ref,eligible_at,next_attempt_at)
	SELECT 'vprj_'||gen_random_uuid()::text,id,customer_id,policy_ref,
	       terminal_at+retain_for,terminal_at+retain_for FROM due
	ON CONFLICT(parent_order_id) DO NOTHING`, limit, w.now())
	if err != nil {
		return fmt.Errorf("privacy retention: discover legacy orders: %w", err)
	}
	return nil
}

func (w *RetentionWorker) processOne(ctx context.Context) (string, bool, error) {
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var jobID, parentID, customerID, policyRef string
	err = tx.QueryRow(ctx, `
		SELECT j.id,j.parent_order_id,j.customer_id,j.policy_ref
		FROM vendor_privacy_retention_jobs j
		WHERE j.status='PENDING' AND j.next_attempt_at <= $1
		ORDER BY j.next_attempt_at,j.id
		LIMIT 1 FOR UPDATE SKIP LOCKED`, w.now()).
		Scan(&jobID, &parentID, &customerID, &policyRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("privacy retention: claim: %w", err)
	}

	var held bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM vendor_data_legal_holds h
		WHERE h.released_at IS NULL AND h.starts_at <= $1
		  AND (h.expires_at IS NULL OR h.expires_at > $1)
		  AND ((h.scope_type='PARENT_ORDER' AND h.scope_ref=$2)
		    OR (h.scope_type='CUSTOMER' AND h.scope_ref=$3)
		    OR (h.scope_type='BUSINESS' AND EXISTS (
		        SELECT 1 FROM commerce_vendor_orders v
		        WHERE v.parent_order_id=$2 AND v.business_id=h.scope_ref)))
	)`, w.now(), parentID, customerID).Scan(&held)
	if err != nil {
		return jobID, true, fmt.Errorf("privacy retention: check legal hold: %w", err)
	}
	if held {
		_, err = tx.Exec(ctx, `UPDATE vendor_privacy_retention_jobs
			SET next_attempt_at=$2::timestamptz + interval '24 hours',last_outcome='LEGAL_HOLD',updated_at=$2
			WHERE id=$1`, jobID, w.now())
		if err != nil {
			return jobID, true, fmt.Errorf("privacy retention: defer legal hold: %w", err)
		}
		return jobID, true, tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `INSERT INTO commerce_order_restricted_evidence
		(parent_order_id,customer_id,address_snapshot,receiver_snapshot,policy_ref,sealed_at)
		SELECT id,customer_id,address_snapshot,receiver_snapshot,$2,$3
		FROM commerce_parent_orders WHERE id=$1
		ON CONFLICT(parent_order_id) DO NOTHING`, parentID, policyRef, w.now())
	if err != nil {
		return jobID, true, fmt.Errorf("privacy retention: seal evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_parent_orders SET
		operational_address_snapshot=jsonb_build_object(
			'address','[REDACTED]','latitude',0,'longitude',0,
			'snapshotted_at',COALESCE(address_snapshot->'snapshotted_at',to_jsonb($2::timestamptz))),
		operational_receiver_snapshot='{}'::jsonb,updated_at=$2
		WHERE id=$1`, parentID, w.now())
	if err != nil {
		return jobID, true, fmt.Errorf("privacy retention: minimize projection: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO vendor_audit_log
		(id,actor_type,actor_id,action,aggregate_type,aggregate_id,correlation_id,after_data,reason)
		VALUES($1,'system','vendor-privacy-retention','privacy.order_receiver_location_minimized',
		       'commerce_parent_order',$2,$3,'{"operational_pii":"MINIMIZED","evidence":"RETAINED"}'::jsonb,
		       'retention policy window elapsed')`, "val_"+uuid.NewString(), parentID, jobID)
	if err != nil {
		return jobID, true, fmt.Errorf("privacy retention: append audit: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_privacy_retention_jobs SET
		status='MINIMIZED',minimized_at=$2,last_outcome='MINIMIZED',last_error_redacted='',updated_at=$2
		WHERE id=$1`, jobID, w.now())
	if err != nil {
		return jobID, true, fmt.Errorf("privacy retention: complete: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return jobID, true, fmt.Errorf("privacy retention: commit: %w", err)
	}
	return jobID, true, nil
}

func (w *RetentionWorker) markFailure(ctx context.Context, jobID string, processErr error) error {
	_, err := w.db.Exec(ctx, `UPDATE vendor_privacy_retention_jobs SET
		attempts=attempts+1,
		status=CASE WHEN attempts+1 >= $2 THEN 'DEAD_LETTERED' ELSE status END,
		next_attempt_at=$3::timestamptz + make_interval(secs => LEAST(3600,power(2,attempts+1))),
		last_error_redacted=$4,updated_at=$3
		WHERE id=$1 AND status='PENDING'`, jobID, w.maxAttempts, w.now(), events.RedactError(processErr))
	return err
}
