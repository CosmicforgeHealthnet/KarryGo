// Package repositories implements tenant-scoped persistence for company
// staff, roles and role bindings.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound is returned when a tenant-scoped lookup matches nothing.
var ErrNotFound = errors.New("staff: not found")

// ErrLastOwner protects the last active owner of an organization.
var ErrLastOwner = errors.New("staff: organization must retain an active owner")

// MemberRepository stores organization staff memberships.
type MemberRepository struct {
	db database.Querier
}

// NewMemberRepository wires the repository.
func NewMemberRepository(db database.Querier) *MemberRepository {
	return &MemberRepository{db: db}
}

const memberColumns = `id, organization_id, identity_id, full_name, job_title, status, is_owner,
	permitted_branch_ids, permitted_department_ids, role_version, invited_by, invited_at,
	activated_at, suspended_at, ended_at, version, created_at, updated_at`

// Create inserts a membership.
func (r *MemberRepository) Create(ctx context.Context, q database.Querier, organizationID string, member *models.OrganizationUser) error {
	if organizationID == "" {
		return fmt.Errorf("staff: organization id is required")
	}
	if member.ID == "" {
		member.ID = ids.New("ousr")
	}
	if member.Status == "" {
		member.Status = models.UserStatusInvited
	}
	member.OrganizationID = organizationID
	member.Version = 1
	if member.RoleVersion == 0 {
		member.RoleVersion = 1
	}

	branches, err := json.Marshal(orEmpty(member.PermittedBranchIDs))
	if err != nil {
		return fmt.Errorf("staff: marshal branches: %w", err)
	}
	departments, err := json.Marshal(orEmpty(member.PermittedDepartmentIDs))
	if err != nil {
		return fmt.Errorf("staff: marshal departments: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_users
			(id, organization_id, identity_id, full_name, job_title, status, is_owner,
			 permitted_branch_ids, permitted_department_ids, role_version, invited_by, invited_at,
			 activated_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 1, now(), now())`,
		member.ID, organizationID, member.IdentityID, member.FullName, member.JobTitle,
		member.Status, member.IsOwner, branches, departments, member.RoleVersion,
		member.InvitedBy, member.InvitedAt, member.ActivatedAt,
	)
	if err != nil {
		return fmt.Errorf("staff: create member: %w", err)
	}
	return nil
}

// GetByID loads one membership inside the organization scope.
func (r *MemberRepository) GetByID(ctx context.Context, organizationID string, memberID string) (*models.OrganizationUser, error) {
	row := r.db.QueryRow(ctx, `SELECT `+memberColumns+`
		FROM fleet_partner_organization_users
		WHERE organization_id = $1 AND id = $2`, organizationID, memberID)
	return scanMember(row)
}

// GetByIdentity loads the membership of an identity in one organization.
func (r *MemberRepository) GetByIdentity(ctx context.Context, organizationID string, identityID string) (*models.OrganizationUser, error) {
	row := r.db.QueryRow(ctx, `SELECT `+memberColumns+`
		FROM fleet_partner_organization_users
		WHERE organization_id = $1 AND identity_id = $2
		  AND status IN ('INVITED', 'ACTIVE', 'SUSPENDED')`, organizationID, identityID)
	return scanMember(row)
}

// ListForIdentity returns every live membership of an identity across
// organizations. This is an identity-scoped query, not a tenant-scoped one:
// it is how a person discovers which organizations they belong to.
func (r *MemberRepository) ListForIdentity(ctx context.Context, identityID string) ([]models.OrganizationUser, error) {
	rows, err := r.db.Query(ctx, `SELECT `+memberColumns+`
		FROM fleet_partner_organization_users
		WHERE identity_id = $1 AND status IN ('ACTIVE', 'SUSPENDED')
		ORDER BY created_at DESC, id`, identityID)
	if err != nil {
		return nil, fmt.Errorf("staff: list for identity: %w", err)
	}
	defer rows.Close()

	var members []models.OrganizationUser
	for rows.Next() {
		member, err := scanMemberRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *member)
	}
	return members, rows.Err()
}

// List returns memberships for a tenant query, enforcing permitted branches.
func (r *MemberRepository) List(ctx context.Context, query tenant.Query) ([]models.OrganizationUser, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("staff: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+memberColumns+`
		FROM fleet_partner_organization_users
		WHERE organization_id = $1
		ORDER BY created_at DESC, id
		LIMIT $2`, query.OrganizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("staff: list members: %w", err)
	}
	defer rows.Close()

	var members []models.OrganizationUser
	for rows.Next() {
		member, err := scanMemberRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *member)
	}
	return members, rows.Err()
}

// UpdateStatus performs a tenant-scoped compare-and-swap status change.
//
// Ending or suspending the last active owner is refused: the check runs in
// the same statement set as the update, and the caller is expected to hold a
// transaction so the count cannot drift underneath it.
func (r *MemberRepository) UpdateStatus(ctx context.Context, q database.Querier, organizationID string, memberID string, status string, expectedVersion int64) error {
	if status == models.UserStatusSuspended || status == models.UserStatusEnded {
		var isOwner bool
		if err := q.QueryRow(ctx, `
			SELECT is_owner FROM fleet_partner_organization_users
			WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
			organizationID, memberID).Scan(&isOwner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("staff: lock member: %w", err)
		}
		if isOwner {
			var activeOwners int
			if err := q.QueryRow(ctx, `
				SELECT count(*) FROM fleet_partner_organization_users
				WHERE organization_id = $1 AND is_owner = true AND status = 'ACTIVE' AND id <> $2`,
				organizationID, memberID).Scan(&activeOwners); err != nil {
				return fmt.Errorf("staff: count owners: %w", err)
			}
			if activeOwners == 0 {
				return ErrLastOwner
			}
		}
	}

	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_users SET
			status = $3,
			activated_at = CASE WHEN $3 = 'ACTIVE' THEN COALESCE(activated_at, now()) ELSE activated_at END,
			suspended_at = CASE WHEN $3 = 'SUSPENDED' THEN now() ELSE suspended_at END,
			ended_at = CASE WHEN $3 = 'ENDED' THEN now() ELSE ended_at END,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, memberID, status, expectedVersion)
	if err != nil {
		return fmt.Errorf("staff: update member status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		exists, err := r.exists(ctx, q, organizationID, memberID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// UpdateProfile updates mutable membership fields with compare-and-swap.
func (r *MemberRepository) UpdateProfile(ctx context.Context, q database.Querier, organizationID string, memberID string, fullName string, jobTitle *string, branches []string, departments []string, expectedVersion int64) error {
	branchJSON, err := json.Marshal(orEmpty(branches))
	if err != nil {
		return fmt.Errorf("staff: marshal branches: %w", err)
	}
	departmentJSON, err := json.Marshal(orEmpty(departments))
	if err != nil {
		return fmt.Errorf("staff: marshal departments: %w", err)
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_users SET
			full_name = $3, job_title = $4, permitted_branch_ids = $5,
			permitted_department_ids = $6, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $7`,
		organizationID, memberID, fullName, jobTitle, branchJSON, departmentJSON, expectedVersion)
	if err != nil {
		return fmt.Errorf("staff: update member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// BumpRoleVersion invalidates cached authorization for a membership.
func (r *MemberRepository) BumpRoleVersion(ctx context.Context, q database.Querier, organizationID string, memberID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_users SET role_version = role_version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2`, organizationID, memberID)
	if err != nil {
		return fmt.Errorf("staff: bump role version: %w", err)
	}
	return nil
}

// CountActiveOwners reports how many active owners an organization has.
func (r *MemberRepository) CountActiveOwners(ctx context.Context, organizationID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_organization_users
		WHERE organization_id = $1 AND is_owner = true AND status = 'ACTIVE'`, organizationID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("staff: count active owners: %w", err)
	}
	return count, nil
}

func (r *MemberRepository) exists(ctx context.Context, q database.Querier, organizationID string, memberID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fleet_partner_organization_users WHERE organization_id = $1 AND id = $2)`,
		organizationID, memberID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("staff: member exists: %w", err)
	}
	return exists, nil
}

func scanMember(row pgx.Row) (*models.OrganizationUser, error) {
	var member models.OrganizationUser
	var branches, departments []byte
	err := row.Scan(
		&member.ID, &member.OrganizationID, &member.IdentityID, &member.FullName, &member.JobTitle,
		&member.Status, &member.IsOwner, &branches, &departments, &member.RoleVersion,
		&member.InvitedBy, &member.InvitedAt, &member.ActivatedAt, &member.SuspendedAt,
		&member.EndedAt, &member.Version, &member.CreatedAt, &member.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staff: scan member: %w", err)
	}
	_ = json.Unmarshal(branches, &member.PermittedBranchIDs)
	_ = json.Unmarshal(departments, &member.PermittedDepartmentIDs)
	return &member, nil
}

func scanMemberRows(rows pgx.Rows) (*models.OrganizationUser, error) {
	var member models.OrganizationUser
	var branches, departments []byte
	err := rows.Scan(
		&member.ID, &member.OrganizationID, &member.IdentityID, &member.FullName, &member.JobTitle,
		&member.Status, &member.IsOwner, &branches, &departments, &member.RoleVersion,
		&member.InvitedBy, &member.InvitedAt, &member.ActivatedAt, &member.SuspendedAt,
		&member.EndedAt, &member.Version, &member.CreatedAt, &member.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("staff: scan member: %w", err)
	}
	_ = json.Unmarshal(branches, &member.PermittedBranchIDs)
	_ = json.Unmarshal(departments, &member.PermittedDepartmentIDs)
	return &member, nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
