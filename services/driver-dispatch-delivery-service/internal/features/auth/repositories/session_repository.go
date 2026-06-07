package authrepositories

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionRepository interface {
	Create(ctx context.Context, session authmodels.Session) (authmodels.Session, error)
	FindByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (authmodels.Session, bool, error)
	GetByID(ctx context.Context, id string) (authmodels.Session, bool, error)
	RotateRefreshToken(ctx context.Context, id string, refreshTokenHash string) error
	Revoke(ctx context.Context, id string) error
}

type PostgresSessionRepository struct {
	db *pgxpool.Pool
}

func NewPostgresSessionRepository(db *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

func (r *PostgresSessionRepository) Create(ctx context.Context, session authmodels.Session) (authmodels.Session, error) {
	if session.ID == "" {
		session.ID = uuid.NewString()
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_rider_sessions (
			id,
			dispatch_rider_id,
			phone_number,
			refresh_token_hash,
			device_id,
			device_type,
			ip_address,
			user_agent,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text, dispatch_rider_id::text, phone_number, refresh_token_hash, device_id, device_type, ip_address, user_agent, expires_at, revoked_at, created_at, updated_at
	`, session.ID, session.DispatchRiderID, session.PhoneNumber, session.RefreshTokenHash, session.DeviceID, session.DeviceType, session.IPAddress, session.UserAgent, session.ExpiresAt)

	return scanSession(row)
}

func (r *PostgresSessionRepository) FindByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (authmodels.Session, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, dispatch_rider_id::text, phone_number, refresh_token_hash, device_id, device_type, ip_address, user_agent, expires_at, revoked_at, created_at, updated_at
		FROM dispatch_rider_sessions
		WHERE refresh_token_hash = $1
		  AND revoked_at IS NULL
		LIMIT 1
	`, refreshTokenHash)

	return scanOptionalSession(row)
}

func (r *PostgresSessionRepository) GetByID(ctx context.Context, id string) (authmodels.Session, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, dispatch_rider_id::text, phone_number, refresh_token_hash, device_id, device_type, ip_address, user_agent, expires_at, revoked_at, created_at, updated_at
		FROM dispatch_rider_sessions
		WHERE id = $1
	`, id)

	return scanOptionalSession(row)
}

func (r *PostgresSessionRepository) RotateRefreshToken(ctx context.Context, id string, refreshTokenHash string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_rider_sessions
		SET refresh_token_hash = $2,
			updated_at = now()
		WHERE id = $1
		  AND revoked_at IS NULL
	`, id, refreshTokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

func (r *PostgresSessionRepository) Revoke(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_rider_sessions
		SET revoked_at = now(),
			updated_at = now()
		WHERE id = $1
		  AND revoked_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

type sessionRow interface {
	Scan(dest ...interface{}) error
}

func scanOptionalSession(row sessionRow) (authmodels.Session, bool, error) {
	session, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.Session{}, false, nil
	}
	if err != nil {
		return authmodels.Session{}, false, err
	}

	return session, true, nil
}

func scanSession(row sessionRow) (authmodels.Session, error) {
	var session authmodels.Session
	var deviceID sql.NullString
	var deviceType sql.NullString
	var ipAddress sql.NullString
	var userAgent sql.NullString
	var revokedAt sql.NullTime

	err := row.Scan(
		&session.ID,
		&session.DispatchRiderID,
		&session.PhoneNumber,
		&session.RefreshTokenHash,
		&deviceID,
		&deviceType,
		&ipAddress,
		&userAgent,
		&session.ExpiresAt,
		&revokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return authmodels.Session{}, err
	}

	if deviceID.Valid {
		session.DeviceID = &deviceID.String
	}
	if deviceType.Valid {
		session.DeviceType = &deviceType.String
	}
	if ipAddress.Valid {
		session.IPAddress = ipAddress.String
	}
	if userAgent.Valid {
		session.UserAgent = userAgent.String
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}

	return session, nil
}
