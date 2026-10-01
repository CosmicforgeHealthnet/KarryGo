package authrepositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type IdentityRepository interface {
	FindByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, bool, error)
	FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error)
	GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error)
	// UpsertByPhone creates (legacy flow) or returns the identity for a phone.
	// signup is announced to the referral programme only on creation.
	UpsertByPhone(ctx context.Context, phoneNumber string, signup authmodels.Signup) (authmodels.Identity, error)
	// CreateForSignup inserts a new identity with the given phone and optional email.
	// Returns apperrors.Conflict if an identity with that phone (or email) already exists.
	CreateForSignup(ctx context.Context, phoneNumber, email string, signup authmodels.Signup) (authmodels.Identity, error)
	// UpdatePhone atomically changes phone_number in dispatch_rider_identities and providers.
	// Returns apperrors.Conflict if newPhone is already in use.
	UpdatePhone(ctx context.Context, identityID, oldPhone, newPhone string) error
	// UpdateEmail sets the email on an existing identity.
	// Returns apperrors.Conflict if the email is already in use by another identity.
	UpdateEmail(ctx context.Context, identityID, email string) error
}

// SignupAnnouncer records a newly created rider for the referral programme
// inside the identity-creation transaction, so the account and its
// announcement commit together or not at all.
type SignupAnnouncer interface {
	AnnounceRider(ctx context.Context, tx pgx.Tx, identity authmodels.Identity, signup authmodels.Signup) error
}

type PostgresIdentityRepository struct {
	db        *pgxpool.Pool
	announcer SignupAnnouncer
}

func NewPostgresIdentityRepository(db *pgxpool.Pool) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{db: db}
}

// SetSignupAnnouncer switches the referral announcement on (non-nil) or off.
func (r *PostgresIdentityRepository) SetSignupAnnouncer(announcer SignupAnnouncer) {
	r.announcer = announcer
}

// announce runs inside the creating transaction.
func (r *PostgresIdentityRepository) announce(ctx context.Context, tx pgx.Tx, identity authmodels.Identity, signup authmodels.Signup) error {
	if r.announcer == nil {
		return nil
	}
	return r.announcer.AnnounceRider(ctx, tx, identity, signup)
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

func (r *PostgresIdentityRepository) UpsertByPhone(ctx context.Context, phoneNumber string, signup authmodels.Signup) (authmodels.Identity, error) {
	id := uuid.NewString()
	var identity authmodels.Identity
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO dispatch_rider_identities (id, phone_number, status)
			VALUES ($1, $2, $3)
			ON CONFLICT (phone_number) DO UPDATE
			SET updated_at = now()
			RETURNING id::text, phone_number, email, status, created_at, updated_at
		`, id, phoneNumber, authmodels.StatusActive)
		var err error
		if identity, err = scanIdentity(row); err != nil {
			return err
		}
		// The returned id is the one generated here only when the row was
		// inserted; a returning rider gets their existing id back.
		if identity.ID == id {
			return r.announce(ctx, tx, identity, signup)
		}
		return nil
	})
	return identity, err
}

func (r *PostgresIdentityRepository) CreateForSignup(ctx context.Context, phoneNumber, email string, signup authmodels.Signup) (authmodels.Identity, error) {
	var emailParam *string
	if email != "" {
		emailParam = &email
	}

	var identity authmodels.Identity
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO dispatch_rider_identities (id, phone_number, email, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id::text, phone_number, email, status, created_at, updated_at
		`, uuid.NewString(), phoneNumber, emailParam, authmodels.StatusActive)
		var err error
		if identity, err = scanIdentity(row); err != nil {
			return err
		}
		return r.announce(ctx, tx, identity, signup)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return authmodels.Identity{}, apperrors.Conflict("An account with this phone number or email already exists.", err)
		}
		return authmodels.Identity{}, err
	}
	return identity, nil
}

func (r *PostgresIdentityRepository) UpdatePhone(ctx context.Context, identityID, oldPhone, newPhone string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE dispatch_rider_identities
		SET phone_number = $3,
			updated_at   = now()
		WHERE id = $1
		  AND phone_number = $2
	`, identityID, oldPhone, newPhone)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.Conflict("This phone number is already in use.", err)
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.Conflict("Phone number did not match current record.", nil)
	}

	tag, err = tx.Exec(ctx, `
		UPDATE providers
		SET phone      = $2,
			updated_at = now()
		WHERE id = $1
	`, identityID, newPhone)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Provider was not found.", nil)
	}

	return tx.Commit(ctx)
}

func (r *PostgresIdentityRepository) UpdateEmail(ctx context.Context, identityID, email string) error {
	var emailParam *string
	if email != "" {
		emailParam = &email
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_rider_identities
		SET email      = $2,
		    updated_at = now()
		WHERE id = $1
	`, identityID, emailParam)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.Conflict("This email address is already in use.", err)
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Identity not found.", nil)
	}
	return nil
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
