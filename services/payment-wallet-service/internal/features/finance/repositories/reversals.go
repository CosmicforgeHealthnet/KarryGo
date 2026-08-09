package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// ReversalRepository owns the durable reversal contract.
//
// It shares the operation infrastructure used by settlements and refunds —
// leases, attempt counting, backoff scheduling and dead-lettering — rather than
// introducing a second execution framework.
type ReversalRepository struct{ db database.Querier }

// NewReversalRepository wires the repository over a pool or transaction.
func NewReversalRepository(db database.Querier) *ReversalRepository {
	return &ReversalRepository{db: db}
}

const reversalColumns = `id,organization_id,original_journal_id,original_reference,reason_code,reason,
	amount_minor,currency,request_fingerprint,idempotency_key,expected_version,status,
	external_operation_reference,attempts,next_attempt_at,COALESCE(lease_owner,''),lease_expires_at,
	correlation_id,causation_id,created_by_actor_type,created_by_actor_id,approved_by_actor_type,
	approved_by_actor_id,approval_reference,approval_policy_version,approved_at,
	COALESCE(resulting_journal_id,''),safe_error_code,version,created_at,updated_at,completed_at`

func reversalColumnsWithPrefix(p string) string {
	return p + `id,` + p + `organization_id,` + p + `original_journal_id,` + p + `original_reference,` +
		p + `reason_code,` + p + `reason,` + p + `amount_minor,` + p + `currency,` +
		p + `request_fingerprint,` + p + `idempotency_key,` + p + `expected_version,` + p + `status,` +
		p + `external_operation_reference,` + p + `attempts,` + p + `next_attempt_at,COALESCE(` +
		p + `lease_owner,''),` + p + `lease_expires_at,` + p + `correlation_id,` + p + `causation_id,` +
		p + `created_by_actor_type,` + p + `created_by_actor_id,` + p + `approved_by_actor_type,` +
		p + `approved_by_actor_id,` + p + `approval_reference,` + p + `approval_policy_version,` +
		p + `approved_at,COALESCE(` + p + `resulting_journal_id,''),` + p + `safe_error_code,` +
		p + `version,` + p + `created_at,` + p + `updated_at,` + p + `completed_at`
}

func scanReversal(row pgx.Row) (*models.Reversal, error) {
	var value models.Reversal
	err := row.Scan(&value.ID, &value.OrganizationID, &value.OriginalJournalID,
		&value.OriginalReference, &value.ReasonCode, &value.Reason, &value.AmountMinor,
		&value.Currency, &value.RequestFingerprint, &value.IdempotencyKey, &value.ExpectedVersion,
		&value.Status, &value.ExternalOperationReference, &value.Attempts, &value.NextAttemptAt,
		&value.LeaseOwner, &value.LeaseExpiresAt, &value.CorrelationID, &value.CausationID,
		&value.CreatedByActorType, &value.CreatedByActorID, &value.ApprovedByActorType,
		&value.ApprovedByActorID, &value.ApprovalReference, &value.ApprovalPolicyVersion,
		&value.ApprovedAt, &value.ResultingJournalID, &value.SafeErrorCode, &value.Version,
		&value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan reversal: %w", err)
	}
	return &value, nil
}

// Create inserts a new reversal request.
func (r *ReversalRepository) Create(ctx context.Context, q database.Querier,
	reversal *models.Reversal) error {
	if q == nil {
		q = r.db
	}
	if reversal.ID == "" {
		reversal.ID = ids.New(ids.PrefixReversal)
	}
	if reversal.Status == "" {
		reversal.Status = models.ReversalRequested
	}
	err := q.QueryRow(ctx, `INSERT INTO finance_reversals
		(id,organization_id,original_journal_id,original_reference,reason_code,reason,amount_minor,
		 currency,request_fingerprint,idempotency_key,expected_version,status,correlation_id,
		 causation_id,created_by_actor_type,created_by_actor_id,next_attempt_at,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,now(),1,now(),now())
		RETURNING version,created_at,updated_at,next_attempt_at`,
		reversal.ID, reversal.OrganizationID, reversal.OriginalJournalID, reversal.OriginalReference,
		reversal.ReasonCode, reversal.Reason, reversal.AmountMinor, reversal.Currency,
		reversal.RequestFingerprint, reversal.IdempotencyKey, reversal.ExpectedVersion,
		reversal.Status, reversal.CorrelationID, reversal.CausationID,
		reversal.CreatedByActorType, reversal.CreatedByActorID).
		Scan(&reversal.Version, &reversal.CreatedAt, &reversal.UpdatedAt, &reversal.NextAttemptAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create reversal: %w", err)
	}
	return nil
}

