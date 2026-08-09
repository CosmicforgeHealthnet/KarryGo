package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const withdrawalColumns = `id, organization_id, COALESCE(branch_id, ''), provider_membership_id,
	provider_id, provider_subwallet_id, amount_minor, currency, destination_reference, policy_mode,
	requested_by_actor_type, requested_by, COALESCE(approval_request_id, ''), decided_by, decided_at,
	decision_reason_code, COALESCE(finance_operation_id, ''), wallet_operation_id,
	wallet_withdrawal_reference, idempotency_key, request_fingerprint, expected_version, status,
	source_version, last_event_id, freshness_state, reconciliation_state, safe_error_code,
	correlation_id, causation_id, version, created_at, updated_at, completed_at`

func scanWithdrawal(row pgx.Row) (models.WithdrawalRequest, error) {
	var w models.WithdrawalRequest
	err := row.Scan(&w.ID, &w.OrganizationID, &w.BranchID, &w.ProviderMembershipID, &w.ProviderID,
		&w.ProviderSubWalletID, &w.AmountMinor, &w.Currency, &w.DestinationReference, &w.PolicyMode,
		&w.RequestedByActorType, &w.RequestedBy, &w.ApprovalRequestID, &w.DecidedBy, &w.DecidedAt,
		&w.DecisionReasonCode, &w.FinanceOperationID, &w.WalletOperationID,
		&w.WalletWithdrawalReference, &w.IdempotencyKey, &w.RequestFingerprint, &w.ExpectedVersion,
		&w.Status, &w.SourceVersion, &w.LastEventID, &w.FreshnessState, &w.ReconciliationState,
		&w.SafeErrorCode, &w.CorrelationID, &w.CausationID, &w.Version, &w.CreatedAt,
		&w.UpdatedAt, &w.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// CreateWithdrawal inserts a withdrawal request, returning the existing one
// when the idempotency key was already used with identical inputs.
func (r *Repository) CreateWithdrawal(ctx context.Context, q database.Querier,
	request *models.WithdrawalRequest) (bool, error) {
	querier := r.q(q)
	existing, err := scanWithdrawal(querier.QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM fleet_partner_provider_withdrawal_requests
		WHERE organization_id = $1 AND idempotency_key = $2 FOR UPDATE`,
		request.OrganizationID, request.IdempotencyKey))
	if err == nil {
		if existing.RequestFingerprint != request.RequestFingerprint {
			return false, ErrFingerprint
		}
		*request = existing
		return false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if request.ID == "" {
		request.ID = ids.New(PrefixWithdrawal)
	}
	err = querier.QueryRow(ctx, `
		INSERT INTO fleet_partner_provider_withdrawal_requests
			(id, organization_id, branch_id, provider_membership_id, provider_id,
			 provider_subwallet_id, amount_minor, currency, destination_reference, policy_mode,
			 requested_by_actor_type, requested_by, approval_request_id, idempotency_key,
			 request_fingerprint, expected_version, status, correlation_id, causation_id)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14,$15,$16,$17,$18,$19)
		RETURNING version, created_at, updated_at`,
		request.ID, request.OrganizationID, request.BranchID, request.ProviderMembershipID,
		request.ProviderID, request.ProviderSubWalletID, request.AmountMinor, request.Currency,
		request.DestinationReference, strings.ToUpper(request.PolicyMode),
		request.RequestedByActorType, request.RequestedBy, request.ApprovalRequestID,
		request.IdempotencyKey, request.RequestFingerprint, request.ExpectedVersion,
		request.Status, request.CorrelationID, request.CausationID).
		Scan(&request.Version, &request.CreatedAt, &request.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			// The open-withdrawal partial unique index refused a second live
			// request for this membership. That is the intended outcome, not a
			// race to retry.
			return false, ErrDuplicate
		}
		return false, fmt.Errorf("companywallet: insert withdrawal: %w", err)
	}
	return true, nil
}

// GetWithdrawal reads one request inside the tenant and branch scope.
func (r *Repository) GetWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string, branchIDs []string) (models.WithdrawalRequest, error) {
	clause, args := branchClause(branchIDs, "branch_id", []any{id, organizationID})
	return scanWithdrawal(r.q(q).QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM fleet_partner_provider_withdrawal_requests
		WHERE id = $1 AND organization_id = $2 AND `+clause, args...))
}

