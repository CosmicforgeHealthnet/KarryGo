// Package supportcontext answers Support's question about a Dispatch delivery:
// what state is it in, who is on it?
//
// Dispatch owns the booking and its trip; Support only ever sees this
// projection. The trip and booking rows carry pickup and dropoff coordinates,
// addresses, receiver contact details and the fare -- none of it is selected
// here, because every one of those would become visible to a support agent
// the moment it crossed this boundary.
package supportcontext

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/supportcase"
)

// Projector reads minimized context for a Dispatch delivery.
type Projector struct {
	db *pgxpool.Pool
}

// NewProjector builds the projector.
func NewProjector(db *pgxpool.Pool) *Projector { return &Projector{db: db} }

// ProjectSupportContext implements supportcase.ContextProjector.
//
// The reference is the customer's booking id -- what a customer complaint
// names. When a trip exists its status wins, because the trip is what is
// actually happening; the booking status alone would say "accepted" long
// after the rider arrived. The actor must be the booking's customer or the
// trip's provider, matched in the same query.
func (p *Projector) ProjectSupportContext(ctx context.Context,
	request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
	if request.ContextType != supportcase.ContextDispatchMission {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	var (
		bookingStatus, tripStatus, customerID, providerID string
		createdAt, updatedAt                              time.Time
		startedAt, arrivedAt, completedAt, cancelledAt    *time.Time
	)
	err := p.db.QueryRow(ctx, `
		SELECT b.status, COALESCE(t.status,''), b.customer_id::text,
		       COALESCE(t.provider_id::text, b.provider_id::text, ''),
		       b.created_at, GREATEST(b.updated_at, COALESCE(t.updated_at, b.updated_at)),
		       t.started_at, t.arrived_at, t.completed_at, COALESCE(t.cancelled_at, b.cancelled_at)
		  FROM dispatch_bookings b
		  LEFT JOIN trips t ON t.booking_id = b.id
		 WHERE b.id::text = $1
		   AND (b.customer_id::text = $2 OR t.provider_id::text = $2 OR b.provider_id::text = $2)`,
		request.SourceRef, request.ExpectedActorRef).
		Scan(&bookingStatus, &tripStatus, &customerID, &providerID, &createdAt, &updatedAt,
			&startedAt, &arrivedAt, &completedAt, &cancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	if err != nil {
		return supportcase.OperationalContext{}, err
	}

	state := bookingStatus
	if tripStatus != "" {
		state = tripStatus
	}
	projected := supportcase.OperationalContext{
		SourceService: "driver-dispatch-delivery-service",
		SourceVersion: updatedAt.UnixMicro(),
		State:         state,
		ActorRefs:     map[string]string{"customer_ref": customerID},
		Milestones:    map[string]time.Time{"created_at": createdAt.UTC(), "updated_at": updatedAt.UTC()},
		ObservedAt:    time.Now().UTC(),
	}
	if providerID != "" {
		projected.ActorRefs["provider_ref"] = providerID
	}
	for name, value := range map[string]*time.Time{"started_at": startedAt, "arrived_at": arrivedAt,
		"completed_at": completedAt, "cancelled_at": cancelledAt} {
		if value != nil {
			projected.Milestones[name] = value.UTC()
		}
	}
	return projected, nil
}
