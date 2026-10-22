package admincommands

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// blockingSQL: an UNSUITABLE decision or a PENDING remediation blocks that
// truck for that booking. With no truck fixed yet, any blocked truck of the
// provider blocks the provider for the booking (fail closed).
const blockingSQL = `SELECT EXISTS(SELECT 1 FROM hauling_cargo_suitability_decisions d JOIN trucks v ON v.id=d.truck_id
 WHERE d.booking_id::text=$1 AND (d.decision='UNSUITABLE' OR d.remediation_state='PENDING')
 AND (d.truck_id::text=$3 OR ($3='' AND v.provider_id::text=$2)))`

func blocked(ctx context.Context, q queryRower, bookingID, providerID, truckID string) (bool, error) {
	var out bool
	err := q.QueryRow(ctx, blockingSQL, bookingID, providerID, truckID).Scan(&out)
	return out, err
}

// SuitabilityGate is the booking matcher/accept gate over owner suitability
// decisions (bookingusecases.CargoSuitabilityGate).
type SuitabilityGate struct{ DB *pgxpool.Pool }

func (g SuitabilityGate) Blocked(ctx context.Context, bookingID, providerID, truckID string) (bool, error) {
	return blocked(ctx, g.DB, bookingID, providerID, truckID)
}
