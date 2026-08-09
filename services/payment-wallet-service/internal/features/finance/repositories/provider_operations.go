package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// ProviderOperationsRepository persists company-authorized provider
// allocations and provider withdrawals.
//
// Both records point at journals rather than carrying amounts of their own
// authority: the journal is the ledger fact, and these rows are the operation
// history that explains why it exists.
type ProviderOperationsRepository struct{ db database.Querier }

// NewProviderOperationsRepository builds the repository.
func NewProviderOperationsRepository(db database.Querier) *ProviderOperationsRepository {
	return &ProviderOperationsRepository{db: db}
}

const providerAllocationColumns = `id, organization_id, organization_wallet_id, earning_account_id,
	provider_id, organization_membership_id, allocation_type, business_reference_type,
	business_reference_id, amount_minor, currency, reason_code, provider_explanation,
	journal_id, COALESCE(reverses_allocation_id, ''), status, source_service, source_reference,
	approval_reference, requested_by_actor_type, requested_by_actor_id, idempotency_key,
	request_fingerprint, correlation_id, causation_id, version, created_at, updated_at`

func scanProviderAllocation(row pgx.Row) (*models.ProviderAllocation, error) {
	var a models.ProviderAllocation
	err := row.Scan(&a.ID, &a.OrganizationID, &a.OrganizationWalletID, &a.EarningAccountID,
		&a.ProviderID, &a.OrganizationMembershipID, &a.AllocationType, &a.BusinessReferenceType,
		&a.BusinessReferenceID, &a.AmountMinor, &a.Currency, &a.ReasonCode, &a.ProviderExplanation,
		&a.JournalID, &a.ReversesAllocationID, &a.Status, &a.SourceService, &a.SourceReference,
		&a.ApprovalReference, &a.RequestedByActorType, &a.RequestedByActorID, &a.IdempotencyKey,
		&a.RequestFingerprint, &a.CorrelationID, &a.CausationID, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan provider allocation: %w", err)
	}
	return &a, nil
}

// CreateAllocation inserts one allocation record.
//
// A unique-violation is translated into ErrConflict rather than being retried:
// the caller has already posted a journal by the time it gets here, so the
// only safe response is to fail the whole transaction and let the read-back
// path decide what actually happened.
func (r *ProviderOperationsRepository) CreateAllocation(ctx context.Context, q database.Querier,
	allocation *models.ProviderAllocation) error {
	if q == nil {
		q = r.db
	}
	if allocation.ID == "" {
		allocation.ID = ids.New(ids.PrefixProviderAlloc)
	}
	if allocation.Status == "" {
		allocation.Status = "POSTED"
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_provider_allocations
			(id, organization_id, organization_wallet_id, earning_account_id, provider_id,
			 organization_membership_id, allocation_type, business_reference_type,
			 business_reference_id, amount_minor, currency, reason_code, provider_explanation,
			 journal_id, reverses_allocation_id, status, source_service, source_reference,
			 approval_reference, requested_by_actor_type, requested_by_actor_id,
			 idempotency_key, request_fingerprint, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,''),$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		RETURNING version, created_at, updated_at`,
		allocation.ID, allocation.OrganizationID, allocation.OrganizationWalletID,
		allocation.EarningAccountID, allocation.ProviderID, allocation.OrganizationMembershipID,
		allocation.AllocationType, allocation.BusinessReferenceType, allocation.BusinessReferenceID,
		allocation.AmountMinor, allocation.Currency, allocation.ReasonCode,
		allocation.ProviderExplanation, allocation.JournalID, allocation.ReversesAllocationID,
		allocation.Status, allocation.SourceService, allocation.SourceReference,
		allocation.ApprovalReference, allocation.RequestedByActorType, allocation.RequestedByActorID,
		allocation.IdempotencyKey, allocation.RequestFingerprint, allocation.CorrelationID,
		allocation.CausationID).
		Scan(&allocation.Version, &allocation.CreatedAt, &allocation.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: insert provider allocation: %w", err)
	}
	return nil
}

// FindAllocationByKey resolves an allocation by its tenant-scoped idempotency
// key.
func (r *ProviderOperationsRepository) FindAllocationByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (*models.ProviderAllocation, error) {
	if q == nil {
		q = r.db
	}
	return scanProviderAllocation(q.QueryRow(ctx, `SELECT `+providerAllocationColumns+`
		FROM finance_provider_allocations WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, key))
}

