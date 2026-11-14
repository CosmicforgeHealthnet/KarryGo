package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// LockOperationVersion reads an operation's version FOR UPDATE.
func (r *Repository) LockOperationVersion(ctx context.Context, q database.Querier, organizationID, id string) (int64, error) {
	var version int64
	err := r.querier(q).QueryRow(ctx, `SELECT version FROM fleet_partner_finance_operations WHERE id=$1 AND organization_id=$2 FOR UPDATE`, id, organizationID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, models.ErrNotFound
	}
	return version, err
}

// LockProjectionVersion reads a projection's version FOR UPDATE.
func (r *Repository) LockProjectionVersion(ctx context.Context, q database.Querier, organizationID, id string) (int64, error) {
	var version int64
	err := r.querier(q).QueryRow(ctx, `SELECT version FROM fleet_partner_finance_projections WHERE id=$1 AND organization_id=$2 FOR UPDATE`, id, organizationID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, models.ErrNotFound
	}
	return version, err
}

func (r *Repository) querier(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

// CurrentProjection reads a CURRENT, fully reconciled projection; anything
// else is ErrNotFound.
func (r *Repository) CurrentProjection(ctx context.Context, organizationID, projectionType, reference string) (models.Projection, error) {
	return scanProjection(r.db.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type=$2 AND wallet_reference=$3
		  AND freshness_state='CURRENT' AND reconciliation_status='IDLE'`, organizationID, projectionType, reference))
}

// LatestSubscriptionStatus reads the newest current subscription status;
// found is false when there is none.
func (r *Repository) LatestSubscriptionStatus(ctx context.Context, organizationID string) (status string, found bool, err error) {
	err = r.db.QueryRow(ctx, `SELECT safe_status FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type='SUBSCRIPTION'
		  AND freshness_state='CURRENT' AND reconciliation_status='IDLE'
		ORDER BY last_synced_at DESC LIMIT 1`, organizationID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return status, err == nil, err
}

// OpenReconciliation reports whether a projection is being reconciled.
func (r *Repository) OpenReconciliation(ctx context.Context, organizationID, projectionType, reference string) (bool, error) {
	var open bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type=$2 AND wallet_reference=$3
		  AND reconciliation_status<>'IDLE')`, organizationID, projectionType, reference).Scan(&open)
	return open, err
}

// PaymentProjectionState reads Fleet's payment-verification projection;
// found is false when Fleet holds none.
func (r *Repository) PaymentProjectionState(ctx context.Context, organizationID, reference string) (freshness, reconciliation string, sourceVersion int64, found bool, err error) {
	err = r.db.QueryRow(ctx, `SELECT freshness_state,reconciliation_status,source_version
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type='PAYMENT_VERIFICATION' AND wallet_reference=$2`,
		organizationID, reference).Scan(&freshness, &reconciliation, &sourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, false, nil
	}
	return freshness, reconciliation, sourceVersion, err == nil, err
}

// TripCompletion reads the Dispatch-owned completion facts of a trip.
func (r *Repository) TripCompletion(ctx context.Context, organizationID, tripID string) (models.TripCompletion, error) {
	var c models.TripCompletion
	err := r.db.QueryRow(ctx, `SELECT freshness_state,trip_status,
		completed_at IS NOT NULL,cancelled_at IS NOT NULL,failed_at IS NOT NULL,
		recipient_confirmed_at IS NOT NULL
		FROM fleet_partner_trip_projections
		WHERE organization_id=$1 AND platform_trip_id=$2`, organizationID, tripID).
		Scan(&c.Freshness, &c.TripStatus, &c.Completed, &c.Cancelled, &c.Failed, &c.RecipientConfirmed)
	return c, err
}

// OperationRequester reads who requested an operation.
func (r *Repository) OperationRequester(ctx context.Context, organizationID, id string) (actorType, actorID string, err error) {
	err = r.db.QueryRow(ctx, `SELECT requested_by_actor_type,requested_by_actor_id
		FROM fleet_partner_finance_operations WHERE id=$1 AND organization_id=$2`,
		id, organizationID).Scan(&actorType, &actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", models.ErrNotFound
	}
	return actorType, actorID, err
}

// ---------------------------------------------------------------------------
// Wallet event ingestion
// ---------------------------------------------------------------------------

// LockSourceVersion reads the last applied version of a Wallet aggregate;
// found is false for a first event.
func (r *Repository) LockSourceVersion(ctx context.Context, q database.Querier, aggregateType, aggregateID string) (version int64, payloadHash string, found bool, err error) {
	err = r.querier(q).QueryRow(ctx, `SELECT source_version,payload_hash FROM fleet_partner_finance_source_versions WHERE aggregate_type=$1 AND aggregate_id=$2 FOR UPDATE`, aggregateType, aggregateID).Scan(&version, &payloadHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	return version, payloadHash, err == nil, err
}

// UpsertProjection writes an event's projection.
func (r *Repository) UpsertProjection(ctx context.Context, q database.Querier, w models.ProjectionWrite) error {
	_, err := r.querier(q).Exec(ctx, `INSERT INTO fleet_partner_finance_projections(id,organization_id,projection_type,subject_type,subject_id,wallet_reference,amount_minor,currency,safe_status,source_version,freshness_state,last_event_id,reconciliation_status,reconciliation_next_attempt_at) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,now()) ON CONFLICT(organization_id,projection_type,wallet_reference) DO UPDATE SET subject_type=EXCLUDED.subject_type,subject_id=EXCLUDED.subject_id,amount_minor=EXCLUDED.amount_minor,currency=EXCLUDED.currency,safe_status=EXCLUDED.safe_status,source_version=EXCLUDED.source_version,freshness_state=EXCLUDED.freshness_state,last_event_id=EXCLUDED.last_event_id,reconciliation_status=EXCLUDED.reconciliation_status,reconciliation_next_attempt_at=now(),reconciliation_attempts=CASE WHEN EXCLUDED.reconciliation_status='IDLE' THEN 0 ELSE fleet_partner_finance_projections.reconciliation_attempts END,reconciliation_safe_error_code='',last_synced_at=now(),version=fleet_partner_finance_projections.version+1,updated_at=now()`, ids.New("fpr"), w.OrganizationID, w.ProjectionType, w.SubjectType, w.SubjectID, w.WalletReference, w.AmountMinor, w.Currency, w.SafeStatus, w.SourceVersion, w.Freshness, w.LastEventID, w.ReconciliationStatus)
	return err
}

// UpsertPaymentVerification writes the PAYMENT_VERIFICATION projection of a
// paid invoice's Wallet payment record.
func (r *Repository) UpsertPaymentVerification(ctx context.Context, q database.Querier, w models.ProjectionWrite) error {
	_, err := r.querier(q).Exec(ctx, `INSERT INTO fleet_partner_finance_projections
				(id,organization_id,projection_type,subject_type,subject_id,wallet_reference,amount_minor,currency,safe_status,source_version,freshness_state,last_event_id,reconciliation_status,reconciliation_next_attempt_at)
				VALUES($1,$2,'PAYMENT_VERIFICATION',$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,now())
				ON CONFLICT(organization_id,projection_type,wallet_reference) DO UPDATE SET
				 amount_minor=EXCLUDED.amount_minor,currency=EXCLUDED.currency,safe_status=EXCLUDED.safe_status,
				 source_version=EXCLUDED.source_version,freshness_state=EXCLUDED.freshness_state,last_event_id=EXCLUDED.last_event_id,
				 reconciliation_status=EXCLUDED.reconciliation_status,reconciliation_next_attempt_at=now(),
				 last_synced_at=now(),version=fleet_partner_finance_projections.version+1,updated_at=now()`,
		ids.New("fpr"), w.OrganizationID, w.SubjectType, w.SubjectID, w.WalletReference, w.AmountMinor, w.Currency,
		w.SafeStatus, w.SourceVersion, w.Freshness, w.LastEventID, w.ReconciliationStatus)
	return err
}

// UpsertSourceVersion records the last applied version of an aggregate.
func (r *Repository) UpsertSourceVersion(ctx context.Context, q database.Querier, aggregateType, aggregateID string, version int64, eventID, payloadHash string) error {
	_, err := r.querier(q).Exec(ctx, `INSERT INTO fleet_partner_finance_source_versions(aggregate_type,aggregate_id,source_version,last_event_id,payload_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(aggregate_type,aggregate_id) DO UPDATE SET source_version=EXCLUDED.source_version,last_event_id=EXCLUDED.last_event_id,payload_hash=EXCLUDED.payload_hash,updated_at=now()`, aggregateType, aggregateID, version, eventID, payloadHash)
	return err
}

// ---------------------------------------------------------------------------
// Operator reads
// ---------------------------------------------------------------------------

// branchFilter renders the branch restriction. An empty branch list means no
// restriction; a non-empty one always includes organization-level rows.
func branchFilter(branchIDs []string, column string) (string, any) {
	if len(branchIDs) == 0 {
		return "TRUE", nil
	}
	return "(" + column + " IS NULL OR " + column + " = ANY($%d))", branchIDs
}

const operationSummaryColumns = `id,organization_id,COALESCE(branch_id,''),subject_type,subject_id,
	operation_type,status,attempts,next_attempt_at,COALESCE(wallet_reference,''),safe_error_code,
	requested_by_actor_type,requested_by_actor_id,correlation_id,expected_version,version,
	created_at,updated_at,completed_at`

func scanOperationSummary(row pgx.Row) (models.OperationSummary, error) {
	var value models.OperationSummary
	err := row.Scan(&value.ID, &value.OrganizationID, &value.BranchID, &value.SubjectType,
		&value.SubjectID, &value.OperationType, &value.Status, &value.Attempts,
		&value.NextAttemptAt, &value.WalletReference, &value.SafeErrorCode,
		&value.RequestedByType, &value.RequestedByID, &value.CorrelationID,
		&value.ExpectedVersion, &value.Version, &value.CreatedAt, &value.UpdatedAt,
		&value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, models.ErrNotFound
	}
	return value, err
}

// ListOperations returns the tenant's durable Wallet operations.
func (r *Repository) ListOperations(ctx context.Context, scope models.Scope, filter models.OperationFilter) ([]models.OperationSummary, error) {
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{scope.OrganizationID, strings.ToUpper(filter.Status),
		strings.ToUpper(filter.OperationType), filter.SubjectID, scope.PageLimit()}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	rows, err := r.db.Query(ctx, `SELECT `+operationSummaryColumns+`
		FROM fleet_partner_finance_operations
		WHERE organization_id=$1 AND ($2='' OR status=$2) AND ($3='' OR operation_type=$3)
		  AND ($4='' OR subject_id=$4) AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OperationSummary, 0)
	for rows.Next() {
		value, scanErr := scanOperationSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// GetOperation reads one operation inside the scope.
func (r *Repository) GetOperation(ctx context.Context, scope models.Scope, id string) (models.OperationSummary, error) {
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{id, scope.OrganizationID}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	return scanOperationSummary(r.db.QueryRow(ctx, `SELECT `+operationSummaryColumns+`
		FROM fleet_partner_finance_operations
		WHERE id=$1 AND organization_id=$2 AND `+clause, args...))
}

// OperationHistory returns an operation's transitions.
func (r *Repository) OperationHistory(ctx context.Context, organizationID, id string) ([]models.HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT id,previous_status,new_status,safe_reason_code,
		actor_type,actor_id,occurred_at FROM fleet_partner_finance_operation_history
		WHERE operation_id=$1 AND organization_id=$2 ORDER BY occurred_at, id`, id, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.HistoryEntry, 0)
	for rows.Next() {
		var entry models.HistoryEntry
		if err := rows.Scan(&entry.ID, &entry.PreviousStatus, &entry.NewStatus,
			&entry.SafeReasonCode, &entry.ActorType, &entry.ActorID, &entry.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// OperationAudit returns the audit trail recorded against an operation.
func (r *Repository) OperationAudit(ctx context.Context, organizationID, id string) ([]models.AuditEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT id,action,resource_type,resource_id,actor_type,
		actor_id,occurred_at FROM fleet_partner_audit_records
		WHERE organization_id=$1 AND resource_type='finance_operation' AND resource_id=$2
		ORDER BY occurred_at, id`, organizationID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AuditEntry, 0)
	for rows.Next() {
		var entry models.AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Action, &entry.ResourceType, &entry.ResourceID,
			&entry.ActorType, &entry.ActorID, &entry.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

const projectionSummaryColumns = `id,organization_id,COALESCE(branch_id,''),projection_type,
	subject_type,subject_id,wallet_reference,amount_minor,COALESCE(currency,''),safe_status,
	source_version,freshness_state,reconciliation_status,reconciliation_attempts,
	reconciliation_safe_error_code,last_synced_at,version`

func scanProjectionSummary(row pgx.Row) (models.ProjectionSummary, error) {
	var value models.ProjectionSummary
	err := row.Scan(&value.ID, &value.OrganizationID, &value.BranchID, &value.ProjectionType,
		&value.SubjectType, &value.SubjectID, &value.WalletReference, &value.AmountMinor,
		&value.Currency, &value.SafeStatus, &value.SourceVersion, &value.FreshnessState,
		&value.ReconciliationStatus, &value.ReconciliationAttempts, &value.SafeErrorCode,
		&value.LastSyncedAt, &value.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, models.ErrNotFound
	}
	return value.WithAmount(), err
}

// ListProjections returns projections; ReconciliationOnly lists only open
// reconciliation cases.
func (r *Repository) ListProjections(ctx context.Context, scope models.Scope, filter models.ProjectionFilter) ([]models.ProjectionSummary, error) {
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{scope.OrganizationID, strings.ToUpper(filter.ProjectionType),
		strings.ToUpper(filter.Freshness), filter.SubjectID, filter.ReconciliationOnly, scope.PageLimit()}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	rows, err := r.db.Query(ctx, `SELECT `+projectionSummaryColumns+`
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND ($2='' OR projection_type=$2)
		  AND ($3='' OR freshness_state=$3) AND ($4='' OR subject_id=$4)
		  AND ($5=false OR reconciliation_status<>'IDLE') AND `+clause+`
		ORDER BY last_synced_at DESC, id DESC LIMIT $6`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ProjectionSummary, 0)
	for rows.Next() {
		value, scanErr := scanProjectionSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// GetProjection reads one projection inside the scope.
func (r *Repository) GetProjection(ctx context.Context, scope models.Scope, id string) (models.ProjectionSummary, error) {
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{id, scope.OrganizationID}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	return scanProjectionSummary(r.db.QueryRow(ctx, `SELECT `+projectionSummaryColumns+`
		FROM fleet_partner_finance_projections
		WHERE id=$1 AND organization_id=$2 AND `+clause, args...))
}

// RequestReadBack schedules an authoritative read-back of an idle or
// dead-lettered projection and reports whether it did.
func (r *Repository) RequestReadBack(ctx context.Context, q database.Querier, organizationID, id string) (bool, error) {
	tag, err := r.querier(q).Exec(ctx, `UPDATE fleet_partner_finance_projections
				SET freshness_state='UNKNOWN',reconciliation_status='PENDING',
					reconciliation_attempts=0,reconciliation_next_attempt_at=now(),
					reconciliation_safe_error_code='',version=version+1,updated_at=now()
				WHERE id=$1 AND organization_id=$2 AND reconciliation_status IN ('IDLE','DEAD_LETTER')`,
		id, organizationID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
