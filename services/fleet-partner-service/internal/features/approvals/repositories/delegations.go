package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const prefixDelegation = "apdl"

const delegationColumns = `id, organization_id, delegator_id, delegate_id, resource_type, action,
	branch_id, effective_from, expires_at, reason, revoked_at, revoked_by,
	revocation_reason, created_by, idempotency_key, version, created_at, updated_at`

// DelegationFilter narrows a delegation listing.
type DelegationFilter struct {
	DelegatorID string
	DelegateID  string
	// Status is one of the derived delegation statuses, or empty for all.
	Status string
	Limit  int
}

// CreateDelegation inserts a delegation at version 1.
func (r *Repository) CreateDelegation(ctx context.Context, q database.Querier,
	organizationID string, delegation *models.Delegation) error {
	if delegation.ID == "" {
		delegation.ID = ids.New(prefixDelegation)
	}
	delegation.OrganizationID = organizationID
	delegation.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_delegations
			(id, organization_id, delegator_id, delegate_id, resource_type, action,
			 branch_id, effective_from, expires_at, reason, created_by,
			 idempotency_key, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1)`,
		delegation.ID, organizationID, delegation.DelegatorID, delegation.DelegateID,
		delegation.ResourceType, delegation.Action, delegation.BranchID,
		delegation.EffectiveFrom, delegation.ExpiresAt, delegation.Reason,
		delegation.CreatedBy, delegation.IdempotencyKey)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: create delegation: %w", err)
	}
	return nil
}

// GetDelegation loads one delegation inside the tenant.
func (r *Repository) GetDelegation(ctx context.Context, q database.Querier,
	organizationID string, delegationID string) (*models.Delegation, error) {
	if q == nil {
		q = r.db
	}
	return scanDelegation(q.QueryRow(ctx, `SELECT `+delegationColumns+`
		FROM fleet_partner_approval_delegations
		WHERE organization_id = $1 AND id = $2`, organizationID, delegationID))
}

// LockDelegation loads one delegation for update.
func (r *Repository) LockDelegation(ctx context.Context, q database.Querier,
	organizationID string, delegationID string) (*models.Delegation, error) {
	return scanDelegation(q.QueryRow(ctx, `SELECT `+delegationColumns+`
		FROM fleet_partner_approval_delegations
		WHERE organization_id = $1 AND id = $2 FOR UPDATE`, organizationID, delegationID))
}

// FindDelegationByIdempotencyKey resolves a retried creation.
func (r *Repository) FindDelegationByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Delegation, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanDelegation(q.QueryRow(ctx, `SELECT `+delegationColumns+`
		FROM fleet_partner_approval_delegations
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// LockDelegator serializes delegation writes for one delegator so the overlap
// check and the insert cannot interleave with a concurrent creation.
func (r *Repository) LockDelegator(ctx context.Context, q database.Querier,
	organizationID string, delegatorID string) error {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"approval-delegation:"+organizationID+":"+delegatorID); err != nil {
		return fmt.Errorf("approvals: lock delegator: %w", err)
	}
	return nil
}

