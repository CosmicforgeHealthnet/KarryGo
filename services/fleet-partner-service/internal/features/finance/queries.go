package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// OperationSummary is the operator-facing view of one durable Wallet
// operation.
//
// It carries the operation's own lifecycle and the opaque Wallet reference —
// never a balance, a ledger account or a journal line. An operator who needs
// to know whether a payout ran does not need to see the books to find out.
type OperationSummary struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	BranchID        string     `json:"branchId,omitempty"`
	SubjectType     string     `json:"subjectType"`
	SubjectID       string     `json:"subjectId"`
	OperationType   string     `json:"operationType"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	NextAttemptAt   time.Time  `json:"nextAttemptAt"`
	WalletReference string     `json:"walletReference,omitempty"`
	SafeErrorCode   string     `json:"safeErrorCode,omitempty"`
	RequestedByType string     `json:"requestedByActorType,omitempty"`
	RequestedByID   string     `json:"requestedByActorId,omitempty"`
	CorrelationID   string     `json:"correlationId,omitempty"`
	ExpectedVersion int64      `json:"expectedVersion"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
}

// ProjectionSummary is the operator-facing view of one finance projection.
type ProjectionSummary struct {
	ID                     string    `json:"id"`
	OrganizationID         string    `json:"organizationId"`
	BranchID               string    `json:"branchId,omitempty"`
	ProjectionType         string    `json:"projectionType"`
	SubjectType            string    `json:"subjectType"`
	SubjectID              string    `json:"subjectId"`
	WalletReference        string    `json:"walletReference"`
	AmountMinor            *int64    `json:"amountMinor,omitempty"`
	Currency               string    `json:"currency,omitempty"`
	SafeStatus             string    `json:"safeStatus"`
	SourceVersion          int64     `json:"sourceVersion"`
	FreshnessState         string    `json:"freshnessState"`
	ReconciliationStatus   string    `json:"reconciliationStatus"`
	ReconciliationAttempts int       `json:"reconciliationAttempts"`
	SafeErrorCode          string    `json:"safeErrorCode,omitempty"`
	LastSyncedAt           time.Time `json:"lastSyncedAt"`
	Version                int64     `json:"version"`
}

// Authoritative reports whether the projection may authorize money movement.
// Only a fresh, fully reconciled projection may.
func (p ProjectionSummary) Authoritative() bool {
	return p.FreshnessState == "CURRENT" && p.ReconciliationStatus == "IDLE"
}

// HistoryEntry is one recorded operation transition.
type HistoryEntry struct {
	ID             string    `json:"id"`
	PreviousStatus string    `json:"previousStatus"`
	NewStatus      string    `json:"newStatus"`
	SafeReasonCode string    `json:"safeReasonCode,omitempty"`
	ActorType      string    `json:"actorType"`
	ActorID        string    `json:"actorId"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// AuditEntry is one recorded operator action.
type AuditEntry struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	ActorType    string    `json:"actorType"`
	ActorID      string    `json:"actorId"`
	OccurredAt   time.Time `json:"occurredAt"`
}

// Scope is the tenant and branch restriction applied to every operator read.
//
// BranchIDs empty means organization-wide authority. A caller restricted to
// branches sees rows in those branches plus rows with no branch at all, which
// are organization-level operations.
type Scope struct {
	OrganizationID string
	BranchIDs      []string
	Limit          int
}

func (s Scope) limit() int {
	if s.Limit <= 0 || s.Limit > 200 {
		return 50
	}
	return s.Limit
}

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

func scanOperationSummary(row pgx.Row) (OperationSummary, error) {
	var value OperationSummary
	err := row.Scan(&value.ID, &value.OrganizationID, &value.BranchID, &value.SubjectType,
		&value.SubjectID, &value.OperationType, &value.Status, &value.Attempts,
		&value.NextAttemptAt, &value.WalletReference, &value.SafeErrorCode,
		&value.RequestedByType, &value.RequestedByID, &value.CorrelationID,
		&value.ExpectedVersion, &value.Version, &value.CreatedAt, &value.UpdatedAt,
		&value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, ErrNotFound
	}
	return value, err
}

// OperationFilter narrows an operation listing.
type OperationFilter struct {
	Status        string
	OperationType string
	SubjectID     string
}

// ListOperations returns the tenant's durable Wallet operations.
func (s *Service) ListOperations(ctx context.Context, scope Scope,
	filter OperationFilter) ([]OperationSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" {
		return nil, ErrNotFound
	}
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{scope.OrganizationID, strings.ToUpper(filter.Status),
		strings.ToUpper(filter.OperationType), filter.SubjectID, scope.limit()}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	rows, err := s.pool.Query(ctx, `SELECT `+operationSummaryColumns+`
		FROM fleet_partner_finance_operations
		WHERE organization_id=$1 AND ($2='' OR status=$2) AND ($3='' OR operation_type=$3)
		  AND ($4='' OR subject_id=$4) AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OperationSummary, 0)
	for rows.Next() {
		value, scanErr := scanOperationSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// GetOperation reads one operation. Ownership is part of the query, so an
// identifier belonging to another tenant is indistinguishable from one that
// does not exist.
func (s *Service) GetOperation(ctx context.Context, scope Scope, id string) (OperationSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" || strings.TrimSpace(id) == "" {
		return OperationSummary{}, ErrNotFound
	}
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{id, scope.OrganizationID}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	return scanOperationSummary(s.pool.QueryRow(ctx, `SELECT `+operationSummaryColumns+`
		FROM fleet_partner_finance_operations
		WHERE id=$1 AND organization_id=$2 AND `+clause, args...))
}

