package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// SessionRepository stores corporate sessions and their rotating refresh
// tokens. Only refresh-token hashes are persisted.
type SessionRepository struct {
	db database.Querier
}

// NewSessionRepository wires the repository.
func NewSessionRepository(db database.Querier) *SessionRepository {
	return &SessionRepository{db: db}
}

const sessionColumns = `id, identity_id, organization_user_id, organization_id, session_family_id,
	refresh_token_hash, status, access_issued_at, refresh_expires_at, last_seen_at,
	ip_hash, user_agent_hash, device_label, role_version, revoked_at, revocation_reason,
	created_at, updated_at`

// Create inserts a session.
func (r *SessionRepository) Create(ctx context.Context, q database.Querier, session *models.Session) error {
	if session.ID == "" {
		session.ID = ids.New("ses")
	}
	if session.SessionFamilyID == "" {
		session.SessionFamilyID = ids.New("sfm")
	}
	if session.Status == "" {
		session.Status = models.SessionStatusActive
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_corporate_sessions
			(id, identity_id, organization_user_id, organization_id, session_family_id,
			 refresh_token_hash, status, access_issued_at, refresh_expires_at, last_seen_at,
			 ip_hash, user_agent_hash, device_label, role_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8, now(), $9, $10, $11, $12, now(), now())`,
		session.ID, session.IdentityID, session.OrganizationUserID, session.OrganizationID,
		session.SessionFamilyID, session.RefreshTokenHash, session.Status, session.RefreshExpiresAt,
		session.IPHash, session.UserAgentHash, session.DeviceLabel, session.RoleVersion,
	)
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

// GetByID loads one session.
func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (*models.Session, error) {
	row := r.db.QueryRow(ctx, `SELECT `+sessionColumns+`
		FROM fleet_partner_corporate_sessions WHERE id = $1`, sessionID)
	return scanSession(row)
}

// LockByRefreshHash loads a session by refresh-token hash FOR UPDATE. It
// matches revoked and compromised sessions too, because finding a revoked
// session by its hash is exactly how token reuse is detected.
func (r *SessionRepository) LockByRefreshHash(ctx context.Context, q database.Querier, refreshHash string) (*models.Session, error) {
	row := q.QueryRow(ctx, `SELECT `+sessionColumns+`
		FROM fleet_partner_corporate_sessions WHERE refresh_token_hash = $1 FOR UPDATE`, refreshHash)
	return scanSession(row)
}

// ListForIdentity returns the identity's own sessions, newest first.
func (r *SessionRepository) ListForIdentity(ctx context.Context, identityID string) ([]models.Session, error) {
	rows, err := r.db.Query(ctx, `SELECT `+sessionColumns+`
		FROM fleet_partner_corporate_sessions
		WHERE identity_id = $1
		ORDER BY created_at DESC, id
		LIMIT 100`, identityID)
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		session, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *session)
	}
	return sessions, rows.Err()
}

// RotateRefresh retires the current session row and issues a successor in the
// same token family, returning the new session.
//
// The old row is kept (REVOKED, reason "rotated") rather than having its hash
// overwritten, because that retired hash is exactly what makes refresh-token
// reuse detectable: replaying a spent token still finds a row, and finding a
// non-ACTIVE row is the reuse signal.
//
// The retire step is conditional on the presented hash, so two concurrent
// refreshes cannot both succeed — the loser sees zero rows affected.
func (r *SessionRepository) RotateRefresh(ctx context.Context, q database.Querier, current *models.Session, nextHash string, refreshExpiresAt time.Time) (*models.Session, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'REVOKED', revoked_at = now(), revocation_reason = 'rotated', updated_at = now()
		WHERE id = $1 AND refresh_token_hash = $2 AND status = 'ACTIVE'`,
		current.ID, current.RefreshTokenHash)
	if err != nil {
		return nil, fmt.Errorf("auth: retire rotated session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrSessionNotRotatable
	}

	successor := &models.Session{
		ID:                 ids.New("ses"),
		IdentityID:         current.IdentityID,
		OrganizationUserID: current.OrganizationUserID,
		OrganizationID:     current.OrganizationID,
		// Same family: revoking the family on reuse kills every descendant.
		SessionFamilyID:  current.SessionFamilyID,
		RefreshTokenHash: nextHash,
		Status:           models.SessionStatusActive,
		RefreshExpiresAt: refreshExpiresAt,
		IPHash:           current.IPHash,
		UserAgentHash:    current.UserAgentHash,
		DeviceLabel:      current.DeviceLabel,
		RoleVersion:      current.RoleVersion,
	}
	if err := r.Create(ctx, q, successor); err != nil {
		return nil, err
	}
	return successor, nil
}

// ErrSessionNotRotatable means the session was not active or the presented
// refresh hash was already rotated away.
var ErrSessionNotRotatable = errors.New("auth: session cannot be rotated")

// Revoke revokes a single session. Idempotent: revoking an already-revoked
// session is a no-op success.
func (r *SessionRepository) Revoke(ctx context.Context, q database.Querier, sessionID string, reason string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'REVOKED', revoked_at = now(), revocation_reason = $2, updated_at = now()
		WHERE id = $1 AND status = 'ACTIVE'`, sessionID, reason)
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// RevokeFamily revokes every session in a token family and marks them
// compromised. Used when refresh-token reuse is detected.
func (r *SessionRepository) RevokeFamily(ctx context.Context, q database.Querier, familyID string, reason string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'COMPROMISED', revoked_at = now(), revocation_reason = $2, updated_at = now()
		WHERE session_family_id = $1 AND status IN ('ACTIVE', 'REVOKED')`, familyID, reason)
	if err != nil {
		return 0, fmt.Errorf("auth: revoke family: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RevokeForIdentity revokes every active session of an identity.
func (r *SessionRepository) RevokeForIdentity(ctx context.Context, q database.Querier, identityID string, reason string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'REVOKED', revoked_at = now(), revocation_reason = $2, updated_at = now()
		WHERE identity_id = $1 AND status = 'ACTIVE'`, identityID, reason)
	if err != nil {
		return 0, fmt.Errorf("auth: revoke identity sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RevokeForOrganizationUser revokes the organization-scoped sessions of one
// membership — used when staff are suspended or ended.
func (r *SessionRepository) RevokeForOrganizationUser(ctx context.Context, q database.Querier, organizationUserID string, reason string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'REVOKED', revoked_at = now(), revocation_reason = $2, updated_at = now()
		WHERE organization_user_id = $1 AND status = 'ACTIVE'`, organizationUserID, reason)
	if err != nil {
		return 0, fmt.Errorf("auth: revoke membership sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RevokeForOrganization revokes every active session scoped to an
// organization — used on suspension and closure.
func (r *SessionRepository) RevokeForOrganization(ctx context.Context, q database.Querier, organizationID string, reason string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET
			status = 'REVOKED', revoked_at = now(), revocation_reason = $2, updated_at = now()
		WHERE organization_id = $1 AND status = 'ACTIVE'`, organizationID, reason)
	if err != nil {
		return 0, fmt.Errorf("auth: revoke organization sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExpireDue marks overdue active sessions expired; used by the worker.
func (r *SessionRepository) ExpireDue(ctx context.Context, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_corporate_sessions SET status = 'EXPIRED', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_corporate_sessions
			WHERE status = 'ACTIVE' AND refresh_expires_at < now()
			LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("auth: expire sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AppendHistory writes an append-only session lifecycle record.
func (r *SessionRepository) AppendHistory(ctx context.Context, q database.Querier, sessionID string, identityID string, fromStatus string, toStatus string, reason string, requestID string, correlationID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_corporate_session_history
			(id, session_id, identity_id, from_status, to_status, reason, request_id, correlation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
		ids.New("sha"), sessionID, identityID, fromStatus, toStatus, reason, requestID, correlationID)
	if err != nil {
		return fmt.Errorf("auth: append session history: %w", err)
	}
	return nil
}

func scanSession(row pgx.Row) (*models.Session, error) {
	var session models.Session
	err := row.Scan(
		&session.ID, &session.IdentityID, &session.OrganizationUserID, &session.OrganizationID,
		&session.SessionFamilyID, &session.RefreshTokenHash, &session.Status,
		&session.AccessIssuedAt, &session.RefreshExpiresAt, &session.LastSeenAt,
		&session.IPHash, &session.UserAgentHash, &session.DeviceLabel, &session.RoleVersion,
		&session.RevokedAt, &session.RevocationReason, &session.CreatedAt, &session.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: scan session: %w", err)
	}
	return &session, nil
}

func scanSessionRows(rows pgx.Rows) (*models.Session, error) {
	var session models.Session
	err := rows.Scan(
		&session.ID, &session.IdentityID, &session.OrganizationUserID, &session.OrganizationID,
		&session.SessionFamilyID, &session.RefreshTokenHash, &session.Status,
		&session.AccessIssuedAt, &session.RefreshExpiresAt, &session.LastSeenAt,
		&session.IPHash, &session.UserAgentHash, &session.DeviceLabel, &session.RoleVersion,
		&session.RevokedAt, &session.RevocationReason, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: scan session: %w", err)
	}
	return &session, nil
}
