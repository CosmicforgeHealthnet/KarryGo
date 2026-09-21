// Package supportcontext answers Support's question about a Taxi booking or a
// Taxi Goods mission: what state is it in, who is on it, is it in recovery?
//
// Taxi owns both; Support only ever sees this projection. No pickup, dropoff,
// route or location is read, no fare or quote, no receiver contact -- each
// would become visible to a support agent the moment it crossed this boundary.
package supportcontext

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/supportcase"
)

// Projector reads minimized context for Taxi bookings and goods missions.
type Projector struct {
	db *pgxpool.Pool
}

// NewProjector builds the projector.
func NewProjector(db *pgxpool.Pool) *Projector { return &Projector{db: db} }

// ProjectSupportContext implements supportcase.ContextProjector.
//
// Every query matches the reference AND the expected actor together, so a job
// the complainant is not party to answers exactly like one that does not
// exist.
func (p *Projector) ProjectSupportContext(ctx context.Context,
	request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
	switch request.ContextType {
	case supportcase.ContextTaxiBooking:
		return p.booking(ctx, request)
	case supportcase.ContextTaxiGoodsMission:
		return p.goodsMission(ctx, request)
	default:
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
}

func (p *Projector) booking(ctx context.Context,
	request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
	var (
		status, customerID, providerID, vehicleID, vehicleClass string
		createdAt, updatedAt                                    time.Time
		recoveryState, recoveryID                               string
	)
	err := p.db.QueryRow(ctx, `
		SELECT b.status, b.customer_id::text, COALESCE(b.provider_id::text,''),
		       COALESCE(b.vehicle_id::text,''), b.vehicle_class, b.created_at, b.updated_at,
		       COALESCE(r.state,''), COALESCE(r.id::text,'')
		  FROM taxi_bookings b
		  LEFT JOIN LATERAL (
		        SELECT state, id FROM taxi_recovery_cases
		         WHERE booking_id = b.id ORDER BY detected_at DESC LIMIT 1) r ON true
		 WHERE b.id::text = $1
		   AND (b.customer_id::text = $2 OR b.provider_id::text = $2)`,
		request.SourceRef, request.ExpectedActorRef).
		Scan(&status, &customerID, &providerID, &vehicleID, &vehicleClass, &createdAt, &updatedAt,
			&recoveryState, &recoveryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	if err != nil {
		return supportcase.OperationalContext{}, err
	}
	return assemble(status, customerID, providerID, vehicleID, vehicleClass,
		createdAt, updatedAt, recoveryState, recoveryID), nil
}

func (p *Projector) goodsMission(ctx context.Context,
	request supportcase.ContextRequest) (supportcase.OperationalContext, error) {
	var (
		status, customerRef, providerID, vehicleID string
		createdAt, updatedAt                       time.Time
		recoveryState, recoveryID                  string
	)
	err := p.db.QueryRow(ctx, `
		SELECT m.status, m.customer_ref, COALESCE(m.provider_id::text,''),
		       COALESCE(m.vehicle_id::text,''), m.created_at, m.updated_at,
		       COALESCE(r.state,''), COALESCE(r.id::text,'')
		  FROM taxi_goods_missions m
		  LEFT JOIN LATERAL (
		        SELECT state, id FROM taxi_recovery_cases
		         WHERE goods_mission_id = m.id ORDER BY detected_at DESC LIMIT 1) r ON true
		 WHERE m.mission_ref = $1
		   AND (m.customer_ref = $2 OR m.provider_id::text = $2)`,
		request.SourceRef, request.ExpectedActorRef).
		Scan(&status, &customerRef, &providerID, &vehicleID, &createdAt, &updatedAt,
			&recoveryState, &recoveryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportcase.OperationalContext{}, supportcase.ErrContextNotFound
	}
	if err != nil {
		return supportcase.OperationalContext{}, err
	}
	return assemble(status, customerRef, providerID, vehicleID, "",
		createdAt, updatedAt, recoveryState, recoveryID), nil
}

func assemble(status, customerRef, providerID, vehicleID, vehicleClass string,
	createdAt, updatedAt time.Time, recoveryState, recoveryID string) supportcase.OperationalContext {
	projected := supportcase.OperationalContext{
		SourceService: "driver-taxi-service",
		SourceVersion: updatedAt.UnixMicro(),
		State:         status,
		ActorRefs:     map[string]string{"customer_ref": customerRef},
		Milestones:    map[string]time.Time{"created_at": createdAt.UTC(), "updated_at": updatedAt.UTC()},
		ObservedAt:    time.Now().UTC(),
	}
	if providerID != "" {
		projected.ActorRefs["provider_ref"] = providerID
	}
	if vehicleID != "" || vehicleClass != "" {
		projected.Asset = &supportcase.AssetSummary{AssetRef: vehicleID, AssetClass: vehicleClass}
	}
	if recoveryID != "" {
		projected.RecoveryStatus = recoveryState
		projected.RecoveryCaseRef = recoveryID
	}
	return projected
}
