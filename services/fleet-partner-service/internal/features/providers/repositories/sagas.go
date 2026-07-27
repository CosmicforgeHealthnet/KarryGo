package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---- Invitations ----

const invitationColumns = `id, organization_id, membership_id, provider_service, legal_name,
	email_normalized, phone_e164, existing_provider_id, invitation_token_hash, status, expires_at,
	delivery_status, notification_reference, attempts, created_by, accepted_at, declined_at,
	consumed_at, version, created_at, updated_at`

// CreateInvitation inserts a provider association invitation.
func (r *Repository) CreateInvitation(ctx context.Context, q database.Querier, organizationID string, invitation *models.Invitation) error {
	if invitation.ID == "" {
		invitation.ID = ids.New("pinv")
	}
	invitation.OrganizationID = organizationID
	invitation.Status = models.InvitationPending
	invitation.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_invitations
			(id, organization_id, membership_id, provider_service, legal_name, email_normalized,
			 phone_e164, existing_provider_id, invitation_token_hash, status, expires_at,
			 delivery_status, attempts, created_by, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING', $10, 'PENDING', 0, $11, 1, now(), now())`,
		invitation.ID, organizationID, invitation.MembershipID, invitation.ProviderService,
		invitation.LegalName, invitation.EmailNormalized, invitation.PhoneE164,
		invitation.ExistingProviderID, invitation.InvitationTokenHash, invitation.ExpiresAt,
		invitation.CreatedBy)
	if err != nil {
		return fmt.Errorf("providers: create invitation: %w", err)
	}
	return nil
}

// GetInvitationByMembership returns the pending invitation of a membership.
func (r *Repository) GetInvitationByMembership(ctx context.Context, organizationID string, membershipID string) (*models.Invitation, error) {
	row := r.db.QueryRow(ctx, `SELECT `+invitationColumns+`
		FROM fleet_partner_provider_invitations
		WHERE organization_id = $1 AND membership_id = $2
		ORDER BY created_at DESC LIMIT 1`, organizationID, membershipID)
	return scanInvitation(row)
}

// SetInvitationOutcome records provider acceptance or decline.
//
// The organization can never call this on the provider's behalf: it is
// driven by events and internal callbacks from the owning provider service.
func (r *Repository) SetInvitationOutcome(ctx context.Context, q database.Querier, invitationID string, status string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_invitations SET
			status = $2,
			accepted_at = CASE WHEN $2 = 'ACCEPTED' THEN now() ELSE accepted_at END,
			declined_at = CASE WHEN $2 = 'DECLINED' THEN now() ELSE declined_at END,
			consumed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'`, invitationID, status)
	if err != nil {
		return fmt.Errorf("providers: set invitation outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationNotUsable
	}
	return nil
}

// ErrInvitationNotUsable means the invitation was already resolved.
var ErrInvitationNotUsable = errors.New("providers: invitation is not usable")

// RecordInvitationDelivery stores the invitation email delivery outcome.
func (r *Repository) RecordInvitationDelivery(ctx context.Context, invitationID string, status string, reference string) error {
	var ref *string
	if reference != "" {
		ref = &reference
	}
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_provider_invitations SET
			delivery_status = $2, notification_reference = COALESCE($3, notification_reference),
			attempts = attempts + 1, updated_at = now()
		WHERE id = $1`, invitationID, status, ref)
	if err != nil {
		return fmt.Errorf("providers: record invitation delivery: %w", err)
	}
	return nil
}

// ExpireDueInvitations marks overdue invitations expired; used by the worker.
func (r *Repository) ExpireDueInvitations(ctx context.Context, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_provider_invitations SET status = 'EXPIRED', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_provider_invitations
			WHERE status = 'PENDING' AND expires_at < now() LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("providers: expire invitations: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanInvitation(row pgx.Row) (*models.Invitation, error) {
	var invitation models.Invitation
	err := row.Scan(&invitation.ID, &invitation.OrganizationID, &invitation.MembershipID,
		&invitation.ProviderService, &invitation.LegalName, &invitation.EmailNormalized,
		&invitation.PhoneE164, &invitation.ExistingProviderID, &invitation.InvitationTokenHash,
		&invitation.Status, &invitation.ExpiresAt, &invitation.DeliveryStatus,
		&invitation.NotificationReference, &invitation.Attempts, &invitation.CreatedBy,
		&invitation.AcceptedAt, &invitation.DeclinedAt, &invitation.ConsumedAt,
		&invitation.Version, &invitation.CreatedAt, &invitation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("providers: scan invitation: %w", err)
	}
	return &invitation, nil
}

// ---- Provisioning sagas ----

const sagaColumns = `id, organization_id, membership_id, provider_service, provider_resolution_status,
	provider_identity_status, association_status, subwallet_status, notification_status,
	overall_status, idempotency_key, attempts, last_error_code, next_attempt_at, version,
	created_at, updated_at`

// CreateSaga inserts a provisioning saga for a membership.
func (r *Repository) CreateSaga(ctx context.Context, q database.Querier, organizationID string, saga *models.ProvisioningSaga) error {
	if saga.ID == "" {
		saga.ID = ids.New("psag")
	}
	saga.OrganizationID = organizationID
	saga.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_provisioning_sagas
			(id, organization_id, membership_id, provider_service, idempotency_key, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 1, now(), now())
		ON CONFLICT (membership_id) DO NOTHING`,
		saga.ID, organizationID, saga.MembershipID, saga.ProviderService, saga.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("providers: create saga: %w", err)
	}
	return nil
}

