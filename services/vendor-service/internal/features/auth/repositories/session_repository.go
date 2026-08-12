package vendorauthrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorauthmodels "cosmicforge/logistics/services/vendor-service/internal/features/auth/models"
)

type SessionRepository struct {
	db *pgxpool.Pool
}

func NewSessionRepository(db *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{db: db}
}

const sessionColumns = `id, staff_id, refresh_token_hash, device_id, user_agent, ip_address,
	expires_at, revoked_at, revoked_reason, created_at, last_used_at`

func (r *SessionRepository) Create(ctx context.Context, s vendorauthmodels.Session) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_sessions (id, staff_id, refresh_token_hash, device_id, user_agent, ip_address, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		s.ID, s.StaffID, s.RefreshTokenHash, s.DeviceID, s.UserAgent, s.IPAddress, s.ExpiresAt)
	return err
}

func (r *SessionRepository) GetByID(ctx context.Context, id string) (vendorauthmodels.Session, error) {
	row := r.db.QueryRow(ctx, `SELECT `+sessionColumns+` FROM vendor_sessions WHERE id = $1`, id)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return vendorauthmodels.Session{}, fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	return s, err
}

// Revoke marks a session revoked. Guarded: revoking an already-revoked
// session is a harmless no-op (RowsAffected 0), never an error — logout being
// called twice must not fail.
func (r *SessionRepository) Revoke(ctx context.Context, id string, reason string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_sessions SET revoked_at = now(), revoked_reason = $2
		WHERE id = $1 AND revoked_at IS NULL`, id, reason)
	return err
}

// RevokeAllForStaff revokes every active session for a staff member — used on
// suspension, so an already-issued refresh token cannot be used to keep a
// suspended staff member's access alive.
func (r *SessionRepository) RevokeAllForStaff(ctx context.Context, staffID string, reason string) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_sessions SET revoked_at = now(), revoked_reason = $2
		WHERE staff_id = $1 AND revoked_at IS NULL`, staffID, reason)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *SessionRepository) Touch(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE vendor_sessions SET last_used_at = now() WHERE id = $1`, id)
	return err
}

func scanSession(row pgx.Row) (vendorauthmodels.Session, error) {
	var s vendorauthmodels.Session
	err := row.Scan(&s.ID, &s.StaffID, &s.RefreshTokenHash, &s.DeviceID, &s.UserAgent, &s.IPAddress,
		&s.ExpiresAt, &s.RevokedAt, &s.RevokedReason, &s.CreatedAt, &s.LastUsedAt)
	return s, err
}
