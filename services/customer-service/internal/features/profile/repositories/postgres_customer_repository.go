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
	UpdateProfilePhoto(ctx context.Context, id, assetID, photoURL string) (profilemodels.Customer, error)
	UpdateProfile(ctx context.Context, id, firstName, lastName string) (profilemodels.Customer, error)

	GetEmergencyContacts(ctx context.Context, customerID string) ([]profilemodels.EmergencyContact, error)
	AddEmergencyContact(ctx context.Context, customerID, name, phone, relationship string) (profilemodels.EmergencyContact, error)
	DeleteEmergencyContact(ctx context.Context, id, customerID string) error
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
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, profile_photo_url, profile_photo_asset_id, created_at, updated_at
	`, id, phone, profilemodels.OnboardingProfileNeeded, profilemodels.StatusActive)

	return scanCustomer(row)
}

func (r *PostgresCustomerRepository) UpsertByEmail(ctx context.Context, email string) (profilemodels.Customer, error) {
	id := uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO customers (id, email, onboarding_status, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, profile_photo_url, profile_photo_asset_id, created_at, updated_at
	`, id, email, profilemodels.OnboardingProfileNeeded, profilemodels.StatusActive)

	return scanCustomer(row)
}

func (r *PostgresCustomerRepository) GetByID(ctx context.Context, id string) (profilemodels.Customer, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, profile_photo_url, profile_photo_asset_id, created_at, updated_at
		FROM customers
		WHERE id = $1
	`, id)

	customer, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilemodels.Customer{}, apperrors.NotFound("Customer could not be found.", err)
	}
	return customer, err
}

func (r *PostgresCustomerRepository) UpdateProfilePhoto(ctx context.Context, id, assetID, photoURL string) (profilemodels.Customer, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE customers
		SET profile_photo_url = $2, profile_photo_asset_id = $3, updated_at = now()
		WHERE id = $1
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, profile_photo_url, profile_photo_asset_id, created_at, updated_at
	`, id, photoURL, assetID)

	customer, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilemodels.Customer{}, apperrors.NotFound("Customer could not be found.", err)
	}
	return customer, err
}

func (r *PostgresCustomerRepository) UpdateProfile(ctx context.Context, id, firstName, lastName string) (profilemodels.Customer, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE customers
		SET first_name = $2, last_name = $3,
		    onboarding_status = CASE WHEN onboarding_status = $4 THEN $5 ELSE onboarding_status END,
		    updated_at = now()
		WHERE id = $1
		RETURNING id::text, COALESCE(phone, ''), COALESCE(email, ''), first_name, last_name, onboarding_status, status, profile_photo_url, profile_photo_asset_id, created_at, updated_at
	`, id, firstName, lastName, profilemodels.OnboardingProfileNeeded, profilemodels.OnboardingComplete)

	customer, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return profilemodels.Customer{}, apperrors.NotFound("Customer could not be found.", err)
	}
	return customer, err
}

func (r *PostgresCustomerRepository) GetEmergencyContacts(ctx context.Context, customerID string) ([]profilemodels.EmergencyContact, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, customer_id::text, name, phone, relationship, created_at
		FROM emergency_contacts
		WHERE customer_id = $1
		ORDER BY created_at ASC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []profilemodels.EmergencyContact
	for rows.Next() {
		var ec profilemodels.EmergencyContact
		if err := rows.Scan(&ec.ID, &ec.CustomerID, &ec.Name, &ec.Phone, &ec.Relationship, &ec.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ec)
	}
	return result, rows.Err()
}

func (r *PostgresCustomerRepository) AddEmergencyContact(ctx context.Context, customerID, name, phone, relationship string) (profilemodels.EmergencyContact, error) {
	var ec profilemodels.EmergencyContact
	err := r.db.QueryRow(ctx, `
		INSERT INTO emergency_contacts (customer_id, name, phone, relationship)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, customer_id::text, name, phone, relationship, created_at
	`, customerID, name, phone, relationship).Scan(
		&ec.ID, &ec.CustomerID, &ec.Name, &ec.Phone, &ec.Relationship, &ec.CreatedAt,
	)
	return ec, err
}

func (r *PostgresCustomerRepository) DeleteEmergencyContact(ctx context.Context, id, customerID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM emergency_contacts WHERE id = $1 AND customer_id = $2
	`, id, customerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Emergency contact not found.", nil)
	}
	return nil
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
		&customer.ProfilePhotoURL,
		&customer.ProfilePhotoAssetID,
		&customer.CreatedAt,
		&customer.UpdatedAt,
	)
	if err != nil {
		return profilemodels.Customer{}, err
	}

	return customer, nil
}