// OperationHistory returns the append-only transition history for one
// operation the caller is entitled to see.
func (s *Service) OperationHistory(ctx context.Context, scope Scope, id string) ([]HistoryEntry, error) {
	if _, err := s.GetOperation(ctx, scope, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,previous_status,new_status,safe_reason_code,
		actor_type,actor_id,occurred_at FROM fleet_partner_finance_operation_history
		WHERE operation_id=$1 AND organization_id=$2 ORDER BY occurred_at, id`, id, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]HistoryEntry, 0)
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.ID, &entry.PreviousStatus, &entry.NewStatus,
			&entry.SafeReasonCode, &entry.ActorType, &entry.ActorID, &entry.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// OperationAudit returns the audit trail recorded against one operation.
func (s *Service) OperationAudit(ctx context.Context, scope Scope, id string) ([]AuditEntry, error) {
	if _, err := s.GetOperation(ctx, scope, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,action,resource_type,resource_id,actor_type,
		actor_id,occurred_at FROM fleet_partner_audit_records
		WHERE organization_id=$1 AND resource_type='finance_operation' AND resource_id=$2
		ORDER BY occurred_at, id`, scope.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AuditEntry, 0)
	for rows.Next() {
		var entry AuditEntry
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

func scanProjectionSummary(row pgx.Row) (ProjectionSummary, error) {
	var value ProjectionSummary
	err := row.Scan(&value.ID, &value.OrganizationID, &value.BranchID, &value.ProjectionType,
		&value.SubjectType, &value.SubjectID, &value.WalletReference, &value.AmountMinor,
		&value.Currency, &value.SafeStatus, &value.SourceVersion, &value.FreshnessState,
		&value.ReconciliationStatus, &value.ReconciliationAttempts, &value.SafeErrorCode,
		&value.LastSyncedAt, &value.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, ErrNotFound
	}
	return value, err
}

// ProjectionFilter narrows a projection listing.
type ProjectionFilter struct {
	ProjectionType string
	Freshness      string
	SubjectID      string
	// ReconciliationOnly restricts the listing to open reconciliation cases.
	ReconciliationOnly bool
}

// ListProjections returns the tenant's finance projections. With
// ReconciliationOnly set it returns exactly the open reconciliation cases.
func (s *Service) ListProjections(ctx context.Context, scope Scope,
	filter ProjectionFilter) ([]ProjectionSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" {
		return nil, ErrNotFound
	}
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{scope.OrganizationID, strings.ToUpper(filter.ProjectionType),
		strings.ToUpper(filter.Freshness), filter.SubjectID, filter.ReconciliationOnly, scope.limit()}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	rows, err := s.pool.Query(ctx, `SELECT `+projectionSummaryColumns+`
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND ($2='' OR projection_type=$2)
		  AND ($3='' OR freshness_state=$3) AND ($4='' OR subject_id=$4)
		  AND ($5=false OR reconciliation_status<>'IDLE') AND `+clause+`
		ORDER BY last_synced_at DESC, id DESC LIMIT $6`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectionSummary, 0)
	for rows.Next() {
		value, scanErr := scanProjectionSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// GetProjection reads one projection inside the caller's scope.
func (s *Service) GetProjection(ctx context.Context, scope Scope, id string) (ProjectionSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" || strings.TrimSpace(id) == "" {
		return ProjectionSummary{}, ErrNotFound
	}
	clause, branches := branchFilter(scope.BranchIDs, "branch_id")
	args := []any{id, scope.OrganizationID}
	if branches != nil {
		args = append(args, branches)
		clause = fmt.Sprintf(clause, len(args))
	}
	return scanProjectionSummary(s.pool.QueryRow(ctx, `SELECT `+projectionSummaryColumns+`
		FROM fleet_partner_finance_projections
		WHERE id=$1 AND organization_id=$2 AND `+clause, args...))
}

// RequestReadBack schedules an authoritative read-back of one projection.
//
// It never writes the projection's value: it only asks the reconciler to go
// and ask the authoritative service. Until that returns, the projection is not
// CURRENT, and so it authorizes nothing.
func (s *Service) RequestReadBack(ctx context.Context, scope Scope, id, actorType, actorID,
	correlationID string) error {
	projection, err := s.GetProjection(ctx, scope, id)
	if err != nil {
		return err
	}
	return s.withAudit(ctx, scope.OrganizationID, actorType, actorID,
		"finance.projection.read_back_requested", "finance_projection", id, correlationID,
		map[string]any{"projectionType": projection.ProjectionType,
			"previousFreshness": projection.FreshnessState},
		func(tx pgx.Tx) error {
			tag, execErr := tx.Exec(ctx, `UPDATE fleet_partner_finance_projections
				SET freshness_state='UNKNOWN',reconciliation_status='PENDING',
					reconciliation_attempts=0,reconciliation_next_attempt_at=now(),
					reconciliation_safe_error_code='',version=version+1,updated_at=now()
				WHERE id=$1 AND organization_id=$2 AND reconciliation_status IN ('IDLE','DEAD_LETTER')`,
				id, scope.OrganizationID)
			if execErr != nil {
				return execErr
			}
			if tag.RowsAffected() == 0 {
				// Already in flight: asking again would reset an in-progress
				// reconciliation and lose its attempt history.
				return ErrReadBackNotAvailable
			}
			return nil
		})
}

// ErrReadBackNotAvailable means a read-back is already in progress.
var ErrReadBackNotAvailable = errors.New("finance: authoritative read-back is already in progress")
