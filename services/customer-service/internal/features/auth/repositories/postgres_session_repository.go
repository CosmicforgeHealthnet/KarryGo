package authrepositories

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "cosmicforge/logistics/services/customer-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == loginBlockedSQLState {
		// The account-standing guard refused a session for a login-blocked
		// customer that raced a concurrent administrative restriction.
		return AccountRestrictedError()
	}
	return err
}

// loginBlockedSQLState is raised by the customer_sessions account-standing
// guard (migration 019) when a session would be created for a customer whose
// administrative standing blocks login.
const loginBlockedSQLState = "KG403"

// AccountRestrictedError is the customer-facing refusal for a login-blocked
// account. It never discloses the administrative reason.
func AccountRestrictedError() error {
	return apperrors.New(http.StatusForbidden, "account_restricted", "This account is restricted. Please contact support.", nil)
}

// Execer is satisfied by a pgx transaction or pool.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// RevokeAllCustomerSessions revokes every still-active refresh session of the
// customer inside the caller's transaction and returns how many were revoked.
func RevokeAllCustomerSessions(ctx context.Context, db Execer, customerID string) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE customer_sessions
		SET revoked_at = now()
		WHERE customer_id = $1 AND revoked_at IS NULL
	`, customerID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
