package finance

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

var (
	ErrNotFound            = errors.New("finance projection: not found")
	ErrFingerprintConflict = errors.New("finance operation: idempotency fingerprint conflict")
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func scanOperation(row pgx.Row) (Operation, error) {
	var o Operation
	err := row.Scan(&o.ID, &o.OrganizationID, &o.SubjectType, &o.SubjectID, &o.OperationType, &o.Payload, &o.PayloadVersion, &o.ExpectedVersion, &o.IdempotencyKey, &o.RequestFingerprint, &o.WalletReference, &o.Status, &o.Attempts, &o.NextAttemptAt, &o.LeaseOwner, &o.LeaseExpiresAt, &o.SafeErrorCode, &o.CorrelationID, &o.CausationID, &o.Version, &o.BranchID, &o.RequestedByActorType, &o.RequestedByActorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	return o, err
}

const opColumns = `id,organization_id,subject_type,subject_id,operation_type,payload,payload_version,expected_version,idempotency_key,request_fingerprint,COALESCE(wallet_reference,''),status,attempts,next_attempt_at,COALESCE(lease_owner,''),lease_expires_at,safe_error_code,correlation_id,causation_id,version,COALESCE(branch_id,''),requested_by_actor_type,requested_by_actor_id`

func (r *Repository) Create(ctx context.Context, q database.Querier, o *Operation) (bool, error) {
	if q == nil {
		q = r.db
	}
	existing, err := scanOperation(q.QueryRow(ctx, `SELECT `+opColumns+` FROM fleet_partner_finance_operations WHERE organization_id=$1 AND idempotency_key=$2 FOR UPDATE`, o.OrganizationID, o.IdempotencyKey))
	if err == nil {
		if existing.RequestFingerprint != o.RequestFingerprint {
			return false, ErrFingerprintConflict
		}
		*o = existing
		return false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if o.ID == "" {
		o.ID = ids.New("fop")
	}
	o.Status = StatusPending
	if o.RequestedByActorType == "" {
		o.RequestedByActorType = "SERVICE"
	}
	err = q.QueryRow(ctx, `INSERT INTO fleet_partner_finance_operations(id,organization_id,subject_type,subject_id,operation_type,payload,payload_version,expected_version,idempotency_key,request_fingerprint,status,correlation_id,causation_id,branch_id,requested_by_actor_type,requested_by_actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16) RETURNING version,next_attempt_at`, o.ID, o.OrganizationID, o.SubjectType, o.SubjectID, o.OperationType, o.Payload, o.PayloadVersion, o.ExpectedVersion, o.IdempotencyKey, o.RequestFingerprint, o.Status, o.CorrelationID, o.CausationID, o.BranchID, o.RequestedByActorType, o.RequestedByActorID).Scan(&o.Version, &o.NextAttemptAt)
	return true, err
}
func (r *Repository) Claim(ctx context.Context, owner string, lease time.Duration, limit int) ([]Operation, error) {
	rows, err := r.db.Query(ctx, `WITH c AS(SELECT id FROM fleet_partner_finance_operations WHERE status IN ('PENDING','RETRYABLE_FAILURE','UNKNOWN','RECONCILING') AND next_attempt_at<=now() AND(lease_expires_at IS NULL OR lease_expires_at<now()) ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE fleet_partner_finance_operations o SET lease_owner=$2,lease_expires_at=now()+$3::interval,status=CASE WHEN o.status='UNKNOWN' THEN 'RECONCILING' ELSE 'PROCESSING' END,version=version+1,updated_at=now() FROM c WHERE o.id=c.id RETURNING `+prefixColumns("o."), limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		o, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func prefixColumns(p string) string {
	return p + "id," + p + "organization_id," + p + "subject_type," + p + "subject_id," + p + "operation_type," + p + "payload," + p + "payload_version," + p + "expected_version," + p + "idempotency_key," + p + "request_fingerprint,COALESCE(" + p + "wallet_reference,'')," + p + "status," + p + "attempts," + p + "next_attempt_at,COALESCE(" + p + "lease_owner,'')," + p + "lease_expires_at," + p + "safe_error_code," + p + "correlation_id," + p + "causation_id," + p + "version,COALESCE(" + p + "branch_id,'')," + p + "requested_by_actor_type," + p + "requested_by_actor_id"
}
func (r *Repository) Outcome(ctx context.Context, q database.Querier, o Operation, status, reference, code string, next time.Time, max int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE fleet_partner_finance_operations SET status=CASE WHEN $3='RETRYABLE_FAILURE' AND attempts+1 >=$7 THEN 'DEAD_LETTER' ELSE $3 END,wallet_reference=COALESCE(NULLIF($4,''),wallet_reference),safe_error_code=$5,next_attempt_at=$6,attempts=attempts+1,lease_owner=NULL,lease_expires_at=NULL,version=version+1,updated_at=now(),completed_at=CASE WHEN $3='SUCCEEDED' THEN now() ELSE completed_at END WHERE id=$1 AND lease_owner=$2`, o.ID, o.LeaseOwner, status, reference, code, next, max)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("finance operation lease lost")
	}
	_, err = q.Exec(ctx, `INSERT INTO fleet_partner_finance_operation_history(id,organization_id,operation_id,previous_status,new_status,safe_reason_code,actor_type,actor_id) VALUES($1,$2,$3,$4,$5,$6,'SYSTEM',$7)`, ids.New("fhs"), o.OrganizationID, o.ID, o.Status, status, code, o.LeaseOwner)
	return err
}
func (r *Repository) Recover(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_finance_operations SET lease_owner=NULL,lease_expires_at=NULL,status=CASE WHEN status='PROCESSING' THEN 'RETRYABLE_FAILURE' ELSE 'UNKNOWN' END WHERE lease_expires_at<now()`)
	return tag.RowsAffected(), err
}
func (r *Repository) Replay(ctx context.Context, id string) error {
	return r.ReplayIn(ctx, r.db, id)
}

func (r *Repository) ReplayIn(ctx context.Context, q database.Querier, id string) error {
	tag, err := q.Exec(ctx, `UPDATE fleet_partner_finance_operations SET status='PENDING',attempts=0,next_attempt_at=now(),safe_error_code='',version=version+1 WHERE id=$1 AND status='DEAD_LETTER'`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

const projectionColumns = `id,organization_id,projection_type,subject_type,subject_id,wallet_reference,amount_minor,COALESCE(currency,''),safe_status,source_version,freshness_state,COALESCE(last_event_id,''),last_synced_at,reconciliation_status,reconciliation_attempts,reconciliation_next_attempt_at,COALESCE(reconciliation_lease_owner,''),reconciliation_lease_expires_at,reconciliation_safe_error_code,version`

func scanProjection(row pgx.Row) (Projection, error) {
	var p Projection
	err := row.Scan(&p.ID, &p.OrganizationID, &p.ProjectionType, &p.SubjectType, &p.SubjectID,
		&p.WalletReference, &p.AmountMinor, &p.Currency, &p.SafeStatus, &p.SourceVersion,
		&p.FreshnessState, &p.LastEventID, &p.LastSyncedAt, &p.ReconciliationStatus,
		&p.ReconciliationAttempts, &p.ReconciliationNextAttemptAt, &p.ReconciliationLeaseOwner,
		&p.ReconciliationLeaseExpiresAt, &p.ReconciliationSafeErrorCode, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func projectionColumnsWithPrefix(p string) string {
	return p + "id," + p + "organization_id," + p + "projection_type," + p + "subject_type," + p + "subject_id," + p + "wallet_reference," + p + "amount_minor,COALESCE(" + p + "currency,'')," + p + "safe_status," + p + "source_version," + p + "freshness_state,COALESCE(" + p + "last_event_id,'')," + p + "last_synced_at," + p + "reconciliation_status," + p + "reconciliation_attempts," + p + "reconciliation_next_attempt_at,COALESCE(" + p + "reconciliation_lease_owner,'')," + p + "reconciliation_lease_expires_at," + p + "reconciliation_safe_error_code," + p + "version"
}

// QueueStale atomically fails old CURRENT projections closed and schedules
// authoritative read-back. The caller records audit/outbox entries in the same
// transaction before committing.
func (r *Repository) QueueStale(ctx context.Context, q database.Querier, cutoff time.Time, limit int) ([]Projection, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `WITH c AS (
		SELECT id FROM fleet_partner_finance_projections
		WHERE freshness_state='CURRENT' AND last_synced_at < $1
		ORDER BY last_synced_at FOR UPDATE SKIP LOCKED LIMIT $2
	) UPDATE fleet_partner_finance_projections p
	SET freshness_state='STALE',reconciliation_status='PENDING',reconciliation_next_attempt_at=now(),
		reconciliation_safe_error_code='',version=version+1,updated_at=now()
	FROM c WHERE p.id=c.id RETURNING `+projectionColumnsWithPrefix("p."), cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Projection
	for rows.Next() {
		p, scanErr := scanProjection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) ClaimProjections(ctx context.Context, owner string, lease time.Duration, limit int) ([]Projection, error) {
	rows, err := r.db.Query(ctx, `WITH c AS (
		SELECT id FROM fleet_partner_finance_projections
		WHERE reconciliation_status IN ('PENDING','RETRYABLE_FAILURE','UNKNOWN')
		  AND reconciliation_next_attempt_at<=now()
		  AND (reconciliation_lease_expires_at IS NULL OR reconciliation_lease_expires_at<now())
		ORDER BY reconciliation_next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1
	) UPDATE fleet_partner_finance_projections p
	SET reconciliation_lease_owner=$2,reconciliation_lease_expires_at=now()+$3::interval,
		reconciliation_status='PROCESSING',freshness_state='RECONCILING',version=version+1,updated_at=now()
	FROM c WHERE p.id=c.id RETURNING `+projectionColumnsWithPrefix("p."), limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Projection
	for rows.Next() {
		p, scanErr := scanProjection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) RecoverProjectionLeases(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_finance_projections
	SET reconciliation_lease_owner=NULL,reconciliation_lease_expires_at=NULL,
		reconciliation_status='UNKNOWN',freshness_state='UNKNOWN',
		reconciliation_next_attempt_at=now(),reconciliation_safe_error_code='lease_expired',
		version=version+1,updated_at=now()
	WHERE reconciliation_status='PROCESSING' AND reconciliation_lease_expires_at<now()`)
	return tag.RowsAffected(), err
}

func (r *Repository) ProjectionOutcome(ctx context.Context, q database.Querier, p Projection,
	reconciliationStatus, freshness, safeStatus, code string, sourceVersion int64, next time.Time, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	finalStatus := reconciliationStatus
	if reconciliationStatus == StatusRetryable && p.ReconciliationAttempts+1 >= maxAttempts {
		finalStatus = StatusDeadLetter
	}
	tag, err := q.Exec(ctx, `UPDATE fleet_partner_finance_projections
	SET reconciliation_status=$3,freshness_state=$4,
		safe_status=CASE WHEN $5='' THEN safe_status ELSE $5 END,
		source_version=GREATEST(source_version,$6),reconciliation_safe_error_code=$7,
		reconciliation_next_attempt_at=$8,reconciliation_attempts=reconciliation_attempts+1,
		reconciliation_lease_owner=NULL,reconciliation_lease_expires_at=NULL,
		last_synced_at=CASE WHEN $3='IDLE' THEN now() ELSE last_synced_at END,
		version=version+1,updated_at=now()
	WHERE id=$1 AND reconciliation_lease_owner=$2`, p.ID, p.ReconciliationLeaseOwner,
		finalStatus, freshness, safeStatus, sourceVersion, code, next)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("finance projection reconciliation lease lost")
	}
	_, err = q.Exec(ctx, `INSERT INTO fleet_partner_finance_projection_history
		(id,organization_id,projection_id,previous_freshness_state,new_freshness_state,
		 previous_reconciliation_status,new_reconciliation_status,safe_reason_code,actor_type,actor_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'SYSTEM',$9)`, ids.New("fph"), p.OrganizationID,
		p.ID, p.FreshnessState, freshness, p.ReconciliationStatus, finalStatus, code, p.ReconciliationLeaseOwner)
	return err
}

func (r *Repository) ReplayProjection(ctx context.Context, id string) error {
	return r.ReplayProjectionIn(ctx, r.db, id)
}

func (r *Repository) ReplayProjectionIn(ctx context.Context, q database.Querier, id string) error {
	tag, err := q.Exec(ctx, `UPDATE fleet_partner_finance_projections
	SET reconciliation_status='PENDING',freshness_state='UNKNOWN',reconciliation_attempts=0,
		reconciliation_next_attempt_at=now(),reconciliation_safe_error_code='',version=version+1,updated_at=now()
	WHERE id=$1 AND reconciliation_status='DEAD_LETTER'`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
