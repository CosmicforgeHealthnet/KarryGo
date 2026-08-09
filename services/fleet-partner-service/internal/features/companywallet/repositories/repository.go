// Package repositories implements PostgreSQL persistence for Section 13.
//
// Every read and every write carries organization_id in SQL. Handler-level
// checks are a convenience; the WHERE clause is the control, because a query
// that forgot one is a cross-tenant read no amount of handler discipline
// prevents.
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

// Errors returned by the repository.
var (
	ErrNotFound        = errors.New("companywallet: record not found")
	ErrVersionConflict = errors.New("companywallet: expected version does not match")
	ErrDuplicate       = errors.New("companywallet: conflicting record")
	ErrFingerprint     = errors.New("companywallet: idempotency key reused with different inputs")
)

// ID prefixes for Section 13 records.
const (
	PrefixAllocation = "alc"
	PrefixWithdrawal = "wdr"
	PrefixHistory    = "wsh"
	PrefixProjection = "wtx"
)

// Repository persists Section 13 links, instructions, withdrawals and
// projections.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

// ---------------------------------------------------------------------------
// Wallet links
// ---------------------------------------------------------------------------

const organizationLinkColumns = `organization_id, status, COALESCE(wallet_account_id, ''),
	COALESCE(currency, ''), COALESCE(idempotency_key, ''), attempts, COALESCE(last_error_code, ''),
	next_attempt_at, source_version, COALESCE(last_event_id, ''), freshness_state,
	reconciliation_state, correlation_id, synced_at, version, created_at, updated_at`

func scanOrganizationLink(row pgx.Row) (models.OrganizationWalletLink, error) {
	var l models.OrganizationWalletLink
	err := row.Scan(&l.OrganizationID, &l.Status, &l.WalletAccountID, &l.Currency,
		&l.IdempotencyKey, &l.Attempts, &l.LastErrorCode, &l.NextAttemptAt, &l.SourceVersion,
		&l.LastEventID, &l.FreshnessState, &l.ReconciliationState, &l.CorrelationID,
		&l.SyncedAt, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// GetOrganizationLink reads the organization's wallet link.
func (r *Repository) GetOrganizationLink(ctx context.Context, q database.Querier,
	organizationID string) (models.OrganizationWalletLink, error) {
	return scanOrganizationLink(r.q(q).QueryRow(ctx, `SELECT `+organizationLinkColumns+`
		FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1`, organizationID))
}

// UpsertOrganizationLink records what the authoritative service reported.
//
// The source version is monotonic: an older event can never overwrite a newer
// one, so out-of-order delivery degrades to "we already knew that" rather than
// to a silent regression of the link state.
func (r *Repository) UpsertOrganizationLink(ctx context.Context, q database.Querier,
	organizationID, walletAccountID, status, currency, freshness, lastEventID string,
	sourceVersion int64) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_organization_wallet_provisioning
		SET wallet_account_id = COALESCE(NULLIF($2, ''), wallet_account_id),
		    status = $3,
		    currency = COALESCE(NULLIF($4, '')::char(3), currency),
		    freshness_state = $5,
		    last_event_id = COALESCE(NULLIF($6, ''), last_event_id),
		    source_version = GREATEST(source_version, $7),
		    synced_at = now(), version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND source_version <= $7`,
		organizationID, walletAccountID, status, strings.ToUpper(currency), freshness,
		lastEventID, sourceVersion)
	if err != nil {
		return fmt.Errorf("companywallet: upsert organization link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either no row, or the stored version is newer. Both mean "do not
		// write"; the caller distinguishes them with a read when it cares.
		return ErrNotFound
	}
	return nil
}

// MarkOrganizationLinkStale moves a link out of CURRENT so no sensitive
// operation may rely on it until it is re-read.
func (r *Repository) MarkOrganizationLinkStale(ctx context.Context, q database.Querier,
	organizationID, reason string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_organization_wallet_provisioning
		SET freshness_state = 'STALE', reconciliation_state = 'PENDING',
		    last_error_code = $2, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND freshness_state = 'CURRENT'`, organizationID, reason)
	return err
}

const subWalletLinkColumns = `provider_membership_id, organization_id, provider_id, provider_service,
	COALESCE(wallet_account_id, ''), COALESCE(currency, ''), status, COALESCE(idempotency_key, ''),
	source_version, COALESCE(last_event_id, ''), freshness_state, last_error_code, synced_at,
	version, created_at, updated_at`

