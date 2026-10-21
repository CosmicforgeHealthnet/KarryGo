package providerapprovalrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerapprovalusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_approval/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
)

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{db: db} }

const driverQuery = `SELECT p.id::text, COALESCE(p.phone, ''), COALESCE(p.first_name, ''), COALESCE(p.last_name, ''),
	COALESCE(p.status, ''), COALESCE(p.onboarding_status, ''),
	(SELECT count(*) FROM trucks t WHERE t.provider_id = p.id AND t.status = 'active')::int
	FROM truck_providers p `

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func find(ctx context.Context, q querier, where string, arg string) (providerapprovalusecases.Driver, error) {
	var d providerapprovalusecases.Driver
	err := q.QueryRow(ctx, driverQuery+where, arg).Scan(&d.ID, &d.Phone, &d.FirstName, &d.LastName, &d.Status, &d.OnboardingStatus, &d.ActiveTrucks)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, apperrors.NotFound("Driver could not be found.", err)
	}
	return d, err
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (providerapprovalusecases.Driver, error) {
	return find(ctx, r.db, `WHERE p.id::text = $1`, id)
}

func (r *PostgresRepository) FindByPhone(ctx context.Context, phone string) (providerapprovalusecases.Driver, error) {
	return find(ctx, r.db, `WHERE p.phone = $1`, phone)
}

// Approve completes the driver's onboarding and, when asked, activates their
// trucks, in one transaction.
func (r *PostgresRepository) Approve(ctx context.Context, id string, activateTrucks bool) (providerapprovalusecases.Driver, error) {
	var d providerapprovalusecases.Driver
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE truck_providers SET onboarding_status = 'complete', updated_at = now() WHERE id::text = $1`, id); err != nil {
			return err
		}
		if activateTrucks {
			if _, err := tx.Exec(ctx, `UPDATE trucks SET status = 'active', updated_at = now() WHERE provider_id::text = $1 AND status <> 'active'`, id); err != nil {
				return err
			}
		}
		var err error
		d, err = find(ctx, tx, `WHERE p.id::text = $1`, id)
		return err
	})
	return d, err
}
