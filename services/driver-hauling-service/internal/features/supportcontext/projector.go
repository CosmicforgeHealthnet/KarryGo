// Package supportcontext answers Support's question about a haulage booking:
// what state is it in, who is on it, and is it in recovery?
//
// Hauling owns the booking; Support only ever sees this projection. What is
// deliberately NOT read here is as important as what is: no pickup or dropoff
// coordinates, no address, no location history, no fare, no receiver contact.
// Every one of those would become visible to a support agent the moment it
// crossed this boundary.
package supportcontext

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/supportcase"
)

// Projector reads the minimized context for one haulage booking.
type Projector struct {
	db *pgxpool.Pool
}

// NewProjector builds the projector.
func NewProjector(db *pgxpool.Pool) *Projector { return &Projector{db: db} }

// ProjectSupportContext implements supportcase.ContextProjector.
//
// The booking is matched on BOTH its id and the expected actor, in one query.
// A booking that exists but belongs to somebody else therefore returns exactly
// what a booking that does not exist returns, and a complaint cannot be used to
// confirm that somebody else's booking exists.
func (p *Projector) ProjectSupportContext(ctx context.Context,
	request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
	if request.ContextType != supportcase.ContextHaulageBooking {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	var (
		status, customerID, providerID, truckID, truckClass string
		createdAt, updatedAt                                time.Time
		recoveryState, recoveryID, triggerCode              string
	)
	err := p.db.QueryRow(ctx, `
		SELECT b.status, b.customer_id::text, COALESCE(b.provider_id::text,''),
		       COALESCE(b.truck_id::text,''), COALESCE(NULLIF(b.vehicle_class,''), b.preferred_truck_type),
		       b.created_at, b.updated_at,
		       COALESCE(r.state,''), COALESCE(r.id::text,''), COALESCE(r.trigger_code,'')
		  FROM haulage_bookings b
		  LEFT JOIN LATERAL (
		        SELECT state, id, trigger_code FROM hauling_recovery_cases
		         WHERE booking_id = b.id ORDER BY created_at DESC LIMIT 1) r ON true
		 WHERE b.id::text = $1
		   AND (b.customer_id::text = $2 OR b.provider_id::text = $2)`,
		request.SourceRef, request.ExpectedActorRef).
		Scan(&status, &customerID, &providerID, &truckID, &truckClass, &createdAt, &updatedAt,
			&recoveryState, &recoveryID, &triggerCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	if err != nil {
		return supportcase.OperationalContext{}, err
	}

	projected := supportcase.OperationalContext{
		SourceService: "driver-hauling-service",
		// updated_at in microseconds is monotonic for a row, which is all a
		// snapshot's source version needs: a later snapshot is never older.
		SourceVersion: updatedAt.UnixMicro(),
		State:         status,
		ActorRefs:     map[string]string{"customer_ref": customerID},
		Milestones:    map[string]time.Time{"created_at": createdAt.UTC(), "updated_at": updatedAt.UTC()},
		ObservedAt:    time.Now().UTC(),
	}
	if providerID != "" {
		projected.ActorRefs["provider_ref"] = providerID
	}
	if truckID != "" || truckClass != "" {
		projected.Asset = &supportcase.AssetSummary{AssetRef: truckID, AssetClass: truckClass}
	}
	if recoveryID != "" {
		projected.RecoveryStatus = recoveryState
		projected.RecoveryCaseRef = recoveryID
		if triggerCode != "" {
			projected.GuaranteeStatus = "RECOVERY_TRIGGERED:" + triggerCode
		}
	}
	return projected, nil
}
