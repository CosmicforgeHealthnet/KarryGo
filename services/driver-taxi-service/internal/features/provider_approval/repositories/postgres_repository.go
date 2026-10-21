package providerapprovalrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerapprovalusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_approval/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
)

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{db: db} }

const driverColumns = `id::text, COALESCE(phone, ''), first_name, last_name, account_status, onboarding_status,
	identity_verification_status, document_compliance_status, vehicle_verification_status, vehicle_operational_status`

func scan(row pgx.Row) (providerapprovalusecases.Driver, error) {
	var d providerapprovalusecases.Driver
	err := row.Scan(&d.ID, &d.Phone, &d.FirstName, &d.LastName, &d.AccountStatus, &d.OnboardingStatus,
		&d.IdentityStatus, &d.DocumentStatus, &d.VehicleVerificationStatus, &d.VehicleOperationalStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, apperrors.NotFound("Driver could not be found.", err)
	}
	return d, err
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (providerapprovalusecases.Driver, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+driverColumns+` FROM taxi_providers WHERE id::text = $1`, id))
}

func (r *PostgresRepository) FindByPhone(ctx context.Context, phone string) (providerapprovalusecases.Driver, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+driverColumns+` FROM taxi_providers WHERE phone = $1`, phone))
}

// Approve marks identity, documents and the car verified. The car counts as
// operational only when the driver has an active vehicle.
func (r *PostgresRepository) Approve(ctx context.Context, id string) (providerapprovalusecases.Driver, error) {
	return scan(r.db.QueryRow(ctx, `
		UPDATE taxi_providers SET
		    identity_verification_status = 'verified',
		    document_compliance_status   = 'compliant',
		    vehicle_verification_status  = 'verified',
		    vehicle_operational_status   = CASE
		        WHEN EXISTS (SELECT 1 FROM vehicles v WHERE v.provider_id = taxi_providers.id AND v.status = 'active')
		        THEN 'active' ELSE vehicle_operational_status END,
		    updated_at = now()
		WHERE id::text = $1
		RETURNING `+driverColumns, id))
}
