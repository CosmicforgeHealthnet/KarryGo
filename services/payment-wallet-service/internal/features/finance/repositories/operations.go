package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// OperationsRepository owns settlements, refunds, reconciliation and statements.
type OperationsRepository struct{ db database.Querier }

func NewOperationsRepository(db database.Querier) *OperationsRepository {
	return &OperationsRepository{db: db}
}

const batchColumns = `id, organization_id, beneficiary_type, beneficiary_reference,
	COALESCE(earning_account_id, ''), COALESCE(wallet_id, ''), currency, total_minor,
	item_count, status, requested_by_actor_type, requested_by_actor_id,
	approved_by_actor_type, approved_by_actor_id, approved_at, idempotency_key,
	request_fingerprint, correlation_id, version, created_at, updated_at`

func scanBatch(row pgx.Row) (*models.SettlementBatch, error) {
	var value models.SettlementBatch
	err := row.Scan(&value.ID, &value.OrganizationID, &value.BeneficiaryType,
		&value.BeneficiaryReference, &value.EarningAccountID, &value.WalletID,
		&value.Currency, &value.TotalMinor, &value.ItemCount, &value.Status,
		&value.RequestedByType, &value.RequestedByID, &value.ApprovedByType,
		&value.ApprovedByID, &value.ApprovedAt, &value.IdempotencyKey,
		&value.RequestFingerprint, &value.CorrelationID, &value.Version,
		&value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan settlement batch: %w", err)
	}
	return &value, nil
}

func (r *OperationsRepository) CreateSettlementBatch(ctx context.Context, q database.Querier,
	batch *models.SettlementBatch, allocations []models.EarningAllocation) error {
	if q == nil {
		q = r.db
	}
	if batch.ID == "" {
		batch.ID = ids.New(ids.PrefixSettlementBatch)
	}
	if batch.Status == "" {
		batch.Status = models.BatchPendingApproval
	}
	batch.ItemCount = len(allocations)
	batch.TotalMinor = 0
	for _, allocation := range allocations {
		batch.TotalMinor += allocation.ProviderEarningMinor
	}
	err := q.QueryRow(ctx, `INSERT INTO finance_settlement_batches
		(id, organization_id, beneficiary_type, beneficiary_reference, earning_account_id,
		 wallet_id, currency, total_minor, item_count, status, requested_by_actor_type,
		 requested_by_actor_id, idempotency_key, request_fingerprint, correlation_id,
		 version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,1,now(),now())
		RETURNING version, created_at, updated_at`, batch.ID, batch.OrganizationID,
		batch.BeneficiaryType, batch.BeneficiaryReference, batch.EarningAccountID,
		batch.WalletID, batch.Currency, batch.TotalMinor, batch.ItemCount, batch.Status,
		batch.RequestedByType, batch.RequestedByID, batch.IdempotencyKey,
		batch.RequestFingerprint, batch.CorrelationID).
		Scan(&batch.Version, &batch.CreatedAt, &batch.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create settlement batch: %w", err)
	}
	for _, allocation := range allocations {
		_, err := q.Exec(ctx, `INSERT INTO finance_settlement_items
			(id,batch_id,organization_id,earning_allocation_id,amount_minor,currency,status,
			 idempotency_key,next_attempt_at,version,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,'PENDING',$7,now(),1,now(),now())`,
			ids.New(ids.PrefixSettlementItem), batch.ID, batch.OrganizationID, allocation.ID,
			allocation.ProviderEarningMinor, batch.Currency, batch.IdempotencyKey+":"+allocation.ID)
		if err != nil {
			return fmt.Errorf("finance: create settlement item: %w", err)
		}
	}
	return nil
}

func (r *OperationsRepository) GetSettlementBatch(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.SettlementBatch, error) {
	if q == nil {
		q = r.db
	}
	return scanBatch(q.QueryRow(ctx, `SELECT `+batchColumns+` FROM finance_settlement_batches
		WHERE id=$1 AND ($2='' OR organization_id=$2)`, id, organizationID))
}

