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

// MarketplaceRepository owns commission rules and earning allocations.
type MarketplaceRepository struct {
	db database.Querier
}

// NewMarketplaceRepository wires the repository.
func NewMarketplaceRepository(db database.Querier) *MarketplaceRepository {
	return &MarketplaceRepository{db: db}
}

const commissionRuleColumns = `id, COALESCE(organization_id, ''), service_type, currency,
	rule_version, commission_bps, fleet_fee_bps, provider_share_bps, organization_share_bps,
	status, effective_from, effective_to, created_at`

func scanCommissionRule(row pgx.Row) (*models.CommissionRule, error) {
	var rule models.CommissionRule
	err := row.Scan(&rule.ID, &rule.OrganizationID, &rule.ServiceType, &rule.Currency,
		&rule.RuleVersion, &rule.CommissionBPS, &rule.FleetFeeBPS, &rule.ProviderShareBPS,
		&rule.OrganizationShareBPS, &rule.Status, &rule.EffectiveFrom, &rule.EffectiveTo,
		&rule.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan commission rule: %w", err)
	}
	return &rule, nil
}

// CreateCommissionRule inserts a new versioned rule.
func (r *MarketplaceRepository) CreateCommissionRule(ctx context.Context, q database.Querier,
	rule *models.CommissionRule) error {
	if q == nil {
		q = r.db
	}
	if rule.ID == "" {
		rule.ID = ids.New(ids.PrefixCommissionRule)
	}
	if rule.Status == "" {
		rule.Status = "ACTIVE"
	}
	if rule.RuleVersion <= 0 {
		if err := q.QueryRow(ctx, `
			SELECT COALESCE(max(rule_version), 0) + 1 FROM finance_commission_rules
			WHERE COALESCE(organization_id, '') = $1 AND service_type = $2 AND currency = $3`,
			rule.OrganizationID, rule.ServiceType, rule.Currency).Scan(&rule.RuleVersion); err != nil {
			return fmt.Errorf("finance: next rule version: %w", err)
		}
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_commission_rules
			(id, organization_id, service_type, currency, rule_version, commission_bps,
			 fleet_fee_bps, provider_share_bps, organization_share_bps, status,
			 effective_from, effective_to, created_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, COALESCE($11, now()), $12, now())
		RETURNING effective_from, created_at`,
		rule.ID, rule.OrganizationID, rule.ServiceType, rule.Currency, rule.RuleVersion,
		rule.CommissionBPS, rule.FleetFeeBPS, rule.ProviderShareBPS, rule.OrganizationShareBPS,
		rule.Status, nullableTime(rule.EffectiveFrom), rule.EffectiveTo).
		Scan(&rule.EffectiveFrom, &rule.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create commission rule: %w", err)
	}
	return nil
}

// ResolveCommissionRule finds the rule that applies to an organization's job at
// a point in time.
//
// An organization-specific rule wins over the platform default. When neither
// exists the caller must block the posting: there is no fallback percentage,
// because a guessed commission is a wrong commission.
func (r *MarketplaceRepository) ResolveCommissionRule(ctx context.Context, q database.Querier,
	organizationID string, serviceType string, currency string,
	at time.Time) (*models.CommissionRule, error) {
	if q == nil {
		q = r.db
	}
	return scanCommissionRule(q.QueryRow(ctx, `
		SELECT `+commissionRuleColumns+` FROM finance_commission_rules
		WHERE service_type = $2 AND currency = $3 AND status = 'ACTIVE'
		  AND (organization_id IS NULL OR organization_id = $1)
		  AND effective_from <= $4
		  AND (effective_to IS NULL OR effective_to > $4)
		ORDER BY (organization_id IS NOT NULL) DESC, rule_version DESC
		LIMIT 1`, organizationID, serviceType, currency, at))
}

// GetCommissionRule loads a rule by ID.
func (r *MarketplaceRepository) GetCommissionRule(ctx context.Context, q database.Querier,
	id string) (*models.CommissionRule, error) {
	if q == nil {
		q = r.db
	}
	return scanCommissionRule(q.QueryRow(ctx,
		`SELECT `+commissionRuleColumns+` FROM finance_commission_rules WHERE id = $1`, id))
}

// RetireCommissionRule closes a rule's effective window without deleting it,
// so historical allocations remain explainable.
func (r *MarketplaceRepository) RetireCommissionRule(ctx context.Context, q database.Querier,
	id string, at time.Time) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_commission_rules SET status = 'RETIRED', effective_to = $2
		WHERE id = $1 AND status = 'ACTIVE'`, id, at)
	if err != nil {
		return fmt.Errorf("finance: retire commission rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const allocationColumns = `id, organization_id, source_service, job_reference, currency,
	rule_id, rule_version, rule_snapshot, gross_minor, commission_minor, fleet_fee_minor,
	provider_earning_minor, organization_earning_minor, provider_reference,
	COALESCE(earning_account_id, ''), COALESCE(wallet_id, ''), journal_id, status,
	idempotency_key, request_fingerprint, correlation_id, causation_id,
	version, created_at, updated_at`

func scanAllocation(row pgx.Row) (*models.EarningAllocation, error) {
	var allocation models.EarningAllocation
	err := row.Scan(&allocation.ID, &allocation.OrganizationID, &allocation.SourceService,
		&allocation.JobReference, &allocation.Currency, &allocation.RuleID, &allocation.RuleVersion,
		&allocation.RuleSnapshot, &allocation.GrossMinor, &allocation.CommissionMinor,
		&allocation.FleetFeeMinor, &allocation.ProviderEarningMinor,
		&allocation.OrganizationEarningMinor, &allocation.ProviderReference,
		&allocation.EarningAccountID, &allocation.WalletID, &allocation.JournalID,
		&allocation.Status, &allocation.IdempotencyKey, &allocation.RequestFingerprint,
		&allocation.CorrelationID, &allocation.CausationID, &allocation.Version,
		&allocation.CreatedAt, &allocation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan allocation: %w", err)
	}
	return &allocation, nil
}

// CreateAllocation records a posted earning split.
func (r *MarketplaceRepository) CreateAllocation(ctx context.Context, q database.Querier,
	allocation *models.EarningAllocation) error {
	if q == nil {
		q = r.db
	}
	if allocation.ID == "" {
		allocation.ID = ids.New(ids.PrefixAllocation)
	}
	if allocation.Status == "" {
		allocation.Status = models.AllocationPosted
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_earning_allocations
			(id, organization_id, source_service, job_reference, currency, rule_id, rule_version,
			 rule_snapshot, gross_minor, commission_minor, fleet_fee_minor, provider_earning_minor,
			 organization_earning_minor, provider_reference, earning_account_id, wallet_id,
			 journal_id, status, idempotency_key, request_fingerprint, correlation_id,
			 causation_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		        NULLIF($15, ''), NULLIF($16, ''), $17, $18, $19, $20, $21, $22, 1, now(), now())
		RETURNING version, created_at, updated_at`,
		allocation.ID, allocation.OrganizationID, allocation.SourceService, allocation.JobReference,
		allocation.Currency, allocation.RuleID, allocation.RuleVersion, allocation.RuleSnapshot,
		allocation.GrossMinor, allocation.CommissionMinor, allocation.FleetFeeMinor,
		allocation.ProviderEarningMinor, allocation.OrganizationEarningMinor,
		allocation.ProviderReference, allocation.EarningAccountID, allocation.WalletID,
		allocation.JournalID, allocation.Status, allocation.IdempotencyKey,
		allocation.RequestFingerprint, allocation.CorrelationID, allocation.CausationID).
		Scan(&allocation.Version, &allocation.CreatedAt, &allocation.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create allocation: %w", err)
	}
	return nil
}

// GetAllocation loads an allocation inside a tenant scope.
func (r *MarketplaceRepository) GetAllocation(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.EarningAllocation, error) {
	if q == nil {
		q = r.db
	}
	return scanAllocation(q.QueryRow(ctx,
		`SELECT `+allocationColumns+` FROM finance_earning_allocations
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
}

// FindAllocationByJob resolves the allocation already posted for a job.
func (r *MarketplaceRepository) FindAllocationByJob(ctx context.Context, q database.Querier,
	organizationID string, sourceService string, jobReference string) (*models.EarningAllocation, error) {
	if q == nil {
		q = r.db
	}
	return scanAllocation(q.QueryRow(ctx,
		`SELECT `+allocationColumns+` FROM finance_earning_allocations
		 WHERE organization_id = $1 AND source_service = $2 AND job_reference = $3`,
		organizationID, sourceService, jobReference))
}

// FindAllocationByIdempotencyKey resolves a replayed allocation.
func (r *MarketplaceRepository) FindAllocationByIdempotencyKey(ctx context.Context,
	q database.Querier, organizationID string, key string) (*models.EarningAllocation, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanAllocation(q.QueryRow(ctx,
		`SELECT `+allocationColumns+` FROM finance_earning_allocations
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// MarkAllocationReversed records that a compensating entry undid the split.
func (r *MarketplaceRepository) MarkAllocationReversed(ctx context.Context, q database.Querier,
	id string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_earning_allocations SET
			status = 'REVERSED', version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2 AND status = 'POSTED'`, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("finance: mark allocation reversed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SettleableAllocations returns posted allocations for a provider that no
// live settlement item already claims.
func (r *MarketplaceRepository) SettleableAllocations(ctx context.Context, q database.Querier,
	earningAccountID string, currency string, limit int) ([]models.EarningAllocation, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx, `
		SELECT `+allocationColumns+` FROM finance_earning_allocations a
		WHERE a.earning_account_id = $1 AND a.currency = $2 AND a.status = 'POSTED'
		  AND NOT EXISTS (
			SELECT 1 FROM finance_settlement_items i
			WHERE i.earning_allocation_id = a.id
			  AND i.status NOT IN ('PERMANENT_FAILURE', 'REVERSED'))
		ORDER BY a.created_at
		LIMIT $3`, earningAccountID, currency, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list settleable allocations: %w", err)
	}
	defer rows.Close()

	var allocations []models.EarningAllocation
	for rows.Next() {
		var allocation models.EarningAllocation
		if err := rows.Scan(&allocation.ID, &allocation.OrganizationID, &allocation.SourceService,
			&allocation.JobReference, &allocation.Currency, &allocation.RuleID,
			&allocation.RuleVersion, &allocation.RuleSnapshot, &allocation.GrossMinor,
			&allocation.CommissionMinor, &allocation.FleetFeeMinor,
			&allocation.ProviderEarningMinor, &allocation.OrganizationEarningMinor,
			&allocation.ProviderReference, &allocation.EarningAccountID, &allocation.WalletID,
			&allocation.JournalID, &allocation.Status, &allocation.IdempotencyKey,
			&allocation.RequestFingerprint, &allocation.CorrelationID, &allocation.CausationID,
			&allocation.Version, &allocation.CreatedAt, &allocation.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan allocation: %w", err)
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}