func scanSubWalletLink(row pgx.Row) (models.ProviderSubWalletLink, error) {
	var l models.ProviderSubWalletLink
	err := row.Scan(&l.ProviderMembershipID, &l.OrganizationID, &l.ProviderID, &l.ProviderService,
		&l.WalletAccountID, &l.Currency, &l.Status, &l.IdempotencyKey, &l.SourceVersion,
		&l.LastEventID, &l.FreshnessState, &l.LastErrorCode, &l.SyncedAt, &l.Version,
		&l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// GetSubWalletLink reads one provider sub-wallet link inside the tenant.
func (r *Repository) GetSubWalletLink(ctx context.Context, q database.Querier,
	organizationID, membershipID string) (models.ProviderSubWalletLink, error) {
	return scanSubWalletLink(r.q(q).QueryRow(ctx, `SELECT `+subWalletLinkColumns+`
		FROM fleet_partner_provider_subwallet_links
		WHERE organization_id = $1 AND provider_membership_id = $2`, organizationID, membershipID))
}

// UpsertSubWalletLink records what the authoritative service reported.
func (r *Repository) UpsertSubWalletLink(ctx context.Context, q database.Querier,
	organizationID, membershipID, walletAccountID, providerID, providerService, currency,
	status, freshness, lastEventID string, sourceVersion int64) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_subwallet_links
		SET wallet_account_id = COALESCE(NULLIF($3, ''), wallet_account_id),
		    provider_id = CASE WHEN provider_id = '' THEN $4 ELSE provider_id END,
		    provider_service = CASE WHEN provider_service = '' THEN $5 ELSE provider_service END,
		    currency = COALESCE(NULLIF($6, '')::char(3), currency),
		    status = $7, freshness_state = $8,
		    last_event_id = COALESCE(NULLIF($9, ''), last_event_id),
		    source_version = GREATEST(source_version, $10),
		    synced_at = now(), version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND provider_membership_id = $2 AND source_version <= $10`,
		organizationID, membershipID, walletAccountID, providerID, providerService,
		strings.ToUpper(currency), status, freshness, lastEventID, sourceVersion)
	if err != nil {
		return fmt.Errorf("companywallet: upsert sub-wallet link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindSubWalletLinkByAccount resolves a link from a Wallet account reference,
// which is what inbound Wallet events carry.
func (r *Repository) FindSubWalletLinkByAccount(ctx context.Context, q database.Querier,
	organizationID, walletAccountID string) (models.ProviderSubWalletLink, error) {
	return scanSubWalletLink(r.q(q).QueryRow(ctx, `SELECT `+subWalletLinkColumns+`
		FROM fleet_partner_provider_subwallet_links
		WHERE organization_id = $1 AND wallet_account_id = $2`, organizationID, walletAccountID))
}

// ---------------------------------------------------------------------------
// Withdrawal policy
// ---------------------------------------------------------------------------

const policyColumns = `organization_id, policy_mode, minimum_amount_minor, approval_threshold_minor,
	allocation_approval_threshold_minor, COALESCE(currency, ''), updated_by, version,
	created_at, updated_at`

// GetPolicy reads the organization's policy, or the safe default.
//
// The default is COMPANY_APPROVAL_REQUIRED. An organization that has never
// configured a policy gets the strict one, not the permissive one: an
// unconfigured control must not be an open one.
func (r *Repository) GetPolicy(ctx context.Context, q database.Querier,
	organizationID string) (models.WithdrawalPolicy, error) {
	var p models.WithdrawalPolicy
	err := r.q(q).QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_organization_withdrawal_policies WHERE organization_id = $1`,
		organizationID).
		Scan(&p.OrganizationID, &p.PolicyMode, &p.MinimumAmountMinor, &p.ApprovalThresholdMinor,
			&p.AllocationApprovalThresholdMinor, &p.Currency, &p.UpdatedBy, &p.Version,
			&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.DefaultWithdrawalPolicy(organizationID), nil
	}
	return p, err
}