// GetAllocation reads one allocation inside the tenant's scope.
func (r *ProviderOperationsRepository) GetAllocation(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.ProviderAllocation, error) {
	if q == nil {
		q = r.db
	}
	return scanProviderAllocation(q.QueryRow(ctx, `SELECT `+providerAllocationColumns+`
		FROM finance_provider_allocations WHERE organization_id = $1 AND id = $2`, organizationID, id))
}

// MarkAllocationReversed flips a posted allocation to REVERSED under its
// expected version.
func (r *ProviderOperationsRepository) MarkAllocationReversed(ctx context.Context, q database.Querier,
	id string, expectedVersion int64) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_provider_allocations
		SET status = 'REVERSED', version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2 AND status = 'POSTED'`, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("finance: reverse provider allocation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// AllocationFilter narrows an allocation listing.
type AllocationFilter struct {
	EarningAccountID string
	AllocationType   string
	Status           string
	From, To         time.Time
	Limit            int
}

// ListAllocations returns tenant-scoped allocations, newest first.
func (r *ProviderOperationsRepository) ListAllocations(ctx context.Context, q database.Querier,
	organizationID string, filter AllocationFilter) ([]models.ProviderAllocation, error) {
	if q == nil {
		q = r.db
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	rows, err := q.Query(ctx, `SELECT `+providerAllocationColumns+`
		FROM finance_provider_allocations
		WHERE organization_id = $1
		  AND ($2 = '' OR earning_account_id = $2)
		  AND ($3 = '' OR allocation_type = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5::timestamptz IS NULL OR created_at >= $5)
		  AND ($6::timestamptz IS NULL OR created_at <= $6)
		ORDER BY created_at DESC, id DESC LIMIT $7`,
		organizationID, filter.EarningAccountID, strings.ToUpper(filter.AllocationType),
		strings.ToUpper(filter.Status), nullableTimestamp(filter.From), nullableTimestamp(filter.To), filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list provider allocations: %w", err)
	}
	defer rows.Close()
	out := make([]models.ProviderAllocation, 0)
	for rows.Next() {
		allocation, scanErr := scanProviderAllocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *allocation)
	}
	return out, rows.Err()
}

func nullableTimestamp(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

const withdrawalColumns = `id, organization_id, earning_account_id, organization_membership_id,
	provider_id, amount_minor, currency, destination_reference, policy_mode, approval_reference,
	status, COALESCE(hold_journal_id, ''), COALESCE(payout_journal_id, ''), external_reference,
	safe_error_code, attempts, next_attempt_at, COALESCE(lease_owner, ''), lease_expires_at,
	requested_by_actor_type, requested_by_actor_id, decided_by_actor_type, decided_by_actor_id,
	decided_at, idempotency_key, request_fingerprint, correlation_id, causation_id, version,
	created_at, updated_at, completed_at`

func scanWithdrawal(row pgx.Row) (*models.ProviderWithdrawal, error) {
	var w models.ProviderWithdrawal
	err := row.Scan(&w.ID, &w.OrganizationID, &w.EarningAccountID, &w.OrganizationMembershipID,
		&w.ProviderID, &w.AmountMinor, &w.Currency, &w.DestinationReference, &w.PolicyMode,
		&w.ApprovalReference, &w.Status, &w.HoldJournalID, &w.PayoutJournalID, &w.ExternalReference,
		&w.SafeErrorCode, &w.Attempts, &w.NextAttemptAt, &w.LeaseOwner, &w.LeaseExpiresAt,
		&w.RequestedByActorType, &w.RequestedByActorID, &w.DecidedByActorType, &w.DecidedByActorID,
		&w.DecidedAt, &w.IdempotencyKey, &w.RequestFingerprint, &w.CorrelationID, &w.CausationID,
		&w.Version, &w.CreatedAt, &w.UpdatedAt, &w.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan provider withdrawal: %w", err)
	}
	return &w, nil
}

// CreateWithdrawal inserts one withdrawal request.
func (r *ProviderOperationsRepository) CreateWithdrawal(ctx context.Context, q database.Querier,
	withdrawal *models.ProviderWithdrawal) error {
	if q == nil {
		q = r.db
	}
	if withdrawal.ID == "" {
		withdrawal.ID = ids.New(ids.PrefixProviderPayout)
	}
	if withdrawal.Status == "" {
		withdrawal.Status = models.WithdrawalRequested
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_provider_withdrawals
			(id, organization_id, earning_account_id, organization_membership_id, provider_id,
			 amount_minor, currency, destination_reference, policy_mode, approval_reference, status,
			 hold_journal_id, requested_by_actor_type, requested_by_actor_id, idempotency_key,
			 request_fingerprint, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13,$14,$15,$16,$17,$18)
		RETURNING version, next_attempt_at, created_at, updated_at`,
		withdrawal.ID, withdrawal.OrganizationID, withdrawal.EarningAccountID,
		withdrawal.OrganizationMembershipID, withdrawal.ProviderID, withdrawal.AmountMinor,
		withdrawal.Currency, withdrawal.DestinationReference, withdrawal.PolicyMode,
		withdrawal.ApprovalReference, withdrawal.Status, withdrawal.HoldJournalID,
		withdrawal.RequestedByActorType, withdrawal.RequestedByActorID, withdrawal.IdempotencyKey,
		withdrawal.RequestFingerprint, withdrawal.CorrelationID, withdrawal.CausationID).
		Scan(&withdrawal.Version, &withdrawal.NextAttemptAt, &withdrawal.CreatedAt, &withdrawal.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: insert provider withdrawal: %w", err)
	}
	return nil
}