func (r *OperationsRepository) FindSettlementBatchByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (*models.SettlementBatch, error) {
	if q == nil {
		q = r.db
	}
	return scanBatch(q.QueryRow(ctx, `SELECT `+batchColumns+` FROM finance_settlement_batches
		WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key))
}

func (r *OperationsRepository) ApproveSettlementBatch(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, actorType, actorID string) error {
	tag, err := q.Exec(ctx, `UPDATE finance_settlement_batches SET status='APPROVED',
		approved_by_actor_type=$3, approved_by_actor_id=$4, approved_at=now(),
		version=version+1, updated_at=now()
		WHERE id=$1 AND version=$2 AND status='PENDING_APPROVAL'`, id, expectedVersion, actorType, actorID)
	if err != nil {
		return fmt.Errorf("finance: approve settlement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

const settlementItemColumns = `id,batch_id,organization_id,COALESCE(earning_allocation_id,''),
	amount_minor,currency,status,attempts,next_attempt_at,COALESCE(lease_owner,''),lease_expires_at,
	external_reference,COALESCE(journal_id,''),COALESCE(reversal_journal_id,''),idempotency_key,
	last_error,version,created_at,updated_at,completed_at`
const settlementItemReturning = `i.id,i.batch_id,i.organization_id,COALESCE(i.earning_allocation_id,''),
	i.amount_minor,i.currency,i.status,i.attempts,i.next_attempt_at,COALESCE(i.lease_owner,''),i.lease_expires_at,
	i.external_reference,COALESCE(i.journal_id,''),COALESCE(i.reversal_journal_id,''),i.idempotency_key,
	i.last_error,i.version,i.created_at,i.updated_at,i.completed_at`

func scanSettlementItem(row pgx.Row) (*models.SettlementItem, error) {
	var value models.SettlementItem
	err := row.Scan(&value.ID, &value.BatchID, &value.OrganizationID, &value.EarningAllocationID,
		&value.AmountMinor, &value.Currency, &value.Status, &value.Attempts, &value.NextAttemptAt,
		&value.LeaseOwner, &value.LeaseExpiresAt, &value.ExternalReference, &value.JournalID,
		&value.ReversalJournalID, &value.IdempotencyKey, &value.LastError, &value.Version,
		&value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan settlement item: %w", err)
	}
	return &value, nil
}

func (r *OperationsRepository) ClaimSettlementItems(ctx context.Context, owner string,
	lease time.Duration, limit int) ([]models.SettlementItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `WITH claimed AS (
		SELECT i.id FROM finance_settlement_items i
		JOIN finance_settlement_batches b ON b.id=i.batch_id
		WHERE b.status IN ('APPROVED','PROCESSING')
		AND i.status IN ('PENDING','RETRYABLE_FAILURE','UNKNOWN','RECONCILING')
		AND i.next_attempt_at<=now() AND (i.lease_expires_at IS NULL OR i.lease_expires_at<now())
		ORDER BY i.next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1)
		UPDATE finance_settlement_items i SET lease_owner=$2,lease_expires_at=now()+$3::interval,
		status=CASE WHEN i.status='UNKNOWN' THEN 'RECONCILING' ELSE 'PROCESSING' END,
		version=version+1,updated_at=now() FROM claimed WHERE i.id=claimed.id
		RETURNING `+settlementItemReturning, limit, owner, lease.String())
	if err != nil {
		return nil, fmt.Errorf("finance: claim settlement items: %w", err)
	}
	defer rows.Close()
	var values []models.SettlementItem
	for rows.Next() {
		value, scanErr := scanSettlementItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, *value)
	}
	return values, rows.Err()
}

func (r *OperationsRepository) SetSettlementOutcome(ctx context.Context, q database.Querier, id, owner, status,
	externalReference, journalID string, next time.Time, lastError string, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_settlement_items SET
		status=CASE WHEN $3='RETRYABLE_FAILURE' AND attempts+1 >= $8 THEN 'DEAD_LETTER' ELSE $3 END,
		attempts=attempts+1,next_attempt_at=$6,external_reference=COALESCE(NULLIF($4,''),external_reference),
		journal_id=COALESCE(NULLIF($5,''),journal_id),last_error=$7,lease_owner=NULL,lease_expires_at=NULL,
		completed_at=CASE WHEN $3='SUCCEEDED' THEN now() ELSE completed_at END,version=version+1,updated_at=now()
		WHERE id=$1 AND lease_owner=$2`, id, owner, status, externalReference, journalID, next, lastError, maxAttempts)
	if err != nil {
		return fmt.Errorf("finance: set settlement outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// RefreshSettlementBatch derives the aggregate state from its authoritative
// items. It is called in the same transaction as an item transition.
func (r *OperationsRepository) RefreshSettlementBatch(ctx context.Context, q database.Querier, batchID string) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `UPDATE finance_settlement_batches b SET status=summary.status,
		version=b.version+1,updated_at=now()
		FROM (SELECT batch_id,CASE
			WHEN bool_and(status='SUCCEEDED') THEN 'COMPLETED'
			WHEN bool_and(status IN ('SUCCEEDED','PERMANENT_FAILURE','DEAD_LETTER')) AND bool_or(status<>'SUCCEEDED') THEN 'PARTIALLY_FAILED'
			ELSE 'PROCESSING' END status
		FROM finance_settlement_items WHERE batch_id=$1 GROUP BY batch_id) summary
		WHERE b.id=summary.batch_id AND b.status IS DISTINCT FROM summary.status`, batchID)
	if err != nil {
		return fmt.Errorf("finance: refresh settlement batch: %w", err)
	}
	return nil
}

const refundColumns = `id,organization_id,original_journal_id,original_reference,amount_minor,currency,
	reason_code,reason,status,requested_by_actor_type,requested_by_actor_id,approved_by_actor_type,
	approved_by_actor_id,approved_at,attempts,next_attempt_at,COALESCE(lease_owner,''),lease_expires_at,
	external_reference,COALESCE(journal_id,''),COALESCE(reversal_journal_id,''),last_error,
	idempotency_key,request_fingerprint,correlation_id,version,created_at,updated_at,completed_at`
const refundReturning = `r.id,r.organization_id,r.original_journal_id,r.original_reference,r.amount_minor,r.currency,
	r.reason_code,r.reason,r.status,r.requested_by_actor_type,r.requested_by_actor_id,r.approved_by_actor_type,
	r.approved_by_actor_id,r.approved_at,r.attempts,r.next_attempt_at,COALESCE(r.lease_owner,''),r.lease_expires_at,
	r.external_reference,COALESCE(r.journal_id,''),COALESCE(r.reversal_journal_id,''),r.last_error,
	r.idempotency_key,r.request_fingerprint,r.correlation_id,r.version,r.created_at,r.updated_at,r.completed_at`

func scanRefund(row pgx.Row) (*models.Refund, error) {
	var value models.Refund
	err := row.Scan(&value.ID, &value.OrganizationID, &value.OriginalJournalID, &value.OriginalReference,
		&value.AmountMinor, &value.Currency, &value.ReasonCode, &value.Reason, &value.Status,
		&value.RequestedByType, &value.RequestedByID, &value.ApprovedByType, &value.ApprovedByID,
		&value.ApprovedAt, &value.Attempts, &value.NextAttemptAt, &value.LeaseOwner, &value.LeaseExpiresAt,
		&value.ExternalReference, &value.JournalID, &value.ReversalJournalID, &value.LastError,
		&value.IdempotencyKey, &value.RequestFingerprint, &value.CorrelationID, &value.Version,
		&value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan refund: %w", err)
	}
	return &value, nil
}

func (r *OperationsRepository) CreateRefund(ctx context.Context, q database.Querier, refund *models.Refund) error {
	if q == nil {
		q = r.db
	}
	if refund.ID == "" {
		refund.ID = ids.New(ids.PrefixRefund)
	}
	if refund.Status == "" {
		refund.Status = models.RefundRequested
	}
	err := q.QueryRow(ctx, `INSERT INTO finance_refunds(id,organization_id,original_journal_id,
		original_reference,amount_minor,currency,reason_code,reason,status,requested_by_actor_type,
		requested_by_actor_id,idempotency_key,request_fingerprint,correlation_id,next_attempt_at,
		version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),1,now(),now())
		RETURNING version,created_at,updated_at,next_attempt_at`, refund.ID, refund.OrganizationID,
		refund.OriginalJournalID, refund.OriginalReference, refund.AmountMinor, refund.Currency,
		refund.ReasonCode, refund.Reason, refund.Status, refund.RequestedByType, refund.RequestedByID,
		refund.IdempotencyKey, refund.RequestFingerprint, refund.CorrelationID).
		Scan(&refund.Version, &refund.CreatedAt, &refund.UpdatedAt, &refund.NextAttemptAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create refund: %w", err)
	}
	return nil
}

func (r *OperationsRepository) GetRefund(ctx context.Context, q database.Querier, organizationID, id string) (*models.Refund, error) {
	if q == nil {
		q = r.db
	}
	return scanRefund(q.QueryRow(ctx, `SELECT `+refundColumns+` FROM finance_refunds WHERE id=$1 AND ($2='' OR organization_id=$2)`, id, organizationID))
}
func (r *OperationsRepository) FindRefundByKey(ctx context.Context, q database.Querier, organizationID, key string) (*models.Refund, error) {
	if q == nil {
		q = r.db
	}
	return scanRefund(q.QueryRow(ctx, `SELECT `+refundColumns+` FROM finance_refunds WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key))
}

func (r *OperationsRepository) ApproveRefund(ctx context.Context, q database.Querier, id string, expectedVersion int64, actorType, actorID string) error {
	tag, err := q.Exec(ctx, `UPDATE finance_refunds SET status='APPROVED',approved_by_actor_type=$3,
		approved_by_actor_id=$4,approved_at=now(),version=version+1,updated_at=now()
		WHERE id=$1 AND version=$2 AND status='REQUESTED'`, id, expectedVersion, actorType, actorID)
	if err != nil {
		return fmt.Errorf("finance: approve refund: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

func (r *OperationsRepository) ClaimRefunds(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Refund, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `WITH claimed AS (SELECT id FROM finance_refunds
		WHERE status IN ('APPROVED','FAILED','UNKNOWN','RECONCILING') AND next_attempt_at<=now()
		AND (lease_expires_at IS NULL OR lease_expires_at<now()) ORDER BY next_attempt_at
		FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE finance_refunds r SET lease_owner=$2,
		lease_expires_at=now()+$3::interval,status=CASE WHEN r.status='UNKNOWN' THEN 'RECONCILING' ELSE 'PROCESSING' END,
		version=version+1,updated_at=now() FROM claimed WHERE r.id=claimed.id RETURNING `+refundReturning, limit, owner, lease.String())
	if err != nil {
		return nil, fmt.Errorf("finance: claim refunds: %w", err)
	}
	defer rows.Close()
	var values []models.Refund
	for rows.Next() {
		value, scanErr := scanRefund(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, *value)
	}
	return values, rows.Err()
}

func (r *OperationsRepository) SetRefundOutcome(ctx context.Context, q database.Querier, id, owner, status, externalReference, journalID string, next time.Time, lastError string, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_refunds SET status=CASE WHEN $3='FAILED' AND attempts+1 >= $8 THEN 'DEAD_LETTER' ELSE $3 END,
		attempts=attempts+1,next_attempt_at=$6,external_reference=COALESCE(NULLIF($4,''),external_reference),
		journal_id=COALESCE(NULLIF($5,''),journal_id),last_error=$7,lease_owner=NULL,lease_expires_at=NULL,
		completed_at=CASE WHEN $3='COMPLETED' THEN now() ELSE completed_at END,version=version+1,updated_at=now()
		WHERE id=$1 AND lease_owner=$2`, id, owner, status, externalReference, journalID, next, lastError, maxAttempts)
	if err != nil {
		return fmt.Errorf("finance: set refund outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (r *OperationsRepository) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	settlements, err := r.db.Exec(ctx, `UPDATE finance_settlement_items SET lease_owner=NULL,lease_expires_at=NULL,
		status=CASE WHEN status='PROCESSING' THEN 'RETRYABLE_FAILURE' WHEN status='RECONCILING' THEN 'UNKNOWN' ELSE status END
		WHERE lease_expires_at<now()`)
	if err != nil {
		return 0, err
	}
	refunds, err := r.db.Exec(ctx, `UPDATE finance_refunds SET lease_owner=NULL,lease_expires_at=NULL,
		status=CASE WHEN status='PROCESSING' THEN 'FAILED' WHEN status='RECONCILING' THEN 'UNKNOWN' ELSE status END
		WHERE lease_expires_at<now()`)
	if err != nil {
		return 0, err
	}
	return settlements.RowsAffected() + refunds.RowsAffected(), nil
}

func (r *OperationsRepository) ReplaySettlement(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `UPDATE finance_settlement_items SET status='RETRYABLE_FAILURE',attempts=0,next_attempt_at=now(),last_error='',version=version+1 WHERE id=$1 AND status='DEAD_LETTER'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}
func (r *OperationsRepository) ReplayRefund(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `UPDATE finance_refunds SET status='FAILED',attempts=0,next_attempt_at=now(),last_error='',version=version+1 WHERE id=$1 AND status='DEAD_LETTER'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (r *OperationsRepository) CreateReconciliationRun(ctx context.Context, q database.Querier, run *models.ReconciliationRun) error {
	if q == nil {
		q = r.db
	}
	if run.ID == "" {
		run.ID = ids.New(ids.PrefixReconciliation)
	}
	return q.QueryRow(ctx, `INSERT INTO finance_reconciliation_runs(id,scope,status,started_at) VALUES($1,$2,'RUNNING',now()) RETURNING status,started_at`, run.ID, run.Scope).Scan(&run.Status, &run.StartedAt)
}
func (r *OperationsRepository) AddFinding(ctx context.Context, q database.Querier, finding *models.ReconciliationFinding) error {
	if q == nil {
		q = r.db
	}
	if finding.ID == "" {
		finding.ID = ids.New(ids.PrefixFinding)
	}
	details := finding.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	return q.QueryRow(ctx, `INSERT INTO finance_reconciliation_findings(id,run_id,organization_id,entity_type,entity_id,finding_code,details,created_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,now()) RETURNING created_at`, finding.ID, finding.RunID, finding.OrganizationID, finding.EntityType, finding.EntityID, finding.FindingCode, details).Scan(&finding.CreatedAt)
}
func (r *OperationsRepository) CompleteReconciliationRun(ctx context.Context, q database.Querier, id string, checked, findings int, runErr string) error {
	if q == nil {
		q = r.db
	}
	status := "COMPLETED"
	if runErr != "" {
		status = "FAILED"
	}
	_, err := q.Exec(ctx, `UPDATE finance_reconciliation_runs SET status=$2,checked_count=$3,findings_count=$4,completed_at=now(),last_error=$5 WHERE id=$1`, id, status, checked, findings, runErr)
	return err
}

func (r *OperationsRepository) CreateStatement(ctx context.Context, q database.Querier, statement *models.Statement) error {
	if q == nil {
		q = r.db
	}
	if statement.ID == "" {
		statement.ID = ids.New(ids.PrefixStatement)
	}
	return q.QueryRow(ctx, `INSERT INTO finance_statements(id,organization_id,statement_type,currency,period_start,period_end,line_count,total_debit_minor,total_credit_minor,content_hash,generated_by_type,generated_by_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now()) RETURNING created_at`, statement.ID, statement.OrganizationID, statement.StatementType, statement.Currency, statement.PeriodStart, statement.PeriodEnd, statement.LineCount, statement.TotalDebitMinor, statement.TotalCreditMinor, statement.ContentHash, statement.GeneratedByType, statement.GeneratedByID).Scan(&statement.CreatedAt)
}