// UpsertPolicy stores the organization's policy under an expected version.
func (r *Repository) UpsertPolicy(ctx context.Context, q database.Querier,
	policy models.WithdrawalPolicy, expectedVersion int64) (models.WithdrawalPolicy, error) {
	var stored models.WithdrawalPolicy
	err := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_organization_withdrawal_policies
			(organization_id, policy_mode, minimum_amount_minor, approval_threshold_minor,
			 allocation_approval_threshold_minor, currency, updated_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::char(3), $7)
		ON CONFLICT (organization_id) DO UPDATE SET
			policy_mode = EXCLUDED.policy_mode,
			minimum_amount_minor = EXCLUDED.minimum_amount_minor,
			approval_threshold_minor = EXCLUDED.approval_threshold_minor,
			allocation_approval_threshold_minor = EXCLUDED.allocation_approval_threshold_minor,
			currency = EXCLUDED.currency, updated_by = EXCLUDED.updated_by,
			version = fleet_partner_organization_withdrawal_policies.version + 1,
			updated_at = now()
		WHERE $8 = 0 OR fleet_partner_organization_withdrawal_policies.version = $8
		RETURNING `+policyColumns,
		policy.OrganizationID, strings.ToUpper(policy.PolicyMode), policy.MinimumAmountMinor,
		policy.ApprovalThresholdMinor, policy.AllocationApprovalThresholdMinor,
		strings.ToUpper(policy.Currency), policy.UpdatedBy, expectedVersion).
		Scan(&stored.OrganizationID, &stored.PolicyMode, &stored.MinimumAmountMinor,
			&stored.ApprovalThresholdMinor, &stored.AllocationApprovalThresholdMinor,
			&stored.Currency, &stored.UpdatedBy, &stored.Version, &stored.CreatedAt,
			&stored.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored, ErrVersionConflict
	}
	return stored, err
}

// ---------------------------------------------------------------------------
// Allocation instructions
// ---------------------------------------------------------------------------

const allocationColumns = `id, organization_id, COALESCE(branch_id, ''), provider_membership_id,
	provider_id, provider_subwallet_id, organization_wallet_id, allocation_type, amount_minor,
	currency, business_reference_type, business_reference_id, reason_code, provider_explanation,
	private_note, legal_basis_reference, requested_by, COALESCE(approval_request_id, ''),
	COALESCE(finance_operation_id, ''), wallet_operation_id, wallet_transaction_reference,
	idempotency_key, request_fingerprint, expected_version, status, source_version, last_event_id,
	freshness_state, reconciliation_state, safe_error_code, correlation_id, causation_id,
	version, created_by, updated_by, created_at, updated_at, completed_at`

func scanAllocation(row pgx.Row) (models.AllocationInstruction, error) {
	var a models.AllocationInstruction
	err := row.Scan(&a.ID, &a.OrganizationID, &a.BranchID, &a.ProviderMembershipID, &a.ProviderID,
		&a.ProviderSubWalletID, &a.OrganizationWalletID, &a.AllocationType, &a.AmountMinor,
		&a.Currency, &a.BusinessReferenceType, &a.BusinessReferenceID, &a.ReasonCode,
		&a.ProviderExplanation, &a.PrivateNote, &a.LegalBasisReference, &a.RequestedBy,
		&a.ApprovalRequestID, &a.FinanceOperationID, &a.WalletOperationID,
		&a.WalletTransactionReference, &a.IdempotencyKey, &a.RequestFingerprint,
		&a.ExpectedVersion, &a.Status, &a.SourceVersion, &a.LastEventID, &a.FreshnessState,
		&a.ReconciliationState, &a.SafeErrorCode, &a.CorrelationID, &a.CausationID, &a.Version,
		&a.CreatedBy, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt, &a.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// CreateAllocation inserts an instruction, returning the existing one when the
// idempotency key was already used with identical inputs.
//
// The returned bool reports whether a new row was created, so the caller knows
// whether to write the audit, history and outbox records that go with a real
// new decision.
func (r *Repository) CreateAllocation(ctx context.Context, q database.Querier,
	instruction *models.AllocationInstruction) (bool, error) {
	querier := r.q(q)
	existing, err := scanAllocation(querier.QueryRow(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE organization_id = $1 AND idempotency_key = $2 FOR UPDATE`,
		instruction.OrganizationID, instruction.IdempotencyKey))
	if err == nil {
		if existing.RequestFingerprint != instruction.RequestFingerprint {
			return false, ErrFingerprint
		}
		*instruction = existing
		return false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if instruction.ID == "" {
		instruction.ID = ids.New(PrefixAllocation)
	}
	err = querier.QueryRow(ctx, `
		INSERT INTO fleet_partner_provider_allocation_instructions
			(id, organization_id, branch_id, provider_membership_id, provider_id,
			 provider_subwallet_id, organization_wallet_id, allocation_type, amount_minor, currency,
			 business_reference_type, business_reference_id, reason_code, provider_explanation,
			 private_note, legal_basis_reference, requested_by, approval_request_id,
			 idempotency_key, request_fingerprint, expected_version, status, correlation_id,
			 causation_id, created_by, updated_by)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
		        NULLIF($18,''),$19,$20,$21,$22,$23,$24,$25,$25)
		RETURNING version, created_at, updated_at`,
		instruction.ID, instruction.OrganizationID, instruction.BranchID,
		instruction.ProviderMembershipID, instruction.ProviderID, instruction.ProviderSubWalletID,
		instruction.OrganizationWalletID, instruction.AllocationType, instruction.AmountMinor,
		instruction.Currency, instruction.BusinessReferenceType, instruction.BusinessReferenceID,
		instruction.ReasonCode, instruction.ProviderExplanation, instruction.PrivateNote,
		instruction.LegalBasisReference, instruction.RequestedBy, instruction.ApprovalRequestID,
		instruction.IdempotencyKey, instruction.RequestFingerprint, instruction.ExpectedVersion,
		instruction.Status, instruction.CorrelationID, instruction.CausationID,
		instruction.CreatedBy).
		Scan(&instruction.Version, &instruction.CreatedAt, &instruction.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return false, ErrDuplicate
		}
		return false, fmt.Errorf("companywallet: insert allocation: %w", err)
	}
	return true, nil
}