// FindWithdrawalByKey resolves a withdrawal by its tenant-scoped idempotency
// key.
func (r *ProviderOperationsRepository) FindWithdrawalByKey(ctx context.Context, q database.Querier,
	organizationID, key string) (*models.ProviderWithdrawal, error) {
	if q == nil {
		q = r.db
	}
	return scanWithdrawal(q.QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM finance_provider_withdrawals WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, key))
}

// GetWithdrawal reads one withdrawal inside the tenant's scope.
func (r *ProviderOperationsRepository) GetWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.ProviderWithdrawal, error) {
	if q == nil {
		q = r.db
	}
	return scanWithdrawal(q.QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM finance_provider_withdrawals WHERE organization_id = $1 AND id = $2`, organizationID, id))
}

// LockWithdrawal reads one withdrawal FOR UPDATE inside the tenant's scope.
func (r *ProviderOperationsRepository) LockWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string) (*models.ProviderWithdrawal, error) {
	if q == nil {
		q = r.db
	}
	return scanWithdrawal(q.QueryRow(ctx, `SELECT `+withdrawalColumns+`
		FROM finance_provider_withdrawals WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
		organizationID, id))
}

// DecideWithdrawal records an approve or reject decision under the expected
// version. The status guard in the WHERE clause is what makes a second
// concurrent decision a no-op rather than an overwrite.
func (r *ProviderOperationsRepository) DecideWithdrawal(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status, actorType, actorID, approvalReference string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_provider_withdrawals
		SET status = $3, decided_by_actor_type = $4, decided_by_actor_id = $5,
		    approval_reference = COALESCE(NULLIF($6, ''), approval_reference),
		    decided_at = now(), next_attempt_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2 AND status IN ('REQUESTED', 'PENDING_APPROVAL')`,
		id, expectedVersion, status, actorType, actorID, approvalReference)
	if err != nil {
		return fmt.Errorf("finance: decide provider withdrawal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// ClaimWithdrawals leases due withdrawals for the execution worker.
func (r *ProviderOperationsRepository) ClaimWithdrawals(ctx context.Context, owner string,
	lease time.Duration, limit int) ([]models.ProviderWithdrawal, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id FROM finance_provider_withdrawals
			WHERE status IN ('APPROVED', 'FAILED_RETRYABLE')
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE finance_provider_withdrawals w
		SET lease_owner = $2, lease_expires_at = now() + $3::interval,
		    status = 'PROCESSING', version = version + 1, updated_at = now()
		FROM due WHERE w.id = due.id
		RETURNING `+withdrawalColumnsPrefixed("w."), limit, owner, lease.String())
	if err != nil {
		return nil, fmt.Errorf("finance: claim provider withdrawals: %w", err)
	}
	defer rows.Close()
	out := make([]models.ProviderWithdrawal, 0)
	for rows.Next() {
		withdrawal, scanErr := scanWithdrawal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *withdrawal)
	}
	return out, rows.Err()
}

