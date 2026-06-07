package authrepositories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
)

type OTPRepository interface {
	Create(ctx context.Context, otp authmodels.OTP) (authmodels.OTP, error)
	LatestByPhone(ctx context.Context, phoneNumber string) (authmodels.OTP, bool, error)
	MarkVerified(ctx context.Context, id string) error
	RecordFailedAttempt(ctx context.Context, id string, attempts int, lockedUntil *time.Time) error
}

type PostgresOTPRepository struct {
	db *pgxpool.Pool
}

func NewPostgresOTPRepository(db *pgxpool.Pool) *PostgresOTPRepository {
	return &PostgresOTPRepository{db: db}
}

func (r *PostgresOTPRepository) Create(ctx context.Context, otp authmodels.OTP) (authmodels.OTP, error) {
	if otp.ID == "" {
		otp.ID = uuid.NewString()
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_rider_otps (
			id,
			phone_number,
			otp_code_hash,
			attempts,
			max_attempts,
			expires_at,
			verified,
			locked_until
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, phone_number, otp_code_hash, attempts, max_attempts, expires_at, verified, locked_until, created_at, updated_at
	`, otp.ID, otp.PhoneNumber, otp.OTPCodeHash, otp.Attempts, otp.MaxAttempts, otp.ExpiresAt, otp.Verified, otp.LockedUntil)

	return scanOTP(row)
}

func (r *PostgresOTPRepository) LatestByPhone(ctx context.Context, phoneNumber string) (authmodels.OTP, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, phone_number, otp_code_hash, attempts, max_attempts, expires_at, verified, locked_until, created_at, updated_at
		FROM dispatch_rider_otps
		WHERE phone_number = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, phoneNumber)

	otp, err := scanOTP(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.OTP{}, false, nil
	}
	if err != nil {
		return authmodels.OTP{}, false, err
	}

	return otp, true, nil
}

func (r *PostgresOTPRepository) MarkVerified(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dispatch_rider_otps
		SET verified = true,
			updated_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (r *PostgresOTPRepository) RecordFailedAttempt(ctx context.Context, id string, attempts int, lockedUntil *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dispatch_rider_otps
		SET attempts = $2,
			locked_until = $3,
			updated_at = now()
		WHERE id = $1
	`, id, attempts, lockedUntil)
	return err
}

type otpRow interface {
	Scan(dest ...interface{}) error
}

func scanOTP(row otpRow) (authmodels.OTP, error) {
	var otp authmodels.OTP
	var lockedUntil sql.NullTime
	err := row.Scan(
		&otp.ID,
		&otp.PhoneNumber,
		&otp.OTPCodeHash,
		&otp.Attempts,
		&otp.MaxAttempts,
		&otp.ExpiresAt,
		&otp.Verified,
		&lockedUntil,
		&otp.CreatedAt,
		&otp.UpdatedAt,
	)
	if err != nil {
		return authmodels.OTP{}, err
	}

	if lockedUntil.Valid {
		otp.LockedUntil = &lockedUntil.Time
	}

	return otp, nil
}