// GetSaga loads the saga of a membership.
func (r *Repository) GetSaga(ctx context.Context, organizationID string, membershipID string) (*models.ProvisioningSaga, error) {
	row := r.db.QueryRow(ctx, `SELECT `+sagaColumns+`
		FROM fleet_partner_provider_provisioning_sagas
		WHERE organization_id = $1 AND membership_id = $2`, organizationID, membershipID)
	return scanSaga(row)
}

// UpdateSagaStep records progress of one saga step.
func (r *Repository) UpdateSagaStep(ctx context.Context, q database.Querier, membershipID string, column string, value string, overall string, errorCode string, nextAttemptAt *time.Time) error {
	// column is a fixed internal literal chosen by the caller from a closed
	// set, never client input.
	switch column {
	case "provider_resolution_status", "provider_identity_status", "association_status",
		"subwallet_status", "notification_status":
	default:
		return fmt.Errorf("providers: unknown saga column %q", column)
	}
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_provisioning_sagas SET
			`+column+` = $2,
			overall_status = $3,
			last_error_code = $4,
			next_attempt_at = $5,
			attempts = attempts + 1,
			version = version + 1,
			updated_at = now()
		WHERE membership_id = $1`, membershipID, value, overall, code, nextAttemptAt)
	if err != nil {
		return fmt.Errorf("providers: update saga step: %w", err)
	}
	return nil
}

// ClaimDueSagas returns sagas ready for another provisioning attempt.
func (r *Repository) ClaimDueSagas(ctx context.Context, limit int) ([]models.ProvisioningSaga, error) {
	rows, err := r.db.Query(ctx, `SELECT `+sagaColumns+`
		FROM fleet_partner_provider_provisioning_sagas
		WHERE overall_status IN ('IN_PROGRESS', 'RECONCILIATION_REQUIRED')
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY updated_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("providers: claim sagas: %w", err)
	}
	defer rows.Close()

	var sagas []models.ProvisioningSaga
	for rows.Next() {
		saga, err := scanSagaRows(rows)
		if err != nil {
			return nil, err
		}
		sagas = append(sagas, *saga)
	}
	return sagas, rows.Err()
}

func scanSaga(row pgx.Row) (*models.ProvisioningSaga, error) {
	var saga models.ProvisioningSaga
	err := row.Scan(&saga.ID, &saga.OrganizationID, &saga.MembershipID, &saga.ProviderService,
		&saga.ProviderResolutionStatus, &saga.ProviderIdentityStatus, &saga.AssociationStatus,
		&saga.SubWalletStatus, &saga.NotificationStatus, &saga.OverallStatus, &saga.IdempotencyKey,
		&saga.Attempts, &saga.LastErrorCode, &saga.NextAttemptAt, &saga.Version,
		&saga.CreatedAt, &saga.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("providers: scan saga: %w", err)
	}
	return &saga, nil
}

func scanSagaRows(rows pgx.Rows) (*models.ProvisioningSaga, error) {
	var saga models.ProvisioningSaga
	err := rows.Scan(&saga.ID, &saga.OrganizationID, &saga.MembershipID, &saga.ProviderService,
		&saga.ProviderResolutionStatus, &saga.ProviderIdentityStatus, &saga.AssociationStatus,
		&saga.SubWalletStatus, &saga.NotificationStatus, &saga.OverallStatus, &saga.IdempotencyKey,
		&saga.Attempts, &saga.LastErrorCode, &saga.NextAttemptAt, &saga.Version,
		&saga.CreatedAt, &saga.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("providers: scan saga: %w", err)
	}
	return &saga, nil
}

// ---- Sub-wallet links ----

// EnsureSubWalletLink creates the sub-wallet projection row.
func (r *Repository) EnsureSubWalletLink(ctx context.Context, q database.Querier, organizationID string, membershipID string, idempotencyKey string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_subwallet_links
			(provider_membership_id, organization_id, status, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, 'REQUESTED', $3, now(), now())
		ON CONFLICT (provider_membership_id) DO NOTHING`,
		membershipID, organizationID, idempotencyKey)
	if err != nil {
		return fmt.Errorf("providers: ensure subwallet link: %w", err)
	}
	return nil
}

// SetSubWalletStatus updates the sub-wallet projection. The account ID is
// stored only from a confirmed response or reconciliation.
func (r *Repository) SetSubWalletStatus(ctx context.Context, q database.Querier, membershipID string, status string, accountID string, sourceVersion int64) error {
	var account *string
	if accountID != "" {
		account = &accountID
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_subwallet_links SET
			status = $2, wallet_account_id = COALESCE($3, wallet_account_id),
			source_version = GREATEST(source_version, $4), synced_at = now(), updated_at = now()
		WHERE provider_membership_id = $1`, membershipID, status, account, sourceVersion)
	if err != nil {
		return fmt.Errorf("providers: set subwallet status: %w", err)
	}
	return nil
}

// GetSubWalletLink loads the sub-wallet projection.
func (r *Repository) GetSubWalletLink(ctx context.Context, organizationID string, membershipID string) (*models.SubWalletLink, error) {
	var link models.SubWalletLink
	err := r.db.QueryRow(ctx, `
		SELECT provider_membership_id, organization_id, wallet_account_id, status, idempotency_key,
			source_service, source_version, last_event_id, synced_at, created_at, updated_at
		FROM fleet_partner_provider_subwallet_links
		WHERE organization_id = $1 AND provider_membership_id = $2`, organizationID, membershipID).
		Scan(&link.ProviderMembershipID, &link.OrganizationID, &link.WalletAccountID, &link.Status,
			&link.IdempotencyKey, &link.SourceService, &link.SourceVersion, &link.LastEventID,
			&link.SyncedAt, &link.CreatedAt, &link.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("providers: scan subwallet link: %w", err)
	}
	return &link, nil
}