func withdrawalColumnsPrefixed(p string) string {
	return p + "id, " + p + "organization_id, " + p + "earning_account_id, " +
		p + "organization_membership_id, " + p + "provider_id, " + p + "amount_minor, " +
		p + "currency, " + p + "destination_reference, " + p + "policy_mode, " +
		p + "approval_reference, " + p + "status, COALESCE(" + p + "hold_journal_id, ''), " +
		"COALESCE(" + p + "payout_journal_id, ''), " + p + "external_reference, " +
		p + "safe_error_code, " + p + "attempts, " + p + "next_attempt_at, COALESCE(" +
		p + "lease_owner, ''), " + p + "lease_expires_at, " + p + "requested_by_actor_type, " +
		p + "requested_by_actor_id, " + p + "decided_by_actor_type, " + p + "decided_by_actor_id, " +
		p + "decided_at, " + p + "idempotency_key, " + p + "request_fingerprint, " +
		p + "correlation_id, " + p + "causation_id, " + p + "version, " + p + "created_at, " +
		p + "updated_at, " + p + "completed_at"
}

// SetWithdrawalOutcome records the result of one execution attempt.
//
// The lease owner is part of the WHERE clause: a worker whose lease expired
// and was recovered by another worker must not be able to write an outcome for
// an attempt somebody else is now running.
func (r *ProviderOperationsRepository) SetWithdrawalOutcome(ctx context.Context, q database.Querier,
	id, owner, status, externalReference, payoutJournalID, safeErrorCode string,
	next time.Time, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_provider_withdrawals
		SET status = CASE WHEN $3 = 'FAILED_RETRYABLE' AND attempts + 1 >= $8 THEN 'DEAD_LETTER' ELSE $3 END,
		    external_reference = COALESCE(NULLIF($4, ''), external_reference),
		    payout_journal_id = COALESCE(NULLIF($5, ''), payout_journal_id),
		    safe_error_code = $6, next_attempt_at = $7, attempts = attempts + 1,
		    lease_owner = NULL, lease_expires_at = NULL,
		    completed_at = CASE WHEN $3 = 'COMPLETED' THEN now() ELSE completed_at END,
		    version = version + 1, updated_at = now()
		WHERE id = $1 AND lease_owner = $2`,
		id, owner, status, externalReference, payoutJournalID, safeErrorCode, next, maxAttempts)
	if err != nil {
		return fmt.Errorf("finance: set provider withdrawal outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("finance: provider withdrawal lease lost")
	}
	return nil
}

// RecoverWithdrawalLeases returns abandoned in-flight withdrawals to a
// retryable state.
//
// The recovered state is FAILED_RETRYABLE rather than APPROVED: the worker
// that vanished may or may not have reached the payout provider, so the next
// attempt has to go through the same read-back path as any other uncertain
// outcome.
func (r *ProviderOperationsRepository) RecoverWithdrawalLeases(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE finance_provider_withdrawals
		SET lease_owner = NULL, lease_expires_at = NULL, status = 'FAILED_RETRYABLE',
		    safe_error_code = 'lease_expired', next_attempt_at = now(),
		    version = version + 1, updated_at = now()
		WHERE status = 'PROCESSING' AND lease_expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("finance: recover provider withdrawal leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ReplayWithdrawal returns a dead-lettered withdrawal to the execution queue.
func (r *ProviderOperationsRepository) ReplayWithdrawal(ctx context.Context, q database.Querier,
	organizationID, id string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_provider_withdrawals
		SET status = 'APPROVED', attempts = 0, safe_error_code = '', next_attempt_at = now(),
		    version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'DEAD_LETTER'`, organizationID, id)
	if err != nil {
		return fmt.Errorf("finance: replay provider withdrawal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// WithdrawalFilter narrows a withdrawal listing.
type WithdrawalFilter struct {
	EarningAccountID string
	Status           string
	From, To         time.Time
	Limit            int
}

// ListWithdrawals returns tenant-scoped withdrawals, newest first.
func (r *ProviderOperationsRepository) ListWithdrawals(ctx context.Context, q database.Querier,
	organizationID string, filter WithdrawalFilter) ([]models.ProviderWithdrawal, error) {
	if q == nil {
		q = r.db
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	rows, err := q.Query(ctx, `SELECT `+withdrawalColumns+`
		FROM finance_provider_withdrawals
		WHERE organization_id = $1
		  AND ($2 = '' OR earning_account_id = $2)
		  AND ($3 = '' OR status = $3)
		  AND ($4::timestamptz IS NULL OR created_at >= $4)
		  AND ($5::timestamptz IS NULL OR created_at <= $5)
		ORDER BY created_at DESC, id DESC LIMIT $6`,
		organizationID, filter.EarningAccountID, strings.ToUpper(filter.Status),
		nullableTimestamp(filter.From), nullableTimestamp(filter.To), filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list provider withdrawals: %w", err)
	}
	defer rows.Close()
	out := make([]models.ProviderWithdrawal, 0)
	for rows.Next() {
		withdrawal, scanErr := scanWithdrawal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *withdrawal)
	}
	return out, rows.Err()
}

// SetSubWalletOwnership attaches Section 13 corporate ownership metadata to an
// existing earning account.
//
// It is deliberately idempotent and refuses to re-point an account at a
// different membership: an account that already belongs to one membership must
// not be silently reassigned to another, because its balance would follow.
func (r *ProviderOperationsRepository) SetSubWalletOwnership(ctx context.Context, q database.Querier,
	accountID, organizationWalletID, providerID, membershipID, serviceCategory string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `UPDATE finance_earning_accounts
		SET organization_wallet_id = COALESCE(NULLIF($2, ''), organization_wallet_id),
		    provider_id = CASE WHEN provider_id = '' THEN $3 ELSE provider_id END,
		    organization_membership_id = CASE WHEN organization_membership_id = '' THEN $4 ELSE organization_membership_id END,
		    provider_service_category = CASE WHEN provider_service_category = '' THEN $5 ELSE provider_service_category END,
		    version = version + 1, updated_at = now()
		WHERE id = $1
		  AND (organization_membership_id = '' OR $4 = '' OR organization_membership_id = $4)
		  AND (provider_service_category = '' OR $5 = '' OR provider_service_category = $5)`,
		accountID, organizationWalletID, providerID, membershipID, strings.ToUpper(serviceCategory))
	if err != nil {
		return fmt.Errorf("finance: set sub-wallet ownership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// SubWalletOwnership is the corporate ownership view of one sub-wallet.
type SubWalletOwnership struct {
	OrganizationWalletID     string
	ProviderID               string
	OrganizationMembershipID string
	ProviderServiceCategory  string
}

// GetSubWalletOwnership reads the corporate ownership metadata.
func (r *ProviderOperationsRepository) GetSubWalletOwnership(ctx context.Context, q database.Querier,
	accountID string) (SubWalletOwnership, error) {
	if q == nil {
		q = r.db
	}
	var value SubWalletOwnership
	err := q.QueryRow(ctx, `SELECT COALESCE(organization_wallet_id, ''), provider_id,
		organization_membership_id, provider_service_category
		FROM finance_earning_accounts WHERE id = $1`, accountID).
		Scan(&value.OrganizationWalletID, &value.ProviderID,
			&value.OrganizationMembershipID, &value.ProviderServiceCategory)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, ErrNotFound
	}
	if err != nil {
		return value, fmt.Errorf("finance: read sub-wallet ownership: %w", err)
	}
	return value, nil
}

// FindEarningAccountByMembership resolves the active sub-wallet for one
// membership, service category and currency.
func (r *ProviderOperationsRepository) FindEarningAccountByMembership(ctx context.Context,
	q database.Querier, organizationID, membershipID, serviceCategory, currency string) (string, error) {
	if q == nil {
		q = r.db
	}
	var id string
	err := q.QueryRow(ctx, `SELECT id FROM finance_earning_accounts
		WHERE organization_id = $1 AND organization_membership_id = $2
		  AND provider_service_category = $3 AND currency = $4 AND status <> 'CLOSED'`,
		organizationID, membershipID, strings.ToUpper(serviceCategory), strings.ToUpper(currency)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("finance: find sub-wallet by membership: %w", err)
	}
	return id, nil
}
