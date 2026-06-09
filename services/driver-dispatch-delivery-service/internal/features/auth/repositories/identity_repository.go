package authrepositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

type IdentityRepository interface {
	FindByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, bool, error)
	FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error)
	GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error)
	UpsertByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, error)
	// CreateForSignup inserts a new identity with the given phone and optional email.
	// Returns apperrors.Conflict if an identity with that phone (or email) already exists.
	CreateForSignup(ctx context.Context, phoneNumber, email string) (authmodels.Identity, error)
}

type PostgresIdentityRepository struct {
	db *pgxpool.Pool
}

func NewPostgresIdentityRepository(db *pgxpool.Pool) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{db: db}
}

func (r *PostgresIdentityRepository) FindByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, phone_number, email, status, created_at, updated_at
		FROM dispatch_rider_identities
		WHERE phone_number = $1
	`, phoneNumber)

	return scanOptionalIdentity(row)
}

func (r *PostgresIdentityRepository) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, phone_number, email, status, created_at, updated_at
		FROM dispatch_rider_identities
		WHERE email = $1
	`, email)

	return scanOptionalIdentity(row)
}

func (r *PostgresIdentityRepository) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, phone_number, email, status, created_at, updated_at
		FROM dispatch_rider_identities
		WHERE id = $1
	`, id)

	return scanOptionalIdentity(row)
}

func (r *PostgresIdentityRepository) UpsertByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_rider_identities (id, phone_number, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (phone_number) DO UPDATE
		SET updated_at = now()
		RETURNING id::text, phone_number, email, status, created_at, updated_at
	`, uuid.NewString(), phoneNumber, authmodels.StatusActive)

	return scanIdentity(row)
}

func (r *PostgresIdentityRepository) CreateForSignup(ctx context.Context, phoneNumber, email string) (authmodels.Identity, error) {
	var emailParam *string
	if email != "" {
		emailParam = &email
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, phone_number, email, status, created_at, updated_at
	`, uuid.NewString(), phoneNumber, emailParam, authmodels.StatusActive)

	identity, err := scanIdentity(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return authmodels.Identity{}, apperrors.Conflict("An account with this phone number or email already exists.", err)
		}
		return authmodels.Identity{}, err
	}
	return identity, nil
}

type identityRow interface {
	Scan(dest ...interface{}) error
}

func scanOptionalIdentity(row identityRow) (authmodels.Identity, bool, error) {
	identity, err := scanIdentity(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return authmodels.Identity{}, false, nil
	}
	if err != nil {
		return authmodels.Identity{}, false, err
	}

	return identity, true, nil
}

func scanIdentity(row identityRow) (authmodels.Identity, error) {
	var identity authmodels.Identity
	err := row.Scan(
		&identity.ID,
		&identity.PhoneNumber,
		&identity.Email,
		&identity.Status,
		&identity.CreatedAt,
		&identity.UpdatedAt,
	)
	if err != nil {
		return authmodels.Identity{}, err
	}

	return identity, nil
}