// LockWithdrawal reads one request FOR UPDATE.
func (r *Repository) LockWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string) (models.WithdrawalRequest, error) {
	return scanWithdrawal(r.q(q).QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM fleet_partner_provider_withdrawal_requests
		WHERE id = $1 AND organization_id = $2 FOR UPDATE`, id, organizationID))
}

// DecideWithdrawal records an approve or reject decision.
//
// The status guard is what makes a second concurrent decision fail rather than
// overwrite the first: two approvers pressing the button at once produce one
// decision and one conflict, never two decisions.
func (r *Repository) DecideWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string, expectedVersion int64, status, decidedBy, reasonCode,
	approvalRequestID string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_withdrawal_requests
		SET status = $4, decided_by = $5, decided_at = now(), decision_reason_code = $6,
		    approval_request_id = COALESCE(NULLIF($7, ''), approval_request_id),
		    version = version + 1, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND version = $3
		  AND status IN ('REQUESTED', 'PENDING_APPROVAL')`,
		id, organizationID, expectedVersion, status, decidedBy, reasonCode, approvalRequestID)
	if err != nil {
		return fmt.Errorf("companywallet: decide withdrawal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// SetWithdrawalStatus advances a request under its expected version.
func (r *Repository) SetWithdrawalStatus(ctx context.Context, q database.Querier,
	organizationID, id string, expectedVersion int64, status, walletOperationID,
	walletReference, safeErrorCode string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_withdrawal_requests
		SET status = $4,
		    wallet_operation_id = COALESCE(NULLIF($5, ''), wallet_operation_id),
		    wallet_withdrawal_reference = COALESCE(NULLIF($6, ''), wallet_withdrawal_reference),
		    safe_error_code = $7,
		    completed_at = CASE WHEN $4 = 'COMPLETED' THEN now() ELSE completed_at END,
		    freshness_state = CASE WHEN $4 = 'COMPLETED' THEN 'CURRENT' ELSE freshness_state END,
		    version = version + 1, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND version = $3
		  AND status NOT IN ('COMPLETED', 'FAILED_PERMANENT', 'REJECTED', 'CANCELLED')`,
		id, organizationID, expectedVersion, status, walletOperationID, walletReference,
		safeErrorCode)
	if err != nil {
		return fmt.Errorf("companywallet: set withdrawal status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// AppendWithdrawalHistory writes one append-only transition record.
func (r *Repository) AppendWithdrawalHistory(ctx context.Context, q database.Querier,
	organizationID, withdrawalID, previous, next, reasonCode, actorType, actorID,
	correlationID string) error {
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_provider_withdrawal_history
			(id, organization_id, withdrawal_id, previous_status, new_status, safe_reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		ids.New(PrefixHistory), organizationID, withdrawalID, previous, next, reasonCode,
		actorType, actorID, correlationID)
	return err
}

// WithdrawalFilter narrows a withdrawal listing to allowlisted fields.
type WithdrawalFilter struct {
	MembershipID string
	Status       string
	Cursor       cursor.Cursor
	HasCursor    bool
	Limit        int
}

// ListWithdrawals returns a tenant-scoped page, newest first.
func (r *Repository) ListWithdrawals(ctx context.Context, organizationID string,
	branchIDs []string, filter WithdrawalFilter) ([]models.WithdrawalRequest, error) {
	args := []any{organizationID, filter.MembershipID, strings.ToUpper(filter.Status)}
	where := `organization_id = $1 AND ($2 = '' OR provider_membership_id = $2)
		AND ($3 = '' OR status = $3)`
	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))
	rows, err := r.db.Query(ctx, `SELECT `+withdrawalColumns+`
		FROM fleet_partner_provider_withdrawal_requests
		WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("companywallet: list withdrawals: %w", err)
	}
	defer rows.Close()
	out := make([]models.WithdrawalRequest, 0)
	for rows.Next() {
		value, scanErr := scanWithdrawal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ClaimWithdrawals leases due requests for the execution worker.
func (r *Repository) ClaimWithdrawals(ctx context.Context, limit int) ([]models.WithdrawalRequest, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+withdrawalColumns+`
		FROM fleet_partner_provider_withdrawal_requests
		WHERE status IN ('APPROVED', 'SUBMISSION_PENDING', 'UNKNOWN', 'RECONCILING', 'FAILED_RETRYABLE')
		ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("companywallet: claim withdrawals: %w", err)
	}
	defer rows.Close()
	out := make([]models.WithdrawalRequest, 0)
	for rows.Next() {
		value, scanErr := scanWithdrawal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ReplayWithdrawal returns a dead-lettered request to the execution queue.
func (r *Repository) ReplayWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_withdrawal_requests
		SET status = 'APPROVED', safe_error_code = '', reconciliation_state = 'IDLE',
		    version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'DEAD_LETTER'`, organizationID, id)
	if err != nil {
		return fmt.Errorf("companywallet: replay withdrawal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Transaction projections
// ---------------------------------------------------------------------------

const transactionColumns = `id, organization_id, COALESCE(branch_id, ''), wallet_reference,
	transaction_reference, subject_type, subject_id, direction, category, amount_minor, currency,
	safe_status, occurred_at, source_version, last_event_id, freshness_state, last_synced_at, version`

func scanTransaction(row pgx.Row) (models.TransactionProjection, error) {
	var t models.TransactionProjection
	err := row.Scan(&t.ID, &t.OrganizationID, &t.BranchID, &t.WalletReference,
		&t.TransactionReference, &t.SubjectType, &t.SubjectID, &t.Direction, &t.Category,
		&t.AmountMinor, &t.Currency, &t.SafeStatus, &t.OccurredAt, &t.SourceVersion,
		&t.LastEventID, &t.FreshnessState, &t.LastSyncedAt, &t.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// UpsertTransaction records a Wallet-reported transaction for display.
func (r *Repository) UpsertTransaction(ctx context.Context, q database.Querier,
	projection models.TransactionProjection) error {
	if projection.ID == "" {
		projection.ID = ids.New(PrefixProjection)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_wallet_transaction_projections
			(id, organization_id, branch_id, wallet_reference, transaction_reference, subject_type,
			 subject_id, direction, category, amount_minor, currency, safe_status, occurred_at,
			 source_version, last_event_id, freshness_state)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (organization_id, wallet_reference, transaction_reference) DO UPDATE SET
			subject_type = EXCLUDED.subject_type, subject_id = EXCLUDED.subject_id,
			direction = EXCLUDED.direction, category = EXCLUDED.category,
			amount_minor = EXCLUDED.amount_minor, currency = EXCLUDED.currency,
			safe_status = EXCLUDED.safe_status, occurred_at = EXCLUDED.occurred_at,
			source_version = GREATEST(fleet_partner_wallet_transaction_projections.source_version,
			                          EXCLUDED.source_version),
			last_event_id = EXCLUDED.last_event_id, freshness_state = EXCLUDED.freshness_state,
			last_synced_at = now(),
			version = fleet_partner_wallet_transaction_projections.version + 1, updated_at = now()
		WHERE fleet_partner_wallet_transaction_projections.source_version <= EXCLUDED.source_version`,
		projection.ID, projection.OrganizationID, projection.BranchID, projection.WalletReference,
		projection.TransactionReference, projection.SubjectType, projection.SubjectID,
		projection.Direction, projection.Category, projection.AmountMinor, projection.Currency,
		projection.SafeStatus, projection.OccurredAt, projection.SourceVersion,
		projection.LastEventID, projection.FreshnessState)
	if err != nil {
		return fmt.Errorf("companywallet: upsert transaction projection: %w", err)
	}
	return nil
}

// TransactionFilter narrows a transaction listing to allowlisted fields.
type TransactionFilter struct {
	WalletReference string
	Category        string
	Direction       string
	Cursor          cursor.Cursor
	HasCursor       bool
	Limit           int
}

// ListTransactions returns a tenant-scoped page, newest first.
func (r *Repository) ListTransactions(ctx context.Context, organizationID string,
	branchIDs []string, filter TransactionFilter) ([]models.TransactionProjection, error) {
	args := []any{organizationID, filter.WalletReference, strings.ToUpper(filter.Category),
		strings.ToUpper(filter.Direction)}
	where := `organization_id = $1 AND ($2 = '' OR wallet_reference = $2)
		AND ($3 = '' OR category = $3) AND ($4 = '' OR direction = $4)`
	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (occurred_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))
	rows, err := r.db.Query(ctx, `SELECT `+transactionColumns+`
		FROM fleet_partner_wallet_transaction_projections
		WHERE `+where+` AND `+clause+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("companywallet: list transactions: %w", err)
	}
	defer rows.Close()
	out := make([]models.TransactionProjection, 0)
	for rows.Next() {
		value, scanErr := scanTransaction(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// GetTransaction reads one projected transaction inside the tenant scope.
func (r *Repository) GetTransaction(ctx context.Context, organizationID, id string,
	branchIDs []string) (models.TransactionProjection, error) {
	clause, args := branchClause(branchIDs, "branch_id", []any{id, organizationID})
	return scanTransaction(r.db.QueryRow(ctx, `SELECT `+transactionColumns+`
		FROM fleet_partner_wallet_transaction_projections
		WHERE id = $1 AND organization_id = $2 AND `+clause, args...))
}