// HasOverlappingDelegation reports whether the delegator already delegates
// the same scope to the same delegate for any part of [from, to).
func (r *Repository) HasOverlappingDelegation(ctx context.Context, q database.Querier,
	candidate *models.Delegation) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fleet_partner_approval_delegations
			WHERE organization_id = $1 AND delegator_id = $2 AND delegate_id = $3
			  AND revoked_at IS NULL
			  AND resource_type IS NOT DISTINCT FROM $4
			  AND action IS NOT DISTINCT FROM $5
			  AND branch_id IS NOT DISTINCT FROM $6
			  AND effective_from < $8 AND expires_at > $7)`,
		candidate.OrganizationID, candidate.DelegatorID, candidate.DelegateID,
		candidate.ResourceType, candidate.Action, candidate.BranchID,
		candidate.EffectiveFrom, candidate.ExpiresAt).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("approvals: overlap check: %w", err)
	}
	return exists, nil
}

// RevokeDelegation ends a live delegation under optimistic concurrency.
func (r *Repository) RevokeDelegation(ctx context.Context, q database.Querier,
	organizationID string, delegationID string, expectedVersion int64,
	revokedBy string, reason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_delegations SET
			revoked_at = now(), revoked_by = $4, revocation_reason = $5,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND revoked_at IS NULL`,
		organizationID, delegationID, expectedVersion, revokedBy, reason)
	if err != nil {
		return fmt.Errorf("approvals: revoke delegation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// ListDelegations returns the organization's delegations.
func (r *Repository) ListDelegations(ctx context.Context, organizationID string,
	filter DelegationFilter, now time.Time) ([]models.Delegation, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `SELECT `+delegationColumns+`
		FROM fleet_partner_approval_delegations
		WHERE organization_id = $1
		  AND ($2 = '' OR delegator_id = $2)
		  AND ($3 = '' OR delegate_id = $3)
		  AND ($4 = ''
		       OR ($4 = 'REVOKED' AND revoked_at IS NOT NULL)
		       OR ($4 = 'EXPIRED' AND revoked_at IS NULL AND expires_at <= $5)
		       OR ($4 = 'SCHEDULED' AND revoked_at IS NULL AND effective_from > $5)
		       OR ($4 = 'ACTIVE' AND revoked_at IS NULL AND effective_from <= $5 AND expires_at > $5))
		ORDER BY created_at DESC, id DESC LIMIT $6`,
		organizationID, filter.DelegatorID, filter.DelegateID, filter.Status, now, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: list delegations: %w", err)
	}
	defer rows.Close()
	return collectDelegations(rows)
}

// ActiveDelegationsTo returns the unrevoked, unexpired delegations naming
// delegateID, read inside the deciding transaction.
func (r *Repository) ActiveDelegationsTo(ctx context.Context, q database.Querier,
	organizationID string, delegateID string, now time.Time) ([]models.Delegation, error) {
	rows, err := q.Query(ctx, `SELECT `+delegationColumns+`
		FROM fleet_partner_approval_delegations
		WHERE organization_id = $1 AND delegate_id = $2 AND revoked_at IS NULL
		  AND effective_from <= $3 AND expires_at > $3
		ORDER BY created_at, id`, organizationID, delegateID, now)
	if err != nil {
		return nil, fmt.Errorf("approvals: read delegations: %w", err)
	}
	defer rows.Close()
	return collectDelegations(rows)
}

// MemberStatus returns an organization user's status inside the tenant.
func (r *Repository) MemberStatus(ctx context.Context, q database.Querier,
	organizationID string, memberID string) (string, error) {
	if q == nil {
		q = r.db
	}
	var status string
	err := q.QueryRow(ctx, `
		SELECT status FROM fleet_partner_organization_users
		WHERE organization_id = $1 AND id = $2`, organizationID, memberID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("approvals: read member: %w", err)
	}
	return status, nil
}

func collectDelegations(rows pgx.Rows) ([]models.Delegation, error) {
	delegations := []models.Delegation{}
	for rows.Next() {
		delegation, err := scanDelegationRow(rows)
		if err != nil {
			return nil, err
		}
		delegations = append(delegations, *delegation)
	}
	return delegations, rows.Err()
}

func scanDelegation(row pgx.Row) (*models.Delegation, error) { return scanDelegationRow(row) }

func scanDelegationRow(row rowScanner) (*models.Delegation, error) {
	var delegation models.Delegation
	err := row.Scan(&delegation.ID, &delegation.OrganizationID, &delegation.DelegatorID,
		&delegation.DelegateID, &delegation.ResourceType, &delegation.Action,
		&delegation.BranchID, &delegation.EffectiveFrom, &delegation.ExpiresAt,
		&delegation.Reason, &delegation.RevokedAt, &delegation.RevokedBy,
		&delegation.RevocationReason, &delegation.CreatedBy, &delegation.IdempotencyKey,
		&delegation.Version, &delegation.CreatedAt, &delegation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: scan delegation: %w", err)
	}
	return &delegation, nil
}
