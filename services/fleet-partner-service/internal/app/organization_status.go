package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
)

// OrganizationStatusFromPool reads an organization's lifecycle status for the
// tenant-route lifecycle gate. It is the authoritative row, not a cached or
// client-supplied value.
func OrganizationStatusFromPool(pool *pgxpool.Pool) middleware.OrganizationStatusFunc {
	return func(ctx context.Context, organizationID string) (string, error) {
		var status string
		err := pool.QueryRow(ctx,
			`SELECT status FROM fleet_partner_organizations WHERE id = $1`, organizationID).Scan(&status)
		return status, err
	}
}
