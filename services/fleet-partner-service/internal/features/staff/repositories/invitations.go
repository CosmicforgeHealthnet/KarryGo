package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// InvitationRepository stores staff invitations. Only the hash of the
// single-use token is persisted.
type InvitationRepository struct {
	db database.Querier
}

// NewInvitationRepository wires the repository.
func NewInvitationRepository(db database.Querier) *InvitationRepository {
	return &InvitationRepository{db: db}
}

const invitationColumns = `id, organization_id, organization_user_id, full_name, email_normalized,
	phone_e164, invitation_token_hash, role_ids, permitted_branch_ids, permitted_department_ids,
	status, expires_at, delivery_status, notification_reference, attempts, created_by,
	accepted_at, consumed_at, version, created_at, updated_at`

// Create inserts a pending invitation.
func (r *InvitationRepository) Create(ctx context.Context, q database.Querier, organizationID string, invitation *models.StaffInvitation) error {
	if invitation.ID == "" {
		invitation.ID = ids.New("inv")
	}
	invitation.OrganizationID = organizationID
	invitation.Status = models.InvitationStatusPending
	invitation.Version = 1

	roleIDs, err := json.Marshal(orEmpty(invitation.RoleIDs))
	if err != nil {
		return fmt.Errorf("staff: marshal role ids: %w", err)
	}
	branches, err := json.Marshal(orEmpty(invitation.PermittedBranchIDs))
	if err != nil {
		return fmt.Errorf("staff: marshal branches: %w", err)
	}
	departments, err := json.Marshal(orEmpty(invitation.PermittedDepartmentIDs))
	if err != nil {
		return fmt.Errorf("staff: marshal departments: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_staff_invitations
			(id, organization_id, organization_user_id, full_name, email_normalized, phone_e164,
			 invitation_token_hash, role_ids, permitted_branch_ids, permitted_department_ids,
			 status, expires_at, delivery_status, attempts, created_by, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'PENDING', $11, 'PENDING', 0, $12, 1, now(), now())`,
		invitation.ID, organizationID, invitation.OrganizationUserID, invitation.FullName,
		invitation.EmailNormalized, invitation.PhoneE164, invitation.InvitationTokenHash,
		roleIDs, branches, departments, invitation.ExpiresAt, invitation.CreatedBy)
	if err != nil {
		return fmt.Errorf("staff: create invitation: %w", err)
	}
	return nil
}

// GetByID loads an invitation inside the organization scope.
func (r *InvitationRepository) GetByID(ctx context.Context, organizationID string, invitationID string) (*models.StaffInvitation, error) {
	row := r.db.QueryRow(ctx, `SELECT `+invitationColumns+`
		FROM fleet_partner_organization_staff_invitations
		WHERE organization_id = $1 AND id = $2`, organizationID, invitationID)
	return scanInvitation(row)
}

// LockByTokenHash loads an invitation by token hash FOR UPDATE. Acceptance is
// cross-tenant by nature — the invitee is not yet a member — so this lookup
// is keyed only by the single-use token hash.
func (r *InvitationRepository) LockByTokenHash(ctx context.Context, q database.Querier, tokenHash string) (*models.StaffInvitation, error) {
	row := q.QueryRow(ctx, `SELECT `+invitationColumns+`
		FROM fleet_partner_organization_staff_invitations
		WHERE invitation_token_hash = $1 FOR UPDATE`, tokenHash)
	return scanInvitation(row)
}

// MarkAccepted consumes the invitation. Conditional on PENDING so a replayed
// token cannot be accepted twice.
func (r *InvitationRepository) MarkAccepted(ctx context.Context, q database.Querier, invitationID string, memberID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_staff_invitations SET
			status = 'ACCEPTED', organization_user_id = $2, accepted_at = now(),
			consumed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'`, invitationID, memberID)
	if err != nil {
		return fmt.Errorf("staff: accept invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationNotUsable
	}
	return nil
}

// ErrInvitationNotUsable means the invitation was already consumed, cancelled
// or has expired.
var ErrInvitationNotUsable = errors.New("staff: invitation is not usable")

// Cancel cancels a pending invitation.
func (r *InvitationRepository) Cancel(ctx context.Context, q database.Querier, organizationID string, invitationID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_staff_invitations SET
			status = 'CANCELLED', version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'PENDING'`, organizationID, invitationID)
	if err != nil {
		return fmt.Errorf("staff: cancel invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationNotUsable
	}
	return nil
}

// RotateToken replaces the token hash and expiry on resend, keeping the
// invited email and phone unchanged.
func (r *InvitationRepository) RotateToken(ctx context.Context, q database.Querier, organizationID string, invitationID string, tokenHash string, expiresAt any) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_staff_invitations SET
			invitation_token_hash = $3, expires_at = $4, attempts = attempts + 1,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'PENDING'`,
		organizationID, invitationID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("staff: rotate invitation token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationNotUsable
	}
	return nil
}

// RecordDelivery stores the invitation email delivery outcome.
func (r *InvitationRepository) RecordDelivery(ctx context.Context, invitationID string, status string, reference string) error {
	var ref *string
	if reference != "" {
		ref = &reference
	}
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_organization_staff_invitations SET
			delivery_status = $2, notification_reference = COALESCE($3, notification_reference), updated_at = now()
		WHERE id = $1`, invitationID, status, ref)
	if err != nil {
		return fmt.Errorf("staff: record invitation delivery: %w", err)
	}
	return nil
}

// ExpireDue marks overdue pending invitations expired; used by the worker.
func (r *InvitationRepository) ExpireDue(ctx context.Context, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_organization_staff_invitations SET status = 'EXPIRED', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_organization_staff_invitations
			WHERE status = 'PENDING' AND expires_at < now() LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("staff: expire invitations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// List returns invitations for an organization.
func (r *InvitationRepository) List(ctx context.Context, organizationID string, limit int) ([]models.StaffInvitation, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+invitationColumns+`
		FROM fleet_partner_organization_staff_invitations
		WHERE organization_id = $1
		ORDER BY created_at DESC, id
		LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("staff: list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []models.StaffInvitation
	for rows.Next() {
		invitation, err := scanInvitationRows(rows)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, *invitation)
	}
	return invitations, rows.Err()
}

func scanInvitation(row pgx.Row) (*models.StaffInvitation, error) {
	var invitation models.StaffInvitation
	var roleIDs, branches, departments []byte
	err := row.Scan(
		&invitation.ID, &invitation.OrganizationID, &invitation.OrganizationUserID,
		&invitation.FullName, &invitation.EmailNormalized, &invitation.PhoneE164,
		&invitation.InvitationTokenHash, &roleIDs, &branches, &departments, &invitation.Status,
		&invitation.ExpiresAt, &invitation.DeliveryStatus, &invitation.NotificationReference,
		&invitation.Attempts, &invitation.CreatedBy, &invitation.AcceptedAt, &invitation.ConsumedAt,
		&invitation.Version, &invitation.CreatedAt, &invitation.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staff: scan invitation: %w", err)
	}
	_ = json.Unmarshal(roleIDs, &invitation.RoleIDs)
	_ = json.Unmarshal(branches, &invitation.PermittedBranchIDs)
	_ = json.Unmarshal(departments, &invitation.PermittedDepartmentIDs)
	return &invitation, nil
}

func scanInvitationRows(rows pgx.Rows) (*models.StaffInvitation, error) {
	var invitation models.StaffInvitation
	var roleIDs, branches, departments []byte
	err := rows.Scan(
		&invitation.ID, &invitation.OrganizationID, &invitation.OrganizationUserID,
		&invitation.FullName, &invitation.EmailNormalized, &invitation.PhoneE164,
		&invitation.InvitationTokenHash, &roleIDs, &branches, &departments, &invitation.Status,
		&invitation.ExpiresAt, &invitation.DeliveryStatus, &invitation.NotificationReference,
		&invitation.Attempts, &invitation.CreatedBy, &invitation.AcceptedAt, &invitation.ConsumedAt,
		&invitation.Version, &invitation.CreatedAt, &invitation.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("staff: scan invitation: %w", err)
	}
	_ = json.Unmarshal(roleIDs, &invitation.RoleIDs)
	_ = json.Unmarshal(branches, &invitation.PermittedBranchIDs)
	_ = json.Unmarshal(departments, &invitation.PermittedDepartmentIDs)
	return &invitation, nil
}
