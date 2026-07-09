package savedlocationrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type SavedLocationRepository interface {
	List(ctx context.Context, customerID string) ([]savedlocationmodels.SavedLocation, error)
	Create(ctx context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error)
	Update(ctx context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error)
	Delete(ctx context.Context, id, customerID string) error
}

type PostgresSavedLocationRepository struct {
	db *pgxpool.Pool
}

func NewPostgresSavedLocationRepository(db *pgxpool.Pool) *PostgresSavedLocationRepository {
	return &PostgresSavedLocationRepository{db: db}
}

const savedLocationColumns = `id::text, customer_id::text, title, address, latitude, longitude, description, created_at, updated_at`

func (r *PostgresSavedLocationRepository) List(ctx context.Context, customerID string) ([]savedlocationmodels.SavedLocation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+savedLocationColumns+`
		FROM saved_locations
		WHERE customer_id = $1
		ORDER BY created_at ASC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []savedlocationmodels.SavedLocation
	for rows.Next() {
		var l savedlocationmodels.SavedLocation
		if err := rows.Scan(&l.ID, &l.CustomerID, &l.Title, &l.Address, &l.Latitude, &l.Longitude, &l.Description, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

func (r *PostgresSavedLocationRepository) Create(ctx context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error) {
	var l savedlocationmodels.SavedLocation
	err := r.db.QueryRow(ctx, `
		INSERT INTO saved_locations (customer_id, title, address, latitude, longitude, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+savedLocationColumns+`
	`, location.CustomerID, location.Title, location.Address, location.Latitude, location.Longitude, location.Description).Scan(
		&l.ID, &l.CustomerID, &l.Title, &l.Address, &l.Latitude, &l.Longitude, &l.Description, &l.CreatedAt, &l.UpdatedAt,
	)
	return l, err
}

func (r *PostgresSavedLocationRepository) Update(ctx context.Context, location savedlocationmodels.SavedLocation) (savedlocationmodels.SavedLocation, error) {
	var l savedlocationmodels.SavedLocation
	err := r.db.QueryRow(ctx, `
		UPDATE saved_locations
		SET title = $3, address = $4, latitude = $5, longitude = $6, description = $7, updated_at = now()
		WHERE id = $1 AND customer_id = $2
		RETURNING `+savedLocationColumns+`
	`, location.ID, location.CustomerID, location.Title, location.Address, location.Latitude, location.Longitude, location.Description).Scan(
		&l.ID, &l.CustomerID, &l.Title, &l.Address, &l.Latitude, &l.Longitude, &l.Description, &l.CreatedAt, &l.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return savedlocationmodels.SavedLocation{}, apperrors.NotFound("Saved location not found.", err)
	}
	return l, err
}

func (r *PostgresSavedLocationRepository) Delete(ctx context.Context, id, customerID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM saved_locations WHERE id = $1 AND customer_id = $2
	`, id, customerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Saved location not found.", nil)
	}
	return nil
}
