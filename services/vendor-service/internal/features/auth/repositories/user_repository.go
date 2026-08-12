// Package vendorauthrepositories implements Postgres persistence for the
// AUTHENTICATION domain (vendor_users, vendor_otp_challenges, vendor_sessions,
// vendor_step_up_grants). Every query is a single, small, explicit statement
// — no ORM, matching the platform-wide convention.
package vendorauthrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is the login identity record (vendor_users). Deliberately independent
// from customer-service/driver-service identity — see
// docs/architecture/01_SERVICE_BOUNDARIES.md.
type User struct {
	ID    string
	Phone *string
	Email *string
}

// ContactIdentifier returns (identifierType, identifierValue, identifierKey)
// for whichever contact this user was registered with — phone preferred over
// email when both are somehow present. Used for step-up OTP delivery, which
// must always target the already-known identity's own registered contact.
func (u User) ContactIdentifier() (string, string, string, error) {
	if u.Phone != nil && *u.Phone != "" {
		return "phone", *u.Phone, "phone:" + *u.Phone, nil
	}
	if u.Email != nil && *u.Email != "" {
		return "email", *u.Email, "email:" + *u.Email, nil
	}
	return "", "", "", fmt.Errorf("vendor user %s has no registered contact identifier", u.ID)
}

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// UpsertByPhone finds-or-creates a vendor_user by normalized phone.
func (r *UserRepository) UpsertByPhone(ctx context.Context, phone string) (User, error) {
	id := "vu_" + uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO vendor_users (id, phone) VALUES ($1, $2)
		ON CONFLICT (phone) WHERE phone IS NOT NULL DO UPDATE SET updated_at = now()
		RETURNING id, phone, email`, id, phone)
	return scanUser(row)
}

// UpsertByEmail finds-or-creates a vendor_user by lowercased email.
func (r *UserRepository) UpsertByEmail(ctx context.Context, email string) (User, error) {
	id := "vu_" + uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO vendor_users (id, email) VALUES ($1, $2)
		ON CONFLICT (email) WHERE email IS NOT NULL DO UPDATE SET updated_at = now()
		RETURNING id, phone, email`, id, email)
	return scanUser(row)
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (User, error) {
	row := r.db.QueryRow(ctx, `SELECT id, phone, email FROM vendor_users WHERE id = $1`, id)
	user, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("vendor user %s: %w", id, ErrNotFound)
	}
	return user, err
}

func scanUser(row pgx.Row) (User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Phone, &u.Email); err != nil {
		return User{}, err
	}
	return u, nil
}

// ErrNotFound is returned by any auth repository lookup that finds no row.
var ErrNotFound = errors.New("vendorauthrepositories: not found")
