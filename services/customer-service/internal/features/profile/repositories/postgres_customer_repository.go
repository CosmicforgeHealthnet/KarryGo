package profilerepositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type CustomerRepository interface {
	UpsertByPhone(ctx context.Context, phone string) (profilemodels.Customer, error)
	UpsertByEmail(ctx context.Context, email string) (profilemodels.Customer, error)
	GetByID(ctx context.Context, id string) (profilemodels.Customer, error)
}

type PostgresCustomerRepository struct {
	db *pgxpool.Pool
}

func NewPostgresCustomerRepository(db *pgxpool.Pool) *PostgresCustomerRepository {
	return &PostgresCustomerRepository{db: db}
}

func (r *PostgresCustomerRepository) UpsertByPhone(ctx context.Context, phone string) (profilemodels.Customer, error) {
	id := uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO customers (id, phone, onboarding_status, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (phone) DO UPDATE SET updated_at = now()
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, created_at, updated_at
	`, id, phone, profilemodels.OnboardingProfileNeeded, profilemodels.StatusActive)

	return scanCustomer(row)
}

func (r *PostgresCustomerRepository) UpsertByEmail(ctx context.Context, email string) (profilemodels.Customer, error) {
	id := uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO customers (id, email, onboarding_status, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, created_at, updated_at
	`, id, email, profilemodels.OnboardingProfileNeeded, profilemodels.StatusActive)

	return scanCustomer(row)
}

func (r *PostgresCustomerRepository) GetByID(ctx context.Context, id string) (profilemodels.Customer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, created_at, updated_at
		FROM customers
		WHERE id = $1
	`, id)

	customer, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilemodels.Customer{}, apperrors.NotFound("Customer could not be found.", err)
	}
	return customer, err
}

type customerRow interface {
	Scan(dest ...interface{}) error
}

func scanCustomer(row customerRow) (profilemodels.Customer, error) {
	var customer profilemodels.Customer
	err := row.Scan(
		&customer.ID,
		&customer.Phone,
		&customer.Email,
		&customer.FirstName,
		&customer.LastName,
		&customer.OnboardingStatus,
		&customer.Status,
		&customer.CreatedAt,
		&customer.UpdatedAt,
	)
	if err != nil {
		return profilemodels.Customer{}, err
	}

	return customer, nil
}