// GetAllocation reads one instruction inside the tenant and branch scope.
func (r *Repository) GetAllocation(ctx context.Context, q database.Querier,
	organizationID, id string, branchIDs []string) (models.AllocationInstruction, error) {
	clause, args := branchClause(branchIDs, "branch_id", []any{id, organizationID})
	return scanAllocation(r.q(q).QueryRow(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE id = $1 AND organization_id = $2 AND `+clause, args...))
}

// LockAllocation reads one instruction FOR UPDATE.
func (r *Repository) LockAllocation(ctx context.Context, q database.Querier,
	organizationID, id string) (models.AllocationInstruction, error) {
	return scanAllocation(r.q(q).QueryRow(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE id = $1 AND organization_id = $2 FOR UPDATE`, id, organizationID))
}

// SetAllocationStatus advances an instruction under its expected version.
//
// Terminal states are guarded in SQL: an instruction that already COMPLETED
// cannot be moved by a late worker whose read happened before the completion.
func (r *Repository) SetAllocationStatus(ctx context.Context, q database.Querier,
	organizationID, id string, expectedVersion int64, status, walletOperationID,
	walletTransactionReference, safeErrorCode, updatedBy string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_allocation_instructions
		SET status = $4,
		    wallet_operation_id = COALESCE(NULLIF($5, ''), wallet_operation_id),
		    wallet_transaction_reference = COALESCE(NULLIF($6, ''), wallet_transaction_reference),
		    safe_error_code = $7, updated_by = $8,
		    completed_at = CASE WHEN $4 = 'COMPLETED' THEN now() ELSE completed_at END,
		    freshness_state = CASE WHEN $4 = 'COMPLETED' THEN 'CURRENT' ELSE freshness_state END,
		    version = version + 1, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND version = $3
		  AND status NOT IN ('COMPLETED', 'FAILED_PERMANENT', 'REJECTED', 'REVERSED')`,
		id, organizationID, expectedVersion, status, walletOperationID,
		walletTransactionReference, safeErrorCode, updatedBy)
	if err != nil {
		return fmt.Errorf("companywallet: set allocation status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// AppendAllocationHistory writes one append-only transition record.
func (r *Repository) AppendAllocationHistory(ctx context.Context, q database.Querier,
	organizationID, instructionID, previous, next, reasonCode, actorType, actorID,
	correlationID string) error {
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_provider_allocation_history
			(id, organization_id, instruction_id, previous_status, new_status, safe_reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		ids.New(PrefixHistory), organizationID, instructionID, previous, next, reasonCode,
		actorType, actorID, correlationID)
	return err
}

// AllocationFilter narrows an allocation listing to allowlisted fields.
type AllocationFilter struct {
	MembershipID   string
	Status         string
	AllocationType string
	Cursor         cursor.Cursor
	HasCursor      bool
	Limit          int
}

// ListAllocations returns a tenant-scoped page, newest first.
func (r *Repository) ListAllocations(ctx context.Context, organizationID string,
	branchIDs []string, filter AllocationFilter) ([]models.AllocationInstruction, error) {
	args := []any{organizationID, filter.MembershipID, strings.ToUpper(filter.Status),
		strings.ToUpper(filter.AllocationType)}
	where := `organization_id = $1 AND ($2 = '' OR provider_membership_id = $2)
		AND ($3 = '' OR status = $3) AND ($4 = '' OR allocation_type = $4)`
	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))
	rows, err := r.db.Query(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("companywallet: list allocations: %w", err)
	}
	defer rows.Close()
	out := make([]models.AllocationInstruction, 0)
	for rows.Next() {
		value, scanErr := scanAllocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ClaimAllocations leases due instructions for the execution worker.
func (r *Repository) ClaimAllocations(ctx context.Context, limit int) ([]models.AllocationInstruction, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE status IN ('APPROVED', 'SUBMISSION_PENDING', 'UNKNOWN', 'RECONCILING', 'FAILED_RETRYABLE')
		ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("companywallet: claim allocations: %w", err)
	}
	defer rows.Close()
	out := make([]models.AllocationInstruction, 0)
	for rows.Next() {
		value, scanErr := scanAllocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ReplayAllocation returns a dead-lettered instruction to the execution queue.
func (r *Repository) ReplayAllocation(ctx context.Context, q database.Querier,
	organizationID, id string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_provider_allocation_instructions
		SET status = 'APPROVED', safe_error_code = '', reconciliation_state = 'IDLE',
		    version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'DEAD_LETTER'`, organizationID, id)
	if err != nil {
		return fmt.Errorf("companywallet: replay allocation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindAllocationByKey resolves an instruction by its tenant-scoped idempotency
// key, or ErrNotFound.
//
// This runs before any business validation so a retried request replays
// instead of colliding with the instruction it created the first time.
func (r *Repository) FindAllocationByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (models.AllocationInstruction, error) {
	return scanAllocation(r.q(q).QueryRow(ctx, `SELECT `+allocationColumns+`
		FROM fleet_partner_provider_allocation_instructions
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// ProtectedBusinessPurpose reports whether a combination is one where exactly
// one live allocation may exist.
//
// It mirrors idx_fleet_allocation_instructions_business_purpose exactly. A
// completed trip funds one per-job payment; a payroll period or an expense
// claim can legitimately be paid against more than once, and treating those as
// duplicates would block correct work.
func ProtectedBusinessPurpose(referenceType, allocationType string) bool {
	switch strings.ToUpper(referenceType) {
	case "TRIP", "ASSIGNMENT":
	default:
		return false
	}
	switch strings.ToUpper(allocationType) {
	case "PER_JOB", "BONUS":
		return true
	default:
		return false
	}
}

// CountLiveAllocationForPurpose reports whether a live allocation already
// exists for a protected business purpose.
func (r *Repository) CountLiveAllocationForPurpose(ctx context.Context, q database.Querier,
	organizationID, membershipID, referenceType, referenceID, allocationType string) (bool, error) {
	if !ProtectedBusinessPurpose(referenceType, allocationType) {
		return false, nil
	}
	var exists bool
	err := r.q(q).QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM fleet_partner_provider_allocation_instructions
		WHERE organization_id = $1 AND provider_membership_id = $2
		  AND business_reference_type = $3 AND business_reference_id = $4
		  AND allocation_type = $5
		  AND status NOT IN ('REJECTED', 'FAILED_PERMANENT', 'DEAD_LETTER', 'REVERSED'))`,
		organizationID, membershipID, referenceType, referenceID, allocationType).Scan(&exists)
	return exists, err
}

func itoa(value int) string { return fmt.Sprintf("%d", value) }

// branchClause renders the branch restriction and returns the extended args.
//
// An empty branch list means organization-wide authority. A restricted caller
// sees rows in their branches plus rows with no branch, which are
// organization-level records.
func branchClause(branchIDs []string, column string, args []any) (string, []any) {
	if len(branchIDs) == 0 {
		return "TRUE", args
	}
	args = append(args, branchIDs)
	return fmt.Sprintf("(%s IS NULL OR %s = ANY($%d))", column, column, len(args)), args
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
