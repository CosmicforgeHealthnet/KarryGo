package assignment

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

// CanonicalTripAdapter reuses the existing Request -> Trip authority. The
// accepted offer is durable before this saga step; retries read the same
// immutable booking payload, and trips.booking_id uniqueness makes creation
// exactly-once from the domain's perspective.
type CanonicalTripAdapter struct {
	db    *pgxpool.Pool
	trips *tripfeature.Service
	links *Repository
}

func NewCanonicalTripAdapter(db *pgxpool.Pool, trips *tripfeature.Service, links *Repository) *CanonicalTripAdapter {
	return &CanonicalTripAdapter{db: db, trips: trips, links: links}
}

func (a *CanonicalTripAdapter) EnsureTrip(ctx context.Context, missionID, providerID string) (string, error) {
	if a == nil || a.trips == nil {
		return "", errors.New("canonical trip authority unavailable")
	}
	var bookingID string
	var payload []byte
	err := a.db.QueryRow(ctx, `SELECT m.booking_id::text,b.booking_payload FROM dispatch_delivery_missions m JOIN request_broadcasts b ON b.booking_id=m.booking_id WHERE m.id=$1`, missionID).Scan(&bookingID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("mission has no canonical booking request")
	}
	if err != nil {
		return "", err
	}
	var source requestfeature.BookingDispatchCreatedEvent
	if err = json.Unmarshal(payload, &source); err != nil {
		return "", err
	}
	trip, err := a.trips.HandleRequestAccepted(ctx, tripfeature.RequestAcceptedEvent{
		Event: tripfeature.TopicRequestAccepted, CorrelationID: source.CorrelationID, BookingID: bookingID, ProviderID: providerID, CustomerID: source.CustomerID,
		PickupAddress: source.PickupAddress, PickupLat: source.PickupLat, PickupLng: source.PickupLng, DropoffAddress: source.DropoffAddress, DropoffLat: source.DropoffLat, DropoffLng: source.DropoffLng,
		FareAmount: source.FareAmount, Currency: source.Currency, ReceiverName: source.ReceiverName, ReceiverPhone: source.ReceiverPhone, PackageDesc: source.PackageDesc, PackageWeight: source.PackageWeight,
		PackageType: source.PackageType, PackageSize: source.PackageSize, IsFragile: source.IsFragile, ServiceTier: source.ServiceTier,
	})
	if err != nil {
		return "", err
	}
	if err = a.links.LinkTrip(ctx, missionID, trip.ID); err != nil {
		return "", err
	}
	_, _ = a.db.Exec(ctx, `UPDATE dispatch_mission_execution_links SET state='TRIP_CREATED',trip_id=$2,updated_at=now() WHERE mission_id=$1 AND (trip_id IS NULL OR trip_id=$2)`, missionID, trip.ID)
	return trip.ID, nil
}
