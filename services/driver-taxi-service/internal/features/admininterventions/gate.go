// Package admininterventions is the Taxi owner's administrative command
// executor (Admin Section 8) and the read-side gate the rest of the service
// consults. It owns incidents, canonical document decisions, reverification
// requests and provider/vehicle restrictions. It never changes trip status,
// completion, location or fares.
package admininterventions

import (
	"context"
	_ "time/tzdata" // canonical expiry needs Africa/Lagos on every host

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reasons returned by the gate. They are the exact tokens produced by the
// taxi_admin_online_block / taxi_admin_assignment_block SQL functions
// (migration 027), so there is a single definition of administrative
// eligibility shared by Go callers and SQL assignment paths.
const (
	ReasonProviderOnlineSuspended       = "provider_online_suspended"
	ReasonProviderReverificationPending = "provider_reverification_required"
	ReasonProviderDocumentInvalid       = "provider_document_invalid"
	ReasonVehicleOutOfService           = "vehicle_out_of_service"
	ReasonVehicleReverificationPending  = "vehicle_reverification_required"
	ReasonVehicleDocumentInvalid        = "vehicle_document_invalid"
	ReasonProviderNoNewAssignments      = "provider_no_new_assignments"
)

// Gate answers "may this provider (on this vehicle) be online / receive new
// work right now". An empty reason means permitted. Errors must be treated
// fail-closed by assignment callers.
type Gate struct{ DB *pgxpool.Pool }

func NewGate(db *pgxpool.Pool) *Gate { return &Gate{DB: db} }

// OnlineBlock reports why the provider may not be online. vehicleID may be
// empty when the vehicle is not yet known.
func (g *Gate) OnlineBlock(ctx context.Context, providerID, vehicleID string) (string, error) {
	var reason string
	err := g.DB.QueryRow(ctx, `SELECT COALESCE(taxi_admin_online_block($1,NULLIF($2,''),now()),'')`, providerID, vehicleID).Scan(&reason)
	return reason, err
}

// AssignmentBlock reports why the provider may not receive new offers or
// reservations. Everything that blocks going online also blocks assignment.
func (g *Gate) AssignmentBlock(ctx context.Context, providerID, vehicleID string) (string, error) {
	var reason string
	err := g.DB.QueryRow(ctx, `SELECT COALESCE(taxi_admin_assignment_block($1,NULLIF($2,''),now()),'')`, providerID, vehicleID).Scan(&reason)
	return reason, err
}