// Get loads one reversal inside a tenant. An empty organizationID means the
// platform scope and is only ever passed by internal recovery paths.
func (r *ReversalRepository) Get(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.Reversal, error) {
	if q == nil {
		q = r.db
	}
	return scanReversal(q.QueryRow(ctx, `SELECT `+reversalColumns+` FROM finance_reversals
		WHERE id=$1 AND ($2='' OR organization_id=$2)`, id, organizationID))
}

// FindByKey resolves a replayed request by its idempotency key.
func (r *ReversalRepository) FindByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (*models.Reversal, error) {
	if q == nil {
		q = r.db
	}
	return scanReversal(q.QueryRow(ctx, `SELECT `+reversalColumns+` FROM finance_reversals
		WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key))
}

// ListFilter narrows a reversal listing.
type ListFilter struct {
	Status            string
	OriginalJournalID string
	Limit             int
}

// List returns a tenant's reversals, newest first.
func (r *ReversalRepository) List(ctx context.Context, q database.Querier,
	organizationID string, filter ListFilter) ([]models.Reversal, error) {
	if q == nil {
		q = r.db
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	rows, err := q.Query(ctx, `SELECT `+reversalColumns+` FROM finance_reversals
		WHERE organization_id=$1 AND ($2='' OR status=$2) AND ($3='' OR original_journal_id=$3)
		ORDER BY created_at DESC, id DESC LIMIT $4`,
		organizationID, filter.Status, filter.OriginalJournalID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list reversals: %w", err)
	}
	defer rows.Close()
	var values []models.Reversal
	for rows.Next() {
		value, scanErr := scanReversal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, *value)
	}
	return values, rows.Err()
}

// SetPendingApproval moves a fresh request into the approval queue.
func (r *ReversalRepository) SetPendingApproval(ctx context.Context, q database.Querier,
	id string, expectedVersion int64) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_reversals SET status='PENDING_APPROVAL',
		version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND status='REQUESTED'`,
		id, expectedVersion)
	if err != nil {
		return fmt.Errorf("finance: queue reversal for approval: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// Approve records the approval decision and releases the reversal for
// execution. The compare-and-set on version and status is what stops two
// approvers from both believing they approved it.
func (r *ReversalRepository) Approve(ctx context.Context, q database.Querier, id string,
	expectedVersion int64, actorType, actorID, approvalReference string, policyVersion int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_reversals SET status='APPROVED',
		approved_by_actor_type=$3,approved_by_actor_id=$4,approval_reference=$5,
		approval_policy_version=$6,approved_at=now(),next_attempt_at=now(),
		version=version+1,updated_at=now()
		WHERE id=$1 AND version=$2 AND status IN ('REQUESTED','PENDING_APPROVAL')`,
		id, expectedVersion, actorType, actorID, approvalReference, policyVersion)
	if err != nil {
		return fmt.Errorf("finance: approve reversal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// Reject terminates a reversal without any ledger effect.
func (r *ReversalRepository) Reject(ctx context.Context, q database.Querier, id string,
	expectedVersion int64, actorType, actorID, reasonCode string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_reversals SET status='REJECTED',
		approved_by_actor_type=$3,approved_by_actor_id=$4,safe_error_code=$5,approved_at=now(),
		completed_at=now(),version=version+1,updated_at=now()
		WHERE id=$1 AND version=$2 AND status IN ('REQUESTED','PENDING_APPROVAL')`,
		id, expectedVersion, actorType, actorID, reasonCode)
	if err != nil {
		return fmt.Errorf("finance: reject reversal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// Claim leases due reversals for one worker. An UNKNOWN row is claimed into
// RECONCILING so the executor reads the outcome back instead of executing
// again — the difference between a duplicate correction and a resolved one.
func (r *ReversalRepository) Claim(ctx context.Context, owner string,
	lease time.Duration, limit int) ([]models.Reversal, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `WITH claimed AS (
		SELECT id FROM finance_reversals
		WHERE status IN ('APPROVED','FAILED_RETRYABLE','UNKNOWN','RECONCILING')
		  AND next_attempt_at<=now()
		  AND (lease_expires_at IS NULL OR lease_expires_at<now())
		ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1)
		UPDATE finance_reversals r SET lease_owner=$2,lease_expires_at=now()+$3::interval,
		status=CASE WHEN r.status='UNKNOWN' THEN 'RECONCILING' ELSE 'PROCESSING' END,
		version=version+1,updated_at=now() FROM claimed WHERE r.id=claimed.id
		RETURNING `+reversalColumnsWithPrefix("r."), limit, owner, lease.String())
	if err != nil {
		return nil, fmt.Errorf("finance: claim reversals: %w", err)
	}
	defer rows.Close()
	var values []models.Reversal
	for rows.Next() {
		value, scanErr := scanReversal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, *value)
	}
	return values, rows.Err()
}

// SetOutcome records one execution attempt's result under the worker's lease.
func (r *ReversalRepository) SetOutcome(ctx context.Context, q database.Querier,
	id, owner, status, externalReference, journalID string, next time.Time,
	safeErrorCode string, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_reversals SET
		status=CASE WHEN $3='FAILED_RETRYABLE' AND attempts+1 >= $8 THEN 'DEAD_LETTER' ELSE $3 END,
		attempts=attempts+1,next_attempt_at=$6,
		external_operation_reference=COALESCE(NULLIF($4,''),external_operation_reference),
		resulting_journal_id=COALESCE(NULLIF($5,''),resulting_journal_id),
		safe_error_code=$7,lease_owner=NULL,lease_expires_at=NULL,
		completed_at=CASE WHEN $3='COMPLETED' THEN now() ELSE completed_at END,
		version=version+1,updated_at=now()
		WHERE id=$1 AND lease_owner=$2`,
		id, owner, status, externalReference, journalID, next, safeErrorCode, maxAttempts)
	if err != nil {
		return fmt.Errorf("finance: set reversal outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// RecoverExpiredLeases returns abandoned reversals to a retryable state.
//
// A row abandoned mid-PROCESSING may or may not have posted; it becomes
// UNKNOWN so the next pass resolves it by read-back rather than by executing.
func (r *ReversalRepository) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE finance_reversals
		SET lease_owner=NULL,lease_expires_at=NULL,safe_error_code='lease_expired',
		status=CASE WHEN status IN ('PROCESSING','RECONCILING') THEN 'UNKNOWN' ELSE status END,
		next_attempt_at=now(),version=version+1,updated_at=now()
		WHERE lease_expires_at<now()`)
	if err != nil {
		return 0, fmt.Errorf("finance: recover reversal leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Replay returns a dead-lettered reversal to the execution queue.
func (r *ReversalRepository) Replay(ctx context.Context, q database.Querier, id string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_reversals SET status='APPROVED',attempts=0,
		next_attempt_at=now(),safe_error_code='',version=version+1,updated_at=now()
		WHERE id=$1 AND status='DEAD_LETTER'`, id)
	if err != nil {
		return fmt.Errorf("finance: replay reversal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// StuckReversals lists reversals whose outcome has stayed ambiguous past the
// cutoff. Reconciliation reports them rather than resolving them silently.
func (r *ReversalRepository) StuckReversals(ctx context.Context, q database.Querier,
	cutoff time.Time, limit int) ([]models.Reversal, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx, `SELECT `+reversalColumns+` FROM finance_reversals
		WHERE status IN ('UNKNOWN','RECONCILING','DEAD_LETTER') AND updated_at < $1
		ORDER BY updated_at LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list stuck reversals: %w", err)
	}
	defer rows.Close()
	var values []models.Reversal
	for rows.Next() {
		value, scanErr := scanReversal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, *value)
	}
	return values, rows.Err()
}

// CompletedWithoutJournal finds reversals that claim completion but recorded no
// compensating journal. It must always return nothing.
func (r *ReversalRepository) CompletedWithoutJournal(ctx context.Context, q database.Querier,
	limit int) ([]string, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx, `SELECT id FROM finance_reversals
		WHERE status='COMPLETED' AND resulting_journal_id IS NULL LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list unposted completed reversals: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("finance: scan unposted completed reversal: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
