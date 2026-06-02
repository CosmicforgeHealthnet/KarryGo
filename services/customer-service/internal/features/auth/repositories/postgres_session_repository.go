package authrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "karrygo/services/customer-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

type RefreshSessionRepository interface {
	Create(ctx context.Context, session authmodels.RefreshSession) error
	GetByID(ctx context.Context, id string) (authmodels.RefreshSession, error)
	Revoke(ctx context.Context, id string) error
}

type PostgresRefreshSessionRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRefreshSessionRepository(db *pgxpool.Pool) *PostgresRefreshSessionRepository {
	return &PostgresRefreshSessionRepository{db: db}
}

func (r *PostgresRefreshSessionRepository) Create(ctx context.Context, session authmodels.RefreshSession) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO customer_sessions (
			id,
			customer_id,
			refresh_token_hash,
			device_id,
			user_agent,
			ip_address,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, session.ID, session.CustomerID, session.RefreshTokenHash, session.DeviceID, session.UserAgent, session.IPAddress, session.ExpiresAt)
	return err
}

func (r *PostgresRefreshSessionRepository) GetByID(ctx context.Context, id string) (authmodels.RefreshSession, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, customer_id::text, refresh_token_hash, device_id, user_agent, ip_address, expires_at, revoked_at, created_at
		FROM customer_sessions
		WHERE id = $1
	`, id)

	var session authmodels.RefreshSession
	err := row.Scan(
		&session.ID,
		&session.CustomerID,
		&session.RefreshTokenHash,
		&session.DeviceID,
		&session.UserAgent,
		&session.IPAddress,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.RefreshSession{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", err)
	}
	if err != nil {
		return authmodels.RefreshSession{}, err
	}

	return session, nil
}

func (r *PostgresRefreshSessionRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE customer_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1
	`, id)
	return err
}
